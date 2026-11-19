# WorkBuddy 固定补丁与安全升级

`update.sh` 用于已有 systemd 部署。它不安装服务、不管理账号/配置/数据，也不替代上游内置更新器；受托管进程的内置安装由上游已有保护及补丁回归测试约束。

## 更新流程

1. `flock` 串行执行；复制固定 `local.patch`，不从生产工作树自动导出或覆盖补丁。
2. fetch 上游，在独立临时 clone 中显式 fetch 已解析的目标 SHA，再检出并完整应用补丁；即使指定 SHA 只在生产 `FETCH_HEAD` 中、不被 branch/tag 引用，也不能依赖 clone 偶然带上对象。生产工作树不 checkout/reset。
3. 检查 Resin、配置接线、DeepSeek max、Global SG目录、托管只读状态和托管更新保护的六个测试存在，再依次执行 Go `vet`、全量 `test -race`、`build`。
4. 只有候选通过且旧服务健康，才保留旧二进制/manifest，使用同目录原子 rename 安装 `bin/wb2api` 并重启。
5. 只有 systemd active、`/healthz` 返回 HTTP **200** 且 JSON `service` 为 **`workbuddy2api`** 才提交新 manifest。503 表示目前不可服务，不算上线成功。

冲突、测试或构建失败均不替换运行二进制。切换开始至健康确认、manifest 提交之间的命令失败、INT/TERM 或健康失败会恢复旧二进制及 manifest，并再次验证恢复后的服务。提交成功后的清理或日志失败仍非零退出，但不撤回健康版本，也不删除本次 run 中的旧二进制与 manifest。退出清理不依赖日志写入成功，保留原失败退出码；普通执行阶段日志写入失败仍明确报错并停止。若恢复也失败，明确报 `CRITICAL`、非零退出，不声称回滚成功。SIGKILL、断电、存储故障导致恢复操作本身失败及同时绕过此锁的人工操作不在自动回退保证内。

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

其余参数：`WB2A_SERVICE` 默认 `workbuddy2api.service`，`WB2A_HEALTH_URL` 默认 `http://127.0.0.1:7863/healthz`，`WB2A_REF` 默认 `main`；`WB2A_BUILD_TIMEOUT` 为每项 Go 门禁的秒数（默认900），`WB2A_HEALTH_ATTEMPTS` 默认30、`WB2A_HEALTH_INTERVAL` 默认1秒，每次 HTTP 最多3秒；`WB2A_KEEP_RUNS` 默认3，表示成功后保留多少个**成功部署**的 run 目录。没有账号/容量而持续503时升级会停止，需先恢复健康，不应放宽门禁。

**保留策略**：成功部署后只留最近 `WB2A_KEEP_RUNS` 个含 `wb2api` 的 run 目录（本次运行的目录永远不删）；没有 `wb2api` 的旧目录（如重复检查的空跑）删除。按完整路径数组和目录修改时间排序，不解析 `ls` 的行，不跟随符号链接；路径包含换行也只删除同一个完整目标。哈希从文件标准输入计算，避免文件名转义污染摘要。失败候选只把 `update.log` 覆写到 `state/last-failure.log`，整个 run 目录删除——源码克隆与构建产物可复现，一次约15MB。已提交部署的后处理失败、回滚本身失败（`CRITICAL`）时保留本次现场供排查。

已有 timer/service 应改为调用这一入口并显式设置上述环境变量。不要再并行运行会自动捕获 patch 或强制 checkout 的旧脚本。

`wb2api-update.timer` 提供现有 `wb2api-update.service` 的每小时检查配置：每小时17分触发，保留最多300秒随机延迟及 `Persistent=true` 的停机补检。安装到 `/etc/systemd/system/wb2api-update.timer` 前备份原文件并执行 `systemd-analyze verify`；安装后 `daemon-reload` 并重启该 timer，以 `systemctl show` 核对实际日历表达式及下次触发时间。只重启 timer 不会直接重启桥；检测到新源码或补丁且通过全部门禁后才切换桥服务。

systemd 环境即使没有 `HOME`/`GOPATH` 也可构建：未设置时，`GOCACHE` 默认使用 `$WB2A_STATE_DIR/cache/build`，`GOMODCACHE` 默认使用 `$WB2A_STATE_DIR/cache/mod`。操作者已有非空值保持不变，应是可写绝对路径；可在部署入口显式指向现有缓存以避免重复下载。脚本不设置或更改 `HOME`。

## 后续上游升级与补丁维护

