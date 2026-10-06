# 远端发布与访问排障

目标为 `root@23.80.89.173` 的 `/root/oc-go-cc`；业务服务 `oc-go-cc.service`，代理监听回环 3456，面板 3445。通过 `https://opencode.9962510.xyz/` 的 Cloudflare → nginx 访问。

## 发布与回退

- 先检查本地/远端工作树与目标提交，按正式 validate 的顺序执行 `go vet ./...`、`go test ./... -v -race`、`go build -o /dev/null ./cmd/routatic-proxy`；提交推送后远端仅做 fast-forward 拉取。
- 权威部署入口为 `scripts/prod-deploy.sh:1`，构建 `.tmp/prod/releases/` 快照、同步目录、切换 `.tmp/prod/current`、重启服务并等待 health；失败时切回旧链接。默认保留5个release，需要保留现有备份时显式设置 `OC_GO_CC_KEEP_RELEASES`。
- 部署可能切断当前 AI 链路，应通过 detached systemd 临时任务或 `nohup setsid` 执行；不能把 SSH 返回或服务 active 当作验收。核对运行二进制的 build commit，并验证回环 health、外网页面/API及实际推理。
- 不读取配置或环境文件中的凭证。设置验证使用脱敏 `/api/proxy/config`，只投影必要字段；修改沿现有部分更新 API，避免覆盖其他设置。

## 白名单与 2026-10-06 故障

- 权威来源 `/etc/security-whitelist.yaml`；`/usr/local/bin/sync-whitelist.sh` 按层生成 fail2ban ignoreip、UFW、DOCKER-USER，并调用 `/usr/local/bin/_geo-render.py` 更新 nginx `geo $allowed_ip`。不要只修生成后的副本。
- 当日设备 IPv6 已变化，权威文件仍只包含旧前缀，导致 nginx 403；随后 `nginx-whitelist-deny` jail 把该地址同时封入本地 ipset 与 Cloudflare。
- 只将已核实的当前地址以 `/128` 加入 fail2ban 层（同时供 nginx 使用），保留旧条目；未扩到整个新网段，也未改变 UFW/Docker 范围。渲染后 `nginx -t` 与 reload 成功。
- 仅更新白名单不会清除既有 Cloudflare 封禁。旧 `cloudflare-token.conf` 的 unban 查询未编码 notes 中空格，curl 失败经管道伪装为成功；本地 unban 后仍返回 Cloudflare 1106。
- 回补 Fail2Ban 官方 [URL 编码修复](https://github.com/fail2ban/fail2ban/commit/c7f8b75e7e013ac893daf78eacfdba98f2c9b689)及 [GET 查询修复](https://github.com/fail2ban/fail2ban/commit/c9b5e845ba5cdfff5fdde3ea1ddc2c97a5cfb22a)，校验并 reload 指定 jail，再通过既有 daemon action 精确解除该地址的遗留规则。未读取 token、全量解封或停用 jail。
- 合成 HTTP 回归证明旧动作 exit 0 却无请求，新动作执行正确 GET+DELETE；同源 IPv6 的 `/`、`/health`、`/api/metrics`、`/v1/models` 均恢复200，既有浏览器同源请求也通过。此次访问修复未重启业务服务。
- 原配置备份位于 `/root/oc-go-cc-whitelist-20261006.mW7h0d/`。IPv6 隐私地址可能再次轮换，届时应重新核实来源并更新权威白名单，不放宽访问门控。

## CompactGate 证据边界

- 2026-10-06 查询本机目标 host 共45528行，最新时间为 `2026-10-01T07:11:00.470Z`；10月2日起无新记录，不能据此声称10月6日渠道已生效。
- 最新5条完整请求191244–191248实际发往该host的模型均为 `cline-pass/deepseek-v4.1-flash`，无渠道 only/order；转换后响应无 `finalProvider`。它们证明旧链路未发送限制，不证明当前上游能否接受限制。
- 真实探测只保留历史模型和协议形态，用最小合成消息替代原始长对话；比较开启/关闭及不存在渠道的对照，关联上游请求字段与实际渠道日志。HTTP 200 或单次 `matched` 均不够证明强制路由。
