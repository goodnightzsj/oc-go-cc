# Progress

## Context Recovery Block

- 当前：1/3；`f135930` 已推送部署并通过健康/数据校验，但生产页面空账本验收失败；已回退旧 release，待 #4/#5 边界修复验收后重新部署。
- 真源：TODO.csv。
- 来源：本项目 Claude 2026-09-04 session、llmdoc/startup.md 与 scripts/prod-deploy.sh。
- 下一步：完成边界修复的最新验证，提交推送并重新部署；原备份保留，生产浏览器继续只读验收。

## 只读远端基线（2026-09-11）

- SSH使用原生配置、BatchMode与StrictHostKeyChecking=yes；未读取私钥或配置中的实际凭证。
- 工作目录 `/root/oc-go-cc`；main HEAD `b555f5e237442b52f44bdbb9a882fc2c899bd12f`；仅无关未跟踪 `.ace-tool/`，必须保留。
- 服务 `oc-go-cc.service` active/running，PID898，`/health`成功；运行release `/root/oc-go-cc/.tmp/prod/releases/20260904193909-9182566bb4a6`。
- 已确认配置与SQLite存在，sqlite3可用；没有读取配置内容或私有请求。脚本只切换release，部署前另做服务端本地备份。

## 提交后恢复核验（2026-09-11）

- 工作区恢复时干净，`f135930` 比 origin/main 领先一个提交；不重复创建功能提交。Claude session 仅用白名单正则提取部署命令，再次确认 push → SSH → prod-deploy 流程，未展开原始会话。
- 当前提交隔离复跑 `go test -race -p 2 ./... -count=1 -json` 退出 0：639 顶层、1029 含子测试通过，18 个有测试包；两个默认关闭的浏览器/CLI 验收保留 #4/#5 的显式记录。证据 `/tmp/oc-go-cc-deploy-verify.eRBwTX/race.jsonl`。
- `node --check internal/gui/assets/app.js` 与 `git diff --check HEAD^ HEAD` 通过。远端主机、仓库、服务 PID898 和旧 release 与预检记录一致；upstream HEAD 仍为 `b214eeb`，origin/main 为 `684235d`。
- 下一步：完成只读独立复核，在远端创建仅 root 可读的配置/SQLite 备份；推送已验证提交并 fast-forward 部署；生产浏览器只做读取和筛选，不执行测试 fixture 的保存/停止操作。

## 首次部署、验证失败与回退

- 已创建服务端备份 `/root/oc-go-cc/.tmp/predeploy-20260911-5DZMJ0D0`（0700）：配置、SQLite 一致性备份、两处既有 catalog、原 release/commit 路径。备份 quick_check=ok，requests=0，provider_usage=1390。
- `git push origin main` 成功；远端 ff-only 到 `f1359301a5551c15f111091f1796be3bf1326e17`，脚本部署 release `20260911053033-1f4b2f90c746` 成功。model catalog 的未知 provider 警告保留；健康轮询首次短暂连接拒绝后成功，不是最终失败。
- 部署后服务 active/running、NRestarts=0；配置散列一致、SQLite quick_check=ok，backup 中 requests/provider_usage 均无丢失，未跟踪 .ace-tool 保留。
- 生产脚本首次在第8检查失败：空账本 models=null，不是混入其他 provider。证据 `/tmp/oc-go-cc-deploy-verify.eRBwTX/production-smoke/result.json` 与 failure.png；pageErrors/blocked 均为空，仅运行到概览。
- 进一步合成测试复现 OpenRouter quota 在无显式平台 Key 时错误使用全局 Key。生产未用实际 Key 复现，未走到该请求。
- 已显式切回 `20260904193909-9182566bb4a6` 并重启，health 与 active 通过；没有恢复旧数据库、覆盖新账本或改变配置。修复及验收见 #4/#5。
