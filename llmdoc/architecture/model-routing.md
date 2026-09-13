# 模型路由与成本路由

两条相互独立的决策：**选哪个场景**（静态规则）与**场景内选哪个模型**（成本路由，可选）。

## 场景判定

| 环节 | 位置 |
|------|------|
| 判定入口与优先级 | `internal/router/scenarios.go:55`（`DetectScenario`），优先级说明 `:47-52` |
| 流式变体 | `internal/router/scenarios.go:264`（`RouteForStreaming`）：非 vision / 非长上下文一律收敛为 `fast`，换取 TTFT |
| Router 包装 | `internal/router/model_router.go:229`（`Route`）、`:451`（`RouteForStreaming`） |

## Override 优先级（唯一权威点）

`internal/handlers/messages.go:535`（`buildModelChain`）是唯一实现，`:547-551` 顺序固定：

1. 精确 `model_overrides` → `RouteWithOverride`（`model_router.go:308`）
2. `model_family_overrides` 子串匹配 → `RouteWithFamilyOverride`（`model_router.go:333`），**最长键优先**（`:339-343`）
3. `respect_requested_model` → `model_router.go:74`（`isRespectRequestedModel`）、`:91`（`resolveRequestedModel`）
4. 场景路由 → `routeOnce`（`messages.go:587`）

未命中时 `routeOnce` 补一条去重的场景兜底链。

## 成本路由

| 环节 | 位置 |
|------|------|
| 入口 | `internal/router/selector.go:34`（`NewSelector`）、`:56`（`SelectCheapest`） |
| 触发条件 | 仅在 `cfg.CostBasedRoutingEnabled()` 且 catalog 非空时，由 `model_router.go:247-262`（Route）与 `:468-483`（RouteForStreaming）调用 |
| 请求侧约束推导 | `model_router.go:155`（`requestConstraints`） |
| 模型过滤 / provider 集合 | `selector.go:197`（`modelMatches`）、`:147`（`providerSet`，全局与场景偏好取交集） |

`cost_routing` 只从 models.dev catalog 取 `providers` 与 `models`；`scenarios` 不在 catalog 内。

### 场景策略字段（权威定义）

`internal/config/config.go:71`（`CostScenario`）：`Description`、`RequiresTools`、`RequiresVision`、`RequiresReasoning`、`MinContextWindow`、`PreferredProviders`。每个字段都是在请求自身已隐含约束之上的**增量**，未配置的场景即"选能服务该请求的最便宜模型"。

键列表在 `internal/config/config.go:54`（`CostScenarioNames`，9 项）；`override` 有意不在列表内（`:52-53`）。

三处必须同步修改：

- 漂移守卫测试 `internal/router/cost_routing_sqlite_test.go:19`（`TestCostScenarioNamesMatchRouterScenarios`），`config.go:51` 的注释即指此
- 未知键在加载期被拒绝：`internal/config/loader.go:477-485`（`validateCostScenarios`），避免拼错静默失效
- 已启用开关：`cost_routing.enabled` 或遗留 `enable_cost_based_routing`
