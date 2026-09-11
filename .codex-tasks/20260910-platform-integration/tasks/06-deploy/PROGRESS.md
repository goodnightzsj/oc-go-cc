# Progress

## Context Recovery Block

- 当前：7/7；AWS表头增量0b28ab2已推送部署并通过生产复验；真实账户状态/权限另保留 #8 未完成。
- 真源：TODO.csv。
- 来源：本项目 Claude 2026-09-04 session、llmdoc/startup.md 与 scripts/prod-deploy.sh。
- 下一步：#7UI/UX只读分析已完成；#8需要用户补足账户权限/配置或明确未公开接口的验收边界。仅文档收尾不重启已验收服务。

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

## 第二次部署与空性能数据补齐

- `7104220a941f0b08ecc5974dcf731824071c50fe` 推送/部署成功，release `20260911054749-5d79176c40cf`，服务active/running、NRestarts=0。新备份 `predeploy-20260911-fix-JpiOdzd8`；配置不变、SQLite健康、原1390条平台记录无缺失。模型列表200、Responses入站GET按合同返回405，未发送真实推理请求。
- 第二轮生产浏览器通过所有 Overview/History 平台切换，到第75项发现 `/api/perf/models` 空结果仍为null。证据 `/tmp/oc-go-cc-deploy-fix-verify.I3hRE1/production-smoke/result.json`；无脚本异常/写请求，尚未进入套餐。当前版本已包含凭证隔离修复，因此保留运行而非退回缺少该修复的版本。
- 扩展原空账本回归覆盖性能端点，六种平台范围均先失败；在唯一HTTP输出处将空slice初始化为空数组，文档同步。修后最新全量641顶层/1039含子测试race、vet、六目标CGO=0构建通过；GUI全包race在测试fixture扩展后另行通过。
- 新增仅测试用的 `ROUTATIC_BROWSER_EMPTY=1`，生产只读脚本对完整空账本再验收204项、29平台/页面组合、21布局通过，0脚本错误/0写请求；原含数据447矩阵证据仍保留。最终自动证据 `/tmp/oc-go-cc-release-final.AVax0F/`。fixture已关闭。

## 最终部署验收

- 运行提交 `7c2d07c435ad6d13fbba1bbc95f1fc3cd84933ef`；release `/root/oc-go-cc/.tmp/prod/releases/20260911055621-3fad77f342fc`；版本 `v0.1.4-beta.53-48-g7c2d07c`，PID8365、active/running、NRestarts=0、health=ok。
- 最后备份 `/root/oc-go-cc/.tmp/predeploy-20260911-final-QPrGa2Mw` 保留，配置散列一致；SQLite quick_check=ok，原 requests/provider_usage 主键无丢失，provider_usage 1390条。无关 .ace-tool 未改动。
- 真实远端GUI经SSH隧道完成204检查、29平台/页面组合、21布局，196个只读请求，无脚本错误或被拦写请求；证据 `/tmp/oc-go-cc-release-final.AVax0F/production-smoke/result.json` 和同目录1440/390截图。
- 5个 analytics/performance/quota 非法 provider 请求都返回400。模型列表200、Responses GET405验证沿用同代码的上一部署轮；未发真实推理/扣费请求。
- 账户实态：Go上游返回403（界面明确显示订阅/Key权限提示）；OpenRouter not_configured、Credits not_configured；CommandCode not_configured；Zen和Bedrock能力unavailable。这里证明错误/未配置呈现正常，不等于账户余额可用。#8保留待办，不缩小验收口径。

## AWS增量部署前预检

- 原生SSH主机信任检查通过，/root/oc-go-cc main7c2d07c；仅无关.ace-tool未跟踪，保持不变。
- 当前release20260911055621-3fad77f342fc、PID8365、active/running、NRestarts0、health=ok。sqlite3存在，既有配置和SQLite路径存在，未读取凭证或私有记录。
- 本地AWS增量最新全量650/1073、六目标、Codex及有数据/空数据浏览器通过；下一步创建私有备份，不改变账户配置或开启收费查询。
- 已完成服务端私有备份 `/root/oc-go-cc/.tmp/predeploy-20260911-aws-SV1xWrJc`，目录0700，保留配置、两个存在的catalog、原提交/release及SQLite一致性快照。quick_check=ok；requests=0，provider_usage=1390，原服务仍active。

## AWS增量发布恢复与生产复验

- 恢复时本地main、origin/main、远端HEAD均为 `464c64f5a4d56619831186f68548fd82853d5ca7`，且远端实际二进制来自release `20260911074726-dc89acaaa9b9`；不重复部署。服务PID62339、active/running、NRestarts0，health=ok，版本v0.1.4-beta.53-49-g464c64f。
- 以原生可信SSH创建临时loopback隧道，运行原有只读production-smoke.cjs：210检查、29平台页面组合、21布局全部通过，196只读请求，0pageerror/0blocked，Chrome152，UI build88ddce0410bd。
- AWS独立账单区和三个配置字段均存在；账单disabled且收费按钮禁用。Go error、Zen no_public_account_api、OpenRouter及CommandCode not_configured均明确呈现；未调用真实推理或AWS收费接口。
- 配置与发布前备份cmp一致；SQLite quick_check=ok，原requests/provider_usage主键缺失数均0，平台记录1390条。私有备份0700与无关.ace-tool保留。
- 证据目录 `/tmp/oc-go-cc-release-resume.cBjqie/production-smoke/` 包含result.json及七页1440/390截图；同目录上层source-check.log证明当前产品源码与650/1073全量验证快照一致。JS语法和git diff --check通过。

## 表头增量发布与生产复验

- 本次先核对当前main/工作区、SSH主机信任、旧release和健康；创建0700备份 `/root/oc-go-cc/.tmp/predeploy-20260911-header-6vid7xWH`，含配置、SQLite一致性快照、catalog和原release/commit。
- `0b28ab2bc6294a018421d2bcdb97347a1dfb2329` 已推送并通过ff-only拉取；既有prod-deploy脚本发布至release `20260911083542-8bef17b7ea86`。运行版本v0.1.4-beta.53-50-g0b28ab2、PID11558、active/running、NRestarts0、health=ok。catalog同步的未知provider警告和重启轮询的一次短暂连接拒绝已保留，不当作最终失败。
- 生产只读浏览器210检查、29平台/页面组合、21布局、197请求通过；0pageerror/0blocked，UI build95403ba4a0a5，Chrome152。未调用真实推理、配置保存或AWS收费接口。
- 配置与备份cmp相同；SQLite quick_check=ok，原requests缺失0，provider_usage改变或缺失0（1390行）；备份权限700、无关.ace-tool保持原样。
- 证据 `/tmp/oc-go-cc-header-final.PYhcpx/production-smoke/`；Go额度实际仍403，其它平台权限/合同状态保留#8。本轮可以进入UIUX分析，但不代表真实账户能力全部验收。