补丁是独立、版本化的输入，不会因一次失败或空工作树被覆写。上游重排补丁涉及代码时仍可能冲突：脚本应停止并保留旧服务；在新隔离源码上更新补丁及其测试，重新走全部门禁后再发布，不自动忽略冲突、不丢弃本地功能。

2026-10-30补丁基线更新为 `9f70e00188c30d859505378ae5d9b9779029dc06`。上游已原生提供国际版DeepSeek/SG的low/high/max，删除对应旧目录hunk及过时注释补丁，保留max行为守卫；GLM守卫按现行菜单区分global glm-5.3与CN glm-5.2，不恢复已下架的global型号。此补丁不再用于旧9ca基线；回退使用此前保留的binary/manifest及对应旧patch。

同日已验证并部署 `53735780d850c645d84c15f009324f4cc3153c65`：新增三个提交处理下架任务及移除夜猫子排程。固定补丁无需修改，Linux隔离候选及原托管入口均通过完整vet/race/build；生产timer已改为每小时检查。上游后续再次改动时仍须重新通过这些门禁。

同日补丁新增Global目录SG例外：`global:deepseek-v4.1-flash-sg` 在真实调用中仍可用，但被上游客户端picker按`paidId`隐藏。仅在本次探测的原始目录确实包含该条目时追加，保留同源价格、上下文、能力与档位；不改CN、不放开其他内部型号、不创建别名或自动切付费渠道，操作者的hidden/allowed仍生效。上游移除该条目后不会靠静态名单继续展示。`global:deepseek-v4.1-flash-1017`与`global:deepseek-v4.1-flash-1017-sg`按顺序实测均HTTP400/上游11102（service info not found），不添加这两个日期入口。新守卫覆盖SG恢复、缺失、去重、非对话过滤、桌面目录权威及真实元数据保留；后续自动升级必须保留并通过。

2026-11-19补丁基线更新为 `34120825e8deef18b1ec194dbb16eea6fe01a016`。上一基线 `96b816d` 之后上游前进 349 个提交，固定补丁自 10-31 起每小时在 `git apply` 处失败、无任何部署；生产 checkout 停在 `f9d190c` 与运行版本无关，运行版本以 manifest 为准。本轮按新源码逐个 hunk 收敛，共 22 文件 45 hunk：

- `internal/updater/integration_test.go` 的 hunk **删除**：上游已自行改用 `binaryName := "wb2api"` 加 Windows 后缀，原补丁内容被取代。
- `internal/upstream/webagent.go` 的四处 hunk 收敛为一条收尾改动：上游已把计数归还读取协程并由 `finish` 发布，但 `finish` 先 `<-done` 再关流，而读协程停在 `Read` 上等 `Close`——SSE 仍开着时（正常情形，完成状态来自状态端点而非流结束）不会返回。首轮按「先关流再 join」改，Linux 门禁的 `TestRunWebAgentTurnUsesSandboxToken` 因关流截断未读帧而 `Chunks=0`；改为先给 500ms 自然读尽窗口、超时才关流并 join，本地 `-count=5` 与 `-race -count=3` 通过，Linux 门禁随后全绿。
- `cmd/server/main.go` 的 `restartRequiredFields` 重锚；`internal/server/handler.go` 的 `recordAttempt` 签名（新增 `latency`、`succeeded`）按上游现有 7 处调用点全部重放，非流式 `failAggregate` 与明确错误透传同新结构合并；`internal/upstream/local_max_test.go` 跟随 `prepareBody` 新增的 `convReqID` 参数补参。

验证：干净 `3412082` checkout 上 `git apply --check`/`apply` 通过、`go vet ./...` 通过；服务器隔离候选跑完 `vet`/`test -race`/`build` 后部署（source `3412082`、patch `b1d189ab`、binary `839f307a`，健康 200、7/7 账号可用；旧版留在该次 run 目录）。升级后 `/panel/api/update` 返回 `update_available=false`——上游已把规则改成 `currentOK && …`，`dev` 版本不再恒定提示。上游每次再改动同一行为单元时仍需重新收敛，`WB2A_REF` 默认 `main` 仍会漂移。



SG补丁已在`5373578`基线上通过Linux隔离与托管两轮vet/race/build并部署：patch `d9f4c60b`、binary `1d24ff8e`（完整值以manifest为准），旧版本保留在`state/runs/update.ZWEuk6`。当前上游目录SG倍率为`x0.03`；极短探针回执`credit=0`不代表入口免费。桥目录恢复后，本项目的站点目录缓存最多仍可能延迟10分钟；不需要为刷新目录重启主代理。

