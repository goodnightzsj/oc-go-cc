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

`state/runs/update.*` 保留候选源码、输入补丁、命令日志、新/旧二进制和 manifest，`state/cache/` 默认持久化 Go 构建与模块缓存，跨次升级复用。脚本不自动清理备份或缓存；需监控整个 state 目录磁盘占用，按独立备份策略处理已确认不再需要的版本。

## 回归测试

```bash
bash -n ops/workbuddy/update.sh ops/workbuddy/update.test.sh
bash ops/workbuddy/update.test.sh
```

测试仅创建合成 Git 仓库，替换 Go、systemctl、curl，不触碰真实服务；Git clone 使用真实 Git 的 `--no-local` transport 路径，防止本地对象整包复制掩盖 FETCH_HEAD 引用缺口。Linux 上使用真实 `flock` 验证并发排他。覆盖无 HOME 的默认缓存及显式缓存保留、精确 SHA（生产 branch/tag 未前进）、patch 冲突、guard 缺失、vet/race/build 失败、重启失败、503/身份错误、信号回退、manifest 回退、重复版本、补丁变化、上游变化和二进制被改动。

缺少 Linux 工具时测试退出77，不能当通过。`WB2A_TEST_PORTABLE=1 bash ops/workbuddy/update.test.sh` 只提供本地功能诊断，显式跳过 native flock/timeout，不能替代 Linux 门禁。测试产物保留在打印的独立临时目录。
