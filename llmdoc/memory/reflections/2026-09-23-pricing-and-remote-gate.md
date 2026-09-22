# 反思：2026-09-23 OpenRouter 峰谷/长上下文计价与远端 403/502 排障

**任务**：给 OpenRouter 加两段计价能力（per-model 时间窗峰谷、`min_prompt_tokens` 长上下文分档），修 GUI 两个列错位 bug，并排掉远端 403/502。

## 做对的事

- **计价方向按"失败往哪边偏"选**。时间窗形态被解析后**故意跳过**（条件未读的 override 会变成无条件，按列表恰好排第一的档定价）；未覆盖的模型回落底价。两处残余误差都是**少收**而不是错收（`internal/catalog/types.go:141`、`record.go:296`）。`allDays` 用 bool 且零值安全同理：漏写保持工作日限制，失败方向是少收（`record.go:104-108`）。
- **倍率对齐的是计价来源的底价，不是载荷 headline**。`hy3` 载荷 lead 0.132/0.528、折扣 0.0825/0.33，models.dev 记的底价是后者，所以倍率是 1.6 不是 2。`TestOpenRouterPeakMatchesThePublishedOverrides`（`peak_test.go:292`）钉住这个算术，并在注释里声明"若 models.dev 改用 headline 底价，这些数字一起错，此测试是暴露点"。
- **架构改动的理由写进代码注释**：`peakSchedules` 从"每平台一条"改为"每 rule 自带 `models`/`windows`/`multiplier`/`allDays`"，因为 `hy3`（1.6x，每天 00-16 UTC）与 DeepSeek 对（2x，工作日）方向+倍率都不同，平台级窗口无法同时表达（`record.go:266-298`）。
- **分档阈值比较对象是完整 prompt**（新鲜输入+缓存读+缓存写，`promptTokensOf`），不是仅新鲜输入——否则缓存重的请求会被放进便宜档，而上游按贵档计费（`requests.go:536-540`）。
- **GUI 两个 bug 用"表头半 + 行半"互相钉住**：`TestPerformanceHeaderEndsWithHealth` 读 index.html 的表头顺序，另一半点级断言渲染行的 cell index（`perf_columns_test.go:9-16`）。单点都不够，合起来才把行对齐到表头。修前该表 sort key 正确、单元格错列，读起来"像有效数据"，比明显坏掉更危险。
- **远端白名单漂移定位到"第四份副本"**：fail2ban / UFW / DOCKER-USER 三层由 `sync-whitelist.sh` 生成，nginx 的 `geo $allowed_ip` 块却是手工维护的第四份。已并入生成（`_geo-render.py` 只重写 geo 块 + `nginx -t` + 失败回滚）。
- 502 追到 TUN 模式 PMTUD 黑洞：实测路径 MTU 到 Cloudflare 1482 / 局域网 1499（差 17 字节 TUN 封装），而 `utun1500` 声明 1500；ICMP "packet too big" 不回传，TLS 握手受害最重。MTU 改 1480 后握手均值 2.92s → 1.38s。**差值必须先量出来再改，不按经验拍 MTU。**

## 教训

1. **两次"注入测试"是静默空操作**。我用 Python `str.replace` 注入缺陷，锚点字符串没匹配上、替换根本没发生，我却输出了"注入成功"。后来用正确锚点重做，守卫才如期失败。**测的是"未注入的代码没报错"，等于没测。** 注入必须断言"替换前后的文件字节确实不同"，否则不得声称守卫已验证。

2. **排序测试是同义反复**。2 行 fixture 里插入顺序、按时间排序、按总 token 排序三者恰好一致，因此删掉修复后测试仍通过。换成 4 行（`requests_query_test.go:22-33`，三种排序各选不同行首）才有区分力。**排序/映射类断言，先想清 fixture 能不能把"正确实现"和"错误实现"的期望输出分开，再写断言。**

3. **把 API key 明文打进了对话**。脱敏正则只匹配 `sk-`，该 key 是 `sk_` 前缀。已向用户披露并建议轮换。**脱敏正则要么覆盖两种前缀，要么干脆不打印疑似凭证的字段**——只匹配一种前缀的安全网等于没有。

