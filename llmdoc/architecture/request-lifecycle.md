# 请求与关闭生命周期

- 应用由 `cmd/routatic-proxy/main.go:228` 的signal.NotifyContext统一接收父context、SIGINT/SIGTERM和托盘退出。
- `internal/server/server.go:203` 的Start只服务并返回监听错误，不监听信号、不自动关闭DB；Shutdown排空HTTP，Close幂等停止retention并释放SQLite。
- `cmd/routatic-proxy/lifecycle.go:13` 同时停止代理和GUI接入，等待两者排空后再关闭共享DB。CLI使用5秒截止；超时返回失败并保留仍在使用的资源，不保证进程强杀后的记账完整。
- `internal/gui/server.go:403` 的Shutdown取消共享后台context、排空HTTP并等待limits与渠道候选扫描循环结束；父context取消不会强制切断在途GUI请求。候选扫描复用此生命周期，无额外请求钩子；每小时读取现有capture，扫描取消后不提交未完成的文件游标或新渠道集合。
- CLI成功排空后关闭capture logger及文件；CaptureBody.Close等待回调，避免logger关闭后仍有后台写channel。
- 配置DB打开失败即启动失败，不静默降级成无持久化服务，也不再额外打开一份默认数据库。
- 回归：`internal/server/lifecycle_test.go`、`internal/gui/lifecycle_test.go`、`internal/client/capture_test.go`；独立记账ID及回显合同在 `internal/handlers/accounting_id_test.go`。
