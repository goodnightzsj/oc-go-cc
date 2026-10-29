# WorkBuddy 固定补丁与安全升级

`update.sh` 用于已有 systemd 部署。它不安装服务、不管理账号/配置/数据，也不替代上游内置更新器；受托管进程的内置安装由上游已有保护及补丁回归测试约束。

## 更新流程

1. `flock` 串行执行；复制固定 `local.patch`，不从生产工作树自动导出或覆盖补丁。
2. fetch 上游，在独立临时 clone 中显式 fetch 已解析的目标 SHA，再检出并完整应用补丁；即使指定 SHA 只在生产 `FETCH_HEAD` 中、不被 branch/tag 引用，也不能依赖 clone 偶然带上对象。生产工作树不 checkout/reset。
3. 检查 Resin、配置接线、DeepSeek max 和托管更新保护的四个测试存在，再依次执行 Go `vet`、全量 `test -race`、`build`。
4. 只有候选通过且旧服务健康，才保留旧二进制/manifest，使用同目录原子 rename 安装 `bin/wb2api` 并重启。
5. 只有 systemd active、`/healthz` 返回 HTTP **200** 且 JSON `service` 为 **`workbuddy2api`** 才提交新 manifest。503 表示目前不可服务，不算上线成功。

冲突、测试或构建失败均不替换运行二进制。切换后的命令失败、INT/TERM 或健康失败会恢复旧二进制及 manifest，并再次验证恢复后的服务。若恢复也失败，明确报 `CRITICAL`、非零退出，不声称回滚成功。SIGKILL、断电及同时绕过此锁的人工操作不在自动回退保证内。

生产 checkout 的 HEAD **不表示运行版本**。权威证据是 state 目录的 `current.manifest`（上游 SHA、patch SHA-256、binary SHA-256）与实际 `bin/wb2api` 的 hash；每次候选目录保留对应源码。三个值均匹配才跳过构建，因此 patch-only 变更和被外部修改的二进制也会重新验证、部署。

## 运行

需要 Bash、Git、Go、util-linux `flock`/`timeout`、`sha256sum`、curl、jq、systemctl 及常规 Unix 工具。以具有目标目录写权限和服务重启权限的用户运行：

```bash
WB2A_REPO=/opt/workbuddy2api-panel \
WB2A_GO_BIN=/usr/local/go/bin/go \
WB2A_PATCH_FILE=/opt/workbuddy-ops/local.patch \
WB2A_STATE_DIR=/var/lib/workbuddy-updater \
bash /opt/workbuddy-ops/update.sh
```

`WB2A_REPO` 必填，必须是已有 checkout 且 `bin/wb2api` 为普通可执行文件（不接受软链接）；所选 systemd unit 的 `ExecStart` 必须指向此文件，工作目录/配置路径必须保持原部署约定。脚本不重写 unit，也不从健康响应推断二进制路径；首次接入及修改 unit 后应核对 MainPID 实际 executable 的 hash。固定补丁必须非空。更新脚本、补丁、state 目录应仅由部署操作者写入；不要把真实配置或凭据放入补丁。

其余参数：`WB2A_SERVICE` 默认 `workbuddy2api.service`，`WB2A_HEALTH_URL` 默认 `http://127.0.0.1:7863/healthz`，`WB2A_REF` 默认 `main`；`WB2A_BUILD_TIMEOUT` 为每项 Go 门禁的秒数（默认900），`WB2A_HEALTH_ATTEMPTS` 默认30、`WB2A_HEALTH_INTERVAL` 默认1秒，每次 HTTP 最多3秒。没有账号/容量而持续503时升级会停止，需先恢复健康，不应放宽门禁。

已有 timer/service 应改为调用这一入口并显式设置上述环境变量。不要再并行运行会自动捕获 patch 或强制 checkout 的旧脚本。

