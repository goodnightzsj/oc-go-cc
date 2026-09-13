# Progress

## Context Recovery Block

- single-full，3/3实测与定位完成；Codex与Claude主请求均成功，另保留Claude自动标题结构化输出的实际兼容缺口。完成的是测试交付，不是全功能兼容或该缺口的实现修复。
- 用户已授权指定DeepSeek模型的两条真实测试；不能扩大成压测或擅自改模型。
- 下一步：不重复消耗测试；若继续实现Claude完整兼容，应先验证指定模型的严格结构化输出合同并无损映射。正式服务未新增CommandCode模型别名，测试隔离实例已退出，凭证副本及隧道已清理。

## 测试准备

- 已完成远端清理关闭和1390条精确子集恢复，官方全量仍等Edge连接，双客户端测试独立推进。
- 官方无认证`GET https://api.commandcode.ai/provider/v1/models`返回69个模型，确有deepseek/deepseek-v4.1-flash（DeepSeek V4.1 Flash，context_length1000000）。没有换为v4-flash。
- 已核实本机codex-cli0.144.3-cometix、Claude Code2.1.263的help；Codex使用临时HOME/CODEX_HOME及Responses并禁用客户端重试，Claude使用bare模式跳过用户配置/keychain/hooks/MCP并禁用工具。
- 当前生产/v1/models353项中CommandCode0，默认Go模型和fallback不改；独立实例使用正式部署二进制，配置仅在远端0700目录内存在，停止时删除；日志和DB不会混入生产历史。

## 2026-09-12 恢复后核验

- 任务旧恢复块未记录随后执行的CLI测试，实际证据优先。`/tmp/oc-go-cc-recovery-20260912.S23g9t/codex-live.log`包含`CC_CODEX_OK`和turn.completed；远端`commandcode-live-sfj9jtjo/requests.db`有唯一对应记录`req-1789167132-1`，provider=commandcode、model=deepseek/deepseek-v4.1-flash、success=1。
- Codex报告input7075、cached4352、output6；DB纯输入2723、缓存读取4352、输出6，2723+4352=7075。DB费用NULL（未核实价格），不写假零。CLI的metadata warning没有改模型，远端routing与streaming completed均为指定模型。
- Claude旧日志只有generate_session_title的unrecognized_model，旧实例日志没有对应入站请求；该次不能视为上游模型/协议失败，也不能计为通过。
- 恢复时旧实例进程已不存在，但临时config.json仍在。runner补充SIGINT/SIGTERM/SIGHUP的finally清理；后续先移除确认无进程使用的旧凭证副本，保留其DB与脱敏日志作为证据。
- 本机`codex exec --help`和`claude --help`重新核实；openai-docs手册缓存校验为current，确认Responses、base_url/env_key及request_max_retries/stream_max_retries配置。

## Claude主请求与附带失败

- 旧实例确认退出后移除其遗留config.json，保留DB。新实例为正式release的相同二进制，路径`/root/oc-go-cc/.tmp/commandcode-live-574theo_`；只监听loopback，正式配置、默认路由和DB不变。
- 首次隧道刚启动即健康探测得到curl7；等待隧道就绪并通过health校验确认同release后才启动CLI。没有在连接失败时发送推理请求。
- 本机Claude2.1.263使用bare、空tools、strict-mcp-config、空setting-sources、no-session-persistence与隔离HOME；返回CC_CLAUDE_OK、end_turn、terminal completed、is_error=false、exit0，未超时。记录req-1789169739-2，model指定正确、provider=commandcode、success1、streaming1、attempt1。
- CLI与DB均input144/output21/cache0；没有伪造费用，DB cost=NULL。CLI total_cost基于unknown价格，未作官方扣费证据。
- 同一次运行自动generate_session_title也发请求；因本地Chat转换器拒绝output_config.format，产生1次流错误和3次nonstream502。源码权威点internal/transformer/request.go:180；并非指定模型主请求或平台Key失效，未静默丢字段修饰成功。
- 官方Provider页面本轮MySearch读取HTTP200（正文缓存2026-09-11T21:47Z），仍声明非Claude模型走Chat、引用标准schema，没有单独保证指定模型的严格json_schema；不能把这一缺口解释为已证明上游绝不支持。未为此继续追加付费探测。
- 输入stop后runner退出0且credential_copy_removed=true；SSH隧道已关闭。原始本机CLI证据`/tmp/oc-go-cc-live-20260912.wkFI6T/claude-live.log`和`claude-debug.log`只含测试内容；摘要`raw/live-result.json`。两个只读委托未回传可验证结论，未计独立审查通过；主要结论来自主线程源代码和实际日志。
