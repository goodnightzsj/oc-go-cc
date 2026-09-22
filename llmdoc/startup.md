# Startup

## 快速了解

先读 `overview/project-overview.md` 建立全局认知，再按需深入：

- 用量与缓存 token 链路：`architecture/usage-pipeline.md`
- provider 选择与平台边界：`architecture/provider-layer.md`
- 模型与成本路由：`architecture/model-routing.md`
- 入站协议与 Responses 边界：`reference/inbound-protocols.md`
- 缓存/计费修复与三端对账：`reference/cache-billing-audit.md`
- 记账与调试硬约束：`must/accounting-baseline.md`

## 常用命令

```bash
make build     # 构建到 bin/routatic-proxy（默认禁 CGO）
make test      # 带 race 全量测试
make run       # 直接运行（不带构建）
```

本地起代理 + 面板：`./bin/routatic-proxy start`（面板 3445，代理 3456）。

## 远端部署（23.80.89.173）

```bash
ssh root@23.80.89.173
cd /root/oc-go-cc && git pull origin main && bash scripts/prod-deploy.sh
```

部署会重启 systemd 服务，期间本地 AI 短暂断连；用 `curl -fsS http://127.0.0.1:3456/health` 验证。面板/历史 DB：`/root/.local/share/routatic-proxy/data.db`；配置默认 `/root/.config/routatic-proxy/config.json`（env 插值 `ROUTATIC_PROXY_API_KEY`）。旧路径 `/root/.config/oc-go-cc/config.json`（`internal/config/loader.go:17`）与旧变量名 `OC_GO_CC_*`（`loader.go:63-72` 的 `legacyEnvNames`）仍作回退兼容。

## 关键约束速查

- 缓存拆分两格式兼容（OpenAI `cached_tokens` + DeepSeek hit/miss），见 `must/accounting-baseline.md`
- seed 价格与测试断言同步更新
- capture 内容敏感，仅调试开启