systemd 环境即使没有 `HOME`/`GOPATH` 也可构建：未设置时，`GOCACHE` 默认使用 `$WB2A_STATE_DIR/cache/build`，`GOMODCACHE` 默认使用 `$WB2A_STATE_DIR/cache/mod`。操作者已有非空值保持不变，应是可写绝对路径；可在部署入口显式指向现有缓存以避免重复下载。脚本不设置或更改 `HOME`。

## 后续上游升级与补丁维护

补丁是独立、版本化的输入，不会因一次失败或空工作树被覆写。上游重排补丁涉及代码时仍可能冲突：脚本应停止并保留旧服务；在新隔离源码上更新补丁及其测试，重新走全部门禁后再发布，不自动忽略冲突、不丢弃本地功能。

2026-10-30补丁基线更新为 `9f70e00188c30d859505378ae5d9b9779029dc06`。上游已原生提供国际版DeepSeek/SG的low/high/max，删除对应旧目录hunk及过时注释补丁，保留max行为守卫；GLM守卫按现行菜单区分global glm-5.3与CN glm-5.2，不恢复已下架的global型号。此补丁不再用于旧9ca基线；回退使用此前保留的binary/manifest及对应旧patch。

此前阻止升级的两处竞争已修复：`RunWebAgentTurn` 的SSE读取协程独占计数，关闭流并等待协程结束后才发布结果，覆盖成功、状态失败、RPC失败、非致命set_model失败及取消；面板目录测试计数改用原子操作，不改变生产缓存或并发。新版非流式取消499与既有错误聚合处理合并，保留上游错误和用量记录，不将客户端取消重新记成502。

上游内置更新测试继续按目标OS使用发布二进制名称，并从独立临时副本启动夹具，不执行/替换父测试程序。Linux下成功、启动崩溃回退、损坏包回退三条路径均有实际子进程验证；macOS不支持内置安装，不能用其跳过结果代替Linux验证。升级仍须完整执行vet/race/build，后续上游再次修改同一行为单元时依旧可能需要人工收敛补丁。

当前补丁还修复上游 `TestRunCatchesUpMissedSlots` 的测试生命周期：取消调度器后等待 `Run` 返回，才恢复 `wakeupGraceDelay` 和调用方的 `time.Local`。原测试在恢复全局时钟时调度器仍可能读取它，导致升级的 race 门禁间歇失败。修复不改变生产排程，也不删测试或降低门禁；若上游吸收同一修复，应在隔离候选中核对后移除这段已被取代的补丁。

非流式聚合补丁保留上游明确错误与已观测用量：标准 `error` 和业务错误帧不会被合成为正常完成；主请求及空截断救援返回502，探测请求向既有调用方报告失败，不因明确错误额外换号。分帧用量按字段保留（显式零仍覆盖），失败计数与 token 观测分离；救援失败时两次执行分别记用量和耗时。未改变探测失败降级为普通对话、救援传输/解析失败保留原截断响应的既有策略。

`state/runs/update.*` 保留候选源码、输入补丁、命令日志、新/旧二进制和 manifest，`state/cache/` 默认持久化 Go 构建与模块缓存，跨次升级复用。脚本不自动清理备份或缓存；需监控整个 state 目录磁盘占用，按独立备份策略处理已确认不再需要的版本。

## 面板内置更新与托管更新的统一方案

以下对比依据上游 `9f70e00` 的 `internal/updater`，两者的“最新”不是同一个值。

| 维度 | 面板内置更新 | 本托管脚本 |
| --- | --- | --- |
| 来源 | GitHub 正式时间版本 Release，排除 draft/prerelease | `WB2A_REF` 指定的源码提交，默认 main |
| 安装物 | 下载平台匹配 ZIP，验证 SHA-256 后替换官方二进制 | 固定 SHA 加本地 patch，依次 vet/race/build |
| 周期 | 启动及每6小时检查；可按上海时间每日安装 | 外部 systemd timer 调用，当前部署每6小时 |
| 补丁 | 官方二进制不含本地 Resin/错误聚合修改；max已由上游提供 | 固定 patch 是构建输入，冲突不切换 |
| 进程与回退 | helper 等旧进程退出、拉起新进程；观察短时存活并回退 | systemd 重启，检查 HTTP 200 与服务身份，失败恢复 binary/manifest |
| 并发边界 | 单个 Manager 的安装锁 | 同一 state 目录的跨进程 flock |