此前阻止升级的两处竞争已修复：`RunWebAgentTurn` 的SSE读取协程独占计数，关闭流并等待协程结束后才发布结果，覆盖成功、状态失败、RPC失败、非致命set_model失败及取消；面板目录测试计数改用原子操作，不改变生产缓存或并发。新版非流式取消499与既有错误聚合处理合并，保留上游错误和用量记录，不将客户端取消重新记成502。

上游内置更新测试继续按目标OS使用发布二进制名称，并从独立临时副本启动夹具，不执行/替换父测试程序。Linux下成功、启动崩溃回退、损坏包回退三条路径均有实际子进程验证；macOS不支持内置安装，不能用其跳过结果代替Linux验证。升级仍须完整执行vet/race/build，后续上游再次修改同一行为单元时依旧可能需要人工收敛补丁。

当前补丁还修复上游 `TestRunCatchesUpMissedSlots` 的测试生命周期：取消调度器后等待 `Run` 返回，才恢复 `wakeupGraceDelay` 和调用方的 `time.Local`。原测试在恢复全局时钟时调度器仍可能读取它，导致升级的 race 门禁间歇失败。修复不改变生产排程，也不删测试或降低门禁；若上游吸收同一修复，应在隔离候选中核对后移除这段已被取代的补丁。

非流式聚合补丁保留上游明确错误与已观测用量：标准 `error` 和业务错误帧不会被合成为正常完成；主请求及空截断救援返回502，探测请求向既有调用方报告失败，不因明确错误额外换号。分帧用量按字段保留（显式零仍覆盖），失败计数与 token 观测分离；救援失败时两次执行分别记用量和耗时。未改变探测失败降级为普通对话、救援传输/解析失败保留原截断响应的既有策略。

2026-11-19本地继续补齐（尚未发布）：非EOF读取错误也返回累计usage与原始error；没有观测不补零，不把传输错误伪装为明确源error帧。首次聚合及救援均保留已报告的token/credit；救援传输失败单独记失败用量与耗时，首段不重复计入救援等待，历史费用摘要累计两段（例如0.5+0.25=0.75）。取消后完成内部记账但停止写响应；上述错误响应与救援回退策略不变。`internal/upstream/local_aggregate_error_test.go`及`internal/server/local_aggregate_error_test.go`保留相关回归，发布前仍须在目标上游revision应用补丁并完成原有门禁。

同日本地复审补齐救援所有出口：首次聚合与救援执行分别结算一次，包括成功、HTTP429/503拒绝、建连失败和聚合失败；没有用量的拒绝也计一次失败，不虚构token或credit。成功救援现在是两次成功尝试，耗时按各次执行计算（1s+2s的平均为1.5s），历史汇总不再二次计费。正常完成的流式/非流式响应即使usage缺失或null也算成功；Has*只决定用量覆盖，明确零仍保留。响应内容、救援回退策略与历史数据不改写。新增回归`TestLocalRescueExecutionAccounting`、`TestLocalSuccessfulUsagePresence`；完整补丁在干净3412082基线的28个文件逐字节一致，macOS上29包fresh race、全仓vet/build通过。本轮未验证生产d40d4b8兼容或Linux安装路径，仍未部署。

`state/runs/update.*` 保留候选源码、输入补丁、命令日志、新/旧二进制和 manifest，`state/cache/` 默认持久化 Go 构建与模块缓存，跨次升级复用。run 目录按上面的保留策略自动清理；`state/cache/` 不清理，需按备份策略处理已确认不再需要的缓存。

## 面板内置更新与托管更新的统一方案

以下对比依据上游 `9f70e00` 的 `internal/updater`，两者的“最新”不是同一个值。

| 维度 | 面板内置更新 | 本托管脚本 |
| --- | --- | --- |
| 来源 | GitHub 正式时间版本 Release，排除 draft/prerelease | `WB2A_REF` 指定的源码提交，默认 main |
| 安装物 | 下载平台匹配 ZIP，验证 SHA-256 后替换官方二进制 | 固定 SHA 加本地 patch，依次 vet/race/build |
| 周期 | 启动及每6小时检查；可按上海时间每日安装 | 外部 systemd timer 调用，本目录提供每1小时配置 |
| 补丁 | 官方二进制不含本地 Resin/错误聚合修改；max已由上游提供 | 固定 patch 是构建输入，冲突不切换 |
| 进程与回退 | helper 等旧进程退出、拉起新进程；观察短时存活并回退 | systemd 重启，检查 HTTP 200 与服务身份，失败恢复 binary/manifest |
| 并发边界 | 单个 Manager 的安装锁 | 同一 state 目录的跨进程 flock |

