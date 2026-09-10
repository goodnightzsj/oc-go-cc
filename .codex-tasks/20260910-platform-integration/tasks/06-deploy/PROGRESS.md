# Progress

## Context Recovery Block

- 当前：1/3；#5验证完成，SSH只读预检通过，尚未push或重启。
- 真源：TODO.csv。
- 来源：本项目 Claude 2026-09-04 session、llmdoc/startup.md 与 scripts/prod-deploy.sh。
- 下一步：提交推送本次改动，服务器内部备份配置与SQLite，fast-forward拉取并执行部署脚本。

## 只读远端基线（2026-09-11）

- SSH使用原生配置、BatchMode与StrictHostKeyChecking=yes；未读取私钥或配置中的实际凭证。
- 工作目录 `/root/oc-go-cc`；main HEAD `b555f5e237442b52f44bdbb9a882fc2c899bd12f`；仅无关未跟踪 `.ace-tool/`，必须保留。
- 服务 `oc-go-cc.service` active/running，PID898，`/health`成功；运行release `/root/oc-go-cc/.tmp/prod/releases/20260904193909-9182566bb4a6`。
- 已确认配置与SQLite存在，sqlite3可用；没有读取配置内容或私有请求。脚本只切换release，部署前另做服务端本地备份。