**默认方案：一个安装负责人，两个来源明确的状态。** 有本地 patch 的 systemd 部署继续由本脚本唯一负责安装和回退；内置更新保留官方 Release 检查，但不允许原地安装。上游 `canInstallLocked` 已禁止 dev 构建、systemd/OpenRC 等托管进程及容器自行安装，现有回归继续保护这个边界。不把面板按钮接到任意 shell，不启动第二套安装器。

面板 `update.auto_enabled/auto_time` 只控制内置安装流程，不会修改 systemd timer。源码构建显示 dev、官方 Release 较旧或仍提示有新版，均不能单独判定托管部署落后；托管版本以 manifest 的 source/patch/binary 三项核实，官方版本以 Release 检查核实。也不能为了消除提示给任意 main 提交冒填正式 Release 版本。

后续若整合面板展示，应读取托管执行器提供的只读状态（运行源 SHA、patch SHA、目标 SHA、检查时间、阶段及安全错误），保留两种来源标签；不要从官方 `update_available` 推导托管更新成功、失败或已是最新。定时器与手动操作仍调用同一执行器、共享同一锁。该展示集成是设计方案，本次测试竞争修复没有新增面板 API 或修改其认证边界。

建议按以下顺序实施后续整合：

1. 先补只读状态：已安装版本继续只认 `current.manifest`，最近检查/失败是另一维度，不能覆盖它。托管执行器原子写入白名单字段，面板沿用现有认证读取；状态缺失或过期显示未知，不显示“已最新”，不把包含账号配置的整个 state 目录暴露给面板。
2. 面板在托管模式显示“由 systemd 管理”、真实源/补丁版本、上次检查和失败阶段；原内置安装开关注明不适用于该部署。第一阶段不增加网页提权或启动脚本能力，手动更新继续走既有运维入口。
3. 补丁持续收敛：上游吸收相同修复或提供等效公开配置后，逐项验证并移除对应 hunk，保留行为回归；上游改动造成冲突时停止更新并提示具体阶段，不能跳过 patch 发布官方二进制。

验收至少覆盖：同时触发只执行一次、官方 Release 与 main 进度不同不会误报、patch 冲突和各项门禁失败保持旧服务、健康失败恢复旧 binary/manifest、状态文件缺失或读取失败不伪装成功。当前切换仍会短暂重启服务，不承诺零中断；单纯替换更新器不能消除这段停顿。

## 回归测试

```bash
bash -n ops/workbuddy/update.sh ops/workbuddy/update.test.sh
bash ops/workbuddy/update.test.sh
```

测试仅创建合成 Git 仓库，替换 Go、systemctl、curl，不触碰真实服务；Git clone 使用真实 Git 的 `--no-local` transport 路径，防止本地对象整包复制掩盖 FETCH_HEAD 引用缺口。Linux 上使用真实 `flock` 验证并发排他。覆盖无 HOME 的默认缓存及显式缓存保留、精确 SHA（生产 branch/tag 未前进）、patch 冲突、guard 缺失、vet/race/build 失败、重启失败、503/身份错误、信号回退、manifest 回退、重复版本、补丁变化、上游变化和二进制被改动。

缺少 Linux 工具时测试退出77，不能当通过。`WB2A_TEST_PORTABLE=1 bash ops/workbuddy/update.test.sh` 只提供本地功能诊断，显式跳过 native flock/timeout，不能替代 Linux 门禁。测试产物保留在打印的独立临时目录。