4. **嵌套 ssh heredoc 把 Python 脚本写进 shell 文件**，损坏了远端 `/usr/local/bin/sync-whitelist.sh`（`bash -n` 报语法错），已从备份恢复。**跨主机写脚本先本地生成 + 本地 `bash -n` + 本地空跑，再 `scp` + `install -m 755`**；不要在 ssh heredoc 里套 heredoc。

5. **部署会切断自己的链路**。被测服务正是本会话的反向后端，重启它必然打断请求。**把部署脚本 detached（`nohup setsid`）跑，再做独立验证。**

6. **24 小时聚合的均值会掩盖窗口内尖峰**。403 我一开始用 24h 聚合看着 0.12% 就断言"不是并发问题"；分段后 14:50–15:18 窗口是 2.065% vs 其他时段 0.032%（64 倍）。最终判断：403 = 上游降级（IP 白名单门），429 才与并发相关。**异常率类结论必须分时段看，不能用全天均值代替窗口分析。**

## 晋升候选

| 教训 | 建议落点 | 判断 |
|------|----------|------|
| 注入测试必须先证明注入生效（#1） | **`must/`** | **该进**。这是验证完整性的硬规则，且不限于本仓库：任何"我注入缺陷、守卫失败了"的声称都必须附"文件确实被改过"的证据，否则是自证。建议新建 `must/verification-integrity.md`，把 #1 与 #2 合成一节"不可失败的测试" |
| 排序/映射 fixture 必须让正确与错误实现的期望输出不同（#2） | **`must/`**（同一条目） | **该进**。与 #1 是同一命题（测试没有区分力），分开写会重复 |
| 计价失败方向：宁少收不错收；倍率对齐计价来源底价（A 节） | **`must/accounting-baseline.md`** | **该进**。现有 must 已管"价格按平台分表/必须带 provider"，方向选择与"对齐哪个来源"是同一类的记账硬约束，补一条即可，不必新建文档 |
| 脱敏正则需覆盖实际前缀，否则不打印疑似凭证字段（#3） | `must/` | 部分进。全局 CLAUDE.md 已有"凭证不得输出"的 Hard 规则，但"正则只匹配 sk- 就以为安全"这个具体失败模式值得写进上面的 `verification-integrity.md` 或远端运维条目 |
| 跨主机脚本：本地生成 + `bash -n` + 空跑，再 scp（#4） | `reference/remote-ops.md`（新建） | 该建。连同"部署服务即切断自身链路要 detached 跑"（#5）、nginx geo 第四份副本漂移、TUN MTU/PMTUD 一起，构成远端运维参考。目前仓库无对应文档 |
| 异常率要分时段，全天均值会掩盖窗口尖峰（#6） | `reference/remote-ops.md` | 该进（同上）。属排障方法，非硬约束，故列 reference 非 must |

**该进 `must/` 的条目**：#1 + #2（合并为"不可失败的测试"，含注入必须证明生效 + fixture 必须能区分实现），以及 A 节的"计价失败方向与对齐来源"补进 `must/accounting-baseline.md`。

## 落地结果（同日回写）

- 已建 `must/verification-integrity.md`：#1 + #2 合成「不可失败的测试」两节（注入必须证明注入生效、fixture 必须能区分正确与错误实现），#3 脱敏并入同篇的「脱敏」节。
- `must/accounting-baseline.md` 新增「峰谷与分档」节：失败方向宁少收不错收、倍率对齐计价来源底价而非 headline、阈值比较完整 prompt 且严格大于、规则形态改为每 rule 自带。
- 未建 `reference/remote-ops.md`（白名单四层生成链 / nginx `geo` / TUN MTU-PMTUD）。已在 `index.md` 的 Gaps 登记，留给下次。

## Reflection handoff

- 本次仅新建本反思文件；按指令未改动 `llmdoc/` 其它文件，因此 `llmdoc/index.md` 的 Memory 列表**尚未登记本篇**，也未把新 Gaps 写进去。下一次 `llmdoc` 维护时补。
- 新增 Gaps（供 index.md 记录）：远端运维面（白名单生成链、nginx geo、TUN MTU/PMTUD）在本仓库无文档；`docs/openrouter.md` 的计价来源与时窗说明已存在，但 llmdoc 侧无对应 reference 条目。
- 待用户决策的晋升动作：`must/verification-integrity.md`（新建）、`must/accounting-baseline.md`（补计价方向一节）、`reference/remote-ops.md`（新建）。本反思不擅自晋升，按 reflector 职责交回。
