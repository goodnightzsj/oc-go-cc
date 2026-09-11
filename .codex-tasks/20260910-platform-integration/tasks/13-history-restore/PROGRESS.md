# Progress

## Context Recovery Block

- single-full，1/5；#2本地清理开关修复并通过，当前#1取证和#3恢复演练；用户已明确授权关闭清理和恢复，旧只读限制不再适用。
- 真源TODO.csv。基线main/560a11f，工作区原为干净。旧官方账单1390、requests0；不能把8月10日单份备份当全部历史，需检查后续备份与官方全分页。
- 已读llmdoc计费审计最终增补：平台timeCreated为完成时间，输入是纯输入、缓存单列；旧platform_reconcile.py金额配对和硬编码不能直接执行。
- 两项只读委托均因Encrypted function output无法解码失败，无结论；主线程接手，不重试或计为独立审查通过。
- 下一步：核对6561条已有官方明细和备份数据；Edge握手未获完成，需要用户允许后才能取得当前全量；不把旧快照当当前全量。禁用清理与平台排序验证后同次部署，再恢复。

## 2026-09-12 本轮继续

- 远端仍b3c947d、release20260911230242-8d1413ba382b、active/running、NRestarts0，requests0；没有重复部署或写入生产。
- 备份只读检查发现9月4日两份：原始请求2072条（8月29日至9月4日）、早期全量6365条（8月6日至9月4日；大部分输入缓存未拆分）。不能直接恢复后者。8月27日多份中间备份含已被后续纠正/去重的数据，需官方对账。
- 仅从本项目Claude session白名单提取证据，最终9月4日11:19Z记录完整翻页抓取6408行；随后增量至6561行。`~/.routatic-sync/records.tsv`现存6561唯一ID，8月6日06:56:37Z至9月4日11:39:44Z、3模型，cost总计2514740738单位。输入42921501、输出5069315；cache_read含官方null，需要显式解析。未运行旧oc_sync.py（其伪造success、非事务REPLACE和六位舍入不适用）。
- 当前DevToolsActivePort仍9222；新Node CDP连接只完成TCP，WebSocket最终超时（退出1），未取得任何页面数据，未重启浏览器或读取Cookie。
- 继承的`TestRetentionPolicy`首次失败在disabled退出断言；修复NewRetention的0默认与负值禁用，run在创建ticker前返回。`go test -race ./internal/storage ./internal/config -count=1`通过（11.433s/2.998s）。

## 2026-09-12 恢复执行

- 主线程重新核实同项目session第3062行的最终6408行全分页和4450行最终文档更正；保留旧文档曾误判缓存字段不存在和索引滞后的更正，不执行旧清表脚本。
- SSH只读核对：仍运行b3c947d，active/running，NRestarts=0；requests仍0，原始账单1390。9月4日普通备份2072行是本地请求，full备份6365行主要是未拆缓存的旧导入。
- 现有Edge9222监听，但单次新连接30秒握手超时。没有重启浏览器、读取Cookie、取得官方新导出或写入生产。保留失败并请求用户在现有Edge允许调试；不自动循环重连。
- 在连接恢复前推进可独立完成的平台展示排序、清理开关发布和CommandCode测试准备；全量历史恢复的验收不缩减。

## 已有精确快照的子集演练

- 用户已授权恢复，当前官方新导出受阻。为继续安全推进，使用现存provider_usage中字段完整的1390条做第一阶段；不把它当全部历史，不导入缺缓存写入字段的6561条旧TSV。
- 远端独立备份`/root/oc-go-cc/.tmp/history-restore-20260912-8rqoazna`（目录0700、数据与配置0600），含data.before.db、config.before.json及rehearsal.db；没有复制凭证或业务库到本机，没有修改生产数据。
- 复用既有`costs sync-requests`在副本dry-run、apply、二次apply通过：1390条、input11402970、output565578、cache_read451281285、cache_creation0、cost_units303965577逐项相等；ambiguous/conflicting/remove均0；二次新增和更新均0。
- 导入行details_known=0、cost_source=provider、usage_trusted=1；既有原始快照没有官方ID，所以沿用现有同步器的内容指纹ID，后续完整官方导出需按同一条账单匹配，不能再并排插入一套官方ID。
- 下一步：全量门禁后部署负值禁用，再恢复这个已核实子集；其余历史对账继续等待原Edge连接，不把部分成功标记为任务完成。

## 2026-09-12 本轮恢复核实

- 仅解析本项目session的3062/4450两条最终助手消息并白名单输出相关段落，确认最终6408行抓取到第130空页、官方ID为`oc-<usg_id>`、纯输入与缓存分列及费用1e-8单位。旧清空请求表动作仅作历史证据，本次不执行。
- SSH重新确认b3c947d、release20260911230242-8d1413ba382b、PID63785、active/running、NRestarts0、health=ok、quick_check=ok；requests0/provider_usage1390。原storage只有analytics_baseline，retention_days有效默认为7；原配置尚未改变。
- 原Edge9222监听，本轮单次30秒握手仍无响应；没有重启、读取Cookie或模拟线上浏览器。已请用户在现有Edge允许调试，并继续可独立完成的发布/子集恢复和CommandCode验证。
- 平台排序及保留策略最终全量666/1103 race、vet、六目标和Codex合成双轮通过；后续生产恢复不得回退到旧的非正数按7天清理版本。
