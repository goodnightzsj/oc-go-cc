# 远端发布与访问排障

目标为 `root@23.80.89.173` 的 `/root/oc-go-cc`；业务服务 `oc-go-cc.service`，代理监听回环 3456，面板 3445。通过 `https://opencode.9962510.xyz/` 的 Cloudflare → nginx 访问。

## 发布与回退

- 先检查本地/远端工作树与目标提交，按正式 validate 的顺序执行 `go vet ./...`、`go test ./... -v -race`、`go build -o /dev/null ./cmd/routatic-proxy`；提交推送后远端仅做 fast-forward 拉取。
- 权威部署入口为 `scripts/prod-deploy.sh:1`，构建 `.tmp/prod/releases/` 快照、同步目录、切换 `.tmp/prod/current`、重启服务并等待 health；失败时切回旧链接。默认保留5个release，需要保留现有备份时显式设置 `OC_GO_CC_KEEP_RELEASES`。
- 部署可能切断当前 AI 链路，应通过 detached systemd 临时任务或 `nohup setsid` 执行；不能把 SSH 返回或服务 active 当作验收。核对运行二进制的 build commit，并验证回环 health、外网页面/API及实际推理。
- 2026-10-06 的 systemd 临时部署需显式 `User=root`，并通过既有 `OC_GO_CC_GO_BIN=/root/.local/go-current/bin/go` 选择 Go 1.25.11。首次任务缺少用户环境，系统Go1.22不满足要求后因HOME未设置而在构建前退出；旧服务未受影响。不改写HOME变量或升级系统Go。
- 不读取配置或环境文件中的凭证。设置验证使用脱敏 `/api/proxy/config`，只投影必要字段；修改沿现有部分更新 API，避免覆盖其他设置。
- 渠道候选发布验收读取 `/api/cline-pass/channels` 的开关、检查时间、错误状态及渠道元数据，核对页面资产与提交一致；不通过保存真实配置或切换debug_capture来做UI探测。启动扫描失败会保留内置/旧有效候选并报告错误，health成功不代表候选扫描成功。

## 白名单与 2026-10-06 故障

- 权威来源 `/etc/security-whitelist.yaml`；`/usr/local/bin/sync-whitelist.sh` 按层生成 fail2ban ignoreip、UFW、DOCKER-USER，并调用 `/usr/local/bin/_geo-render.py` 更新 nginx `geo $allowed_ip`。不要只修生成后的副本。
- 当日设备 IPv6 已变化，权威文件仍只包含旧前缀，导致 nginx 403；随后 `nginx-whitelist-deny` jail 把该地址同时封入本地 ipset 与 Cloudflare。
- 初次修复只将已核实设备地址以 `/128` 加入 fail2ban 层（同时供 nginx 使用）。用户随后明确授权改为网段：当前权威条目为 `2409:8a28:eef:c6f0::/64`，本次新增的两条 `/128` 已删除，其他原有网段保留。
- 仅更新白名单不会清除既有 Cloudflare 封禁。旧 `cloudflare-token.conf` 的 unban 查询未编码 notes 中空格，curl 失败经管道伪装为成功；本地 unban 后仍返回 Cloudflare 1106。
- 回补 Fail2Ban 官方 [URL 编码修复](https://github.com/fail2ban/fail2ban/commit/c7f8b75e7e013ac893daf78eacfdba98f2c9b689)及 [GET 查询修复](https://github.com/fail2ban/fail2ban/commit/c9b5e845ba5cdfff5fdde3ea1ddc2c97a5cfb22a)，校验并 reload 指定 jail，再通过既有 daemon action 精确解除该地址的遗留规则。未读取 token、全量解封或停用 jail。
- 合成 HTTP 回归证明旧动作 exit 0 却无请求，新动作执行正确 GET+DELETE；同源 IPv6 的 `/`、`/health`、`/api/metrics`、`/v1/models` 均恢复200，既有浏览器同源请求也通过。此次访问修复未重启业务服务。
- 同网段内后64位临时地址轮换现在由 `/64` 覆盖，无需逐地址添加；该放行范围也包括网段内其他设备。运营商若更换前64位，旧规则仍会失效，需要核实新前缀后更新权威来源。
- `/64` 合并后，原临时地址与从未单独登记的本机同网段稳定地址访问公开HTTPS页面及health均200；nginx/fail2ban校验及reload通过，业务进程未重启。实际运行ignoreip和nginx geo均只保留该 `/64`，不再重复两条 `/128`。
- 初始故障备份仍在 `/root/oc-go-cc-whitelist-20261006.mW7h0d/`；网段合并前的YAML、nginx和fail2ban白名单备份在 `/root/oc-go-cc-whitelist-prefix-20261006.RvjnpL/`。被替代的两个临时应用脚本与两个候选YAML移至后者的 `retired-ipv6-128/`，可恢复；Cloudflare解封修复保留。
- 用户随后授权同步其他服务：当前 `/64` 的权威条目增加 `ufw`、`docker` 层，覆盖 `8091/9090`；SSH `22/tcp` 和 Resin `2260/tcp` 按既有独立策略加入 UFW。生成脚本只管理 `docker_ports`，不自动同步 SSH/Resin，未来前缀变化需同时维护这两项。
- 新增四条 UFW TCP 允许规则使用 `prepend`，避免被既有端口 DENY 遮蔽；DOCKER-USER 的两条新 IPv6 ACCEPT 位于 DROP 之前。差异断言证明旧网段、其他规则及 Cloudflare-only `80/443` 未改，nginx/fail2ban白名单内容不变。同步前备份在 `/root/oc-go-cc-firewall-sync-20261006.KwVNYU/`。
- 同网段未单独登记的稳定地址已实测连通 IPv6 SSH（收到协议标识）和 Resin TCP，公开页面及health均200，业务PID未变。Docker `8091/9090` 目前仅绑定 IPv4；白名单已同步，但不代表新增 IPv6 监听或完成 IPv6 应用层验收。

## CompactGate 证据边界

- 2026-10-06 查询本机目标 host 共45528行，最新时间为 `2026-10-01T07:11:00.470Z`；10月2日起无新记录，不能据此声称10月6日渠道已生效。
- 最新5条完整请求191244–191248实际发往该host的模型均为 `cline-pass/deepseek-v4.1-flash`，无渠道 only/order；转换后响应无 `finalProvider`。它们证明旧链路未发送限制，不证明当前上游能否接受限制。
- 真实探测只保留历史模型和协议形态，用最小合成消息替代原始长对话；比较开启/关闭及不存在渠道的对照，关联上游请求字段与实际渠道日志。HTTP 200 或单次 `matched` 均不够证明强制路由。
- 当日部署后六条真实对照已验证双字段注入/关闭与观测正常，但 Alibaba 和不存在渠道均被路由到 DeepSeek 并正常完成，说明该账户的 `cline-pass/deepseek-v4.1-flash` 当前忽略此限制。实验后恢复默认关闭，详细矩阵见 [ClinePass 实测](../../docs/cline-pass.md)。
