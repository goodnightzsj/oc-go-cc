# Progress

- 2026-08-13 14:08 +08:00: Confirmed the direct-refresh race: hash activation runs in a microtask while `AnalyticsModule.init()` waits 250 ms, so `queryParams()` can serialize empty dates before its guarded fetch.
- 2026-08-13 14:09 +08:00: Defined the regression contract: analytics initialization must be synchronous and the delayed 250 ms bootstrap must remain absent.
- 2026-08-13 14:10 +08:00: Replaced the timer with synchronous initialization. The focused GUI contract test and `node --check` pass.
- 2026-08-13 14:12 +08:00: Full `go test ./...` and `git diff --check` pass; scoped diff contains no unrelated deployment-script edits.