**默认方案：一个安装负责人，两个来源明确的状态。** 有本地 patch 的 systemd 部署继续由本脚本唯一负责安装和回退；内置更新保留官方 Release 检查，但不允许原地安装。上游 `canInstallLocked` 已禁止 dev 构建、systemd/OpenRC 等托管进程及容器自行安装，现有回归继续保护这个边界。不把面板按钮接到任意 shell，不启动第二套安装器。

面板 `update.auto_enabled/auto_time` 只控制内置安装流程，不会修改 systemd timer。源码构建显示 dev、官方 Release 较旧或仍提示有新版，均不能单独判定托管部署落后；托管版本以 manifest 的 source/patch/binary 三项核实，官方版本以 Release 检查核实。也不能为了消除提示给任意 main 提交冒填正式 Release 版本。

2026-11-19本地新增只读展示，固定补丁仍基于 `3412082`，尚未部署。脚本取得同一更新锁后原子写入 `last-check.json`：检查及阶段时间、阶段、结果、目标 source/patch、退出码和回退失败标记。阶段覆盖 prepare/fetch/checkout/patch/vet/test/build/health/activate/commit/cleanup，不写原始错误、日志、配置或凭据。状态发布失败明确非零退出，不阻止已有回退逻辑；候选失败不能覆盖 `current.manifest` 中的已部署版本。

面板沿现有认证的 `/panel/api/update` 和 overview 快照读取 `Manager.Status().managed`，不新增安装入口。只读取 `current.manifest` 与 `last-check.json`，各最多16KiB；校验哈希、时间和枚举，返回固定错误码，不泄露文件路径或原始内容。检查记录缺失、坏数据或超过两小时未更新时明确显示未知。源码、补丁和二进制哈希表示**上次提交的部署记录**，不代替实际 binary/MainPID 核验；官方 Release 的检查按钮、提示与托管信息分开标注，不能由 `update_available` 推导托管成功或已最新。

Linux 受托管进程默认读取 `/var/lib/workbuddy-updater`；使用其他位置时须在更新 service 和桥 service 中同时设置 `WB2A_STATE_DIR`，并让桥进程能读取上述两个记录。显式设置该目录同样禁用内置安装。不要把整个 state 目录作为静态资源公开，或为此放开账号/配置文件权限。网页仅展示外部服务负责安装和回退，说明原内置安装设置不控制托管 timer；手动更新仍走本脚本和同一锁。

上游吸收相同修复或提供等效公开配置后，逐项验证并移除对应 hunk，保留行为回归；冲突时停止更新并显示失败阶段，不能跳过 patch 发布官方二进制。当前切换仍会短暂重启服务，不承诺零中断。

## 回归测试

```bash
bash -n ops/workbuddy/update.sh ops/workbuddy/update.test.sh
bash ops/workbuddy/update.test.sh
```

测试仅创建合成 Git 仓库，替换 Go、systemctl、curl，不触碰真实服务；Git clone 使用真实 Git 的 `--no-local` transport 路径，防止本地对象整包复制掩盖 FETCH_HEAD 引用缺口。Linux 上使用真实 `flock` 验证并发排他。覆盖无 HOME 的默认缓存及显式缓存保留、精确 SHA（生产 branch/tag 未前进）、patch 冲突、guard 缺失、vet/race/build 失败、重启失败、503/身份错误、信号回退、manifest 回退、重复版本、补丁变化、上游变化和二进制被改动；日志不可写叠加重启失败、TERM 或恢复服务失败时仍尝试回退，检查原退出码和明确失败报告。另覆盖提交后清理失败仍保留回退资料、含换行的 state 路径不越界、检查阶段/退出码与失败一致。

补丁内 `TestLocalManagedStatus` 及 panel 回归覆盖已部署版本与失败候选分离、缺失/损坏/过期/超大记录、字段白名单、原有认证、官方 Release 文案与托管失败展示。此轮 macOS 上 updater/panel 的 race 测试通过，不代表 Linux 原生安装路径或线上页面已经验收。

缺少 Linux 工具时测试退出77，不能当通过。`WB2A_TEST_PORTABLE=1 bash ops/workbuddy/update.test.sh` 只提供本地功能诊断，显式跳过 native flock/timeout，不能替代 Linux 门禁。测试产物保留在打印的独立临时目录。

CI与共享release validate现显式运行 `WB2A_TEST_PORTABLE=0 bash ops/workbuddy/update.test.sh`，位于delivery故障测试之后。实际workflow命令经隔离夹具验证：外部portable=1不能弱化门禁，0/23/77逐一传播；本地macOS的native退出77与portable通过分别记录，不代表GitHub或Linux已运行成功。
