/* ── i18n ────────────────────────────────────────────────────────── */
const TRANSLATIONS = {
  en: {
    'lang.toggle': '中文',
    'shell.console': 'Console',
    'shell.workspace': 'Workspace',
    'shell.management': 'Management',
    'shell.navigation': 'Primary navigation',
    'shell.platforms': 'Five platforms. One workspace.',
    'shell.skip': 'Skip to content',
    'theme.light': 'Switch to light theme',
    'theme.dark': 'Switch to dark theme',
    'overview.description': 'Traffic, recorded cost and service health at a glance.',
    'analytics.description': 'Explore token composition, recorded costs and activity over time.',
    'quota.description': 'Account limits and subscriptions, with the local ledger kept separate.',
    'data.retained': 'Retained',
    'data.localEmpty': 'No retained requests in this view',
    'data.localEmptyHint': 'Check the platform, filters and retention policy. An empty local ledger does not mean no platform usage.',
    'history.advanced': 'Advanced filters',
    'history.previous': 'Previous',
    'history.next': 'Next',
    'history.noMatches': 'No requests match these filters',
    'history.emptyFiltersHint': 'Adjust the platform or date range, or reset the filters. Only records still kept by this instance can be searched.',
    'perf.description': 'Compare measured latency and known outcomes across models.',
    'perf.p50Hint': 'Typical request latency',
    'perf.p90Hint': '90% of samples finish within this time',
    'perf.p99Hint': 'Tail latency of the slowest requests',
    'perf.emptyHint': 'No measured samples in this view. Imported bills can have usage without latency or outcome details.',
    'fallback.description': 'Set the model order for each routing scenario.',
    'fallback.guideTitle': 'Routing order',
    'fallback.orderHint': 'The primary model comes from the scenario configuration. Drag or use the arrow buttons to arrange the fallback models below it.',
    'fallback.saveTitle': 'Review, then apply',
    'fallback.saveHint': 'Preview the chain before saving. Save applies your pending fallback changes; platform credentials stay unchanged.',
    'setting.platformsHint': 'Configure each platform independently. Only changed fields are saved; masked credentials stay untouched.',
    'setting.bedrockHint': 'Inference & billing identity',
    'setting.openrouterHint': 'Inference & account credits',
    'setting.commandcodeHint': 'Native Messages & Alpha account',
    'setting.server': 'Server',
    'setting.logging': 'Logging',
    'status.checking': 'Checking…',
    'export.csv': 'Export CSV',
    'export.working': 'Exporting…',
    'export.ok': 'Exported {n} rows',
    'export.empty': 'No history to export',
    'export.fail': 'Export failed',
    'status.stale': 'No response',
    'status.staleFor': 'No response {secs}s',
    'status.running': 'Running',
    'status.stopped': 'Stopped',
    'status.connected': 'Connected',
    'tab.overview': 'Overview',
    'tab.history': 'History',
    'tab.performance': 'Performance',
    'tab.fallback': 'Fallback',
    'tab.analytics': 'Analytics',
    'tab.quota': 'Usage & Billing',
    'tab.settings': 'Settings',
    'quota.title': 'Usage & Billing',
    'quota.openSettings': 'Platform settings',
    'quota.sourceDetails': 'Source details',
    'quota.refresh': 'Refresh',
    'quota.bottleneck': 'Tightest window',
    'quota.remaining': 'Remaining',
    'quota.nextReset': 'Next reset',
    'quota.plan': 'Plan',
    'quota.rolling5h': '5-hour rolling',
    'quota.weekly': 'Weekly',
    'quota.monthly': 'Monthly',
    'quota.used': 'Used',
    'quota.limit': 'Limit',
    'quota.left': 'Left',
    'quota.leftShort': 'left',
    'quota.resetsIn': 'Resets in {d}',
    'quota.resetsAtTime': 'at {time}',
    'quota.resetUnknown': 'Reset time unknown',
    'quota.derived': 'Dollars derived from plan limits',
    'quota.exhausted': 'Exhausted',
    'quota.keyLabel': 'Key',
    'quota.keyCount': '{n} keys',
    'quota.noKey': 'No OpenCode Go API key configured',
    'quota.noKeyHint': 'Add a key under Settings → OpenCode Go to see the plan quota here.',
    'quota.loadFail': 'Could not load quota',
    'quota.updated': 'Updated {time}',
    'quota.cached': 'cached',
    'quota.endpoint': 'Endpoint {url}',
    'quota.unofficial': 'Undocumented upstream endpoint; the response shape may change.',
    'quota.modelLimits': 'Local model costs in this plan window',
    'quota.modelLimitsNote': 'Go records in this instance, including estimates; not an account bill · allowances from Go docs · updated {time}',
    'quota.model': 'Model',
    'quota.modelUsed': 'Local recorded cost',
    'quota.modelAllowance': 'Monthly quota',
    'quota.percent': '%',
    'quota.total': 'Total',
    'quota.poolShare': 'Local costs as a share of the documented allowances; not the official account percentage',
    'quota.indexLagNotice': 'Account gauges come from the upstream quota endpoint. Model costs below come from this instance and may cover fewer requests.',
    'quota.multiKeyUsage': 'Local records do not identify the account used. Per-model account usage is unavailable when multiple keys are configured.',
    'quota.localUsageFail': 'Local model costs could not be loaded.',
    'quota.notRetrieved': '{provider}: quota not retrieved',
    'quota.providerUnavailable': 'This dashboard does not query plan limits for this platform. Its local request records remain available in History and Analytics.',
    'quota.viewLocal': 'View local requests',
    'quota.localTitle': 'Local ledger',
    'quota.localNote': '{provider} · Last {days} days (UTC), as recorded by this instance. Includes estimates; not an account bill or balance.',
    'quota.localLoadFail': 'Could not load the local ledger. Refresh to retry. ',
    'quota.unknownCosts': 'Requests with unknown cost',
    'quota.status.available': 'Account data retrieved',
    'quota.status.partial': 'Some account data could not be retrieved',
    'quota.status.not_configured': 'Account credentials not configured',
    'quota.status.unavailable': 'Account data unavailable',
    'quota.status.error': 'Account data lookup failed',
    'quota.source.upstream_api': 'Source: upstream quota API',
    'quota.source.official_api': 'Source: official account API',
    'quota.source.official_alpha_api': 'Source: CommandCode Alpha account API',
    'quota.source.none': 'Official account data not retrieved',
    'quota.reason.no_public_account_api': 'No public account quota API is documented for this platform. Use its official console for billing; the local ledger is shown below.',
    'quota.reason.aws_billing_disabled': 'AWS billing queries are disabled. Enable them in Settings with an explicit account ID and a separate AWS SDK identity.',
    'quota.reason.aws_billing_refresh_required': 'Ready for a manual billing query. Platform changes and automatic refresh never call the paid AWS API.',
    'quota.reason.aws_billing_no_data': 'No matching Bedrock billing data was returned for this account and period. This is not a zero balance; check the account, service coverage and AWS data delay.',
    'aws.billingTitle': 'AWS official service costs',
    'aws.billingQuery': 'Query AWS billing (paid)',
    'aws.billingHint': 'Cost Explorer charges per API request, including pagination. Queries use a separate IAM identity and are never automatic. Ordinary refresh reads the cached snapshot only.',
    'aws.billingScope': 'Last 30 complete UTC days · UnblendedCost for the services listed below. Not an account balance or this proxy’s ledger; AWS data can be delayed or revised.',
    'aws.billingTotal': 'Reported service cost',
    'aws.billingAccount': 'Billing account ID',
    'aws.billingPeriod': 'UTC period (end exclusive)',
    'aws.billingServices': 'Included AWS service names',
    'aws.billingEstimated': 'AWS estimate; subject to revision',
    'aws.billingReported': 'Reported by AWS',
    'aws.billingEnable': 'Enable manual AWS billing queries (paid)',
    'aws.billingProfile': 'AWS SDK profile (optional)',
    'aws.billingConfigHint': 'Account ID is required when enabled. The service uses standard AWS SDK credentials or the selected profile, never a Bedrock inference key. IAM must allow ce:GetDimensionValues and ce:GetCostAndUsage.',
    'quota.noProviderKey': 'No {provider} inference API key configured',
    'quota.noProviderKeyHint': 'Add a provider key in Settings to query its key usage.',
    'quota.yes': 'Yes',
    'quota.no': 'No',
    'openrouter.managementKey': 'Management API key (optional)',
    'openrouter.managementHint': 'Optional management key used only to query account balance, never for inference.',
    'openrouter.keyScope': 'Key limits are spending caps, not account balances. Key usage and BYOK usage are shown separately; key balances are never added together.',
    'openrouter.accountCredits': 'Account credits',
    'openrouter.totalCredits': 'Total credits',
    'openrouter.totalUsage': 'Account credit usage',
    'openrouter.balance': 'Account balance',
    'openrouter.noManagementKey': 'Account balance not queried. Add an optional OpenRouter management API key in Settings; inference keys are not used for this lookup.',
    'openrouter.creditsFail': 'Account balance could not be retrieved',
    'openrouter.limit': 'Key spending cap',
    'openrouter.limitRemaining': 'Key cap remaining',
    'openrouter.limitReset': 'Key cap reset interval',
    'openrouter.noLimit': 'This key has no spending cap',
    'openrouter.usage': 'OpenRouter usage',
    'openrouter.byokUsage': 'BYOK usage',
    'openrouter.includeByok': 'BYOK counts toward the key cap',
    'openrouter.freeTier': 'Free-tier key',
    'openrouter.expires': 'Key expiry',
    'cmd.gotoQuota': 'Go to Usage & Billing',
    'overview.title': 'Dashboard',
    'analytics.title': 'Usage Analytics',
    'analytics.refresh': 'Refresh',
    'analytics.autoRefreshOff': 'Auto-refresh: Off',
    'analytics.autoRefresh5s': 'Auto-refresh: 5s',
    'analytics.autoRefresh30s': 'Auto-refresh: 30s',
    'analytics.autoRefresh60s': 'Auto-refresh: 60s',
    'analytics.totalRequests': 'Total Requests',
    'analytics.requestsUnit': 'requests',
    'analytics.totalTokens': 'Total Tokens',
    'analytics.inout': 'in / out',
    'analytics.cost': 'Cost',
    'analytics.currencyUSD': 'USD',
    'analytics.unknownCosts': '{n} requests with unknown cost; totals include known costs only',
    'analytics.knownSubtotal': 'Known {value}',
    'analytics.utc': 'Analytics dates and time buckets use UTC',
    'label.apiKeys': 'API keys (comma-separated, takes precedence over the single key)',
    'label.apiKeysHint': 'Replace the entire list to edit masked keys, or clear it to use the single key. Environment overrides still take precedence.',
    'commandcode.keyHint': 'Uses its own API key; global keys are never sent to CommandCode.',
    'commandcode.quotaHint': 'Queries the official Alpha account endpoints with this platform’s API key. No browser session is required. Alpha response fields may change.',
    'commandcode.accountScope': 'Official Alpha account data · USD-denominated usage credits, not cash or this instance’s ledger. Each key is shown separately; balances are not added together.',
    'commandcode.credits': 'Usage credits remaining',
    'commandcode.freeCredits': 'Free credits',
    'commandcode.monthlyCredits': 'Monthly credits remaining',
    'commandcode.purchasedCredits': 'Purchased credits',
    'commandcode.monthlyLimitUnknown': 'The Alpha API does not report the monthly grant. No monthly utilization percentage is inferred.',
    'commandcode.windowLimits': 'Rolling usage limits',
    'commandcode.noWindowLimits': 'No rolling limits reported for this account',
    'commandcode.noSubscription': 'No subscription returned by the account API',
    'commandcode.billingPeriod': 'Official billing period (UTC)',
    'commandcode.cancelAtEnd': 'Cancels at period end',
    'commandcode.usageSummary': 'Official usage summary',
    'commandcode.usageCredits': 'Usage credits consumed',
    'commandcode.officialCost': 'Official cost',
    'commandcode.ledger': 'Recorded by this instance',
    'commandcode.ledgerRequests': 'Requests recorded here',
    'commandcode.ledgerCost': 'Cost recorded here',
    'commandcode.ledgerGap': 'Not seen by this proxy',
    'commandcode.ledgerHint': 'The official figures cover every request on the account, including other clients and modes. This instance only records what passed through it, so a gap is traffic that did not.',
    'commandcode.usagePeriod': 'Usage scope',
    'commandcode.billingPeriodScope': 'Current billing period',
    'commandcode.usage': 'Official usage',
    'commandcode.billing': 'Official billing',
    'commandcode.keys': 'Manage API keys',
    'commandcode.docs': 'Provider API docs',
    'commandcode.zdr': 'Zero Data Retention (ZDR)',
    'label.chatURL': 'Chat Completions URL',
    'label.messagesURL': 'Anthropic Messages URL',
    'label.requestTimeout': 'Request timeout (ms)',
    'label.streamIdleTimeout': 'Stream idle timeout (ms)',
    'label.streamingTimeout': 'Streaming attempt timeout (ms)',
    'label.timeoutHint': '0 uses the provider default. Idle timeout limits gaps between streamed data; attempt timeout limits the entire streaming request.',
    'history.localScope': 'Statistics use records kept by this instance, including synced bills and local estimates. Direct-to-platform traffic may not be recorded here.',
    'history.costSource': 'Cost source',
    'history.allCostSources': 'All cost sources',
    'history.costEstimated': 'Local estimate',
    'history.costProvider': 'Synced platform bill',
    'history.loadFail': 'History could not be refreshed; displayed data may be outdated. ',
    'analytics.p95Latency': 'p95 Latency',
    'analytics.latencyUnit': 'request-weighted',
    'analytics.cacheHitShort': 'cache hit',
    'analytics.cacheHitTitle': 'Cache read as a share of all prompt tokens (input + cache read + cache write)',
    'analytics.byModel': 'Model distribution',
    'analytics.byProvider': 'Platform distribution',
    'analytics.noData': 'No data',
    'analytics.noTrend': 'No trend data',
    'data.loading': 'Loading selected data…',
    'data.loadFail': 'Could not load the selected data. Refresh to retry. ',
    'data.invalid': 'Invalid data response',
    'analytics.singleDay': '(single-day data)',
    'analytics.dailyTrend': 'Daily Token Trend',
    'analytics.requestTrend': 'Request trend',
    'analytics.tokenTrend': 'Token trend',
    'analytics.throughput': 'Last-minute throughput',
    'analytics.granularity': 'Granularity',
    'analytics.hour': 'Hour',
    'analytics.day': 'Day',
    'analytics.freshInput': 'Fresh input',
    'analytics.generatedOutput': 'Generated output',
    'analytics.reusedInput': 'Reused input',
    'analytics.newCacheInput': 'New cached input',
    'analytics.periodDetails': 'Period details',
    'analytics.period': 'Period',
    'analytics.knownErrors': 'Known errors',
    'analytics.modelDetails': 'Model details',
    'analytics.knownRecords': '{n} records with details',
    'analytics.retainedRange': 'Query range {from} - {to} (UTC) · limited to retained records',
    'analytics.last7d': 'Last 7 days',
    'analytics.last30d': 'Last 30 days',
    'analytics.last90d': 'Last 90 days',
    'analytics.inputTokens': 'Input tokens',
    'analytics.outputTokens': 'Output tokens',
    'analytics.cacheTokens': 'Cache tokens',
    'analytics.reasoningTokens': 'Reasoning Tokens',
    'analytics.reasoningNote': 'included in output',
    'analytics.requests': 'Requests',
    'analytics.avgLatency': 'Average latency',
    'analytics.successRate': 'Success rate',
    'analytics.fallbackRate': 'Fallback rate',
    'history.filteredRequests': 'Filtered requests',
    'history.successRate': 'Success rate',
    'history.filteredTokens': 'Tokens',
    'history.filteredCost': 'Recorded cost',
    'history.modelDistribution': 'Models',
    'history.providerDistribution': 'Platforms',
    'history.scenarioDistribution': 'Scenarios',
    'history.distribution': 'Distribution details',
    'history.perPage': 'Rows per page',
    'history.streaming': 'Streaming',
    'history.nonStreaming': 'Non-streaming',
    'history.peakWindow': 'Billed at 2x (deepseek weekday peak UTC 01-04/06-10)',
    'filter.dateRange': 'Date range',
    'filter.today': 'Today',
    'filter.clear': 'Clear',
    'filter.apply': 'Apply',
    'action.cancel': 'Cancel',
    'detail.title': 'Request details',
    'detail.subtitle': 'Routing, billing, and token details',
    'detail.close': 'Close request details',
    'detail.requestId': 'Request ID',
    'detail.time': 'Time',
    'detail.model': 'Model',
    'detail.provider': 'Provider',
    'detail.scenario': 'Scenario',
    'detail.requestType': 'Request type',
    'detail.streaming': 'Streaming',
    'detail.nonStreaming': 'Non-streaming',
    'detail.attempt': 'Attempt',
    'detail.inputTokens': 'Input',
    'detail.promptTokens': 'Prompt',
    'detail.cacheRead': 'Cache read',
    'detail.cacheCreation': 'Cache write',
    'detail.outputTokens': 'Output',
    'detail.duration': 'Duration',
    'detail.billingWindow': 'Billing window',
    'detail.peak': 'Peak',
    'detail.offPeak': 'Off-peak',
    'setting.activeSite': 'Active platform',
    'setting.activeSiteUnrestricted': 'Not restricted',
    'setting.activeSiteHint': 'Routes every request through the selected platform.',
    'setting.activeSiteUnavailable': 'No credential is configured for this platform.',
    'setting.activeSiteNotShown': 'Active platform "{site}" is not offered here, so these views show all platforms.',
    'detail.status': 'Status',
    'detail.success': 'Success',
    'detail.failed': 'Failed',
    'detail.unknown': 'Unknown',
    'detail.unavailable': 'Not available',
    'detail.error': 'Error',
    'cmd.startProxy': 'Start Proxy',
    'cmd.stopProxy': 'Stop Proxy',
    'cmd.gotoOverview': 'Go to Overview',
    'cmd.gotoHistory': 'Go to History',
    'cmd.gotoPerformance': 'Go to Performance',
    'cmd.gotoFallback': 'Go to Fallback',
    'cmd.gotoAnalytics': 'Go to Analytics',
    'cmd.gotoSettings': 'Go to Settings',
    'cmd.refreshData': 'Refresh Data',
    'metric.total': 'Total Requests',
    'metric.success': 'Success',
    'metric.failed': 'Failed',
    'metric.streamed': 'Streamed',
    'section.modelDist': 'Model Distribution',
    'empty.noData': 'No data yet',
    'filter.allModels': 'All Models',
    'filter.allProviders': 'All platforms',
    'filter.model': 'Model',
    'filter.provider': 'Provider',
    'filter.scenario': 'Scenario',
    'filter.startDate': 'Start date',
    'filter.endDate': 'End date',
    'filter.allStatuses': 'All statuses',
    'filter.success': 'Success only',
    'filter.failed': 'Failed only',
    'filter.allStreams': 'All request types',
    'filter.streaming': 'Streaming only',
    'filter.nonStreaming': 'Non-streaming only',
    'filter.reset': 'Reset',
    'history.title': 'Request History',
    'history.searchPlaceholder': 'Search ID, model, provider, scenario, or error…',
    'analytics.viewRequests': 'View requests',
    'th.time': 'Time',
    'th.model': 'Model',
    'th.modelPlatform': 'Model / Platform',
    'th.tokens': 'Tokens',
    'th.scenario': 'Scenario',
    'th.inputTokens': 'Input Tokens',
    'th.promptTokens': 'Prompt Tokens',
    'th.outputTokens': 'Output Tokens',
    'th.cost': 'Cost',
    'th.duration': 'Duration',
    'th.status': 'Status',
    'empty.noHistory': 'No history yet',
    'setting.proxy': 'Proxy Service',
    'setting.proxyDesc': 'Start or stop the proxy HTTP service',
    'setting.autostart': 'Start on Boot',
    'setting.autostartDesc': 'Auto-start routatic-proxy at login (launchd)',
    'setting.notify': 'Desktop Notifications',
    'setting.notifyDesc': 'Notify on failures or model switches',
    'setting.language': 'Language',
    'setting.languageDesc': 'Switch interface language',
    'setting.catalog': 'Catalog',
    'setting.catalogNotSynced': 'Catalog not synced',
    'setting.catalogAge': 'Last synced: {age}',
    'section.proxyConfig': 'Proxy Configuration',
    'setting.platformJump': 'Jump to platform',
    'setting.runtime': 'Runtime & tools',
    'setting.changeCount': '{n} unsaved fields',
    'setting.noChanges': 'All changes saved',
    'setting.invalidChanges': 'Some fields need attention before saving',
    'placeholder.envOrEmpty': 'Use env var or leave empty',
    'placeholder.notSet': 'Not configured',
    'label.globalKey': 'Global API Key (optional)',
    'label.host': 'Listen Address (Host)',
    'label.port': 'Listen Port (Port)',
    'btn.save': 'Save & Apply Config',
    'btn.refreshCatalog': 'Refresh catalog',
    'status.saving': 'Saving…',
    'status.saveOk': 'Config saved successfully!',
    'status.saveFail': 'Save failed: ',
    'status.networkError': 'Network error, save failed',
    'status.count': ' entries',
    'status.filtered': ' (filtered)',
    'badge.success': 'Success',
    'badge.fail': 'Fail',
    'port.info': 'Listening port: —',
    'save.unloaded': 'Config not loaded, cannot save',
    'fallback.scenario': 'Scenario',
    'fallback.default': 'Default',
    'fallback.streaming': 'Streaming',
    'fallback.longContext': 'Long Context',
    'fallback.chainOrder': 'Fallback Chain Order',
    'fallback.addModel': '+ Add Model',
    'fallback.preview': 'Preview',
    'fallback.save': 'Save',
    'fallback.empty': 'No models configured',
    'fallback.previewTitle': 'Fallback Chain Preview',
    'fallback.selectModel': 'Select a model',
    'fallback.saving': 'Saving fallback chain...',
    'fallback.saved': 'Fallback chain saved successfully!',
    'fallback.saveFailed': 'Failed to save fallback chain',
    'fallback.noChanges': 'No changes to save',
    'fallback.unsaved': 'Unsaved chain changes',
    'fallback.moveUp': 'Move {model} up',
    'fallback.moveDown': 'Move {model} down',
    'fallback.remove': 'Remove {model}',
    'fallback.primary': 'Primary model',
    'perf.lastHour': 'Last Hour',
    'perf.last24h': 'Last 24 Hours',
    'perf.last7d': 'Last 7 Days',
    'perf.allTime': 'All Time',
    'perf.th.model': 'Model',
    'perf.th.count': 'Samples',
    'perf.sampleHint': 'Samples count requests with measured latency. Success % uses known outcomes. P50 / P90 / P99 show the latency at each percentile; small samples can be misleading.',
    'perf.knownSamples': '{n} requests with known outcomes',
    'perf.th.successRate': 'Success %',
    'perf.th.avg': 'Avg (ms)',
    'perf.th.p50': 'P50',
    'perf.th.p90': 'P90',
    'perf.th.p99': 'P99',
    'perf.empty': 'No performance data',
    'setting.backup': 'Backup Configuration',
    'setting.backupDesc': 'Export current config as JSON file',
    'setting.restore': 'Restore Configuration',
    'setting.restoreDesc': 'Import config from JSON file',
    'btn.export': 'Export',
    'btn.import': 'Import',
    'label.anonymize': 'Anonymize',
    'status.exporting': 'Exporting...',
    'status.exportOk': 'Config exported successfully!',
    'status.exportFail': 'Export failed: ',
    'status.importing': 'Importing...',
    'status.importOk': 'Config imported successfully!',
    'status.importFail': 'Import failed: ',
    'status.importInvalid': 'Invalid config file',
    'modal.importPreview': 'Import Preview',
    'modal.importConfirm': 'Apply this configuration?',
    'btn.apply': 'Apply',
    'btn.cancel': 'Cancel',
    'setting.testModel': 'Test Model',
    'setting.testModelDesc': 'Send a quick test request to verify model connectivity',
    'btn.testModel': 'Test Model',
    'test.title': 'Quick Model Test',
    'test.selectModel': 'Select a model...',
    'test.send': 'Send',
    'test.promptPlaceholder': 'Enter your prompt...',
    'test.latency': 'Latency:',
    'test.tokens': 'Tokens:',
    'test.copy': 'Copy',
    'test.copied': 'Copied!',
    'test.sending': 'Sending...',
    'test.noModel': 'Please select a model',
    'test.noPrompt': 'Please enter a prompt',
    'test.error': 'Error: ',
    'test.networkError': 'Network error',
    'toast.catalogSynced': 'Catalog synced',
    'toast.catalogSyncFailed': 'Catalog sync failed: ',
    'toast.catalogNetworkError': 'Catalog sync network error',
    'toast.proxyStarted': 'Proxy started',
    'toast.proxyStopped': 'Proxy stopped',
    'toast.proxyActionFailed': 'Action failed',
    'toast.networkError': 'Network error',
  },
  zh: {
    'lang.toggle': 'English',
    'shell.console': '控制台',
    'shell.workspace': '工作空间',
    'shell.management': '平台管理',
    'shell.navigation': '主要导航',
    'shell.platforms': '五个平台，一个工作空间。',
    'shell.skip': '跳转到主要内容',
    'theme.light': '切换为浅色主题',
    'theme.dark': '切换为深色主题',
    'overview.description': '快速了解请求流量、已记录费用与服务状态。',
    'analytics.description': '按时段查看 Token 组成、已记录费用与调用趋势。',
    'quota.description': '账户额度与订阅集中展示，本实例账本独立核对。',
    'data.retained': '保留累计',
    'data.localEmpty': '当前视图没有保留的请求记录',
    'data.localEmptyHint': '请核对平台、筛选条件与保留策略。本地记录为空，不代表平台账户没有用量。',
    'history.advanced': '高级筛选',
    'history.previous': '上一页',
    'history.next': '下一页',
    'history.noMatches': '没有符合筛选条件的请求',
    'history.emptyFiltersHint': '可调整平台、日期范围或重置筛选；这里只能检索本实例仍保留的记录。',
    'perf.description': '按模型比较实测耗时与已知请求结果。',
    'perf.p50Hint': '典型请求耗时',
    'perf.p90Hint': '90% 的样本在此耗时内完成',
    'perf.p99Hint': '观察长尾慢请求',
    'perf.emptyHint': '当前视图没有实测样本。同步账单可能有用量，但没有耗时与执行结果。',
    'fallback.description': '为不同路由场景设置清晰的模型优先顺序。',
    'fallback.guideTitle': '路由顺序',
    'fallback.orderHint': '主模型来自场景配置。可拖拽或使用上下箭头，调整后续降级模型的优先顺序。',
    'fallback.saveTitle': '预览后应用',
    'fallback.saveHint': '保存前可预览模型链。保存会应用尚未保存的降级修改，不改变平台凭证。',
    'setting.platformsHint': '每个平台独立配置，仅保存修改过的字段，未修改的脱敏凭证保持不变。',
    'setting.bedrockHint': '推理接口与账单身份',
    'setting.openrouterHint': '推理接口与账户点数',
    'setting.commandcodeHint': '原生 Messages 与 Alpha 账户',
    'setting.server': '服务端',
    'setting.logging': '日志',
    'status.checking': '检查中…',
    'export.csv': '导出 CSV',
    'export.working': '导出中…',
    'export.ok': '已导出 {n} 条',
    'export.empty': '暂无历史可导出',
    'export.fail': '导出失败',
    'status.stale': '无响应',
    'status.staleFor': '已 {secs} 秒无响应',
    'status.running': '运行中',
    'status.stopped': '已停止',
    'status.connected': '已连接',
    'tab.overview': '概览',
    'tab.history': '历史请求',
    'tab.fallback': '降级策略',
    'tab.settings': '设置',
    'tab.analytics': '用量分析',
    'tab.quota': '用量与账单',
    'quota.title': '用量与账单',
    'quota.openSettings': '平台设置',
    'quota.sourceDetails': '数据源详情',
    'quota.refresh': '刷新',
    'quota.bottleneck': '最紧窗口',
    'quota.remaining': '剩余额度',
    'quota.nextReset': '下次重置',
    'quota.plan': '套餐',
    'quota.rolling5h': '5 小时滚动',
    'quota.weekly': '本周',
    'quota.monthly': '本月',
    'quota.used': '已用',
    'quota.limit': '限额',
    'quota.left': '剩余',
    'quota.leftShort': '剩余',
    'quota.resetsIn': '{d} 后重置',
    'quota.resetsAtTime': '{time}',
    'quota.resetUnknown': '重置时间未知',
    'quota.derived': '美元数按套餐限额推算',
    'quota.exhausted': '已用满',
    'quota.keyLabel': '密钥',
    'quota.keyCount': '{n} 个密钥',
    'quota.noKey': '未配置 OpenCode Go 密钥',
    'quota.noKeyHint': '在「设置 → OpenCode Go」填入密钥后，即可在此查看套餐额度。',
    'quota.loadFail': '额度加载失败',
    'quota.updated': '更新于 {time}',
    'quota.cached': '缓存',
    'quota.endpoint': '数据源 {url}',
    'quota.unofficial': '上游端点未公开，响应结构可能变化。',
    'quota.modelLimits': '当前套餐窗口的本地模型费用',
    'quota.modelLimitsNote': '此实例的 Go 记录（含估算），不等同于账户账单 · 额度来自 Go 文档 · 更新于 {time}',
    'quota.model': '模型',
    'quota.modelUsed': '本地记录费用',
    'quota.modelAllowance': '每月配额',
    'quota.percent': '%',
    'quota.total': '总计',
    'quota.poolShare': '本地费用按公开模型额度计算的占比，不代表官方账户占比',
    'quota.indexLagNotice': '账户仪表来自上游额度接口；下方模型费用来自此实例记录，可能不包含账户的全部请求。',
    'quota.multiKeyUsage': '本地记录没有账户归属。配置多个密钥时，不提供按账户计算的模型用量。',
    'quota.localUsageFail': '本地模型费用加载失败。',
    'quota.notRetrieved': '{provider}：额度未获取',
    'quota.providerUnavailable': '面板暂不查询此平台的套餐额度，仍可在历史请求和用量分析中查看本地记录。',
    'quota.viewLocal': '查看本地请求',
    'quota.localTitle': '本实例账本',
    'quota.localNote': '{provider} · 最近 {days} 天（UTC）的本实例记录，包含估算；不等同于账户账单或余额。',
    'quota.localLoadFail': '本实例账本加载失败，请刷新重试。',
    'quota.unknownCosts': '费用未知的请求数',
    'quota.status.available': '账户数据已获取',
    'quota.status.partial': '部分账户数据获取失败',
    'quota.status.not_configured': '尚未配置账户凭证',
    'quota.status.unavailable': '账户数据不可获取',
    'quota.status.error': '账户数据查询失败',
    'quota.source.upstream_api': '来源：上游额度接口',
    'quota.source.official_api': '来源：官方账户接口',
    'quota.source.official_alpha_api': '来源：CommandCode Alpha 账户接口',
    'quota.source.none': '尚未获取官方账户数据',
    'quota.reason.no_public_account_api': '此平台尚未公开账户额度 API。请在官方控制台查看账单；下方仍提供本实例账本。',
    'quota.reason.aws_billing_disabled': 'AWS 账单查询默认关闭。请在设置中启用，并指定账单账户 ID 和独立 AWS SDK 身份。',
    'quota.reason.aws_billing_refresh_required': '可手动查询账单。切换平台和自动刷新均不会调用收费 AWS 接口。',
    'quota.reason.aws_billing_no_data': '该账户和时段未返回匹配的 Bedrock 账单数据；不代表余额为零。请核对账户、服务范围及 AWS 数据延迟。',
    'aws.billingTitle': 'AWS 官方服务费用',
    'aws.billingQuery': '查询 AWS 账单（收费）',
    'aws.billingHint': 'Cost Explorer 按 API 请求收费，分页也会产生请求。查询使用独立 IAM 身份，绝不自动调用；普通刷新只读取缓存快照。',
    'aws.billingScope': '最近 30 个完整 UTC 日 · 下列服务的 UnblendedCost，不是账户余额或本实例账本；AWS 数据可能延迟或修订。',
    'aws.billingTotal': '已报告服务费用',
    'aws.billingAccount': '账单账户 ID',
    'aws.billingPeriod': 'UTC 时段（不含结束日）',
    'aws.billingServices': '纳入合计的 AWS 服务名',
    'aws.billingEstimated': 'AWS 预估，可能修订',
    'aws.billingReported': 'AWS 已报告',
    'aws.billingEnable': '启用手动 AWS 账单查询（收费）',
    'aws.billingProfile': 'AWS SDK 配置档案（可选）',
    'aws.billingConfigHint': '启用时必须填写账户 ID。服务使用标准 AWS SDK 凭证链或所选配置档案，不使用 Bedrock 推理密钥；IAM 需允许 ce:GetDimensionValues 和 ce:GetCostAndUsage。',
    'quota.noProviderKey': '未配置 {provider} 推理 API 密钥',
    'quota.noProviderKeyHint': '在设置中添加该平台密钥后，可查询密钥用量。',
    'quota.yes': '是',
    'quota.no': '否',
    'openrouter.managementKey': '管理 API 密钥（可选）',
    'openrouter.managementHint': '可选的管理密钥，仅用于查询账户余额，不用于推理。',
    'openrouter.keyScope': '密钥限额是消费上限，不是账户余额。平台用量与 BYOK 用量分别展示，不合计多个密钥的剩余额度。',
    'openrouter.accountCredits': '账户点数',
    'openrouter.totalCredits': '点数总额',
    'openrouter.totalUsage': '账户点数已用',
    'openrouter.balance': '账户余额',
    'openrouter.noManagementKey': '尚未查询账户余额。可在设置中添加 OpenRouter 管理 API 密钥；此查询不会使用推理密钥。',
    'openrouter.creditsFail': '账户余额获取失败',
    'openrouter.limit': '密钥消费上限',
    'openrouter.limitRemaining': '密钥剩余额度',
    'openrouter.limitReset': '密钥额度重置周期',
    'openrouter.noLimit': '此密钥未设置消费上限',
    'openrouter.usage': 'OpenRouter 用量',
    'openrouter.byokUsage': 'BYOK 用量',
    'openrouter.includeByok': 'BYOK 计入密钥限额',
    'openrouter.freeTier': '免费层级密钥',
    'openrouter.expires': '密钥到期时间',
    'cmd.gotoQuota': '前往用量与账单',
    'overview.title': '仪表盘',
    'analytics.title': '用量分析',
    'analytics.refresh': '刷新',
    'analytics.autoRefreshOff': '自动刷新：关闭',
    'analytics.autoRefresh5s': '自动刷新：5 秒',
    'analytics.autoRefresh30s': '自动刷新：30 秒',
    'analytics.autoRefresh60s': '自动刷新：60 秒',
    'analytics.totalRequests': '总请求数',
    'analytics.requestsUnit': '次',
    'analytics.totalTokens': '总 Token',
    'analytics.inout': '输入 / 输出',
    'analytics.cost': '费用',
    'analytics.currencyUSD': '美元（USD）',
    'analytics.unknownCosts': '{n} 条请求费用未知；合计只包含已知费用',
    'analytics.knownSubtotal': '已知 {value}',
    'analytics.utc': '用量分析的日期与时间桶统一使用 UTC',
    'label.apiKeys': 'API Keys（逗号分隔，优先于单个密钥）',
    'label.apiKeysHint': '修改已脱敏的密钥时请替换完整列表；清空列表后使用单个密钥。环境变量覆盖仍然优先。',
    'commandcode.keyHint': '使用独立 API 密钥；不会向 CommandCode 发送全局密钥。',
    'commandcode.quotaHint': '使用本平台 API 密钥查询官方 Alpha 账户接口，无需浏览器登录态；Alpha 响应字段可能变化。',
    'commandcode.accountScope': '官方 Alpha 账户数据 · 美元计价用量点数，不是现金余额或本实例账本。每个密钥分别展示，不合计余额。',
    'commandcode.credits': '剩余用量点数',
    'commandcode.freeCredits': '免费点数',
    'commandcode.monthlyCredits': '月度剩余点数',
    'commandcode.purchasedCredits': '购买点数',
    'commandcode.monthlyLimitUnknown': 'Alpha 接口未返回月度发放总额，因此不推算月度使用百分比。',
    'commandcode.windowLimits': '滚动用量限制',
    'commandcode.noWindowLimits': '此账户未报告滚动窗口限制',
    'commandcode.noSubscription': '账户接口未返回订阅',
    'commandcode.billingPeriod': '官方账单周期（UTC）',
    'commandcode.cancelAtEnd': '周期结束时取消',
    'commandcode.usageSummary': '官方用量汇总',
    'commandcode.usageCredits': '已消耗用量点数',
    'commandcode.officialCost': '官方费用',
    'commandcode.ledger': '本实例记录',
    'commandcode.ledgerRequests': '本实例记录的请求数',
    'commandcode.ledgerCost': '本实例记录的费用',
    'commandcode.ledgerGap': '未经本代理',
    'commandcode.ledgerHint': '官方口径覆盖该账户的全部请求，含其他客户端与其他 mode；本实例只记录经过它的流量，差额即未经过本代理的部分。',
    'commandcode.usagePeriod': '用量范围',
    'commandcode.billingPeriodScope': '当前账单周期',
    'commandcode.usage': '官方用量',
    'commandcode.billing': '官方账单',
    'commandcode.keys': '管理 API 密钥',
    'commandcode.docs': 'Provider API 文档',
    'commandcode.zdr': '零数据保留（ZDR）',
    'label.chatURL': 'Chat Completions 完整地址',
    'label.messagesURL': 'Anthropic Messages 完整地址',
    'label.requestTimeout': '请求超时（毫秒）',
    'label.streamIdleTimeout': '流式空闲超时（毫秒）',
    'label.streamingTimeout': '流式单次请求超时（毫秒）',
    'label.timeoutHint': '0 使用平台默认值。空闲超时限制流式数据间隔，单次请求超时限制整个流式请求。',
    'history.localScope': '统计来自此实例保留的记录，包含同步账单与本地估算。客户端直连平台的流量可能不在这里。',
    'history.costSource': '费用来源',
    'history.allCostSources': '全部费用来源',
    'history.costEstimated': '本地估算',
    'history.costProvider': '已同步平台账单',
    'history.loadFail': '历史请求刷新失败，已显示的数据可能过期。',
    'analytics.p95Latency': 'p95 延迟',
    'analytics.latencyUnit': '按请求数加权',
    'analytics.cacheHitShort': '缓存命中',
    'analytics.cacheHitTitle': '缓存读取占全部输入 Token 的比例（输入 + 缓存读 + 缓存写）',
    'analytics.byModel': '模型分布',
    'analytics.byProvider': '平台分布',
    'analytics.noData': '暂无数据',
    'analytics.noTrend': '暂无趋势数据',
    'data.loading': '正在加载所选数据…',
    'data.loadFail': '所选数据加载失败，请刷新重试。',
    'data.invalid': '数据响应格式无效',
    'analytics.singleDay': '（仅单日数据）',
    'analytics.dailyTrend': '每日 Token 趋势',
    'analytics.requestTrend': '请求趋势',
    'analytics.tokenTrend': 'Token 趋势',
    'analytics.throughput': '近一分钟吞吐',
    'analytics.granularity': '粒度',
    'analytics.hour': '小时',
    'analytics.day': '天',
    'analytics.freshInput': '未命中缓存的输入',
    'analytics.generatedOutput': '模型生成输出',
    'analytics.reusedInput': '已复用输入',
    'analytics.newCacheInput': '新增缓存输入',
    'analytics.periodDetails': '时段明细',
    'analytics.period': '时段',
    'analytics.knownErrors': '已知错误',
    'analytics.modelDetails': '模型明细',
    'analytics.knownRecords': '{n} 条有详情记录',
    'analytics.retainedRange': '查询范围 {from} - {to}（UTC）· 仅含仍保留的记录',
    'analytics.last7d': '最近 7 天',
    'analytics.last30d': '最近 30 天',
    'analytics.last90d': '最近 90 天',
    'analytics.inputTokens': '输入 Token',
    'analytics.outputTokens': '输出 Token',
    'analytics.cacheTokens': '缓存 Token',
    'analytics.cacheTokensLegend': '缓存',
    'analytics.reasoningTokens': '推理 Token',
    'analytics.reasoningNote': '已包含在输出 Token 中',
    'analytics.requests': '请求数',
    'analytics.avgLatency': '平均延迟',
    'analytics.successRate': '成功率',
    'analytics.fallbackRate': '降级率',
    'history.filteredRequests': '筛选请求数',
    'history.successRate': '成功率',
    'history.filteredTokens': 'Token 总量',
    'history.filteredCost': '已记录费用',
    'history.modelDistribution': '模型分布',
    'history.providerDistribution': '平台分布',
    'history.scenarioDistribution': '场景分布',
    'history.distribution': '分布明细',
    'history.perPage': '每页',
    'history.streaming': '流式',
    'history.nonStreaming': '非流式',
    'history.peakWindow': '2 倍计费时段（deepseek 工作日高峰 UTC 01-04/06-10）',
    'filter.dateRange': '日期范围',
    'filter.today': '今天',
    'filter.clear': '清除',
    'filter.apply': '应用',
    'action.cancel': '取消',
    'detail.title': '请求详情',
    'detail.subtitle': '路由、费用与 Token 明细',
    'detail.close': '关闭请求详情',
    'detail.requestId': '请求 ID',
    'detail.time': '请求时间',
    'detail.model': '模型',
    'detail.provider': '供应商',
    'detail.scenario': '使用场景',
    'detail.requestType': '请求类型',
    'detail.streaming': '流式请求',
    'detail.nonStreaming': '非流式请求',
    'detail.attempt': '尝试次数',
    'detail.inputTokens': '输入',
    'detail.promptTokens': 'Prompt',
    'detail.cacheRead': '缓存读取',
    'detail.cacheCreation': '缓存写入',
    'detail.outputTokens': '输出',
    'detail.duration': '耗时',
    'detail.billingWindow': '计费时段',
    'detail.peak': '高峰',
    'detail.offPeak': '非高峰',
    'setting.activeSite': '当前平台',
    'setting.activeSiteUnrestricted': '不限制',
    'setting.activeSiteHint': '所有请求都走选中的平台。',
    'setting.activeSiteUnavailable': '该平台未配置凭证。',
    'setting.activeSiteNotShown': '当前平台「{site}」未在此列出，各视图按全部平台展示。',
    'detail.status': '状态',
    'detail.success': '成功',
    'detail.failed': '失败',
    'detail.unknown': '未知',
    'detail.unavailable': '暂无记录',
    'detail.error': '错误信息',
    'cmd.startProxy': '启动代理',
    'cmd.stopProxy': '停止代理',
    'cmd.gotoOverview': '前往概览',
    'cmd.gotoHistory': '前往历史',
    'cmd.gotoPerformance': '前往性能',
    'cmd.gotoFallback': '前往降级策略',
    'cmd.gotoAnalytics': '前往用量分析',
    'cmd.gotoSettings': '前往设置',
    'cmd.refreshData': '刷新数据',
    'metric.total': '总请求数',
    'metric.success': '成功',
    'metric.failed': '失败',
    'metric.streamed': '流式请求',
    'section.modelDist': '模型调用分布',
    'empty.noData': '暂无数据',
    'filter.allModels': '全部模型',
    'filter.allProviders': '全部平台',
    'filter.model': '模型',
    'filter.provider': '供应商',
    'filter.scenario': '场景',
    'filter.startDate': '开始日期',
    'filter.endDate': '结束日期',
    'filter.allStatuses': '全部状态',
    'filter.success': '仅成功',
    'filter.failed': '仅失败',
    'filter.allStreams': '全部请求类型',
    'filter.streaming': '仅流式',
    'filter.nonStreaming': '仅非流式',
    'filter.reset': '重置',
    'history.title': '请求记录',
    'history.searchPlaceholder': '搜索请求 ID、模型、供应商、场景或错误…',
    'analytics.viewRequests': '查看请求',
    'th.time': '时间',
    'th.model': '模型',
    'th.modelPlatform': '模型 / 平台',
    'th.tokens': 'Token',
    'th.scenario': '场景',
    'th.inputTokens': '输入 Token',
    'th.promptTokens': 'Prompt Token',
    'th.outputTokens': '输出 Token',
    'th.cost': '费用',
    'th.duration': '耗时',
    'th.status': '状态',
    'empty.noHistory': '暂无历史请求',
    'setting.proxy': '代理服务',
    'setting.proxyDesc': '启动或停止代理 HTTP 服务',
    'setting.autostart': '开机自启',
    'setting.autostartDesc': '登录时自动启动 routatic-proxy（launchd）',
    'setting.notify': '桌面通知',
    'setting.notifyDesc': '请求失败或切换模型时发送系统通知',
    'setting.language': '语言',
    'setting.languageDesc': '切换界面语言',
    'setting.catalog': '模型目录',
    'setting.catalogNotSynced': '模型目录未同步',
    'setting.catalogAge': '上次同步：{age}',
    'section.proxyConfig': '服务代理配置',
    'setting.platformJump': '定位平台',
    'setting.runtime': '运行状态与其他操作',
    'setting.changeCount': '{n} 个字段尚未保存',
    'setting.noChanges': '所有修改已保存',
    'setting.invalidChanges': '部分字段需修正后才能保存',
    'placeholder.envOrEmpty': '使用环境变量或留空',
    'placeholder.notSet': '未配置',
    'label.globalKey': 'Global API Key (可选)',
    'label.host': '监听地址 (Host)',
    'label.port': '监听端口 (Port)',
    'btn.save': '保存并应用配置',
    'btn.refreshCatalog': '刷新模型目录',
    'status.saving': '保存中…',
    'status.saveOk': '配置保存并应用成功！',
    'status.saveFail': '保存失败: ',
    'status.networkError': '网络错误，保存失败',
    'status.count': ' 条',
    'status.filtered': '（已筛选）',
    'badge.success': '成功',
    'badge.fail': '失败',
    'port.info': '监听端口：—',
    'save.unloaded': '未加载当前配置，无法保存',
    'setting.testModel': '测试模型',
    'setting.testModelDesc': '发送快速测试请求以验证模型连接',
    'btn.testModel': '测试模型',
    'test.title': '快速模型测试',
    'test.selectModel': '选择模型...',
    'test.send': '发送',
    'test.promptPlaceholder': '输入测试提示词...',
    'test.latency': '延迟：',
    'test.tokens': 'Token：',
    'test.copy': '复制',
    'test.copied': '已复制！',
    'test.sending': '发送中...',
    'test.noModel': '请选择模型',
    'test.noPrompt': '请输入提示词',
    'test.error': '错误：',
    'test.networkError': '网络错误',
    'toast.catalogSynced': '模型目录已同步',
    'toast.catalogSyncFailed': '目录同步失败：',
    'toast.catalogNetworkError': '目录同步网络错误',
    'toast.proxyStarted': '代理已启动',
    'toast.proxyStopped': '代理已停止',
    'toast.proxyActionFailed': '操作失败',
    'toast.networkError': '网络错误',
    'fallback.scenario': '使用场景',
    'fallback.default': '默认',
    'fallback.streaming': '流式请求',
    'fallback.longContext': '长上下文',
    'fallback.chainOrder': '降级链顺序',
    'fallback.addModel': '+ 添加模型',
    'fallback.preview': '预览',
    'fallback.save': '保存',
    'fallback.empty': '未配置模型',
    'fallback.previewTitle': '降级链预览',
    'fallback.selectModel': '选择模型',
    'fallback.saving': '保存中...',
    'fallback.saved': '降级链保存成功！',
    'fallback.saveFailed': '保存失败',
    'fallback.noChanges': '无更改',
    'fallback.unsaved': '降级链有未保存的修改',
    'fallback.moveUp': '上移 {model}',
    'fallback.moveDown': '下移 {model}',
    'fallback.remove': '移除 {model}',
    'fallback.primary': '主模型',
    'perf.lastHour': '最近 1 小时',
    'perf.last24h': '最近 24 小时',
    'perf.last7d': '最近 7 天',
    'perf.allTime': '全部时间',
    'perf.th.model': '模型',
    'perf.th.count': '耗时样本',
    'perf.sampleHint': '样本数仅计入有耗时记录的请求；成功率按已知结果计算。P50 / P90 / P99 表示相应分位的耗时，样本较少时不宜据此判断稳定性能。',
    'perf.knownSamples': '{n} 条已知结果的请求',
    'perf.th.successRate': '成功率',
    'perf.th.avg': '平均延迟',
    'perf.th.p50': 'P50',
    'perf.th.p90': 'P90',
    'perf.th.p99': 'P99',
    'perf.empty': '暂无性能数据',
    'setting.backup': '备份配置',
    'setting.backupDesc': '导出当前配置为 JSON 文件',
    'setting.restore': '恢复配置',
    'setting.restoreDesc': '从 JSON 文件导入配置',
    'btn.export': '导出',
    'btn.import': '导入',
    'label.anonymize': '脱敏',
    'status.exporting': '导出中...',
    'status.exportOk': '配置导出成功！',
    'status.exportFail': '导出失败：',
    'status.importing': '导入中...',
    'status.importOk': '配置导入成功！',
    'status.importFail': '导入失败：',
    'status.importInvalid': '无效的配置文件',
    'modal.importPreview': '导入预览',
    'modal.importConfirm': '应用此配置？',
    'btn.apply': '应用',
    'btn.cancel': '取消',
    'tab.logs': '日志',
    'tab.performance': '性能',
  }
};

let currentLang = localStorage.getItem('routatic-proxy-lang') || 'en';

function t(key) {
  return (TRANSLATIONS[currentLang] && TRANSLATIONS[currentLang][key]) || key;
}

function applyTranslations() {
  // Update all data-i18n elements
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    el.textContent = t(key);
  });
  // Update placeholder attributes for inputs
  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    const key = el.getAttribute('data-i18n-placeholder');
    el.placeholder = t(key);
  });
  document.querySelectorAll('[data-i18n-aria-label]').forEach(el => {
    el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria-label')));
  });
  // Update <option> labels (data-i18n-option)
  document.querySelectorAll('[data-i18n-option]').forEach(el => {
    const key = el.getAttribute('data-i18n-option');
    el.textContent = t(key);
  });
  // Update the language toggle text
  const langBtn = document.getElementById('btn-lang-toggle');
  if (langBtn) {
    langBtn.innerHTML = '<span data-i18n="lang.toggle">' + t('lang.toggle') + '</span>';
  }
  window.CustomSelect?.syncAll();
  window.HistoryDateRange?.syncLabel();
  AnalyticsModule?.syncDateRange?.();
}

function toggleLanguage() {
  currentLang = currentLang === 'en' ? 'zh' : 'en';
  localStorage.setItem('routatic-proxy-lang', currentLang);
  document.documentElement.lang = currentLang;
  applyTranslations();
  // Re-render dynamic content
  renderModelList(lastModelCounts);
  renderHistory();
  renderHistoryPager();
  PerfModule.render();
  // Analytics charts and distributions carry inline strings; reload them
  // so they pick up the new language instead of keeping stale ones.
  if (activeTab === 'analytics') AnalyticsModule.load(true);
  QuotaModule.render();
  QuotaModule.renderLocalUsage();
  FallbackModule.renderChain();
  updateConfigChangeCount();
  if (lastOverviewView) renderOverviewUsage(lastOverviewView.data, lastOverviewView.trend, lastOverviewView.latency);
}

// Apply translations on load
document.addEventListener('DOMContentLoaded', () => {
  document.documentElement.lang = currentLang;
  applyTranslations();
  const exportBtn = document.getElementById('history-export');
  if (exportBtn) exportBtn.addEventListener('click', exportHistoryCSV);
});

function syncThemeControl() {
  const key = getComputedStyle(document.documentElement).colorScheme === 'light' ? 'theme.dark' : 'theme.light';
  const button = document.getElementById('btn-theme-toggle');
  button.setAttribute('data-i18n-aria-label', key);
  button.setAttribute('aria-label', t(key));
  const label = document.getElementById('theme-action');
  label.dataset.i18n = key;
  label.textContent = t(key);
}

function toggleTheme() {
  const theme = getComputedStyle(document.documentElement).colorScheme === 'light' ? 'dark' : 'light';
  document.documentElement.dataset.theme = theme;
  localStorage.setItem('routatic-proxy-theme', theme);
  syncThemeControl();
}

document.addEventListener('DOMContentLoaded', () => {
  const saved = localStorage.getItem('routatic-proxy-theme');
  if (saved === 'light' || saved === 'dark') document.documentElement.dataset.theme = saved;
  syncThemeControl();
  document.getElementById('btn-theme-toggle').addEventListener('click', toggleTheme);
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', syncThemeControl);
});

/* ── Themed form controls ─────────────────────────────────────── */
window.CustomSelect = {
  instances: new Map(),

  init() {
    document.querySelectorAll('select').forEach(select => this.enhance(select));
    document.addEventListener('pointerdown', event => {
      if (!event.target.closest('.theme-select')) this.closeAll();
    });
  },

  enhance(select) {
    if (this.instances.has(select)) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'theme-select';
    if (select.classList.contains('flex-1')) wrapper.classList.add('theme-select-flex');
    if (select.classList.contains('w-full')) wrapper.classList.add('theme-select-full');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'theme-select-trigger';
    button.setAttribute('aria-haspopup', 'listbox');
    button.setAttribute('aria-expanded', 'false');
    const label = document.createElement('span');
    label.className = 'theme-select-label';
    const chevron = document.createElement('span');
    chevron.className = 'theme-select-chevron';
    chevron.setAttribute('aria-hidden', 'true');
    chevron.textContent = '⌄';
    button.append(label, chevron);
    const list = document.createElement('div');
    list.className = 'theme-select-list';
    list.setAttribute('role', 'listbox');
    list.hidden = true;
    select.parentNode.insertBefore(wrapper, select);
    wrapper.append(button, list, select);
    select.classList.add('native-select-source');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');
    const state = { select, wrapper, button, label, list };
    this.instances.set(select, state);
    select.addEventListener('change', () => this.sync(select));
    button.addEventListener('click', () => this.toggle(state));
    button.addEventListener('keydown', event => {
      if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        event.preventDefault();
        this.open(state, event.key === 'ArrowUp' || event.key === 'End');
      } else if (event.key === 'Escape') {
        this.close(state);
      }
    });
    list.addEventListener('keydown', event => this.onListKeydown(event, state));
    new MutationObserver(() => this.render(state)).observe(select, {
      childList: true, subtree: true, characterData: true, attributes: true,
    });
    this.render(state);
  },

  render(state) {
    const { select, list } = state;
    list.replaceChildren();
    Array.from(select.options).forEach((option, index) => {
      const item = document.createElement('button');
      item.type = 'button';
      item.className = 'theme-select-option';
      item.setAttribute('role', 'option');
      item.dataset.index = String(index);
      item.textContent = option.textContent;
      item.disabled = option.disabled;
      item.addEventListener('click', () => {
        select.selectedIndex = index;
        select.dispatchEvent(new Event('change', { bubbles: true }));
        this.close(state);
        state.button.focus();
      });
      list.append(item);
    });
    this.sync(select);
  },

  sync(select) {
    const state = this.instances.get(select);
    if (!state) return;
    const option = select.options[select.selectedIndex];
    state.label.textContent = option?.textContent || '';
    state.button.disabled = select.disabled;
    state.button.setAttribute('aria-label', select.getAttribute('aria-label') || state.label.textContent);
    state.list.querySelectorAll('[role="option"]').forEach((item, index) => {
      const selected = index === select.selectedIndex;
      item.setAttribute('aria-selected', String(selected));
      item.classList.toggle('selected', selected);
    });
  },

  syncAll() {
    this.instances.forEach((_, select) => this.sync(select));
  },

  toggle(state) {
    if (state.list.hidden) this.open(state); else this.close(state);
  },

  open(state, focusLast = false) {
    this.closeAll(state);
    state.list.hidden = false;
    // Panels near the viewport bottom (e.g. the history pager) would overflow
    // below the fold; flip them above the trigger when there is room.
    const trigger = state.button.getBoundingClientRect();
    const panelH = state.list.getBoundingClientRect().height;
    if (trigger.bottom + panelH + 5 > window.innerHeight && trigger.top - panelH - 5 > 0) {
      state.list.style.top = 'auto';
      state.list.style.bottom = 'calc(100% + 5px)';
    } else {
      state.list.style.top = 'calc(100% + 5px)';
      state.list.style.bottom = 'auto';
    }
    state.wrapper.classList.add('open');
    state.button.setAttribute('aria-expanded', 'true');
    const options = [...state.list.querySelectorAll(':scope > button:not(:disabled)')];
    const selected = state.list.querySelector('[aria-selected="true"]:not(:disabled)');
    (focusLast ? options.at(-1) : selected || options[0])?.focus();
  },

  close(state) {
    state.list.hidden = true;
    state.wrapper.classList.remove('open');
    state.button.setAttribute('aria-expanded', 'false');
  },

  closeAll(except) {
    this.instances.forEach(state => { if (state !== except) this.close(state); });
  },

  onListKeydown(event, state) {
    const options = [...state.list.querySelectorAll(':scope > button:not(:disabled)')];
    const index = options.indexOf(document.activeElement);
    if (event.key === 'Escape') {
      event.preventDefault();
      this.close(state);
      state.button.focus();
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      document.activeElement?.click();
      return;
    }
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1 :
      event.key === 'ArrowDown' ? Math.min(options.length - 1, index + 1) :
      event.key === 'ArrowUp' ? Math.max(0, index - 1) : -1;
    if (next >= 0) {
      event.preventDefault();
      options[next]?.focus();
    }
  },
};

window.HistoryDateRange = {
  init() {
    this.root = document.getElementById('history-date-range');
    this.trigger = document.getElementById('history-date-trigger');
    this.popover = document.getElementById('history-date-popover');
    this.start = document.getElementById('history-start');
    this.end = document.getElementById('history-end');
    this.startDisplay = document.getElementById('history-start-display');
    this.endDisplay = document.getElementById('history-end-display');
    if (!this.root) return;
    this.trigger.addEventListener('click', () => this.toggle());
    document.getElementById('history-date-apply')?.addEventListener('click', () => this.apply());
    document.getElementById('history-date-clear')?.addEventListener('click', () => this.clear(true));
    this.root.querySelectorAll('[data-days]').forEach(button => {
      button.addEventListener('click', () => this.preset(Number(button.dataset.days)));
    });
    document.addEventListener('pointerdown', event => {
      if (!event.target.closest('#history-date-range')) this.close();
    });
    this.popover.addEventListener('keydown', event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        this.close();
        this.trigger.focus();
      }
    });
    this.syncFromHidden();
  },

  valid(value) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
    return dateInputValue(new Date(`${value}T00:00:00`)) === value;
  },

  toggle() {
    if (this.popover.hidden) this.open(); else this.close();
  },

  open() {
    this.popover.hidden = false;
    this.trigger.setAttribute('aria-expanded', 'true');
    this.startDisplay.focus();
  },

  close() {
    if (!this.popover) return;
    this.popover.hidden = true;
    this.trigger.setAttribute('aria-expanded', 'false');
  },

  apply() {
    const start = this.startDisplay.value.trim();
    const end = this.endDisplay.value.trim();
    this.startDisplay.classList.toggle('invalid', !!start && !this.valid(start));
    this.endDisplay.classList.toggle('invalid', !!end && !this.valid(end));
    if ((start && !this.valid(start)) || (end && !this.valid(end)) || (start && end && start > end)) return;
    this.start.value = start;
    this.end.value = end;
    this.start.dispatchEvent(new Event('change', { bubbles: true }));
    this.end.dispatchEvent(new Event('change', { bubbles: true }));
    this.syncLabel();
    this.close();
  },

  clear(notify) {
    this.start.value = '';
    this.end.value = '';
    this.startDisplay.value = '';
    this.endDisplay.value = '';
    this.syncLabel();
    this.close();
    if (notify) this.start.dispatchEvent(new Event('change', { bubbles: true }));
  },

  preset(days) {
    const end = new Date();
    const start = new Date(end);
    start.setDate(start.getDate() - Math.max(0, days - 1));
    this.startDisplay.value = dateInputValue(start);
    this.endDisplay.value = dateInputValue(end);
    this.apply();
  },

  syncFromHidden() {
    if (!this.root) return;
    this.startDisplay.value = this.start.value;
    this.endDisplay.value = this.end.value;
    this.syncLabel();
  },

  syncLabel() {
    const label = document.getElementById('history-date-label');
    if (!label) return;
    label.textContent = this.start?.value || this.end?.value
      ? `${this.start.value || '…'} – ${this.end.value || '…'}`
      : t('filter.dateRange');
  },
};

document.addEventListener('DOMContentLoaded', () => {
  window.CustomSelect.init();
  window.HistoryDateRange.init();
});

/* global state */
let allHistory = [];
let lastModelCounts = {};

/* ── Performance Module ───────────────────────────────────────────── */
const PerfModule = {
  data: null,
  loadSeq: 0,
  query: '',
  error: '',
  sortField: 'count',
  sortDir: 'desc',
  timeRange: 'all',

  init() {
    const timeRangeSelect = document.getElementById('perf-time-range');
    if (timeRangeSelect) {
      timeRangeSelect.addEventListener('change', (e) => {
        this.timeRange = e.target.value;
        this.refresh();
      });
    }
    document.getElementById('perf-provider')?.addEventListener('change', () => this.refresh());
    document.getElementById('btn-refresh-perf')?.addEventListener('click', () => this.refresh());

    document.querySelectorAll('.perf-table .sortable').forEach(th => {
      th.addEventListener('click', () => {
        const field = th.dataset.sort;
        if (this.sortField === field) {
          this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
        } else {
          this.sortField = field;
          this.sortDir = 'desc';
        }
        document.querySelectorAll('.perf-table .sortable').forEach(s => {
          s.classList.remove('asc', 'desc');
          s.setAttribute('aria-sort', 'none');
        });
        th.classList.add(this.sortDir);
        th.setAttribute('aria-sort', this.sortDir === 'asc' ? 'ascending' : 'descending');
        this.render();
      });
    });
  },

  async refresh() {
    const seq = ++this.loadSeq;
    const params = new URLSearchParams({range: this.timeRange});
    const provider = document.getElementById('perf-provider')?.value;
    if (provider) params.set('provider', provider);
    if (this.query !== params.toString() || !this.data) {
      this.query = params.toString();
      this.data = null;
      this.error = '';
      this.render();
    }
    const errorEl = document.getElementById('perf-error');
    if (errorEl) errorEl.hidden = true;
    try {
      const data = await fetchJSON(`/api/perf/models?${params}`);
      if (seq !== this.loadSeq) return;
      if (data !== null && !Array.isArray(data)) throw new Error(t('data.invalid'));
      this.data = data || [];
      this.error = '';
      this.render();
    } catch (e) {
      if (seq !== this.loadSeq) return;
      this.data = null;
      this.error = e.message;
      this.render();
      if (errorEl) {
        errorEl.textContent = t('data.loadFail') + e.message;
        errorEl.hidden = false;
      }
      console.error('PerfModule refresh failed:', e);
    }
  },

  render() {
    const tbody = document.getElementById('perf-tbody');
    if (!tbody) return;

    if (!this.data || this.data.length === 0) {
      const key = this.data ? 'empty.noData' : this.error ? 'detail.unavailable' : 'data.loading';
      tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + (this.data ? emptyStateContent(key, 'perf.emptyHint') : t(key)) + '</td></tr>';
      return;
    }

    const sorted = [...this.data].sort((a, b) => {
      const value = row => this.sortField === 'success_rate'
        ? (row.success != null && row.failed != null && row.success + row.failed > 0 ? row.success / (row.success + row.failed) : -1)
        : row[this.sortField];
      let aVal = value(a);
      let bVal = value(b);
      if (aVal == null) aVal = 0;
      if (bVal == null) bVal = 0;
      if (typeof aVal === 'string') aVal = aVal.toLowerCase();
      if (typeof bVal === 'string') bVal = bVal.toLowerCase();
      if (aVal < bVal) return this.sortDir === 'asc' ? -1 : 1;
      if (aVal > bVal) return this.sortDir === 'asc' ? 1 : -1;
      return 0;
    });

    tbody.innerHTML = sorted.map(row => {
      const known = row.success != null && row.failed != null ? Number(row.success) + Number(row.failed) : 0;
      const successRate = known > 0 ? (row.success / known * 100).toFixed(1) : null;
      const successClass = successRate == null ? '' : successRate >= 99 ? 'success-rate' : (successRate >= 95 ? '' : 'error-rate');
      const latency = value => row.count > 0 ? fmt(value) : '—';
      return `
        <tr data-provider="${escapeHtml(row.provider || '')}">
          <td class="perf-model">${escapeHtml(row.model)}<br><small>${escapeHtml(providerLabel(row.provider))}</small></td>
          <td>${fmt(row.count)}</td>
          <td class="${successClass}" title="${escapeHtml(t('perf.knownSamples').replace('{n}', known.toLocaleString()))}">${successRate == null ? '—' : successRate + '%'}</td>
          <td class="${this.getLatencyClass(row.avg_ms)}">${latency(row.avg_ms)}</td>
          <td class="${this.getLatencyClass(row.p50_ms)}">${latency(row.p50_ms)}</td>
          <td class="${this.getLatencyClass(row.p90_ms)}">${latency(row.p90_ms)}</td>
          <td class="${this.getLatencyClass(row.p99_ms)}">${latency(row.p99_ms)}</td>
        </tr>
      `;
    }).join('');
  },

  getLatencyClass(ms) {
    if (ms == null) return '';
    if (ms < 1000) return 'latency-cell latency-fast';
    if (ms < 2000) return 'latency-cell latency-medium';
    return 'latency-cell latency-slow';
  }
};

/* ── Tab switching (hash-routed) ───────────────────────────────── */
// activateTab shows the named tab and keeps the URL hash in sync so each
// panel is deep-linkable / resumable (#overview, #analytics, #history, ...).
function activateTab(name) {
  const tabs = [...document.querySelectorAll('.tab')];
  const tabEl = tabs.find(tab => tab.dataset.tab === name);
  const panel = document.getElementById('tab-' + name);
  if (!tabEl || !panel) return;
  document.querySelectorAll('.chart-tip').forEach(tip => { tip.style.display = 'none'; });
  tabs.forEach(tab => {
    tab.classList.toggle('active', tab === tabEl);
    tab.setAttribute('aria-current', tab === tabEl ? 'page' : 'false');
  });
  document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
  tabEl.scrollIntoView?.({block: 'nearest', inline: 'nearest'});
  panel.classList.add('active');
  const heading = document.getElementById('active-page-title');
  heading.dataset.i18n = 'tab.' + name;
  heading.textContent = t('tab.' + name);
  activeTab = name;
  if (name === 'overview') refreshOverviewUsage();
  if (name === 'performance') PerfModule.refresh();
  if (name === 'analytics' && AnalyticsModule && AnalyticsModule.load) {
    AnalyticsModule.load(true);
  }
  if (name === 'quota') QuotaModule.load(true);
  refreshCurrentTab();
}

document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    // Activate immediately; hashchange skips the already active page.
    if (location.hash !== '#' + tab.dataset.tab) {
      location.hash = tab.dataset.tab;
    }
    activateTab(tab.dataset.tab);
  });
  tab.addEventListener('keydown', event => {
    const tabs = [...document.querySelectorAll('.tab')];
    const index = tabs.indexOf(tab);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1
      : ['ArrowRight', 'ArrowDown'].includes(event.key) ? (index + 1) % tabs.length
      : ['ArrowLeft', 'ArrowUp'].includes(event.key) ? (index + tabs.length - 1) % tabs.length : -1;
    if (next < 0) return;
    event.preventDefault();
    tabs[next].focus();
    tabs[next].click();
  });
});

/* ── View state in the URL ─────────────────────────────────────── */
// A view is the active tab *and* the filters that decide what it shows, so the
// whole of it goes in the hash: a view can be linked, survive a reload, and be
// reached with the browser's back button. Only the tab name used to be
// routable, so paging into history and then pressing back left the tab instead
// of stepping back one page.
//
// Several controls drive one concept - the platform filter appears on five tabs
// - so a key binds a list of element ids. The first non-empty value wins when
// serialising, and every bound control is set when applying.
const VIEW_CONTROLS = {
  platform: ['provider-filter', 'overview-provider', 'perf-provider', 'analytics-provider', 'quota-provider'],
  // The history and analytics date ranges are separate keys because they are
  // separate controls with different defaults - history starts unbounded, the
  // analytics view opens on the last seven days. Sharing one key made the
  // analytics default the history view's range on every link.
  from: ['history-start'],
  to: ['history-end'],
  afrom: ['analytics-start'],
  ato: ['analytics-end'],
  q: ['history-search'],
  model: ['model-filter'],
  status: ['status-filter'],
  streaming: ['streaming-filter'],
  cost: ['cost-source-filter'],
  scenario: ['scenario-filter'],
  size: ['history-page-size'],
  days: ['quota-local-days'],
  range: ['perf-time-range'],
};

// Set while a hash is being applied. Assigning a control's value is silent, but
// the change event dispatched to trigger its handler is not, and without this
// the hash would be rewritten while it is being read.
let suppressViewStateSync = false;

// applyActiveSite points every platform selector at the configured site, which
// is the right default but not an override: a link that names a platform has
// already chosen one for the view, and re-pointing the selectors would silently
// discard it. Saving the config clears the pin, because changing the active
// site is a deliberate change to what the views open on.
//
// The pin records which platform the link named, not merely that it named one.
// A link naming a platform this deployment no longer offers cannot be honoured -
// the selector has no such option, so the view would open on "all platforms"
// while the proxy routes to the active site, and that mismatch would persist for
// the whole session because the pin never expires by itself.
let viewPlatformPinned = '';

// The default each control is measured against. Selects and inputs declare it
// in markup, which the DOM exposes as defaultValue. The analytics date range
// does not: it is computed from today when the module initialises, so its value
// before any URL is applied is the only correct answer, and it is captured once
// at boot.
const viewDefaults = new Map();

function captureViewDefaults() {
  for (const ids of Object.values(VIEW_CONTROLS)) {
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el && !el.defaultValue) viewDefaults.set(id, el.value ?? '');
    }
  }
}

function controlIsDefault(id, el) {
  if (el.defaultValue) return el.value === el.defaultValue;
  return el.value === (viewDefaults.get(id) ?? '');
}

// A default is only usable when it is actually known. A <select> always has
// one - its first option, or the one marked selected. An input has one only if
// boot captured it; resetting an input to an invented empty value would wipe a
// range the panel computed for itself.
function controlHasDefault(id, el) {
  return el.tagName === 'SELECT' || viewDefaults.has(id);
}

function defaultControlValue(id, el) {
  return el.defaultValue || viewDefaults.get(id) || '';
}

// buildViewHash serialises the current view. A parameter is written only when
// its control differs from its default, so a link carries the reader's choices
// rather than the panel's own opening state. History pagination and sort order
// are plain variables rather than controls, so they are appended separately
// under the same rule.
function buildViewHash() {
  const params = new URLSearchParams();
  for (const [key, ids] of Object.entries(VIEW_CONTROLS)) {
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el && el.value && !controlIsDefault(id, el)) {
        params.set(key, el.value);
        break;
      }
    }
  }
  if (activeTab === 'history') {
    if (historyPage > 1) params.set('page', String(historyPage));
    if (currentSort.field !== 'start_time') params.set('sort', currentSort.field);
    if (currentSort.dir !== 'desc') params.set('dir', currentSort.dir);
  }
  const query = params.toString();
  return '#' + (activeTab || 'overview') + (query ? '?' + query : '');
}

// syncViewState writes the current view into the URL. `replace` is for changes
// that are not navigation - typing in the search box, or a reload settling the
// page number - so the back button does not collect one entry per keystroke.
function syncViewState({replace = false} = {}) {
  if (suppressViewStateSync) return;
  const next = buildViewHash();
  if (next === (location.hash || '')) return;
  if (replace) history.replaceState(null, '', next);
  else location.hash = next;
}

function parseViewHash(hash) {
  const [name, query = ''] = (hash || '').replace(/^#/, '').split('?');
  return {name: name || 'overview', params: new URLSearchParams(query)};
}

// applyViewState restores a view from the URL and reports whether it moved
// history pagination, which has no control of its own to trigger a reload.
function applyViewState(params) {
  const before = `${historyPage}|${currentSort.field}|${currentSort.dir}`;
  suppressViewStateSync = true;
  try {
    for (const [key, ids] of Object.entries(VIEW_CONTROLS)) {
      for (const id of ids) {
        const el = document.getElementById(id);
        if (!el) continue;
        // An absent parameter means the default, not "leave whatever is there":
        // stepping back from a link that named a filter to one that omits it has
        // to undo the choice, or the URL and the panel disagree about the view.
        const value = params.has(key) ? (params.get(key) || '')
          : controlHasDefault(id, el) ? defaultControlValue(id, el) : el.value;
        if (el.value === value) continue;
        el.value = value;
        // The tab's own handler listens for this; a silent assignment would
        // leave the filter applied in the URL but not in the data.
        el.dispatchEvent(new Event('change', {bubbles: true}));
      }
    }
    if (params.has('page')) historyPage = Math.max(1, Number(params.get('page')) || 1);
    if (params.has('sort')) currentSort = {field: params.get('sort'), dir: currentSort.dir};
    if (params.has('dir')) currentSort = {field: currentSort.field, dir: params.get('dir')};
  } finally {
    suppressViewStateSync = false;
  }
  window.CustomSelect?.syncAll();
  return `${historyPage}|${currentSort.field}|${currentSort.dir}` !== before;
}

// Respond to back/forward and manual hash edits.
window.addEventListener('hashchange', () => {
  const {name, params} = parseViewHash(location.hash);
  const historyMoved = applyViewState(params);
  if (name !== activeTab) {
    activateTab(name);
    return;
  }
  if (historyMoved) refreshCurrentTab();
});

/* ── Polling ───────────────────────────────────────────────────── */
let perfPollTimer = null;
let perfPollCounter = 0;
let activeTab = 'overview';
let overviewDays = 7;
let overviewBreakdownMetric = 'requests';
let lastOverviewView = null;
let overviewLoadSeq = 0;
let overviewQuery = '';
// Upper bound on how many history rows are rendered into the DOM at once.
// Keeps long-session history tables fast while the count reflects all rows.
const HISTORY_RENDER_LIMIT = 200;

function startPolling() {
  refreshAll();
  PerfModule.init();
  PerfModule.refresh();
  // The active site is not a metric: it changes when an operator changes it,
  // which is rare, so it is polled slowly rather than with the 3s core loop.
  startActiveSitePolling();
  // Core metrics stay warm globally; tab-specific work only runs when visible.
  setInterval(refreshCore, 3000);
  setInterval(refreshCurrentTab, 3000);
  perfPollTimer = setInterval(() => {
    perfPollCounter++;
    if (perfPollCounter >= 2) {
      PerfModule.refresh();
      perfPollCounter = 0;
    }
  }, 3000);
}

// Lightweight poll: metrics + catalog age only, always on.
async function refreshCore() {
  await Promise.all([refreshMetrics(), refreshCatalogAge()]);
}

// Full refresh (used by manual triggers / token change). Kept for backward
// compatibility with debouncedRefresh.
async function refreshAll() {
  await Promise.all([refreshMetrics(), refreshHistory(), refreshConfig(), refreshCatalogAge()]);
}

// Refresh only tab-specific data; refreshCore already handles shared metrics.
async function refreshCurrentTab() {
  switch (activeTab) {
    case 'history':
      await refreshHistory();
      break;
    case 'quota':
      await QuotaModule.load();
      break;
    case 'settings':
      await refreshConfig();
      break;
  }
}

// Debounced refresh for manual triggers (keyboard shortcuts)
let refreshDebounceTimer = null;
function debouncedRefresh() {
  if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
  refreshDebounceTimer = setTimeout(() => {
    refreshAll();
    refreshDebounceTimer = null;
  }, 300);
}

/* ── /api/metrics ──────────────────────────────────────────────── */
async function refreshMetrics() {
  await Promise.all([refreshServiceStatus(), activeTab === 'overview' ? refreshOverviewUsage() : null]);
}

async function refreshServiceStatus() {
  try {
    // Process health remains global; usage failures must not mark the proxy down.
    const d = await fetchJSON('/api/metrics');
    markPollOk();

    // status badge
    const running = d.proxy_running;
    const connected = d.connected_to_existing;
    const dot  = document.getElementById('status-dot');
    const text = document.getElementById('status-text');
    dot.className = 'status-dot ' + (running ? 'running' : 'stopped');
    if (running && connected) {
      text.textContent = t('status.connected');
    } else if (running) {
      text.textContent = t('status.running');
    } else {
      text.textContent = t('status.stopped');
    }

    // port info
    const portEl = document.getElementById('port-info');
    if (d.port) {
      portEl.textContent = (currentLang === 'zh' ? '监听端口：' : 'Listening port: ') + d.port;
    }

    // model list
    lastModelCounts = d.model_counts || {};
    renderModelList(lastModelCounts);

    // proxy toggle sync
    const proxyToggle = document.getElementById('toggle-proxy');
    if (proxyToggle && !proxyToggle._changing) proxyToggle.checked = running;
  } catch (e) {
    // Startup races are expected, so the first failures stay quiet; markPollFail
    // only surfaces the stale badge once several polls in a row have failed.
    markPollFail();
  }
}

async function refreshOverviewUsage() {
  const seq = ++overviewLoadSeq;
  const params = new URLSearchParams({days: String(overviewDays)});
  const latencyParams = new URLSearchParams({range: `${overviewDays}d`});
  const provider = document.getElementById('overview-provider')?.value;
  if (provider) {
    params.set('provider', provider);
    latencyParams.set('provider', provider);
  }
  if (overviewQuery !== params.toString() || !lastOverviewView) {
    overviewQuery = params.toString();
    clearOverviewUsage(true);
  }
  const errorEl = document.getElementById('overview-error');
  if (errorEl) errorEl.hidden = true;
  try {
    const [usage, trend, latency] = await Promise.all([
      fetchJSON(`/api/analytics/summary?${params}&compare=1`),
      fetchJSON(`/api/analytics/tokens/trend?${params}`),
      fetchJSON(`/api/perf/aggregate?${latencyParams}`),
    ]);
    if (seq !== overviewLoadSeq) return;
    if (!usage?.summary || !trend || (trend.trend !== null && !Array.isArray(trend.trend)) || !latency) {
      throw new Error(t('data.invalid'));
    }
    renderOverviewUsage(usage, trend.trend || [], latency);
  } catch (e) {
    if (seq !== overviewLoadSeq) return;
    clearOverviewUsage();
    if (errorEl) {
      errorEl.textContent = t('data.loadFail') + e.message;
      errorEl.hidden = false;
    }
  }
}

function clearOverviewUsage(loading = false) {
  lastOverviewView = null;
  document.getElementById('overview-empty').hidden = true;
  document.getElementById('overview-insights').hidden = false;
  ['m-total', 'm-tokens', 'm-cache-hit', 'm-throughput', 'm-success', 'm-cost'].forEach(id => {
    document.getElementById(id).textContent = loading ? '…' : '—';
    document.getElementById(id + '-note').textContent = '';
  });
  ['overview-generated', 'overview-latency'].forEach(id => { document.getElementById(id).textContent = ''; });
  ['overview-request-trend', 'overview-token-trend', 'overview-provider-distribution', 'overview-model-distribution'].forEach(id => {
    document.getElementById(id).innerHTML = `<div class="empty-state">${t(loading ? 'data.loading' : 'detail.unavailable')}</div>`;
  });
}

function renderOverviewUsage(data, trend, latency) {
  const summary = data.summary || {};
  document.getElementById('overview-empty').hidden = summary.total_requests !== 0;
  document.getElementById('overview-insights').hidden = summary.total_requests === 0;
  const input = Number(summary.input_tokens || 0);
  const output = Number(summary.output_tokens || 0);
  const cacheRead = Number(summary.cache_read_tokens || 0);
  const cacheWrite = Number(summary.cache_creation_tokens || 0);
  const total = input + output + cacheRead + cacheWrite;
  const prompt = input + cacheRead + cacheWrite;
  const today = data.today || {};
  const retained = data.retained || {};
  const lastMinute = data.last_minute || {};
  const compactTotal = item => hasUsageTokens(item) ? fmtTok(totalUsageTokens(item)) : '—';
  const set = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };
  set('m-total', fmt(summary.total_requests));
  set('m-cost', fmtAggregateCost(summary));
  set('m-tokens', hasUsageTokens(summary) ? fmtTok(total) : '—');
  set('m-cache-hit', hasUsageTokens(summary) && prompt > 0 ? `${(cacheRead / prompt * 100).toFixed(1)}%` : '—');
  set('m-total-note', today.total_requests != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} (UTC) ${fmt(today.total_requests)} · ${t('data.retained')} ${fmt(retained.total_requests)}`
    : `${overviewDays} ${currentLang === 'zh' ? '天' : 'days'}`);
  set('m-tokens-note', today.input_tokens != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} (UTC) ${compactTotal(today)} · ${t('data.retained')} ${compactTotal(retained)}`
    : '');
  set('m-cache-hit-note', summary.cache_read_tokens != null ? `${fmtTok(cacheRead)} ${currentLang === 'zh' ? '读取' : 'read'}` : '');
  set('m-cost-note', Number(summary.unknown_cost_requests || 0) > 0 ? costCoverageNote(summary) : today.est_cost_usd != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} (UTC) ${fmtAggregateCost(today)} · ${t('data.retained')} ${fmtAggregateCost(retained)}`
    : t('analytics.currencyUSD'));
  set('m-throughput', lastMinute.total_requests != null ? `${fmt(lastMinute.total_requests)} RPM` : '—');
  set('m-throughput-note', hasUsageTokens(lastMinute) ? `${compactTotal(lastMinute)} TPM` : '');
  const known = Number(summary.known_requests || 0);
  set('m-success', known > 0 && summary.success_rate != null ? `${(Number(summary.success_rate) * 100).toFixed(1)}%` : '—');
  set('m-success-note', t('analytics.knownRecords').replace('{n}', known.toLocaleString()));
  set('overview-generated', new Date().toLocaleString(undefined, {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'}));
  set('overview-latency', Number(latency?.avg_latency_ms || 0) > 0
    ? `· ${t('analytics.avgLatency')} ${fmtDuration(latency.avg_latency_ms)}`
    : '');

  const withTotal = items => (items || []).map(item => ({
    ...item,
    total_tokens: Number(item.input_tokens || 0) + Number(item.output_tokens || 0)
      + Number(item.cache_read_tokens || 0) + Number(item.cache_creation_tokens || 0),
  }));
  const filledTrend = fillRecentDailyTrend(trend || [], overviewDays);
  lastOverviewView = {data, trend: trend || [], latency};
  AnalyticsModule.renderRequestTrend(filledTrend, 'overview-request-trend');
  AnalyticsModule.renderTokenLines(filledTrend, 'overview-token-trend');
  const valueKey = overviewBreakdownMetric === 'cost' ? 'cost_usd'
    : overviewBreakdownMetric === 'tokens' ? 'total_tokens' : 'requests';
  AnalyticsModule.renderDistribution('overview-provider-distribution', withTotal(data.providers), valueKey, 'provider');
  AnalyticsModule.renderDistribution('overview-model-distribution', withTotal(data.models), valueKey, 'model');
}

function renderModelList(counts) {
  lastModelCounts = counts;
  const list = document.getElementById('model-list');
  const entries = Object.entries(counts).sort((a, b) => b[1] - a[1]);
  if (entries.length === 0) {
    list.innerHTML = '<div class="empty-state">' + t('empty.noData') + '</div>';
    return;
  }
  const max = entries[0][1];
  list.innerHTML = entries.slice(0, 10).map(([model, count]) => `
    <div class="model-row">
      <div class="model-name" title="${escapeHtml(model)}">${escapeHtml(model)}</div>
      <div class="model-bar-wrap">
        <div class="model-bar" style="width:${Math.round(count/max*100)}%"></div>
      </div>
      <div class="model-count">${count}</div>
    </div>
  `).join('');
}

/* ── /api/history ──────────────────────────────────────────────── */
// Pagination state for the request history table.
let historyPage = 1;
let historySize = 50;
let historyTotal = 0;
let historyLoadSeq = 0;
let historyQuery = '';

function dateInputValue(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function utcDateInputValue(date) {
  return date.toISOString().slice(0, 10);
}

function historyDateBoundary(value, endOfDay) {
  if (!value) return '';
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) return '';
  if (endOfDay) date.setDate(date.getDate() + 1);
  return date.toISOString();
}

function historyQueryParams(page = historyPage, size = historySize) {
  const params = new URLSearchParams({ page: String(page), size: String(size) });
  const values = {
    search: document.getElementById('history-search')?.value.trim(),
    model: document.getElementById('model-filter')?.value.trim(),
    provider: document.getElementById('provider-filter')?.value.trim(),
    scenario: document.getElementById('scenario-filter')?.value.trim(),
    success: document.getElementById('status-filter')?.value,
    streaming: document.getElementById('streaming-filter')?.value,
    cost_source: document.getElementById('cost-source-filter')?.value,
  };
  Object.entries(values).forEach(([key, value]) => {
    if (value) params.set(key, value);
  });
  const start = historyDateBoundary(document.getElementById('history-start')?.value, false);
  const end = historyDateBoundary(document.getElementById('history-end')?.value, true);
  if (start) params.set('start', start);
  if (end) params.set('end', end);
  params.set('sort', currentSort.field);
  params.set('order', currentSort.dir);
  return params;
}

function historyHasFilters() {
  return ['history-search', 'history-start', 'history-end', 'model-filter', 'provider-filter',
    'scenario-filter', 'status-filter', 'streaming-filter', 'cost-source-filter']
    .some(id => document.getElementById(id)?.value);
}

function resetHistoryFilters(refresh = true) {
  ['history-search', 'history-start', 'history-end', 'model-filter', 'provider-filter',
    'scenario-filter', 'status-filter', 'streaming-filter', 'cost-source-filter'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.value = '';
  });
  window.CustomSelect?.syncAll();
  window.HistoryDateRange?.syncFromHidden();
  syncAdvancedFilters(false);
  historyPage = 1;
  if (refresh) refreshHistory();
}

function syncAdvancedFilters(reveal = true) {
  const count = ['model-filter', 'scenario-filter', 'streaming-filter', 'cost-source-filter']
    .filter(id => document.getElementById(id)?.value.trim()).length;
  document.getElementById('history-advanced-count').textContent = count ? String(count) : '';
  if (reveal && count) document.getElementById('history-advanced-filters').open = true;
}

async function refreshHistory() {
  syncAdvancedFilters(false);
  const seq = ++historyLoadSeq;
  const errorEl = document.getElementById('history-error');
  try {
    const params = historyQueryParams();
    if (historyQuery !== params.toString()) {
      historyQuery = params.toString();
      clearHistoryView(true);
    }
    const [r, summaryResponse] = await Promise.all([
      fetch(`/api/history?${params}`),
      fetch(`/api/history/summary?${params}`),
    ]);
    if (!r.ok) throw new Error(`HTTP ${r.status}: ${(await r.text()).trim()}`);
    if (!summaryResponse.ok) throw new Error(`Summary HTTP ${summaryResponse.status}: ${(await summaryResponse.text()).trim()}`);
    const [data, summary] = await Promise.all([r.json(), summaryResponse.json()]);
    if (seq !== historyLoadSeq) return;
    // New paginated shape: { items, total, page, size }. Tolerate the old
    // bare-array shape too.
    if (Array.isArray(data)) {
      allHistory = data;
      historyTotal = data.length;
      historyPage = 1;
    } else {
      allHistory = data.items || [];
      historyTotal = data.total || 0;
      // Clamp page if the dataset shrank after a delete.
      const maxPage = Math.max(1, Math.ceil(historyTotal / historySize));
      if (historyPage > maxPage) historyPage = maxPage;
    }
    renderHistory();
    renderHistoryPager();
    renderHistorySummary(summary);
    if (errorEl) errorEl.hidden = true;
  } catch(e) {
    if (seq !== historyLoadSeq) return;
    if (!lastHistorySummary) clearHistoryView();
    if (errorEl) {
      errorEl.textContent = t('history.loadFail') + e.message;
      errorEl.hidden = false;
    }
  }
}

let lastHistorySummary = null;
let historyBreakdownMetric = 'tokens';

function clearHistoryView(loading = false) {
  allHistory = [];
  historyTotal = 0;
  lastHistorySummary = null;
  const tip = document.getElementById('history-token-tip');
  if (tip) tip.style.display = 'none';
  if (loading) document.getElementById('history-error').hidden = true;
  const message = t(loading ? 'data.loading' : 'detail.unavailable');
  document.getElementById('history-tbody').innerHTML = `<tr><td colspan="7" class="empty-state">${message}</td></tr>`;
  ['history-summary-requests', 'history-summary-success', 'history-summary-tokens', 'history-summary-cost'].forEach(id => {
    document.getElementById(id).textContent = loading ? '…' : '—';
  });
  ['history-count', 'history-pageinfo', 'history-pagetotal', 'history-summary-token-note', 'history-summary-cost-note'].forEach(id => {
    document.getElementById(id).textContent = '';
  });
  ['history-model-breakdown', 'history-provider-breakdown', 'history-scenario-breakdown'].forEach(id => {
    document.getElementById(id).innerHTML = `<span class="compact-breakdown-value">${message}</span>`;
  });
  ['btn-history-prev', 'btn-history-next'].forEach(id => { document.getElementById(id).disabled = true; });
}

function renderHistorySummary(summary) {
  lastHistorySummary = summary;
  const set = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };
  set('history-summary-requests', Number(summary.total_requests || 0).toLocaleString());
  set('history-summary-success', Number(summary.success_rows || 0) > 0
    ? `${(Number(summary.success_rate || 0) * 100).toFixed(1)}%`
    : '—');
  set('history-summary-tokens', fmtTok(Number(summary.total_tokens || 0)));
  set('history-summary-token-note', [
    `${t('analytics.inputTokens')} ${fmtTok(summary.input_tokens || 0)}`,
    `${t('analytics.outputTokens')} ${fmtTok(summary.output_tokens || 0)}`,
    `${t('detail.cacheRead')} ${fmtTok(summary.cache_read_tokens || 0)}`,
    `${t('detail.cacheCreation')} ${fmtTok(summary.cache_creation_tokens || 0)}`,
  ].join(' · '));
  set('history-summary-cost', fmtAggregateCost(summary));
  set('history-summary-cost-note', costCoverageNote(summary));
  renderCompactBreakdown('history-model-breakdown', summary.models || []);
  renderCompactBreakdown('history-provider-breakdown', summary.providers || [], 'provider');
  renderCompactBreakdown('history-scenario-breakdown', summary.scenarios || []);
}

function renderCompactBreakdown(id, items, dimension) {
  const root = document.getElementById(id);
  if (!root) return;
  const metricKey = historyBreakdownMetric === 'cost' ? 'cost_usd' : historyBreakdownMetric;
  const sorted = [...items].sort((a, b) => dimension === 'provider'
    ? compareProviderDisplay(a.name, b.name)
    : Number(b[metricKey] || 0) - Number(a[metricKey] || 0));
  const top = dimension === 'provider' ? sorted : sorted.slice(0, 5);
  if (!top.length) {
    root.innerHTML = `<span class="compact-breakdown-value">${t('analytics.noData')}</span>`;
    return;
  }
  const total = items.reduce((sum, item) => sum + Number(item[metricKey] || 0), 0) || 1;
  root.innerHTML = top.map(item => {
    const name = !item.name || item.name === 'unknown' ? t('detail.unknown') : item.name;
    const label = item.provider ? `${name} (${item.provider})` : name;
    const value = Number(item[metricKey] || 0);
    return `
    <div class="compact-breakdown-row">
      <i class="compact-breakdown-fill" style="width:${Math.max(value > 0 ? 2 : 0, value / total * 100).toFixed(1)}%"></i>
      <span class="compact-breakdown-name">${escapeHtml(label)}</span>
      <span class="compact-breakdown-stat"><strong>${Number(item.requests || 0).toLocaleString()}</strong><small>${t('analytics.requests')}</small></span>
      <span class="compact-breakdown-stat"><strong>${fmtTok(Number(item.tokens || 0))}</strong><small>Token</small></span>
      <span class="compact-breakdown-stat" title="${escapeHtml(costCoverageNote(item))}"><strong>${fmtAggregateCost(item)}</strong><small>${t('analytics.cost')}</small></span>
    </div>`;
  }).join('');
}

document.querySelectorAll('#history-breakdown-metric button').forEach(button => {
  button.addEventListener('click', () => {
    historyBreakdownMetric = button.dataset.metric;
    button.parentElement.querySelectorAll('button').forEach(item => item.classList.toggle('active', item === button));
    if (lastHistorySummary) renderHistorySummary(lastHistorySummary);
  });
});
function historyGoToPage(p) {
  if (p < 1) p = 1;
  const max = Math.max(1, Math.ceil(historyTotal / historySize));
  if (p > max) p = max;
  historyPage = p;
  syncViewState();
  refreshHistory();
}

function renderHistoryPager() {
  const max = Math.max(1, Math.ceil(historyTotal / historySize));
  const pageInfo = document.getElementById('history-pageinfo');
  const pageTotal = document.getElementById('history-pagetotal');
  const prev = document.getElementById('btn-history-prev');
  const next = document.getElementById('btn-history-next');
  if (pageInfo) pageInfo.textContent = currentLang === 'zh' ? `第 ${historyPage} / ${max} 页` : `Page ${historyPage} / ${max}`;
  if (pageTotal) pageTotal.textContent = currentLang === 'zh' ? `共 ${historyTotal} 条` : `${historyTotal} records`;
  if (prev) prev.disabled = historyPage <= 1;
  if (next) next.disabled = historyPage >= max;
  // The loaded page can differ from the one asked for (a shrinking result set
  // clamps it), so settle the URL on what is actually shown. replaceState, not
  // the hash setter: this runs during a load and must not push an entry or
  // fire a hashchange back into the loader.
  syncViewState({replace: true});
}

// Each platform's published peak-pricing rule, mirroring history.peakSchedules
// (internal/history/record.go) - the Go side owns the answer and
// TestPeakSchedulesMatchTheBackend keeps this mirror honest.
//
// Both platforms name their covered models row by row in their pricing table,
// so the set is listed rather than matched by substring: the older
// deepseek-v3/r1/chat families, the "fast" variant and the dated snapshots all
// carry a single rate and must stay off-peak.
const PEAK_FAMILIES = ['deepseek-v4.1-flash', 'deepseek-v4-flash', 'deepseek-v4-flash-vision-exp', 'deepseek-v4-pro'];
const PEAK_SCHEDULES = {
  'opencode-go': {models: PEAK_FAMILIES, windows: [[1, 4], [6, 10]], multiplier: 2},
  commandcode: {models: PEAK_FAMILIES, windows: [[1, 4], [6, 10]], multiplier: 2},
};

// Derive the peak multiplier from provider + model + start_time, matching
// history.ProviderPeakMultiplier (weekday UTC 01-04 / 06-10). Used in place of
// the stored column so a row the backfill has not reached still shows the
// badge; an explicit stored multiplier > 1 wins, because the platform's billing
// clock can sit a second or two from our start_time at a window boundary.
function effectivePeakMultiplier(h) {
  const stored = Number(h.peak_multiplier);
  if (stored > 1) return stored;
  // An empty provider keeps the pre-provider-column reading, like the backend:
  // opencode-go is the platform a blank provider means.
  const provider = String(h.provider || '').replace(/_/g, '-') || 'opencode-go';
  const schedule = PEAK_SCHEDULES[provider];
  if (!schedule) return 1;
  const model = String(h.model || '').toLowerCase().split('/').pop();
  if (schedule.models.indexOf(model) < 0) return 1;
  const t = new Date(h.start_time);
  if (isNaN(t.getTime())) return 1;
  const day = t.getUTCDay();
  if (day === 0 || day === 6) return 1;
  const hour = t.getUTCHours();
  const inWindow = schedule.windows.some(w => hour >= w[0] && hour < w[1]);
  return inWindow ? schedule.multiplier : 1;
}

// Peak or off-peak as billed, for the request detail view.
function billingWindowLabel(record) {
  const pm = effectivePeakMultiplier(record);
  return pm > 1 ? t('detail.peak') + ' \u00d7' + pm : t('detail.offPeak');
}

function renderHistory() {
  const tbody = document.getElementById('history-tbody');
  document.getElementById('history-count').textContent =
    historyTotal + t('status.count') + (historyHasFilters() ? t('status.filtered') : '');

  if (allHistory.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + (historyHasFilters()
      ? emptyStateContent('history.noMatches', 'history.emptyFiltersHint')
      : emptyStateContent('empty.noHistory', 'data.localEmptyHint')) + '</td></tr>';
    return;
  }

  // Cap the DOM to the most recent rows to avoid rebuilding a huge <tbody>
  // after long sessions. The count above still reflects the full (filtered)
  // set, so the user only loses DOM rendering cost, not information.
  const limited = allHistory.length > HISTORY_RENDER_LIMIT
    ? allHistory.slice(0, HISTORY_RENDER_LIMIT)
    : allHistory;

  tbody.innerHTML = limited.map(h => {
    const rowId = h.id || `${h.start_time}_${h.model || 'unknown'}_${h.duration_ms || 0}`;
    const cost = h.cost_usd != null ? fmtCost(h.cost_usd) : '—';
    const detailsKnown = historyHasDetails(h);
    const streamLabel = detailsKnown
      ? (h.streaming ? t('history.streaming') : t('history.nonStreaming'))
      : t('detail.unknown');
    const totalTokens = Number(h.input_tokens || 0) + Number(h.output_tokens || 0)
      + Number(h.cache_read_tokens || 0) + Number(h.cache_creation_tokens || 0);
    const pm = effectivePeakMultiplier(h);
    const peakMark = pm > 1
      ? ' <span class="badge badge-peak" title="' + t('history.peakWindow') + '">Peak ×' + pm + '</span>'
      : '';
    return `
    <tr data-id="${escapeHtml(rowId)}" tabindex="0" aria-haspopup="dialog" data-provider="${escapeHtml(h.provider || '')}" style="cursor: pointer;">
      <td><time class="history-timestamp" datetime="${escapeHtml(h.start_time || '')}">${fmtTime(h.start_time)}<small>${fmtDate(h.start_time)}</small></time>${peakMark}</td>
      <td><div class="history-status-stack">${detailsKnown ? `<span class="badge ${h.success ? 'badge-success' : 'badge-error'}" title="${h.success ? t('badge.success') : t('badge.fail')}">${h.success ? t('badge.success') : t('badge.fail')}</span>` : `<span class="badge badge-unknown" title="${t('detail.unknown')}">${t('detail.unknown')}</span>`}<small class="history-stream-state">${streamLabel}</small></div></td>
      <td><div class="history-model-cell"><strong title="${escapeHtml(h.model)}">${escapeHtml(h.model) || '—'}</strong><small>${escapeHtml(providerLabel(h.provider))}</small></div></td>
      <td><span class="badge badge-scene" title="${t('detail.scenario')}: ${escapeHtml(h.scenario) || '—'}">${escapeHtml(h.scenario) || '—'}</span></td>
      <td><button type="button" class="history-token-trigger" data-token-id="${escapeHtml(rowId)}" aria-label="${t('detail.title')}">${totalTokens.toLocaleString()}</button></td>
      <td>${cost}<br><small>${costSourceLabel(h.cost_source)}</small></td>
      <td>${detailsKnown ? fmtDuration(h.duration_ms) : '—'}</td>
    </tr>
  `}).join('');

  bindHistoryTokenTooltips(tbody);

  // Add pointer and keyboard handlers for detail modal.
  tbody.querySelectorAll('tr[data-id]').forEach(row => {
    const open = function() {
      const rowId = this.dataset.id;
      const record = allHistory.find(h => {
        const expectedId = h.id || `${h.start_time}_${h.model || 'unknown'}_${h.duration_ms || 0}`;
        return expectedId === rowId;
      });
      if (record) showHistoryDetail(record);
    };
    row.addEventListener('click', open);
    row.addEventListener('keydown', function(e) {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        open.call(this);
      }
    });
  });
}

function bindHistoryTokenTooltips(root) {
  let tip = document.getElementById('history-token-tip');
  if (!tip) {
    tip = document.createElement('div');
    tip.id = 'history-token-tip';
    tip.className = 'chart-tip history-token-tip';
    document.body.appendChild(tip);
  }
  const hide = () => { tip.style.display = 'none'; };
  root.querySelectorAll('.history-token-trigger').forEach(trigger => {
    const show = () => {
      const record = allHistory.find(item => {
        const id = item.id || `${item.start_time}_${item.model || 'unknown'}_${item.duration_ms || 0}`;
        return id === trigger.dataset.tokenId;
      });
      if (!record) return;
      const prompt = Number(record.input_tokens || 0) + Number(record.cache_read_tokens || 0) + Number(record.cache_creation_tokens || 0);
      const total = prompt + Number(record.output_tokens || 0);
      const rate = prompt > 0 ? Number(record.cache_read_tokens || 0) / prompt * 100 : 0;
      tip.innerHTML = chartTooltipMarkup(currentLang === 'zh' ? 'Token 明细' : 'Token details', [
        {label:t('detail.inputTokens'),value:Number(record.input_tokens||0).toLocaleString(),color:'#818cf8'},
        {label:t('detail.cacheRead'),value:Number(record.cache_read_tokens||0).toLocaleString(),color:'#fbbf24'},
        {label:t('detail.cacheCreation'),value:Number(record.cache_creation_tokens||0).toLocaleString(),color:'#fb7185'},
        {label:t('detail.outputTokens'),value:Number(record.output_tokens||0).toLocaleString(),color:'#34d399'},
      ], [
        {label:t('analytics.cacheHitShort'),value:prompt>0?`${rate.toFixed(1)}%`:'—'},
        {label:t('analytics.totalTokens'),value:total.toLocaleString()},
      ]);
      tip.style.display = 'block';
      const rect = trigger.getBoundingClientRect();
      const bounds = tip.getBoundingClientRect();
      const left = Math.max(12, Math.min(innerWidth - bounds.width - 12, rect.right - bounds.width));
      const top = rect.bottom + bounds.height + 10 <= innerHeight ? rect.bottom + 8 : Math.max(12, rect.top - bounds.height - 8);
      tip.style.left = `${left}px`;
      tip.style.top = `${top}px`;
    };
    trigger.addEventListener('pointerenter', show);
    trigger.addEventListener('pointerleave', hide);
    trigger.addEventListener('focus', show);
    trigger.addEventListener('blur', hide);
    trigger.addEventListener('click', hide);
  });
}

/* ── /api/config ───────────────────────────────────────────────── */
async function refreshConfig() {
  try {
    const r = await fetch('/api/config');
    if (!r.ok) return;
    const d = await r.json();
    const autostartToggle = document.getElementById('toggle-autostart');
    const notifyToggle    = document.getElementById('toggle-notify');
    if (autostartToggle && !autostartToggle._changing) autostartToggle.checked = !!d.autostart;
    if (notifyToggle    && !notifyToggle._changing)    notifyToggle.checked    = !!d.notify;
  } catch(e) {}
}

/* ── /api/catalog/lock & /api/catalog/sync ─────────────────────── */
async function refreshCatalogAge() {
  try {
    const r = await fetch('/api/catalog/lock');
    if (!r.ok) return;
    const d = await r.json();
    const el = document.getElementById('catalog-age');
    if (!el) return;
    if (!d.synced) {
      el.textContent = t('setting.catalogNotSynced');
      return;
    }
    el.textContent = t('setting.catalogAge').replace('{age}', fmtAge(d.age_seconds));
  } catch(e) {}
}

async function refreshCatalog() {
  const btn = document.getElementById('btn-refresh-catalog');
  if (btn) {
    btn.disabled = true;
    btn.textContent = currentLang === 'zh' ? '同步中…' : 'Syncing…';
  }
  try {
    const r = await fetch('/api/catalog/sync', { method: 'POST' });
    if (r.ok) {
      await refreshCatalogAge();
      toast(t('toast.catalogSynced'), 'success');
    } else {
      const txt = await r.text();
      console.error('Catalog refresh failed:', txt);
      toast(t('toast.catalogSyncFailed') + txt, 'error');
    }
  } catch(e) {
    console.error('Catalog refresh network error:', e);
    toast(t('toast.catalogNetworkError'), 'error');
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = t('btn.refreshCatalog');
    }
  }
}

/* ── Toggle actions ────────────────────────────────────────────── */
async function toggleProxy(el) {
  el._changing = true;
  try {
    const action = el.checked ? 'start' : 'stop';
    const r = await fetch('/api/proxy/' + action, { method: 'POST' });
    if (r.ok) {
      toast(el.checked ? t('toast.proxyStarted') : t('toast.proxyStopped'), 'success');
    } else {
      el.checked = !el.checked;
      toast(t('toast.proxyActionFailed'), 'error');
    }
  } catch(e) {
    el.checked = !el.checked;
    toast(t('toast.networkError'), 'error');
  }
  setTimeout(() => { el._changing = false; }, 1000);
}

async function toggleAutostart(el) {
  el._changing = true;
  try {
    const r = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ autostart: el.checked })
    });
    if (!r.ok) { el.checked = !el.checked; }
  } catch(e) { el.checked = !el.checked; }
  setTimeout(() => { el._changing = false; }, 1000);
}

async function toggleNotify(el) {
  el._changing = true;
  try {
    const r = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ notify: el.checked })
    });
    if (!r.ok) { el.checked = !el.checked; }
  } catch(e) { el.checked = !el.checked; }
  setTimeout(() => { el._changing = false; }, 1000);
}

/* ── CSV export ────────────────────────────────────────────────── */
// Model names and error messages reach this file from upstream responses, so a
// value starting with = + - @ would run as a formula when the CSV is opened in
// Excel or Sheets. Prefixing those with a quote neutralises them.
function escapeCSV(value) {
  if (value == null) return '';
  const str = String(value);
  const escaped = str.replace(/"/g, '""');
  if (/^[=+\-@\t\r]/.test(str)) return `"'${escaped}"`;
  if (/[,"\n\r]/.test(str)) return `"${escaped}"`;
  return str;
}

const HISTORY_CSV_COLUMNS = [
  ['id', r => r.id],
  ['start_time', r => r.start_time],
  ['model', r => r.model],
  ['provider', r => r.provider],
  ['scenario', r => r.scenario],
  ['input_tokens', r => r.input_tokens],
  ['prompt_tokens', r => r.prompt_tokens],
  ['cache_read_tokens', r => r.cache_read_tokens],
  ['cache_creation_tokens', r => r.cache_creation_tokens],
  ['output_tokens', r => r.output_tokens],
  ['cost_usd', r => r.cost_usd],
  ['cost_source', r => r.cost_source],
  ['details_known', r => historyHasDetails(r)],
  ['duration_ms', r => historyHasDetails(r) ? r.duration_ms : null],
  ['streaming', r => historyHasDetails(r) ? r.streaming : null],
  ['attempt', r => historyHasDetails(r) ? r.attempt : null],
  ['success', r => historyHasDetails(r) ? r.success : null],
  ['error_msg', r => r.error_msg || ''],
];

// Export walks the API rather than the rendered page so the file covers the
// whole history, not just the page currently on screen. Pages are fetched one
// after another on purpose: this is a local proxy and a burst of parallel
// requests would compete with live traffic for no real gain.
async function exportHistoryCSV() {
  const btn = document.getElementById('history-export');
  if (btn) { btn.disabled = true; btn.textContent = t('export.working'); }
  try {
    const size = 500;
    const params = historyQueryParams(1, size);
    const rows = [];
    for (let page = 1; ; page++) {
      params.set('page', String(page));
      const r = await fetch(`/api/history?${params}`);
      if (!r.ok) throw new Error(`history page ${page}: ${r.status}`);
      const d = await r.json();
      const items = d.items || [];
      rows.push(...items);
      if (items.length < size || rows.length >= (d.total || 0)) break;
    }
    if (!rows.length) { toast(t('export.empty')); return; }

    const csv = [
      HISTORY_CSV_COLUMNS.map(c => escapeCSV(c[0])).join(','),
      ...rows.map(r => HISTORY_CSV_COLUMNS.map(c => escapeCSV(c[1](r))).join(',')),
    ].join('\n');

    // The BOM is what makes Excel read the file as UTF-8 instead of the local
    // codepage, which otherwise mangles non-ASCII model and error text.
    const blob = new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `routatic-proxy-history-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    toast(t('export.ok').replace('{n}', rows.length));
  } catch (e) {
    toast(t('export.fail'));
    console.error('CSV export failed:', e);
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = t('export.csv'); }
  }
}

/* ── Connection health ─────────────────────────────────────────── */
// The dashboard polls every few seconds and used to swallow every fetch error,
// so a dead backend looked identical to an idle one: stale numbers, green dot,
// no hint anything was wrong. Track consecutive failures instead and surface a
// stale badge once a couple of polls in a row have failed, which keeps a single
// dropped request from flapping the UI.
const CONN = { fails: 0, threshold: 2, lastOk: null };

function markPollOk() {
  CONN.fails = 0;
  CONN.lastOk = Date.now();
  const el = document.getElementById('conn-stale');
  if (el) el.hidden = true;
}

function markPollFail() {
  CONN.fails++;
  if (CONN.fails < CONN.threshold) return;
  const el = document.getElementById('conn-stale');
  if (!el) return;
  el.hidden = false;
  const secs = CONN.lastOk ? Math.round((Date.now() - CONN.lastOk) / 1000) : null;
  el.textContent = secs != null
    ? t('status.staleFor').replace('{secs}', secs)
    : t('status.stale');
  // A stale panel must not keep claiming the proxy is up.
  const dot = document.getElementById('status-dot');
  if (dot) dot.className = 'status-dot stale';
}

/* ── Helpers ───────────────────────────────────────────────────── */
// Platform identity lives in Go (internal/site): which platforms exist, what
// they are called, their order and their visibility. This map holds only what
// the dashboard adds on top - a colour, and the labels the selectors offer.
// site_parity_test.go fails if the two lists drift, so hiding a platform is a
// registry change plus this map, not two independently editable lists.
//
// HIDDEN_PLATFORMS keeps the labels for platforms the dashboard no longer
// offers: history and analytics rows recorded under them must still render a
// name instead of a raw id.
const HIDDEN_PLATFORMS = {
  'opencode-zen': {name: 'OpenCode Zen', color: '#34d399'},
  'aws-bedrock': {name: 'AWS Bedrock', color: '#fbbf24'},
  'openrouter': {name: 'OpenRouter', color: '#fb7185'},
};

const PROVIDERS = {
  'opencode-go': {name: 'OpenCode Go', color: '#818cf8'},
  'commandcode': {name: 'CommandCode', color: '#22d3ee'},
};

function providerInfo(provider) {
  const id = String(provider || '').replace(/_/g, '-');
  return PROVIDERS[id] || HIDDEN_PLATFORMS[id] || null;
}

// Every platform in presentation order, hidden ones included, mirroring
// site.Order. A row recorded under a platform the dashboard no longer offers
// must still sort where the registry puts it rather than after every unknown
// name.
const PROVIDER_ORDER = [...Object.keys(PROVIDERS), ...Object.keys(HIDDEN_PLATFORMS)];

// Display only: configured routing and fallback chains keep their own order.
function compareProviderDisplay(a, b) {
  const rank = provider => {
    const index = PROVIDER_ORDER.indexOf(String(provider || '').replace(/_/g, '-'));
    return index < 0 ? PROVIDER_ORDER.length : index;
  };
  return rank(a) - rank(b) || String(a || '').localeCompare(String(b || ''));
}

async function fetchJSON(url, options) {
  const response = await fetch(url, options);
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${(await response.text()).trim()}`);
  return response.json();
}

function providerColor(provider) {
  return providerInfo(provider)?.color || '#98989d';
}

function providerLabel(provider) {
  // A hidden platform still has a name, so rows recorded under it keep
  // rendering one instead of falling back to the raw id.
  return providerInfo(provider)?.name || provider || t('detail.unknown');
}

function emptyStateContent(titleKey, hintKey) {
  return `<strong>${escapeHtml(t(titleKey))}</strong><p class="empty-state-note">${escapeHtml(t(hintKey))}</p>`;
}

function historyHasDetails(record) {
  return record.details_known !== false && typeof record.success === 'boolean';
}

function costSourceLabel(source) {
  return source === 'provider' ? t('history.costProvider')
    : source === 'estimated' ? t('history.costEstimated') : t('detail.unknown');
}

function viewProviderHistory(provider) {
  resetHistoryFilters(false);
  document.getElementById('provider-filter').value = provider;
  window.CustomSelect?.syncAll();
  location.hash = 'history';
  activateTab('history');
}

function fmt(n) { return n != null ? Number(n).toLocaleString() : '—'; }
function fmtTok(n) {
  const value = Number(n || 0);
  if (value >= 1_000_000_000_000) return (value / 1_000_000_000_000).toFixed(1) + 'T';
  if (value >= 1_000_000_000) return (value / 1_000_000_000).toFixed(1) + 'B';
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1) + 'M';
  if (value >= 1000) return (value / 1000).toFixed(1) + 'K';
  return String(value);
}

function totalUsageTokens(item) {
  return Number(item?.input_tokens || 0) + Number(item?.output_tokens || 0)
    + Number(item?.cache_read_tokens || 0) + Number(item?.cache_creation_tokens || 0);
}

function hasUsageTokens(item) {
  return ['input_tokens', 'output_tokens', 'cache_read_tokens', 'cache_creation_tokens']
    .every(key => Number.isFinite(item?.[key]));
}

function fillRecentDailyTrend(points, days) {
  const byDate = new Map((points || []).map(point => [point.date, point]));
  const end = new Date();
  end.setUTCHours(0, 0, 0, 0);
  const start = new Date(end);
  start.setUTCDate(start.getUTCDate() - Math.max(0, days - 1));
  const result = [];
  while (start <= end) {
    const date = utcDateInputValue(start);
    result.push(byDate.get(date) || {date, requests:0, known_requests:0, error_requests:0, input_tokens:0, output_tokens:0, cache_read_tokens:0, cache_creation_tokens:0, cost_usd:0});
    start.setUTCDate(start.getUTCDate() + 1);
  }
  return result;
}

// Costs here are often fractions of a cent, so a fixed 2-decimal format
// collapses real spend to "$0.00". Widen the precision for small amounts
// and keep the familiar 2 decimals once the total is worth reading.
function fmtCost(v) {
  if (v == null || !isFinite(v)) return '—';
  const n = Number(v);
  if (n === 0) return '$0.00';
  const abs = Math.abs(n);
  if (abs < 0.01) return '$' + n.toFixed(abs < 0.001 ? 5 : 4);
  if (abs < 1) return '$' + n.toFixed(3);
  return '$' + n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

// Aggregate amounts are known subtotals, not zero-cost promises for unpriced rows.
function fmtAggregateCost(item) {
  const unknown = Number(item?.unknown_cost_requests || 0);
  const requests = Number(item?.total_requests ?? item?.requests ?? 0);
  if (unknown > 0 && unknown >= requests) return '—';
  const value = fmtCost(item?.cost_usd ?? item?.est_cost_usd);
  return unknown > 0 ? t('analytics.knownSubtotal').replace('{value}', value) : value;
}

function costCoverageNote(item) {
  const unknown = Number(item?.unknown_cost_requests || 0);
  return unknown > 0 ? t('analytics.unknownCosts').replace('{n}', unknown.toLocaleString()) : t('analytics.currencyUSD');
}

function chartTooltipMarkup(title, rows, footer) {
  const body = rows.map(row => `
    <div class="tip-row">
      <span class="tip-dot" style="background:${row.color || '#818cf8'}"></span>
      <span class="tip-label">${escapeHtml(row.label)}</span>
      <strong class="tip-val">${escapeHtml(row.value)}</strong>
    </div>`).join('');
  const tail = (footer || []).length ? `<div class="tip-footer">${footer.map(row => `
    <div><span>${escapeHtml(row.label)}</span><strong>${escapeHtml(row.value)}</strong></div>`).join('')}</div>` : '';
  return `<div class="tip-title">${escapeHtml(title)}</div><div class="tip-body">${body}</div>${tail}`;
}

function bindPlotTooltip(root, tip, options) {
  const svg = root?.querySelector('svg');
  const target = root?.querySelector('.usage-chart-scrub');
  const cursor = root?.querySelector('.usage-chart-cursor');
  const cursorLine = cursor?.querySelector('.usage-chart-crosshair');
  const cursorPoints = [...(cursor?.querySelectorAll('.usage-chart-cursor-point') || [])];
  if (!svg || !target || !tip || !cursor || !cursorLine || !options?.pointCount) return;

  let currentIndex = 0;
  let pinnedIndex = -1;
  let pointerInside = false;
  const maxIndex = options.pointCount - 1;
  const clampIndex = index => Math.max(0, Math.min(maxIndex, index));
  const indexFromPointer = event => {
    const bounds = svg.getBoundingClientRect();
    const viewBox = svg.viewBox.baseVal;
    const svgX = viewBox.x + (event.clientX - bounds.left) / Math.max(1, bounds.width) * viewBox.width;
    const progress = (svgX - options.plotLeft) / Math.max(1, options.plotWidth);
    return clampIndex(Math.round(progress * maxIndex));
  };
  const clientPoint = index => {
    const bounds = svg.getBoundingClientRect();
    const viewBox = svg.viewBox.baseVal;
    const x = options.xForIndex(index);
    return {
      x: bounds.left + (x - viewBox.x) / viewBox.width * bounds.width,
      y: bounds.top + (options.plotTop - viewBox.y) / viewBox.height * bounds.height,
    };
  };
  const place = (index, event) => {
    const bounds = root.getBoundingClientRect();
    const fallback = clientPoint(index);
    const x = Number.isFinite(event?.clientX) ? event.clientX : fallback.x;
    const y = Number.isFinite(event?.clientY) ? event.clientY : fallback.y;
    tip.style.display = 'block';
    const tipBounds = tip.getBoundingClientRect();
    const localX = x - bounds.left;
    const localY = y - bounds.top;
    const left = Math.max(8, Math.min(bounds.width - tipBounds.width - 8, localX + 14));
    const below = localY + 14;
    const top = below + tipBounds.height <= bounds.height - 4
      ? below
      : Math.max(4, localY - tipBounds.height - 14);
    tip.style.left = `${left}px`;
    tip.style.top = `${top}px`;
  };
  const hide = () => {
    tip.style.display = 'none';
    cursor.classList.remove('is-visible');
  };
  const show = (index, event) => {
    currentIndex = clampIndex(index);
    const x = options.xForIndex(currentIndex);
    cursorLine.setAttribute('x1', x);
    cursorLine.setAttribute('x2', x);
    const markers = options.markersForIndex(currentIndex);
    cursorPoints.forEach((point, markerIndex) => {
      const marker = markers[markerIndex];
      point.classList.toggle('is-visible', Boolean(marker));
      if (!marker) return;
      point.setAttribute('cx', marker.x);
      point.setAttribute('cy', marker.y);
    });
    cursor.classList.add('is-visible');
    tip.innerHTML = options.contentForIndex(currentIndex);
    target.setAttribute('aria-valuenow', String(currentIndex + 1));
    target.setAttribute('aria-valuetext', tip.textContent.replace(/\s+/g, ' ').trim());
    place(currentIndex, event);
  };

  target.addEventListener('pointerenter', event => {
    pointerInside = true;
    show(pinnedIndex >= 0 ? pinnedIndex : indexFromPointer(event), event);
  });
  target.addEventListener('pointermove', event => {
    if (pinnedIndex < 0) show(indexFromPointer(event), event);
  });
  target.addEventListener('pointerleave', () => {
    pointerInside = false;
    if (pinnedIndex < 0) hide();
  });
  target.addEventListener('focus', () => show(currentIndex));
  target.addEventListener('blur', () => {
    if (pinnedIndex < 0 && !pointerInside) hide();
  });
  target.addEventListener('click', event => {
    const index = indexFromPointer(event);
    pinnedIndex = pinnedIndex === index ? -1 : index;
    tip.dataset.pinned = pinnedIndex < 0 ? '' : String(pinnedIndex);
    show(index, event);
  });
  target.addEventListener('keydown', event => {
    let nextIndex = currentIndex;
    if (event.key === 'ArrowLeft') nextIndex--;
    else if (event.key === 'ArrowRight') nextIndex++;
    else if (event.key === 'Home') nextIndex = 0;
    else if (event.key === 'End') nextIndex = maxIndex;
    else if (event.key === 'Enter' || event.key === ' ') {
      pinnedIndex = pinnedIndex === currentIndex ? -1 : currentIndex;
      tip.dataset.pinned = pinnedIndex < 0 ? '' : String(pinnedIndex);
      show(currentIndex);
      event.preventDefault();
      return;
    } else if (event.key === 'Escape') {
      pinnedIndex = -1;
      tip.dataset.pinned = '';
      hide();
      return;
    } else return;
    event.preventDefault();
    currentIndex = clampIndex(nextIndex);
    if (pinnedIndex >= 0) {
      pinnedIndex = currentIndex;
      tip.dataset.pinned = String(pinnedIndex);
    }
    show(currentIndex);
  });
}

function escapeHtml(str) {
  if (!str && str !== 0) return '';
  return String(str).replace(/[&<>"']/g, function(c) {
    return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'})[c];
  });
}

function fmtTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  const hh = d.getHours().toString().padStart(2,'0');
  const mm = d.getMinutes().toString().padStart(2,'0');
  const ss = d.getSeconds().toString().padStart(2,'0');
  return hh + ':' + mm + ':' + ss;
}

function fmtDate(iso) {
  const date = new Date(iso);
  if (!iso || !Number.isFinite(date.getTime())) return '—';
  return date.toLocaleDateString(currentLang === 'zh' ? 'zh-CN' : 'en-US', {year: 'numeric', month: 'short', day: 'numeric'});
}

function fmtDuration(ms) {
  if (!ms && ms !== 0) return '—';
  if (ms < 1000) return ms + ' ms';
  return (ms / 1000).toFixed(1) + ' s';
}

function fmtAge(seconds) {
  if (seconds == null || seconds < 0) return '—';
  if (seconds < 60) return seconds + (currentLang === 'zh' ? ' 秒前' : ' seconds ago');
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes + (currentLang === 'zh' ? ' 分钟前' : ' minutes ago');
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours + (currentLang === 'zh' ? ' 小时前' : ' hours ago');
  const days = Math.floor(hours / 24);
  return days + (currentLang === 'zh' ? ' 天前' : ' days ago');
}

/* ── Proxy Config Form ─────────────────────────────────────────── */
let currentProxyConfig = null;

// Map of config field paths to element IDs for loading and saving.
// Each entry: [jsonPath, elementId, type, transform]
// Only the platforms the dashboard offers are bindable. Their inputs exist in
// index.html; a hidden platform has none, so a binding here would name an
// element that is not on the page. site_parity_test.go holds this to the
// registry.
const CONFIG_FIELDS = [
  // Server
  ['host', 'cfg-host', 'string'],
  ['port', 'cfg-port', 'int'],
  ['api_key', 'cfg-global-key', 'string'],
  ['api_keys', 'cfg-global-keys', 'keys'],
  ['hot_reload', 'cfg-hot-reload', 'bool'],

  // OpenCode Go
  ['opencode_go.base_url', 'cfg-go-base-url', 'string'],
  ['opencode_go.anthropic_base_url', 'cfg-go-anthropic-url', 'string'],
  ['opencode_go.responses_base_url', 'cfg-go-responses-url', 'string'],
  ['opencode_go.api_key', 'cfg-go-api-key', 'string'],
  ['opencode_go.api_keys', 'cfg-go-api-keys', 'keys'],
  ['opencode_go.timeout_ms', 'cfg-go-timeout', 'int'],
  ['opencode_go.stream_timeout_ms', 'cfg-go-stream-timeout', 'int'],
  ['opencode_go.streaming_timeout_ms', 'cfg-go-streaming-timeout', 'int'],

  // CommandCode uses independent credentials and complete native API URLs.
  ['commandcode.base_url', 'cfg-commandcode-base-url', 'string'],
  ['commandcode.anthropic_base_url', 'cfg-commandcode-anthropic-url', 'string'],
  ['commandcode.api_key', 'cfg-commandcode-api-key', 'string'],
  ['commandcode.api_keys', 'cfg-commandcode-api-keys', 'keys'],
  ['commandcode.timeout_ms', 'cfg-commandcode-timeout', 'int'],
  ['commandcode.stream_timeout_ms', 'cfg-commandcode-stream-timeout', 'int'],
  ['commandcode.streaming_timeout_ms', 'cfg-commandcode-streaming-timeout', 'int'],
  ['commandcode.zero_data_retention', 'cfg-commandcode-zdr', 'bool'],

  // Logging
  // Routing scope. The option list carries the visible platforms; which of
  // them this deployment can actually use comes from /api/sites, because the
  // credential rule lives in config and must not be restated here.
  ['active_site', 'cfg-active-site', 'string'],

  ['logging.level', 'cfg-log-level', 'string'],
];

// Deep-set a value in an object by dot-separated path.
function deepSet(obj, path, value) {
  const parts = path.split('.');
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    if (!cur[parts[i]] || typeof cur[parts[i]] !== 'object') cur[parts[i]] = {};
    cur = cur[parts[i]];
  }
  cur[parts[parts.length - 1]] = value;
}

// Deep-get a value from an object by dot-separated path.
function deepGet(obj, path) {
  return path.split('.').reduce((o, k) => (o != null ? o[k] : undefined), obj);
}

// Read a field from the form and produce its typed value (or undefined if unchanged).
function readFieldValue(field) {
  const el = document.getElementById(field[1]);
  if (!el) return undefined;
  const raw = el.value !== undefined ? el.value : '';
  if (field[2] === 'bool') {
    const v = el.checked;
    // Compare with current config to detect actual changes
    const current = deepGet(currentProxyConfig, field[0]);
    return v === !!current ? undefined : v;
  }
  if (field[2] === 'int') {
    const current = deepGet(currentProxyConfig, field[0]);
    if (raw.trim() === '' && current == null) return undefined;
    const v = Number(raw);
    if (!Number.isSafeInteger(v) || v < 0) throw new Error(`${field[0]}: ${currentLang === 'zh' ? '请输入非负整数' : 'Enter a non-negative integer'}`);
    return v === current ? undefined : v;
  }
  if (field[2] === 'keys') {
    const keys = raw.split(/[,\n]/).map(key => key.trim()).filter(Boolean);
    const current = deepGet(currentProxyConfig, field[0]) || [];
    return JSON.stringify(keys) === JSON.stringify(current) ? undefined : keys;
  }
  // string
  const v = raw;
  const current = deepGet(currentProxyConfig, field[0]);
  return v === (current || '') ? undefined : v;
}

async function loadProxyConfig() {
  try {
    const r = await fetch('/api/proxy/config');
    if (!r.ok) throw new Error(await r.text());
    currentProxyConfig = await r.json();
    if (!currentProxyConfig) return;

    for (const [path, id, type] of CONFIG_FIELDS) {
      const el = document.getElementById(id);
      if (!el) continue;
      const val = deepGet(currentProxyConfig, path);
      if (type === 'bool') {
        el.checked = !!val;
      } else if (type === 'int') {
        el.value = val != null ? val : '';
      } else if (type === 'keys') {
        el.value = (val || []).join(', ');
      } else {
        el.value = val || '';
      }
    }
    updateConfigChangeCount();
  } catch (e) {
    console.error('Failed to load proxy config:', e);
    showSaveStatus(t('save.unloaded') + ': ' + e.message, 'error');
  }
}

function updateConfigChangeCount() {
  const label = document.getElementById('config-change-count');
  if (!label) return;
  if (!currentProxyConfig) {
    label.textContent = t('save.unloaded');
    return;
  }
  let changed = 0;
  let invalid = false;
  for (const field of CONFIG_FIELDS) {
    try {
      if (readFieldValue(field) !== undefined) changed++;
    } catch (_) {
      invalid = true;
    }
  }
  label.textContent = invalid ? t('setting.invalidChanges')
    : changed ? t('setting.changeCount').replace('{n}', changed) : t('setting.noChanges');
}

function openProviderSettings(provider) {
  if (!PROVIDERS[provider]) return;
  const section = document.querySelector(`[data-settings-provider="${provider}"]`);
  if (!section) return;
  location.hash = 'settings';
  activateTab('settings');
  section.open = true;
  const picker = document.getElementById('settings-provider-jump');
  if (picker) picker.value = provider;
  window.CustomSelect?.syncAll();
  section.scrollIntoView({block: 'start'});
  section.querySelector('summary')?.focus();
}

document.addEventListener('DOMContentLoaded', () => {
  for (const [, id] of CONFIG_FIELDS) {
    const field = document.getElementById(id);
    field?.addEventListener('input', updateConfigChangeCount);
    field?.addEventListener('change', updateConfigChangeCount);
  }
  applySelectableSites();
  applyActiveSite();
  const viewControlIds = new Set(Object.values(VIEW_CONTROLS).flat());
  document.addEventListener('change', event => {
    if (!viewControlIds.has(event.target?.id)) return;
    // applyActiveSite sets these controls from the active site and dispatches
    // change so each tab reloads its data. That is the default being applied,
    // not a choice the reader made, so it must not be written into the link -
    // otherwise the site default pins itself on first open and every later
    // platform switch on the server stops reaching this session.
    if (applyingSiteDefault) return;
    syncViewState();
  });
  document.getElementById('settings-provider-jump')?.addEventListener('change', event => {
    const provider = event.target.value;
    queueMicrotask(() => openProviderSettings(provider));
  });
});

// The dashboard's platform selectors open on the active site. Routing is
// scoped to one platform, so defaulting the views to a mixed "all platforms"
// would show mostly history the deployment no longer produces.
const PLATFORM_SELECTORS = ['overview-provider', 'provider-filter', 'perf-provider', 'analytics-provider', 'quota-provider'];

// applyActiveSite follows a platform switch made anywhere other than this page.
// The active site is read at boot and after a save, so a change made in the
// config file (hot_reload) or in another window never reached an open dashboard:
// every view kept filtering by the platform it rendered on entry. Polling the
// same endpoint closes that, and the value is only applied when it actually
// moved, so an unchanged site causes no work.
const ACTIVE_SITE_POLL_MS = 30000;
let lastActiveSite = null;

// Set while applyActiveSite applies the site default, so the change events it
// dispatches reload each tab without being mistaken for a reader's choice.
let applyingSiteDefault = false;

async function applyActiveSite() {
  let active;
  try {
    active = (await fetchJSON('/api/sites')).active || '';
  } catch (_) {
    return; // leave the selectors as rendered rather than guessing
  }
  lastActiveSite = active;
  if (!active) return;
  // A link naming a platform still in the list wins; the pin is dropped rather
  // than obeyed when the platform is gone, so a stale link cannot hold every
  // view on "all platforms" while routing goes to the active site.
  if (viewPlatformPinned) {
    const known = PLATFORM_SELECTORS.some(id => {
      const select = document.getElementById(id);
      return select && [...select.options].some(o => o.value === viewPlatformPinned && !o.disabled);
    });
    if (known) return;
    viewPlatformPinned = '';
  }
  let moved = false;
  let offered = false;
  applyingSiteDefault = true;
  try {
    for (const id of PLATFORM_SELECTORS) {
      const select = document.getElementById(id);
      if (!select) continue;
      const option = [...select.options].find(o => o.value === active);
      // The dashboard only offers visible platforms, but config accepts any
      // known one, so an active site can be real and unrepresentable here.
      // Saying nothing would leave the views on all platforms while routing
      // goes to one of them - a difference the operator cannot see.
      if (!option || option.disabled) continue;
      offered = true;
      if (select.value === active) continue;
      select.value = active;
      select.dispatchEvent(new Event('change', {bubbles: true}));
      moved = true;
    }
  } finally {
    applyingSiteDefault = false;
  }
  renderActiveSiteNote(offered ? '' : active);
  // The panels moved with the platform, so the link must follow: it would
  // otherwise still name the platform this session is no longer showing.
  if (moved) syncViewState({replace: true});
}

// renderActiveSiteNote shows why the views cannot open on the active platform.
function renderActiveSiteNote(site) {
  const el = document.getElementById('active-site-note');
  if (!el) return;
  el.hidden = !site;
  el.textContent = site
    ? t('setting.activeSiteNotShown').replace('{site}', providerInfo(site)?.name || site)
    : '';
}

function startActiveSitePolling() {
  setInterval(() => { applyActiveSite(); }, ACTIVE_SITE_POLL_MS);
}

// Mark the platforms this deployment can actually route to. The rule lives in
// config - a platform's own keys, plus the global key for the platforms allowed
// to fall back to it - and the server answers it, so this only applies the
// answer. A platform with no credential stays visible but cannot be chosen:
// picking it would route every request into a 401.
async function applySelectableSites() {
  const select = document.getElementById('cfg-active-site');
  if (!select) return;
  let sites;
  try {
    sites = (await fetchJSON('/api/sites')).sites || [];
  } catch (_) {
    return; // leave the selector as rendered rather than guessing
  }
  const selectable = new Map(sites.map(s => [s.id, s.selectable]));
  for (const option of select.options) {
    if (!option.value) continue;
    const ok = selectable.get(option.value) === true;
    option.disabled = !ok;
    option.title = ok ? '' : t('setting.activeSiteUnavailable');
  }
  // A stored value that is no longer selectable must not look chosen.
  if (select.value && selectable.get(select.value) !== true) select.value = '';
}

async function saveProxyConfig() {
  if (!currentProxyConfig) {
    showSaveStatus(t('save.unloaded'), 'error');
    return;
  }

  const saveBtn = document.getElementById('btn-save-cfg');
  saveBtn.disabled = true;
  saveBtn.textContent = t('status.saving');

  try {
  // Build a patch object with only changed fields.
  const patch = {};
  for (const field of CONFIG_FIELDS) {
    const v = readFieldValue(field);
    if (v !== undefined) {
      deepSet(patch, field[0], v);
    }
  }

  // If nothing changed, no-op.
  if (Object.keys(patch).length === 0) {
    showSaveStatus(t('fallback.noChanges'), 'success');
    return;
  }

    const r = await fetch('/api/proxy/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    });

    if (r.ok) {
      showSaveStatus(t('status.saveOk'), 'success');
      // Reload the full config from the server to stay in sync.
      await loadProxyConfig();
      viewPlatformPinned = '';
      // A saved active_site or key change also moves what every other tab is
      // showing: those selectors are set from /api/sites, which until now was
      // only read at page load, so a new platform only took effect after a
      // manual refresh. Re-derive both here so the rest of the dashboard
      // follows the save it just acknowledged.
      await applySelectableSites();
      await applyActiveSite();
    } else {
      const txt = await r.text();
      showSaveStatus(t('status.saveFail') + txt, 'error');
    }
  } catch (e) {
    showSaveStatus(t('status.saveFail') + e.message, 'error');
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = t('btn.save');
  }
}

function showSaveStatus(msg, type) {
  const status = document.getElementById('save-status');
  status.textContent = msg;
  status.className = 'save-status ' + type;
  setTimeout(() => {
    status.textContent = '';
    status.className = 'save-status';
  }, 4000);
}

/* ── Toast notifications ───────────────────────────────────────── */
// Lightweight, self-cleaning toast for transient action feedback (saved,
// synced, proxy started/stopped, errors). Reuses a single element; a second
// call while one is visible replaces it and restarts the timer.
let toastTimer = null;
function toast(message, type) {
  const el = document.getElementById('toast');
  if (!el) return;
  el.textContent = message;
  el.className = 'toast ' + (type || 'info');
  // Force reflow so re-adding the visible class on repeat messages replays
  // the fade-in transition.
  void el.offsetWidth;
  el.classList.add('visible');
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el.classList.remove('visible');
    toastTimer = null;
  }, 3000);
}

function togglePasswordVisibility(id) {
  const input = document.getElementById(id);
  if (input.type === 'password') {
    input.type = 'text';
  } else {
    input.type = 'password';
  }
}

/* ── History Search ────────────────────────────────────────────── */
let historyRefreshTimer = null;

function scheduleHistoryRefresh() {
  syncAdvancedFilters(false);
  historyLoadSeq++;
  historyPage = 1;
  clearHistoryView(true);
  if (historyRefreshTimer) clearTimeout(historyRefreshTimer);
  historyRefreshTimer = setTimeout(() => {
    historyRefreshTimer = null;
    refreshHistory();
  }, 250);
}

['history-search', 'model-filter', 'scenario-filter'].forEach(id => {
  document.getElementById(id)?.addEventListener('input', scheduleHistoryRefresh);
});
['history-start', 'history-end', 'provider-filter', 'status-filter', 'streaming-filter', 'cost-source-filter'].forEach(id => {
  document.getElementById(id)?.addEventListener('change', scheduleHistoryRefresh);
});
document.getElementById('history-reset')?.addEventListener('click', () => resetHistoryFilters());
document.getElementById('history-page-size')?.addEventListener('change', event => {
  const nextSize = Number(event.target.value);
  if (![25, 50, 100].includes(nextSize)) return;
  historySize = nextSize;
  historyPage = 1;
  refreshHistory();
});

/* ── History Sorting ───────────────────────────────────────────── */
let currentSort = { field: 'start_time', dir: 'desc' };

// Scope to the History table: .perf-table has its own .sortable headers with a
// dedicated handler, and an unscoped selector would bind both, so one click
// would sort Performance and silently clear History's sort state.
document.querySelectorAll('.history-table .sortable').forEach(th => {
  th.addEventListener('click', function() {
    const field = this.dataset.sort;
    if (currentSort.field === field) {
      currentSort.dir = currentSort.dir === 'asc' ? 'desc' : 'asc';
    } else {
      currentSort.field = field;
      currentSort.dir = 'desc';
    }
    // Update visual indicators and aria-sort (History headers only).
    document.querySelectorAll('.history-table .sortable').forEach(s => {
      s.classList.remove('asc', 'desc');
      s.setAttribute('aria-sort', 'none');
    });
    this.classList.add(currentSort.dir);
    this.setAttribute('aria-sort', currentSort.dir === 'asc' ? 'ascending' : 'descending');
    historyPage = 1;
    syncViewState();
    refreshHistory();
  });
});

/* ── History Detail Modal ──────────────────────────────────────── */
const modal = document.getElementById('history-modal');
const modalBody = document.getElementById('modal-body');
const modalClose = document.getElementById('modal-close');
let modalReturnFocus = null;

function showHistoryDetail(record) {
  modal.querySelector('.modal-footer')?.remove();
  const tokenValue = value => value != null ? Number(value).toLocaleString() : '—';
  const detailsKnown = historyHasDetails(record);
  const statusLabel = detailsKnown ? (record.success ? t('detail.success') : t('detail.failed')) : t('detail.unknown');
  modalBody.innerHTML = `
    <div class="detail-summary">
      <div>
        <div class="detail-model">${escapeHtml(record.model || '—')}</div>
        <div class="detail-context">
          <span>${escapeHtml(record.provider || '—')}</span>
          <span>${escapeHtml(record.scenario || '—')}</span>
          <span>${fmtDate(record.start_time)} ${fmtTime(record.start_time)}</span>
        </div>
      </div>
      <div class="detail-outcome">
        <strong class="detail-cost">${record.cost_usd != null ? fmtCost(record.cost_usd) : '—'}</strong>
        <span class="detail-status ${detailsKnown ? (record.success ? 'success' : '') : 'unknown'}">${statusLabel}</span>
      </div>
    </div>
    <div class="detail-token-grid">
      <div class="detail-token"><span>${t('detail.inputTokens')}</span><strong>${tokenValue(record.input_tokens)}</strong></div>
      <div class="detail-token"><span>${t('detail.promptTokens')}</span><strong>${tokenValue(record.prompt_tokens)}</strong></div>
      <div class="detail-token"><span>${t('detail.cacheRead')}</span><strong>${tokenValue(record.cache_read_tokens)}</strong></div>
      <div class="detail-token"><span>${t('detail.cacheCreation')}</span><strong>${tokenValue(record.cache_creation_tokens)}</strong></div>
      <div class="detail-token"><span>${t('detail.outputTokens')}</span><strong>${tokenValue(record.output_tokens)}</strong></div>
    </div>
    <div class="detail-metadata">
      <div class="detail-row"><span class="detail-label">${t('detail.requestId')}</span><span class="detail-value">${escapeHtml(record.id || '—')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('history.costSource')}</span><span class="detail-value">${costSourceLabel(record.cost_source)}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.billingWindow')}</span><span class="detail-value">${billingWindowLabel(record)}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.requestType')}</span><span class="detail-value">${detailsKnown ? t(record.streaming ? 'detail.streaming' : 'detail.nonStreaming') : t('detail.unavailable')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.attempt')}</span><span class="detail-value">${detailsKnown ? (record.attempt || 1) : t('detail.unavailable')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.duration')}</span><span class="detail-value">${detailsKnown ? fmtDuration(record.duration_ms) : t('detail.unavailable')}</span></div>
    </div>
    ${record.error_msg ? `<div class="detail-error"><strong>${t('detail.error')}</strong><br>${escapeHtml(record.error_msg)}</div>` : ''}
  `;
  openHistoryModal('detail.title');
}

function openHistoryModal(titleKey) {
  const title = document.getElementById('modal-title');
  title.dataset.i18n = titleKey;
  title.textContent = t(titleKey);
  modalReturnFocus = document.activeElement;
  if (typeof modal?.showModal === 'function' && !modal.open) modal.showModal();
  else modal?.classList.add('visible');
}

function closeHistoryModal() {
  if (modal?.open) modal.close();
  else modal?.classList.remove('visible');
}

modalClose?.addEventListener('click', closeHistoryModal);
modal?.addEventListener('close', function() {
  modal.querySelector('.modal-footer')?.remove();
  const target = modalReturnFocus;
  modalReturnFocus = null;
  if (target && typeof target.focus === 'function') target.focus();
});
modal?.addEventListener('click', function(e) {
  if (e.target === modal) closeHistoryModal();
});

/* ── Command Palette ───────────────────────────────────────────── */
const commandPalette = document.getElementById('command-palette');
const commandInput = document.getElementById('command-input');
let commandPaletteOpen = false;

function openCommandPalette() {
  commandPaletteOpen = true;
  commandPalette.classList.add('visible');
  commandInput.value = '';
  commandInput.focus();
  updateCommandList('');
}

function closeCommandPalette() {
  commandPaletteOpen = false;
  commandPalette.classList.remove('visible');
}

function updateCommandList(query) {
  const items = document.querySelectorAll('.command-item');
  const q = query.toLowerCase();
  let firstVisible = null;
  items.forEach(item => {
    const label = item.querySelector('.command-item-label').textContent.toLowerCase();
    const isVisible = label.includes(q);
    item.classList.toggle('hidden', !isVisible);
    if (isVisible && !firstVisible) firstVisible = item;
  });
  // Update aria-activedescendant to first visible item
  const commandInput = document.getElementById('command-input');
  if (firstVisible) {
    commandInput?.setAttribute('aria-activedescendant', firstVisible.id);
  } else {
    commandInput?.setAttribute('aria-activedescendant', '');
  }
}

commandInput?.addEventListener('input', function(e) {
  updateCommandList(e.target.value);
});

commandInput?.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeCommandPalette();
  } else if (e.key === 'Enter') {
    const selected = document.querySelector('.command-item.selected') || document.querySelector('.command-item:not(.hidden)');
    if (selected) executeCommand(selected.dataset.action);
    closeCommandPalette();
  }
});

document.querySelectorAll('.command-item').forEach(item => {
  item.addEventListener('click', function() {
    executeCommand(this.dataset.action);
    closeCommandPalette();
  });
});

function executeCommand(action) {
  switch (action) {
    case 'start-proxy':
      document.getElementById('toggle-proxy').checked = true;
      toggleProxy(document.getElementById('toggle-proxy'));
      break;
    case 'stop-proxy':
      document.getElementById('toggle-proxy').checked = false;
      toggleProxy(document.getElementById('toggle-proxy'));
      break;
    case 'goto-overview':
      document.querySelector('[data-tab="overview"]').click();
      break;
    case 'goto-history':
      document.querySelector('[data-tab="history"]').click();
      break;
    case 'goto-performance':
      document.querySelector('[data-tab="performance"]').click();
      break;
    case 'goto-fallback':
      document.querySelector('[data-tab="fallback"]').click();
      break;
    case 'goto-analytics':
      document.querySelector('[data-tab="analytics"]').click();
      break;
    case 'goto-quota':
      document.querySelector('[data-tab="quota"]').click();
      break;
    case 'goto-settings':
      document.querySelector('[data-tab="settings"]').click();
      break;
    case 'refresh':
      debouncedRefresh();
      break;
  }
}


commandPalette?.addEventListener('click', function(e) {
  if (e.target === commandPalette) closeCommandPalette();
});

/* ── Keyboard Shortcuts ───────────────────────────────────────── */
document.addEventListener('keydown', function(e) {
  // Command palette: Cmd/Ctrl + K
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else {
      openCommandPalette();
    }
  }
  // Search history: Cmd/Ctrl + F
  if ((e.metaKey || e.ctrlKey) && e.key === 'f') {
    const historyTab = document.getElementById('tab-history');
    if (historyTab.classList.contains('active')) {
      e.preventDefault();
      document.getElementById('history-search')?.focus();
    }
  }
  // Tab shortcuts: Cmd/Ctrl + 1..7
  if ((e.metaKey || e.ctrlKey) && ['1', '2', '3', '4', '5', '6', '7'].includes(e.key)) {
    e.preventDefault();
    const tabs = ['overview', 'history', 'performance', 'fallback', 'analytics', 'quota', 'settings'];
    document.querySelector(`[data-tab="${tabs[parseInt(e.key) - 1]}"]`)?.click();
  }
  // Escape to close modals (use if-else to ensure only one action)
  if (e.key === 'Escape') {
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else if (TestModule.testModal?.classList.contains('visible')) {
      TestModule.close();
    } else if (modal.classList.contains('visible')) {
      closeHistoryModal();
    }
  }
});

/* ── Accordion Sections ────────────────────────────────────────── */
function initAccordions() {
  document.querySelectorAll('.accordion-header').forEach(header => {
    header.addEventListener('click', function() {
      const section = this.closest('.accordion-section');
      const wasExpanded = section.classList.contains('expanded');

      // Collapse all other sections (optional: remove for multi-expand)
      document.querySelectorAll('.accordion-section').forEach(s => {
        s.classList.remove('expanded');
      });

      // Toggle this section
      if (!wasExpanded) {
        section.classList.add('expanded');
      }
    });
  });
}

// Initialize on load
document.addEventListener('DOMContentLoaded', initAccordions);

/* ── Config Backup/Restore ─────────────────────────────────────── */
async function exportConfig() {
  const btn = document.getElementById('btn-export-config');
  btn.disabled = true;
  btn.textContent = t('status.exporting');

  try {
    const response = await fetch('/api/config/export');
    if (!response.ok) {
      throw new Error(await response.text());
    }

    const blob = await response.blob();
    const downloadUrl = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = downloadUrl;
    a.download = 'routatic-proxy-config.json';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(downloadUrl);

    showSaveStatus(t('status.exportOk'), 'success');
  } catch (e) {
    showSaveStatus(t('status.exportFail') + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = t('btn.export');
    applyTranslations();
  }
}

function importConfig() {
  document.getElementById('import-file').click();
}

async function handleConfigImport(file) {
  if (!file) {
    showSaveStatus(t('status.importInvalid'), 'error');
    return;
  }

  const btn = document.getElementById('btn-import-config');
  btn.disabled = true;
  btn.textContent = t('status.importing');

  try {
    const content = await file.text();
    const config = JSON.parse(content);
    const response = await fetch('/api/config/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ config, apply: false })
    });
    if (!response.ok) throw new Error(await response.text());
    const preview = await response.json();

    const previewHtml = `
      <div class="detail-row">
        <span class="detail-label">${t('modal.importConfirm')}</span>
      </div>
      <pre style="max-height: 300px; overflow: auto; padding: 12px; font-size: 12px; white-space: pre-wrap; word-break: break-all;">${escapeHtml(JSON.stringify(preview.config, null, 2))}</pre>
    `;

    modalBody.innerHTML = previewHtml;

    const footerHtml = `
      <div class="modal-footer" style="padding: 12px 16px; display: flex; gap: 8px; justify-content: flex-end;">
        <button class="btn btn-small" id="btn-import-cancel">${t('btn.cancel')}</button>
        <button class="btn btn-small btn-primary" id="btn-import-apply">${t('btn.apply')}</button>
      </div>
    `;

    const existingFooter = modal.querySelector('.modal-footer');
    if (existingFooter) existingFooter.remove();

    modal.querySelector('.modal-content').insertAdjacentHTML('beforeend', footerHtml);

    openHistoryModal('modal.importPreview');

    document.getElementById('btn-import-cancel').onclick = () => {
      closeHistoryModal();
      const footer = modal.querySelector('.modal-footer');
      if (footer) footer.remove();
    };

    const applyBtn = document.getElementById('btn-import-apply');
    applyBtn.onclick = async () => {
      applyBtn.disabled = true;
      try {
        const response = await fetch('/api/config/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ config: config, apply: true })
        });

        if (!response.ok) {
          throw new Error(await response.text());
        }

        closeHistoryModal();
        const footer = modal.querySelector('.modal-footer');
        if (footer) footer.remove();

        showSaveStatus(t('status.importOk'), 'success');
        await loadProxyConfig();
      } catch (e) {
        showSaveStatus(t('status.importFail') + e.message, 'error');
      } finally {
        applyBtn.disabled = false;
      }
    };
  } catch (e) {
    showSaveStatus(t('status.importFail') + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = t('btn.import');
    applyTranslations();
    document.getElementById('import-file').value = '';
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-export-config')?.addEventListener('click', exportConfig);
  document.getElementById('btn-import-config')?.addEventListener('click', importConfig);
  document.getElementById('import-file')?.addEventListener('change', function(e) {
    if (e.target.files && e.target.files[0]) {
      handleConfigImport(e.target.files[0]);
    }
  });
});

/* ── Fallback Chain Editor ─────────────────────────────────────── */
function configModelKey(model) {
  return (model.provider || 'opencode-go').replace(/_/g, '-') + '/' + model.model_id;
}

const FallbackModule = {
  chains: {},
  currentScenario: 'default',
  originalChains: null,
  availableModels: [],

  init() {
    this.loadConfig();
  },

  async loadConfig() {
    try {
      const r = await fetch('/api/proxy/config');
      if (!r.ok) return;
      const config = await r.json();

      // Keep the complete model config, including its protocol and capabilities.
      const modelMap = new Map();
      const candidates = [
        ...Object.values(config.models || {}),
        ...Object.values(config.model_overrides || {}),
        ...Object.values(config.model_family_overrides || {}),
        ...Object.values(config.fallbacks || {}).flat(),
      ];
      for (const model of candidates) {
        if (model.model_id && !modelMap.has(configModelKey(model))) modelMap.set(configModelKey(model), { ...model });
      }
      this.availableModels = [...modelMap.values()];

      // Discover all scenario keys from config.models
      const scenarioKeys = [...new Set([...Object.keys(config.models || {}), ...Object.keys(config.fallbacks || {})])];
      this.chains = {};
      for (const key of scenarioKeys) {
        this.chains[key] = this.parseFallbackChain(config, key);
      }

      // Populate scenario dropdown
      const sel = document.getElementById('fallback-scenario');
      if (sel) {
        sel.innerHTML = scenarioKeys.map(k =>
          `<option value="${escapeHtml(k)}">${escapeHtml(k.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()))}</option>`
        ).join('');
        this.currentScenario = scenarioKeys[0] || 'default';
        sel.value = this.currentScenario;
      }

      this.populateAddSelect();
      this.originalChains = JSON.parse(JSON.stringify(this.chains));
      this.renderChain();
    } catch (e) {
      console.error('Failed to load fallback config:', e);
    }
  },

  parseFallbackChain(config, scenario) {
    if (config.fallbacks && config.fallbacks[scenario]) {
      return config.fallbacks[scenario].map(m => ({...m}));
    }
    return [];
  },

  populateAddSelect() {
    const addSel = document.getElementById('fallback-add-model');
    if (!addSel) return;
    const chain = this.chains[this.currentScenario] || [];
    const available = this.availableModels
      .filter(m => !chain.some(e => configModelKey(e) === configModelKey(m)))
      .sort((a, b) => compareProviderDisplay(a.provider || 'opencode-go', b.provider || 'opencode-go')
        || a.model_id.localeCompare(b.model_id));
    addSel.innerHTML = '<option value="">' + t('fallback.selectModel') + '</option>' +
      available.map(m =>
        `<option value="${escapeHtml(configModelKey(m))}">${escapeHtml(m.model_id)} (${escapeHtml(m.provider || 'opencode-go')})</option>`
      ).join('');
    addSel.disabled = available.length === 0;
  },

  onAddSelectChange() {
    const addSel = document.getElementById('fallback-add-model');
    const modelKey = addSel.value;
    if (!modelKey) return;
    const model = this.availableModels.find(m => configModelKey(m) === modelKey);
    if (model) {
      if (!this.chains[this.currentScenario]) this.chains[this.currentScenario] = [];
      this.chains[this.currentScenario].push({ ...model });
      this.renderChain();
    }
    addSel.value = '';
    this.populateAddSelect();
  },

  renderChain() {
    const list = document.getElementById('fallback-chain');
    const chain = this.chains[this.currentScenario];
    const primary = currentProxyConfig?.models?.[this.currentScenario];
    const primaryEl = document.getElementById('fallback-primary');
    if (primaryEl) primaryEl.textContent = primary ? configModelKey(primary) : '—';
    this.setStatus(t(this.originalChains && JSON.stringify(this.chains) !== JSON.stringify(this.originalChains)
      ? 'fallback.unsaved' : 'fallback.noChanges'));

    if (!chain || chain.length === 0) {
      list.innerHTML = '<li class="empty-state">' + t('fallback.empty') + '</li>';
      list.classList.remove('has-items');
      this.populateAddSelect();
      return;
    }

    list.classList.add('has-items');
    list.innerHTML = chain.map((entry, index) => {
      const modelId = entry.model_id || entry;
      const displayName = modelId;
      const provider = entry.provider || 'opencode-go';
      const identity = `${displayName} (${PROVIDERS[provider.replace(/_/g, '-')]?.name || provider})`;
      const label = key => escapeHtml(t(key).replace('{model}', identity));
      return `
        <li class="fallback-item" draggable="true" data-index="${index}">
          <span class="handle" aria-hidden="true">⋮⋮</span>
          <span class="chain-order" aria-hidden="true">${index + 1}</span>
          <span class="model-name">${escapeHtml(displayName)}</span>
          ${provider ? '<span class="model-meta">' + escapeHtml(provider) + '</span>' : ''}
          <span class="chain-actions">
            <button type="button" class="chain-move" data-direction="up" onclick="FallbackModule.moveModel(${index}, ${index - 1}, 'up')" aria-label="${label('fallback.moveUp')}" title="${label('fallback.moveUp')}" ${index === 0 ? 'disabled' : ''}>↑</button>
            <button type="button" class="chain-move" data-direction="down" onclick="FallbackModule.moveModel(${index}, ${index + 1}, 'down')" aria-label="${label('fallback.moveDown')}" title="${label('fallback.moveDown')}" ${index === chain.length - 1 ? 'disabled' : ''}>↓</button>
            <button type="button" class="chain-remove" onclick="FallbackModule.removeModel(${index})" aria-label="${label('fallback.remove')}" title="${label('fallback.remove')}">×</button>
          </span>
        </li>
      `;
    }).join('');

    this.populateAddSelect();
    this.setupDragDrop();
  },

  setupDragDrop() {
    const items = document.querySelectorAll('.fallback-item');

    items.forEach(item => {
      item.addEventListener('dragstart', (e) => this.onDragStart(e));
      item.addEventListener('dragover', (e) => this.onDragOver(e));
      item.addEventListener('dragleave', (e) => this.onDragLeave(e));
      item.addEventListener('drop', (e) => this.onDrop(e));
      item.addEventListener('dragend', (e) => this.onDragEnd(e));
    });
  },

  onDragStart(e) {
    e.target.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', e.target.dataset.index);
  },

  onDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const dragging = document.querySelector('.fallback-item.dragging');
    if (dragging !== e.currentTarget) {
      e.currentTarget.classList.add('drag-over');
    }
  },

  onDragLeave(e) {
    e.currentTarget.classList.remove('drag-over');
  },

  onDrop(e) {
    e.preventDefault();
    const source = e.dataTransfer.getData('text/plain');
    const fromIndex = source.trim() ? Number(source) : NaN;
    const toIndex = Number(e.currentTarget.dataset.index);

    e.currentTarget.classList.remove('drag-over');

    this.moveModel(fromIndex, toIndex);
  },

  moveModel(fromIndex, toIndex, direction) {
    const chain = this.chains[this.currentScenario];
    if (!chain || !Number.isInteger(fromIndex) || !Number.isInteger(toIndex)
      || fromIndex < 0 || toIndex < 0 || fromIndex >= chain.length || toIndex >= chain.length || fromIndex === toIndex) return;
    const [entry] = chain.splice(fromIndex, 1);
    chain.splice(toIndex, 0, entry);
    this.renderChain();
    if (direction) {
      const row = document.querySelector(`.fallback-item[data-index="${toIndex}"]`);
      const moved = row?.querySelector(`[data-direction="${direction}"]`);
      (moved && !moved.disabled ? moved : row?.querySelector('.chain-move:not(:disabled), .chain-remove'))?.focus();
    }
  },

  onDragEnd(e) {
    e.target.classList.remove('dragging');
    document.querySelectorAll('.fallback-item').forEach(item => {
      item.classList.remove('drag-over');
    });
  },

  onScenarioChange() {
    const select = document.getElementById('fallback-scenario');
    this.currentScenario = select.value;
    this.renderChain();
    this.populateAddSelect();
    document.getElementById('fallback-preview').style.display = 'none';
  },

  removeModel(index) {
    const chain = this.chains[this.currentScenario];
    if (chain) {
      chain.splice(index, 1);
      this.renderChain();
      const next = document.querySelector(`.fallback-item[data-index="${Math.min(index, chain.length - 1)}"] .chain-remove`);
      const picker = document.getElementById('fallback-add-model');
      (next || window.CustomSelect?.instances.get(picker)?.button || picker)?.focus();
    }
  },

  setStatus(message, type = '') {
    const status = document.getElementById('fallback-status');
    if (!status) return;
    status.textContent = message;
    status.className = 'page-meta ' + (type === 'error' ? 'is-error' : '');
  },

  preview() {
    const previewEl = document.getElementById('fallback-preview');
    const contentEl = document.getElementById('fallback-preview-content');
    const chain = this.chains[this.currentScenario];

    if (!chain || chain.length === 0) {
      contentEl.innerHTML = '<div class="empty-state">' + t('fallback.empty') + '</div>';
    } else {
      contentEl.innerHTML = '<div class="fallback-preview-chain">' +
        chain.map((entry, i) => {
          const modelId = entry.model_id || entry;
          const displayName = `${modelId} (${entry.provider || 'opencode-go'})`;
          return `
            <span class="fallback-preview-model ${i === 0 ? 'primary' : ''}">${escapeHtml(displayName)}</span>
            ${i < chain.length - 1 ? '<span class="fallback-preview-arrow">→</span>' : ''}
          `;
        }).join('') +
        '</div>';
    }

    previewEl.style.display = 'block';
  },

  async save() {
    const hasChanges = this.originalChains && (
      JSON.stringify(this.chains) !== JSON.stringify(this.originalChains)
    );

    if (!hasChanges) {
      this.setStatus(t('fallback.noChanges'));
      return;
    }

    const saveBtn = document.querySelector('.fallback-actions .btn-primary');
    if (saveBtn) {
      saveBtn.disabled = true;
      saveBtn.textContent = t('fallback.saving');
    }

    try {
      const patch = { fallbacks: { ...this.chains } };
      const r = await fetch('/api/proxy/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch)
      });

      if (r.ok) {
        this.setStatus(t('fallback.saved'), 'success');
        this.originalChains = JSON.parse(JSON.stringify(this.chains));
        if (currentProxyConfig) currentProxyConfig.fallbacks = JSON.parse(JSON.stringify(this.chains));
      } else {
        const txt = await r.text();
        this.setStatus(t('fallback.saveFailed') + ': ' + txt, 'error');
      }
    } catch (e) {
      this.setStatus(t('fallback.saveFailed') + ': ' + e.message, 'error');
    } finally {
      if (saveBtn) {
        saveBtn.disabled = false;
        saveBtn.textContent = t('fallback.save');
      }
    }
  }
};

document.addEventListener('DOMContentLoaded', () => {
  FallbackModule.init();
});

/* ── Boot ──────────────────────────────────────────────────────── */
loadProxyConfig();
startPolling();
// Activate the tab from the URL hash (deep-link / refresh resume). Defaults
// to overview when no hash is present. Deferred to a microtask so any
// const modules (e.g. AnalyticsModule) defined later in this script have
// been initialized — otherwise accessing them here hits a TDZ error.
queueMicrotask(() => {
  // The defaults are read before the URL is applied, so a link cannot become
  // the baseline its own parameters are measured against.
  captureViewDefaults();
  const {name, params} = parseViewHash(location.hash);
  viewPlatformPinned = params.get('platform') || '';
  applyViewState(params);
  activateTab(name);
});

// History pagination controls.
const btnHistPrev = document.getElementById('btn-history-prev');
const btnHistNext = document.getElementById('btn-history-next');
if (btnHistPrev) btnHistPrev.addEventListener('click', () => historyGoToPage(historyPage - 1));
if (btnHistNext) btnHistNext.addEventListener('click', () => historyGoToPage(historyPage + 1));

const TestModule = {
  testModal: null,
  testPrompt: null,
  testResponse: null,
  testModelSelect: null,
  testLatency: null,
  testTokens: null,
  testSendBtn: null,
  testCopyBtn: null,
  testModalClose: null,
  testHistoryHint: null,

  STORAGE_KEY: 'routatic-test-prompt-history',
  MAX_HISTORY: 5,

  init() {
    this.testModal = document.getElementById('test-modal');
    this.testPrompt = document.getElementById('test-prompt');
    this.testResponse = document.getElementById('test-response');
    this.testModelSelect = document.getElementById('test-model');
    this.testLatency = document.getElementById('test-latency');
    this.testTokens = document.getElementById('test-tokens');
    this.testSendBtn = document.getElementById('btn-test-send');
    this.testCopyBtn = document.getElementById('btn-test-copy');
    this.testModalClose = document.getElementById('test-modal-close');
    this.testHistoryHint = document.getElementById('test-history-hint');

    document.getElementById('btn-test-model')?.addEventListener('click', () => this.open());
    this.testModalClose?.addEventListener('click', () => this.close());
    this.testModal?.addEventListener('click', (e) => {
      if (e.target === this.testModal) this.close();
    });
    this.testSendBtn?.addEventListener('click', () => this.sendTest());
    this.testCopyBtn?.addEventListener('click', () => this.copyResponse());
    this.testPrompt?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        this.sendTest();
      }
    });
    this.loadHistory();
  },

  open() {
    this.populateModels();
    this.testModal?.classList.add('visible');
    if (this.testPrompt) {
      this.testPrompt.value = '';
      this.testPrompt.focus();
    }
    this.resetResponse();
  },

  close() {
    this.testModal?.classList.remove('visible');
  },

  async populateModels() {
    if (!this.testModelSelect) return;
    this.testModelSelect.innerHTML = '<option value="">Select a model...</option>';

    try {
      const r = await fetch('/api/proxy/config');
      if (!r.ok) return;
      const data = await r.json();
      // Only collect model_overrides and model_family_overrides — the
      // top-level "models" keys are routing scenarios (fast, default,
      // long_context, etc.), not real model IDs.
      const targets = {...data.model_family_overrides, ...data.model_overrides};
      Object.keys(targets).sort((a, b) => compareProviderDisplay(targets[a].provider || 'opencode-go', targets[b].provider || 'opencode-go')
        || a.localeCompare(b)).forEach(id => {
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = `${id} · ${configModelKey(targets[id])}`;
        this.testModelSelect.appendChild(opt);
      });
    } catch (e) {}
  },

  resetResponse() {
    if (this.testResponse) this.testResponse.innerHTML = '';
    if (this.testLatency) this.testLatency.textContent = '—';
    if (this.testTokens) this.testTokens.textContent = '—';
  },

  async sendTest() {
    if (!this.testPrompt || !this.testModelSelect || !this.testResponse) return;

    const model = this.testModelSelect.value;
    const prompt = this.testPrompt.value.trim();
    if (!model) {
      this.resetResponse();
      if (this.testResponse) this.testResponse.innerHTML = `<div class="error">${t('test.noModel')}</div>`;
      return;
    }
    if (!prompt) {
      this.resetResponse();
      if (this.testResponse) this.testResponse.innerHTML = `<div class="error">${t('test.noPrompt')}</div>`;
      return;
    }

    this.saveToHistory(prompt);
    this.testSendBtn.disabled = true;
    this.testSendBtn.textContent = t('test.sending');
    this.resetResponse();

    const start = performance.now();
    try {
      const r = await fetch('/api/test/send', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: model,
          max_tokens: 1024,
          messages: [{ role: 'user', content: prompt }]
        })
      });

      const latency = Math.round(performance.now() - start);
      if (this.testLatency) this.testLatency.textContent = latency + ' ms';

      if (!r.ok) {
        this.testResponse.innerHTML = '';
        const pre = document.createElement('pre');
        pre.textContent = t('test.error') + r.status + ': ' + (await r.text());
        this.testResponse.appendChild(pre);
        return;
      }
      const text = await r.text();
      let content = text;
      try {
        const j = JSON.parse(text);
        if (j.content && Array.isArray(j.content)) {
          content = j.content.map(c => c.text || '').join('\n');
        } else if (j.error) {
          content = 'Error: ' + (j.error.message || JSON.stringify(j.error));
        }
      } catch (_) {}

      const pre = document.createElement('pre');
      pre.textContent = content;
      this.testResponse.innerHTML = '';
      this.testResponse.appendChild(pre);

      const usage = this.extractUsage(text);
      if (usage && this.testTokens) {
        this.testTokens.textContent = `${usage.input || 0} in / ${usage.output || 0} out`;
      }
    } catch (e) {
      const pre = document.createElement('pre');
      pre.textContent = t('test.error') + e.message;
      this.testResponse.innerHTML = '';
      this.testResponse.appendChild(pre);
    } finally {
      this.testSendBtn.disabled = false;
      this.testSendBtn.textContent = t('test.send');
    }
  },

  extractUsage(text) {
    try {
      const j = JSON.parse(text);
      if (j.usage) return { input: j.usage.input_tokens, output: j.usage.output_tokens };
    } catch (_) {}
    const m = text.match(/"input_tokens":\s*(\d+).*?"output_tokens":\s*(\d+)/s);
    if (m) return { input: parseInt(m[1]), output: parseInt(m[2]) };
    return null;
  },

  loadHistory() {
    try {
      const history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
      if (history.length > 0 && this.testHistoryHint) {
        this.testHistoryHint.innerHTML = history.slice(0, this.MAX_HISTORY)
          .map(p => `<span title="${escapeHtml(p)}">${escapeHtml(p.substring(0, 20))}${p.length > 20 ? '...' : ''}</span>`)
          .join('');
        this.testHistoryHint.querySelectorAll('span').forEach((el, i) => {
          el.addEventListener('click', () => {
            const history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
            if (history[i]) {
              this.testPrompt.value = history[i];
              this.testPrompt.focus();
            }
          });
        });
      }
    } catch (e) {}
  },

  saveToHistory(prompt) {
    try {
      let history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
      history = [prompt, ...history.filter(p => p !== prompt)].slice(0, this.MAX_HISTORY);
      localStorage.setItem(this.STORAGE_KEY, JSON.stringify(history));
      this.loadHistory();
    } catch (e) {}
  },

  async copyResponse() {
    const pre = this.testResponse.querySelector('pre');
    if (!pre || !pre.textContent) return;

    try {
      await navigator.clipboard.writeText(pre.textContent);
      const originalText = this.testCopyBtn.innerHTML;
      this.testCopyBtn.innerHTML = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align: middle; margin-right: 4px;"><polyline points="20 6 9 17 4 12"></polyline></svg>${t('test.copied')}`;
      this.testCopyBtn.classList.add('copied');
      setTimeout(() => {
        this.testCopyBtn.innerHTML = originalText;
        this.testCopyBtn.classList.remove('copied');
        this.testCopyBtn.classList.remove('copied');
      }, 2000);
    } catch (e) {}
  }
};

document.addEventListener('DOMContentLoaded', () => TestModule.init());

/* ── Analytics Tab (minimal, vanilla JS + SVG/CSS) ─────────────── */
const AnalyticsModule = {
  loadSeq: 0,
  ready: false,
  query: '',
  breakdownMetric: 'tokens',
  granularity: 'day',
  currentView: null,
  currentTrend: [],
  seriesVisibility: {},

  init() {
    const refreshBtn = document.getElementById('btn-refresh-analytics');
    if (refreshBtn) refreshBtn.addEventListener('click', () => this.load(true));
    document.getElementById('analytics-provider')?.addEventListener('change', () => this.load(true));
    document.getElementById('overview-provider')?.addEventListener('change', refreshOverviewUsage);
    document.getElementById('btn-refresh-overview')?.addEventListener('click', refreshOverviewUsage);
    document.querySelectorAll('#analytics-breakdown-metric button').forEach(button => {
      button.addEventListener('click', () => {
        this.breakdownMetric = button.dataset.metric;
        button.parentElement.querySelectorAll('button').forEach(item => item.classList.toggle('active', item === button));
        if (this.currentView) this.renderDistributions(this.currentView);
      });
    });
    document.querySelectorAll('#analytics-granularity button').forEach(button => {
      button.addEventListener('click', () => {
        this.granularity = button.dataset.granularity;
        button.parentElement.querySelectorAll('button').forEach(item => item.classList.toggle('active', item === button));
        this.load(true);
      });
    });
    document.querySelectorAll('#overview-range button').forEach(button => {
      button.addEventListener('click', () => {
        overviewDays = Number(button.dataset.days || 7);
        button.parentElement.querySelectorAll('button').forEach(item => item.classList.toggle('active', item === button));
        refreshOverviewUsage();
      });
    });
    document.querySelectorAll('#overview-breakdown-metric button').forEach(button => {
      button.addEventListener('click', () => {
        overviewBreakdownMetric = button.dataset.metric;
        button.parentElement.querySelectorAll('button').forEach(item => item.classList.toggle('active', item === button));
        if (lastOverviewView) renderOverviewUsage(lastOverviewView.data, lastOverviewView.trend, lastOverviewView.latency);
      });
    });
    this.initDateRange();
  },

  initDateRange() {
    const end = new Date();
    const start = new Date(end);
    start.setUTCDate(start.getUTCDate() - 6);
    document.getElementById('analytics-start').value = utcDateInputValue(start);
    document.getElementById('analytics-end').value = utcDateInputValue(end);
    this.syncDateRange();
    document.getElementById('analytics-date-trigger')?.addEventListener('click', () => this.toggleDateRange());
    document.getElementById('analytics-date-cancel')?.addEventListener('click', () => this.closeDateRange());
    document.getElementById('analytics-date-apply')?.addEventListener('click', () => this.applyDateRange());
    document.querySelectorAll('#analytics-date-popover [data-days]').forEach(button => {
      button.addEventListener('click', () => this.presetDateRange(Number(button.dataset.days || 7)));
    });
    document.addEventListener('pointerdown', event => {
      if (!event.target.closest('#analytics-date-range')) this.closeDateRange();
    });
  },

  toggleDateRange() {
    const popover = document.getElementById('analytics-date-popover');
    if (!popover) return;
    if (popover.hidden) {
      this.syncDateRange();
      popover.hidden = false;
      document.getElementById('analytics-date-trigger')?.setAttribute('aria-expanded', 'true');
      document.getElementById('analytics-start-display')?.focus();
    } else {
      this.closeDateRange();
    }
  },

  closeDateRange() {
    const popover = document.getElementById('analytics-date-popover');
    if (popover) popover.hidden = true;
    document.getElementById('analytics-date-trigger')?.setAttribute('aria-expanded', 'false');
  },

  syncDateRange() {
    const start = document.getElementById('analytics-start')?.value || '';
    const end = document.getElementById('analytics-end')?.value || '';
    const startDisplay = document.getElementById('analytics-start-display');
    const endDisplay = document.getElementById('analytics-end-display');
    if (startDisplay) startDisplay.value = start;
    if (endDisplay) endDisplay.value = end;
    const label = document.getElementById('analytics-date-label');
    if (label) label.textContent = start && end ? `${start} → ${end} (UTC)` : t('filter.dateRange');
  },

  presetDateRange(days) {
    const end = new Date();
    const start = new Date(end);
    start.setUTCDate(start.getUTCDate() - Math.max(0, days - 1));
    document.getElementById('analytics-start-display').value = utcDateInputValue(start);
    document.getElementById('analytics-end-display').value = utcDateInputValue(end);
  },

  applyDateRange() {
    const start = document.getElementById('analytics-start-display');
    const end = document.getElementById('analytics-end-display');
    const valid = value => {
      if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
      const date = new Date(`${value}T00:00:00Z`);
      return !Number.isNaN(date.getTime()) && utcDateInputValue(date) === value;
    };
    start.classList.toggle('invalid', !valid(start.value));
    end.classList.toggle('invalid', !valid(end.value));
    if (!valid(start.value) || !valid(end.value) || start.value > end.value) return;
    const span = (new Date(`${end.value}T00:00:00Z`) - new Date(`${start.value}T00:00:00Z`)) / 86400000 + 1;
    if (span > 92) {
      end.classList.add('invalid');
      return;
    }
    document.getElementById('analytics-start').value = start.value;
    document.getElementById('analytics-end').value = end.value;
    this.syncDateRange();
    this.closeDateRange();
    this.load(true);
  },

  queryParams() {
    const start = document.getElementById('analytics-start')?.value;
    const end = document.getElementById('analytics-end')?.value;
    const from = new Date(`${start}T00:00:00Z`);
    const to = new Date(`${end}T00:00:00Z`);
    to.setUTCDate(to.getUTCDate() + 1);
    const params = new URLSearchParams({
      from: from.toISOString(),
      to: to.toISOString(),
      granularity: this.granularity,
    });
    const provider = document.getElementById('analytics-provider')?.value;
    if (provider) params.set('provider', provider);
    return params;
  },

  async load(force) {
    const seq = ++this.loadSeq;
    const params = this.queryParams();
    const genEl = document.getElementById('analytics-generated');
    if (!this.ready || this.query !== params.toString()) {
      this.query = params.toString();
      this.clearView(true);
    }
    const errorEl = document.getElementById('analytics-error');
    if (errorEl) errorEl.hidden = true;

    try {
      const [summary, trend] = await Promise.all([
        fetchJSON(`/api/analytics/summary?${params}`),
        fetchJSON(`/api/analytics/tokens/trend?${params}`)
      ]);
      if (seq !== this.loadSeq) return;
      if (!summary?.summary || !trend || (trend.trend !== null && !Array.isArray(trend.trend))) {
        throw new Error(t('data.invalid'));
      }
      this.currentView = summary;
      this.currentTrend = this.fillTrend(trend.trend || []);
      document.getElementById('analytics-empty').hidden = summary.summary.total_requests !== 0;
      document.getElementById('analytics-insights').hidden = summary.summary.total_requests === 0;
      this.renderKPIs(summary);
      this.renderDistributions(summary);
      this.renderRequestTrend(this.currentTrend, 'analytics-request-trend');
      this.renderTokenLines(this.currentTrend, 'analytics-token-trend');
      this.renderPeriodTable(this.currentTrend);
      this.renderModelTable(summary.models || []);
      this.renderRetainedRange(summary.summary || {});
      this.ready = true;
      if (genEl) {
        const ts = summary.generated_at ? new Date(summary.generated_at) : new Date();
        genEl.textContent = '· ' + ts.toLocaleDateString(undefined, {month:'short', day:'numeric'});
      }
    } catch (e) {
      if (seq !== this.loadSeq) return;
      console.error('Analytics error:', e);
      this.clearView();
      if (errorEl) {
        errorEl.textContent = t('data.loadFail') + e.message;
        errorEl.hidden = false;
      }
    }
  },

  clearView(loading = false) {
    this.ready = false;
    this.currentView = null;
    this.currentTrend = [];
    document.getElementById('analytics-empty').hidden = true;
    document.getElementById('analytics-insights').hidden = false;
    ['kpi-requests','kpi-tokens','kpi-cost','kpi-input','kpi-cache-rate','kpi-output','kpi-cache-read','kpi-cache-write'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.textContent = loading ? '…' : '—';
    });
    ['analytics-generated','analytics-period-count','analytics-retained-range','kpi-tokens-note','kpi-cost-note','kpi-cache-rate-note'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.textContent = '';
    });
    const message = t(loading ? 'data.loading' : 'detail.unavailable');
    ['provider-distribution', 'analytics-request-trend', 'analytics-token-trend'].forEach(id => {
      document.getElementById(id).innerHTML = `<div class="empty-state">${message}</div>`;
    });
    document.getElementById('analytics-period-tbody').innerHTML = `<tr><td colspan="9" class="empty-state">${message}</td></tr>`;
    document.getElementById('analytics-model-tbody').innerHTML = `<tr><td colspan="5" class="empty-state">${message}</td></tr>`;
  },

  renderKPIs(data) {
    const s = data.summary || {};
    const fmt = (n) => n != null ? Number(n).toLocaleString() : '—';
    const tokenValue = n => n != null ? fmtTok(n) : '—';
    document.getElementById('kpi-requests').textContent = fmt(s.total_requests);
    // Total tokens = input (non-cached new) + output + cache (read+creation).
    // This matches the billing structure and avoids underreporting when a large
    // fraction of input is served from cache at lower cost.
    document.getElementById('kpi-tokens').textContent = hasUsageTokens(s) ? fmtTok(totalUsageTokens(s)) : '—';
    document.getElementById('kpi-tokens-note').textContent = t('analytics.knownRecords').replace('{n}', Number(s.known_requests || 0).toLocaleString());
    document.getElementById('kpi-input').textContent = tokenValue(s.input_tokens);
    document.getElementById('kpi-output').textContent = tokenValue(s.output_tokens);

    // Cache hit rate is an input-side ratio: the denominator is everything that
    // arrived as prompt (fresh input + cache read + cache creation). Dividing by
    // total tokens instead would fold output in and drift with response length.
    const promptTok = (s.input_tokens||0) + (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    document.getElementById('kpi-cost').textContent = fmtAggregateCost(s);
    document.getElementById('kpi-cost-note').textContent = costCoverageNote(s);
    document.getElementById('kpi-cache-read').textContent = tokenValue(s.cache_read_tokens);
    document.getElementById('kpi-cache-rate').textContent = hasUsageTokens(s) && promptTok > 0
      ? `${((s.cache_read_tokens || 0) / promptTok * 100).toFixed(1)}%`
      : '—';
    document.getElementById('kpi-cache-rate-note').textContent = s.cache_read_tokens != null ? `${fmtTok(s.cache_read_tokens)} ${currentLang === 'zh' ? '读取' : 'read'}` : '';
    document.getElementById('kpi-cache-write').textContent = tokenValue(s.cache_creation_tokens);
  },

  renderDistributions(summary) {
    const withTotal = (items) => (items || []).map(it => ({
      ...it,
      total_tokens: (it.input_tokens||0) + (it.output_tokens||0)
        + (it.cache_read_tokens||0) + (it.cache_creation_tokens||0),
    }));
    const valueKey = this.breakdownMetric === 'cost' ? 'cost_usd'
      : this.breakdownMetric === 'requests' ? 'requests' : 'total_tokens';
    this.renderDistribution('provider-distribution', withTotal(summary.providers), valueKey, 'provider');
  },

  renderDistribution(containerId, items, valueKey, dimension) {
    const root = document.getElementById(containerId);
    if (!root) return;
    const normalized = (items || []).map(item => ({
      ...item,
      total_tokens: item.total_tokens ?? totalUsageTokens(item),
      cost_usd: item.cost_usd ?? item.est_cost_usd ?? 0,
    })).sort((a, b) => dimension === 'provider'
      ? compareProviderDisplay(a.provider, b.provider)
      : Number(b[valueKey] || 0) - Number(a[valueKey] || 0));
    root.classList.toggle('is-single', normalized.length === 1);
    if (!normalized.length) {
      root.innerHTML = `<div class="empty-state">${t('analytics.noData')}</div>`;
      return;
    }
    const max = Math.max(...normalized.map(item => Number(item[valueKey] || 0))) || 1;
    const total = normalized.reduce((sum, item) => sum + Number(item[valueKey] || 0), 0) || 1;
    const incompleteCost = valueKey === 'cost_usd' && normalized.some(item => Number(item.unknown_cost_requests || 0) > 0);
    const formatValue = value => valueKey === 'cost_usd' ? fmtCost(value)
      : valueKey === 'requests' ? Number(value || 0).toLocaleString() : fmtTok(value);
    const visible = dimension === 'provider' ? normalized : normalized.slice(0, 12);
    root.innerHTML = visible.map(item => {
      const rawLabel = dimension === 'model' ? item.model : item.provider;
      const name = !rawLabel || rawLabel === 'unknown' ? t('detail.unknown') : rawLabel;
      const label = dimension === 'provider' ? providerLabel(rawLabel)
        : item.provider ? `${name} (${providerLabel(item.provider)})` : name;
      const value = Number(item[valueKey] || 0);
      const share = value / total * 100;
      const meta = `${Number(item.requests || 0).toLocaleString()} ${t('analytics.requests')} · ${fmtTok(item.total_tokens)} Token · ${fmtAggregateCost(item)}`;
      return `<div class="analytics-distribution-row" data-provider="${this.escapeHtml(item.provider || '')}">
        <div class="analytics-distribution-label"><span title="${this.escapeHtml(label)}">${this.escapeHtml(label)}</span><strong>${valueKey === 'cost_usd' ? fmtAggregateCost(item) : formatValue(value)}</strong></div>
        <div class="analytics-distribution-track"><span style="width:${Math.max(value > 0 ? 2 : 0, value / max * 100).toFixed(1)}%;--distribution-color:${providerColor(item.provider)}"></span></div>
        <small title="${this.escapeHtml(costCoverageNote(item))}">${meta}${incompleteCost ? '' : ` · ${share.toFixed(1)}%`}</small>
      </div>`;
    }).join('');
  },

  fillTrend(points) {
    const byKey = new Map((points || []).map(point => [point.date, point]));
    const startValue = document.getElementById('analytics-start')?.value;
    const endValue = document.getElementById('analytics-end')?.value;
    if (!startValue || !endValue) return points || [];
    const rows = [];
    const blank = date => ({date, requests:0, known_requests:0, error_requests:0, input_tokens:0, output_tokens:0, cache_read_tokens:0, cache_creation_tokens:0, cost_usd:0});
    if (this.granularity === 'hour') {
      const cursor = new Date(`${startValue}T00:00:00Z`);
      const end = new Date(`${endValue}T00:00:00Z`);
      end.setUTCDate(end.getUTCDate() + 1);
      while (cursor < end) {
        const key = cursor.toISOString().slice(0, 13) + ':00:00Z';
        rows.push(byKey.get(key) || blank(key));
        cursor.setUTCHours(cursor.getUTCHours() + 1);
      }
      return rows;
    }
    const cursor = new Date(`${startValue}T00:00:00Z`);
    const end = new Date(`${endValue}T00:00:00Z`);
    while (cursor <= end) {
      const key = utcDateInputValue(cursor);
      rows.push(byKey.get(key) || blank(key));
      cursor.setUTCDate(cursor.getUTCDate() + 1);
    }
    return rows;
  },

  trendLabel(value) {
    const date = String(value || '');
    if (date.includes('T')) {
      return new Date(date).toLocaleString(undefined, {month:'2-digit', day:'2-digit', hour:'2-digit', timeZone:'UTC'}) + ' UTC';
    }
    return date.slice(5);
  },

  chartGrid(points, width, height, inset, max, formatValue) {
    const plotW = width - inset.left - inset.right;
    const plotH = height - inset.top - inset.bottom;
    const y = value => inset.top + (1 - Number(value || 0) / Math.max(1, max)) * plotH;
    const x = index => points.length <= 1 ? inset.left + plotW / 2 : inset.left + index / (points.length - 1) * plotW;
    let markup = '';
    for (let step = 0; step <= 4; step++) {
      const value = max * step / 4;
      const yy = y(value);
      markup += `<line class="usage-grid-line" x1="${inset.left}" x2="${width-inset.right}" y1="${yy}" y2="${yy}"></line>`;
      markup += `<text class="usage-axis-label" x="${inset.left-8}" y="${yy+3}" text-anchor="end">${this.escapeHtml(formatValue(value))}</text>`;
    }
    const stride = Math.max(1, Math.ceil(points.length / 5));
    points.forEach((point, index) => {
      if (index % stride !== 0 && index !== points.length - 1) return;
      markup += `<text class="usage-axis-label" x="${x(index)}" y="${height-7}" text-anchor="middle">${this.escapeHtml(this.trendLabel(point.date))}</text>`;
    });
    return {markup, x, y, plotW, plotH};
  },

  visibleChartSeries(containerId, series) {
    const state = this.seriesVisibility[containerId] || (this.seriesVisibility[containerId] = {});
    series.forEach(item => {
      if (typeof state[item.key] !== 'boolean') state[item.key] = true;
    });
    return series.filter(item => state[item.key]);
  },

  chartLegend(series, visible) {
    const visibleKeys = new Set(visible.map(item => item.key));
    return series.map(item => `<button type="button" class="usage-legend-toggle ${item.className}" data-series="${item.key}" aria-pressed="${visibleKeys.has(item.key)}"><span>${item.label}</span></button>`).join('');
  },

  bindChartLegend(root, containerId, series, render) {
    root?.querySelectorAll('.usage-legend-toggle').forEach(button => {
      button.addEventListener('click', () => {
        const visible = this.visibleChartSeries(containerId, series);
        const key = button.dataset.series;
        if (visible.length === 1 && visible[0].key === key) return;
        this.seriesVisibility[containerId][key] = !this.seriesVisibility[containerId][key];
        render();
      });
    });
  },

  renderRequestTrend(points, containerId) {
    const root = document.getElementById(containerId);
    if (!root) return;
    if (!points.length || points.every(point => Number(point.requests || 0) === 0 && Number(point.error_requests || 0) === 0)) {
      root.innerHTML = `<div class="empty-state">${t('analytics.noTrend')}</div>`;
      return;
    }
    const series = [
      {key:'requests',label:t('analytics.requests'),className:'is-requests',color:'#818cf8'},
      {key:'error_requests',label:t('analytics.knownErrors'),className:'is-errors',color:'#fb7185'},
    ];
    const visible = this.visibleChartSeries(containerId, series);
    const width=720, height=236, inset={top:16,right:18,bottom:32,left:52};
    const max = Math.max(1, ...visible.flatMap(item => points.map(point => Number(point[item.key] || 0))));
    const chart = this.chartGrid(points, width, height, inset, max, value => fmtTok(Math.round(value)));
    const lines = visible.map(item => `<polyline class="usage-chart-line ${item.className}" points="${points.map((point,index) => `${chart.x(index)},${chart.y(point[item.key])}`).join(' ')}"></polyline>`).join('');
    const dots = visible.map(item => points.map((point,index) => item.key === 'requests' || Number(point[item.key] || 0) > 0 ? `<circle class="usage-chart-point ${item.className}" cx="${chart.x(index)}" cy="${chart.y(point[item.key])}" r="${item.key === 'requests' ? 3 : 2.6}"></circle>` : '').join('')).join('');
    root.innerHTML = `<div class="usage-chart-stage"><div class="usage-chart-legend">${this.chartLegend(series, visible)}</div><svg viewBox="0 0 ${width} ${height}" role="img">${chart.markup}${lines}${dots}<g class="usage-chart-cursor" aria-hidden="true"><line class="usage-chart-crosshair" y1="${inset.top}" y2="${inset.top+chart.plotH}"></line>${visible.map(item=>`<circle class="usage-chart-cursor-point ${item.className}" r="4"></circle>`).join('')}</g><rect class="usage-chart-scrub" x="${inset.left}" y="${inset.top}" width="${chart.plotW}" height="${chart.plotH}" tabindex="0" role="slider" aria-label="${this.escapeHtml(t('analytics.requests'))}" aria-valuemin="1" aria-valuemax="${points.length}" aria-valuenow="1"></rect></svg><div class="chart-tip" id="${containerId}-tip"></div></div>`;
    this.bindChartLegend(root, containerId, series, () => this.renderRequestTrend(points, containerId));
    bindPlotTooltip(root.querySelector('.usage-chart-stage'), root.querySelector('.chart-tip'), {
      pointCount: points.length,
      plotLeft: inset.left,
      plotTop: inset.top,
      plotWidth: chart.plotW,
      xForIndex: chart.x,
      markersForIndex: index => visible.map(item => ({x:chart.x(index),y:chart.y(points[index][item.key])})),
      contentForIndex: index => {
      const point=points[index];
      return chartTooltipMarkup(this.trendLabel(point.date), visible.map(item => ({label:item.label,value:Number(point[item.key]||0).toLocaleString(),color:item.color})), [{label:t('analytics.cost'),value:fmtAggregateCost(point)}]);
      },
    });
  },

  renderTokenLines(points, containerId='token-trend') {
    const root = document.getElementById(containerId);
    if (!root) return;
    if (!points.length || points.every(point => totalUsageTokens(point) === 0)) {
      root.innerHTML = `<div class="empty-state">${t('analytics.noTrend')}</div>`;
      return;
    }
    const series = [
      {key:'input_tokens',label:t('analytics.inputTokens'),className:'is-input',color:'#818cf8'},
      {key:'output_tokens',label:t('analytics.outputTokens'),className:'is-output',color:'#34d399'},
      {key:'cache_read_tokens',label:t('detail.cacheRead'),className:'is-cache-read',color:'#fbbf24'},
      {key:'cache_creation_tokens',label:t('detail.cacheCreation'),className:'is-cache-write',color:'#fb7185'},
      {key:'cache_rate',label:t('analytics.cacheHitShort'),className:'is-cache-rate',color:'#22d3ee',rate:true},
    ];
    const visible = this.visibleChartSeries(containerId, series);
    const visibleTokens = visible.filter(item => !item.rate);
    const showRate = visible.some(item => item.rate);
    const width=720, height=252, inset={top:18,right:50,bottom:32,left:56};
    const max = Math.max(1, ...visibleTokens.flatMap(item => points.map(point => Number(point[item.key]||0))));
    const chart = this.chartGrid(points,width,height,inset,max,value=>fmtTok(Math.round(value)));
    const rateY = value => inset.top + (1 - Number(value||0)/100) * chart.plotH;
    const rate = point => {
      const prompt=Number(point.input_tokens||0)+Number(point.cache_read_tokens||0)+Number(point.cache_creation_tokens||0);
      return prompt > 0 ? Number(point.cache_read_tokens||0)/prompt*100 : 0;
    };
    const lines = visibleTokens.map(item => `<polyline class="usage-chart-line ${item.className}" points="${points.map((point,index)=>`${chart.x(index)},${chart.y(point[item.key])}`).join(' ')}"></polyline>`).join('');
    const rateLine = showRate ? `<polyline class="usage-chart-line is-cache-rate" points="${points.map((point,index)=>`${chart.x(index)},${rateY(rate(point))}`).join(' ')}"></polyline>` : '';
    const dots = visibleTokens.map(item => points.map((point,index)=>Number(point[item.key]||0)>0?`<circle class="usage-chart-point ${item.className}" cx="${chart.x(index)}" cy="${chart.y(point[item.key])}" r="2.6"></circle>`:'').join('')).join('');
    const rightAxis = showRate ? [0,25,50,75,100].map(value=>`<text class="usage-axis-label is-rate" x="${width-inset.right+8}" y="${rateY(value)+3}">${value}%</text>`).join('') : '';
    root.innerHTML=`<div class="usage-chart-stage"><div class="usage-chart-legend">${this.chartLegend(series, visible)}</div><svg viewBox="0 0 ${width} ${height}" role="img">${chart.markup}${rightAxis}${lines}${rateLine}${dots}<g class="usage-chart-cursor" aria-hidden="true"><line class="usage-chart-crosshair" y1="${inset.top}" y2="${inset.top+chart.plotH}"></line>${visible.map(item=>`<circle class="usage-chart-cursor-point ${item.className}" r="4"></circle>`).join('')}</g><rect class="usage-chart-scrub" x="${inset.left}" y="${inset.top}" width="${chart.plotW}" height="${chart.plotH}" tabindex="0" role="slider" aria-label="${this.escapeHtml(t('analytics.totalTokens'))}" aria-valuemin="1" aria-valuemax="${points.length}" aria-valuenow="1"></rect></svg><div class="chart-tip" id="${containerId}-tip"></div></div>`;
    this.bindChartLegend(root, containerId, series, () => this.renderTokenLines(points, containerId));
    bindPlotTooltip(root.querySelector('.usage-chart-stage'),root.querySelector('.chart-tip'),{
      pointCount: points.length,
      plotLeft: inset.left,
      plotTop: inset.top,
      plotWidth: chart.plotW,
      xForIndex: chart.x,
      markersForIndex: index => visible.map(item => ({x:chart.x(index),y:item.rate ? rateY(rate(points[index])) : chart.y(points[index][item.key])})),
      contentForIndex: index => {
      const point=points[index];
      const footer = [
        {label:t('analytics.requests'),value:Number(point.requests||0).toLocaleString()},
        {label:t('analytics.cost'),value:fmtAggregateCost(point)},
      ];
      if (showRate) footer.unshift({label:t('analytics.cacheHitShort'),value:`${rate(point).toFixed(1)}%`});
      return chartTooltipMarkup(this.trendLabel(point.date),visibleTokens.map(item=>({label:item.label,value:Number(point[item.key]||0).toLocaleString(),color:item.color})),footer);
      },
    });
  },

  renderPeriodTable(points) {
    const body=document.getElementById('analytics-period-tbody');
    if (!body) return;
    body.innerHTML=points.map(point=>`<tr><td>${this.escapeHtml(this.trendLabel(point.date))}</td><td>${Number(point.requests||0).toLocaleString()}</td><td>${Number(point.known_requests||0)>0?Number(point.error_requests||0).toLocaleString():'—'}</td><td>${Number(point.input_tokens||0).toLocaleString()}</td><td>${Number(point.output_tokens||0).toLocaleString()}</td><td>${Number(point.cache_read_tokens||0).toLocaleString()}</td><td>${Number(point.cache_creation_tokens||0).toLocaleString()}</td><td><strong>${totalUsageTokens(point).toLocaleString()}</strong></td><td title="${this.escapeHtml(costCoverageNote(point))}">${fmtAggregateCost(point)}</td></tr>`).join('');
    const count=document.getElementById('analytics-period-count');
    if (count) count.textContent=`${points.length} ${this.granularity==='hour'?t('analytics.hour'):t('analytics.day')}`;
  },

  renderModelTable(models, containerId = 'analytics-model-tbody') {
    const body=document.getElementById(containerId);
    if (!body) return;
    body.innerHTML=(models||[]).map(item=>{
      const prompt=Number(item.input_tokens||0)+Number(item.cache_read_tokens||0)+Number(item.cache_creation_tokens||0);
      const rate=prompt>0?Number(item.cache_read_tokens||0)/prompt*100:0;
      return `<tr data-provider="${this.escapeHtml(item.provider || '')}"><td><code title="${this.escapeHtml(item.model || '')}">${this.escapeHtml(item.model||t('detail.unknown'))}</code><br><small>${this.escapeHtml(providerLabel(item.provider))}</small></td><td>${Number(item.requests||0).toLocaleString()}</td><td>${prompt>0?rate.toFixed(1)+'%':'—'}</td><td>${totalUsageTokens(item).toLocaleString()}</td><td title="${this.escapeHtml(costCoverageNote(item))}">${fmtAggregateCost(item)}</td></tr>`;
    }).join('') || `<tr><td colspan="5" class="empty-state">${t('analytics.noData')}</td></tr>`;
  },

  renderRetainedRange(summary) {
    const root=document.getElementById('analytics-retained-range');
    if (!root) return;
    const from=document.getElementById('analytics-start')?.value||'';
    const to=document.getElementById('analytics-end')?.value||'';
    root.textContent=t('analytics.retainedRange').replace('{from}',from).replace('{to}',to);
  },

  escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
  }
};

// Initialize dates and listeners before the queued hash activation runs.
AnalyticsModule.init();

/* ── Platform accounts and local ledger ───────────────────────── */
// Account capabilities come from /api/quota. Local records are fetched
// independently, so an unavailable account API never hides this instance's usage.
const QUOTA_WINDOWS = [
  { key: 'rolling_5h', label: 'quota.rolling5h' },
  { key: 'weekly', label: 'quota.weekly' },
  { key: 'monthly', label: 'quota.monthly' },
];
// The dashboard polls every 3s; quota only moves as fast as real spend, so
// unforced reloads are throttled well below the server-side cache TTL.
const QUOTA_MIN_RELOAD_MS = 20000;
const QUOTA_RING_RADIUS = 54;
const QUOTA_RING_LENGTH = 2 * Math.PI * QUOTA_RING_RADIUS;

const QuotaModule = {
  provider: 'opencode-go',
  accountProvider: '',
  view: null,
  loadSeq: 0,
  loading: false,
  lastLoadedAt: 0,
  accountError: '',
  localView: null,
  localQuery: '',
  localLoadSeq: 0,
  localLoading: false,
  localLastLoadedAt: 0,
  localError: '',

  init() {
    document.getElementById('btn-refresh-quota')?.addEventListener('click', () => this.load(true));
    document.getElementById('btn-fetch-bedrock-billing')?.addEventListener('click', () => {
      if (this.provider === 'aws-bedrock') return this.loadAccounts(true, true);
    });
    document.getElementById('quota-provider')?.addEventListener('change', event => {
      this.provider = event.target.value;
      return this.load(true);
    });
    document.getElementById('quota-local-days')?.addEventListener('change', () => this.loadLocalUsage(true));
    document.getElementById('quota-view-local')?.addEventListener('click', () => viewProviderHistory(this.provider));
    document.getElementById('quota-open-settings')?.addEventListener('click', () => openProviderSettings(this.provider));
    // A single ticker drives every countdown on the page and idles while the
    // Quota tab is hidden.
    setInterval(() => { if (activeTab === 'quota') this.tickCountdowns(); }, 1000);
  },

  async load(force) {
    await Promise.all([this.loadAccounts(force), this.loadLocalUsage(force)]);
  },

  async loadAccounts(force, billingRefresh = false) {
    const provider = this.provider;
    const changed = this.accountProvider !== provider;
    if (!changed && this.loading) return;
    if (!changed && !force && Date.now() - this.lastLoadedAt < QUOTA_MIN_RELOAD_MS) return;
    const seq = ++this.loadSeq;
    this.accountProvider = provider;
    this.loading = true;
    this.accountError = '';
    if (changed) this.view = null;
    this.render();
    try {
      const params = new URLSearchParams({provider});
      if (force) params.set('refresh', '1');
      const manualBilling = provider === 'aws-bedrock' && billingRefresh;
      if (manualBilling) params.set('billing_refresh', '1');
      const view = await fetchJSON(`/api/quota?${params}`, manualBilling ? {method: 'POST'} : undefined);
      if (seq !== this.loadSeq || provider !== this.provider) return;
      if (!view || view.provider !== provider || !['available', 'partial', 'not_configured', 'unavailable', 'error'].includes(view.status)) {
        throw new Error(t('data.invalid'));
      }
      this.view = view;
      this.lastLoadedAt = Date.now();
    } catch (e) {
      if (seq !== this.loadSeq || provider !== this.provider) return;
      this.view = null;
      this.accountError = e.message || String(e);
    } finally {
      if (seq === this.loadSeq) {
        this.loading = false;
        this.render();
      }
    }
  },

  async loadLocalUsage(force) {
    const provider = this.provider;
    const days = document.getElementById('quota-local-days')?.value || '30';
    const params = new URLSearchParams({provider, days});
    const changed = this.localQuery !== params.toString();
    if (!changed && this.localLoading) return;
    if (!changed && !force && Date.now() - this.localLastLoadedAt < QUOTA_MIN_RELOAD_MS) return;
    const seq = ++this.localLoadSeq;
    this.localQuery = params.toString();
    this.localLoading = true;
    this.localError = '';
    if (changed) this.localView = null;
    this.renderLocalUsage();
    this.syncRefreshButton();
    try {
      const data = await fetchJSON(`/api/analytics/summary?${params}`);
      if (seq !== this.localLoadSeq || provider !== this.provider) return;
      if (!data?.summary || data.provider !== provider) throw new Error(t('data.invalid'));
      this.localView = data;
      this.localLastLoadedAt = Date.now();
    } catch (e) {
      if (seq !== this.localLoadSeq || provider !== this.provider) return;
      this.localView = null;
      this.localError = e.message || String(e);
    } finally {
      if (seq === this.localLoadSeq) {
        this.localLoading = false;
        this.renderLocalUsage();
        this.syncRefreshButton();
      }
    }
  },

  renderLocalUsage() {
    const days = document.getElementById('quota-local-days')?.value || '30';
    this.setText('quota-local-note', t('quota.localNote').replace('{provider}', PROVIDERS[this.provider]?.name || this.provider).replace('{days}', days));
    const error = document.getElementById('quota-local-error');
    if (error) {
      error.hidden = !this.localError;
      error.textContent = this.localError ? t('quota.localLoadFail') + this.localError : '';
    }
    const data = this.localView;
    if (!data) {
      ['quota-local-requests', 'quota-local-tokens', 'quota-local-cost', 'quota-local-unknown'].forEach(id => this.setText(id, this.localLoading ? '…' : '—'));
      this.setText('quota-local-cost-note', '');
      const body = document.getElementById('quota-local-model-tbody');
      if (body) body.innerHTML = `<tr><td colspan="5" class="empty-state">${t(this.localLoading ? 'data.loading' : 'detail.unavailable')}</td></tr>`;
      return;
    }
    const s = data.summary;
    this.setText('quota-local-requests', fmt(s.total_requests));
    this.setText('quota-local-tokens', hasUsageTokens(s) ? fmtTok(totalUsageTokens(s)) : '—');
    this.setText('quota-local-cost', fmtAggregateCost(s));
    this.setText('quota-local-unknown', fmt(s.unknown_cost_requests));
    this.setText('quota-local-cost-note', costCoverageNote(s));
    AnalyticsModule.renderModelTable(data.models || [], 'quota-local-model-tbody');
  },

  syncRefreshButton() {
    const button = document.getElementById('btn-refresh-quota');
    if (button) button.disabled = this.loading || this.localLoading;
    const billingButton = document.getElementById('btn-fetch-bedrock-billing');
    if (billingButton) billingButton.disabled = this.loading || this.view?.reason === 'aws_billing_disabled';
  },

  render() {
    const isGo = this.provider === 'opencode-go';
    const isOpenRouter = this.provider === 'openrouter';
    const isBedrock = this.provider === 'aws-bedrock';
    const isCommandCode = this.provider === 'commandcode';
    const go = document.getElementById('quota-go');
    const openrouter = document.getElementById('quota-openrouter');
    const bedrock = document.getElementById('quota-bedrock');
    const commandcode = document.getElementById('quota-commandcode');
    const unavailable = document.getElementById('quota-unavailable');
    if (go) go.hidden = !isGo;
    if (openrouter) openrouter.hidden = !isOpenRouter;
    if (bedrock) bedrock.hidden = !isBedrock;
    if (commandcode) commandcode.hidden = !isCommandCode;
    if (unavailable) unavailable.hidden = isGo || isOpenRouter || isBedrock || isCommandCode;
    this.syncRefreshButton();
    const view = this.view?.provider && this.view.provider !== this.provider ? null : this.view;
    const goSummary = document.getElementById('quota-go-summary');
    if (goSummary) goSummary.hidden = !isGo || view?.accounts?.length !== 1
      || !this.windowsOf(view.accounts[0]).some(item => item.window.has_percent);
    const meta = [];
    if (view?.status) meta.push(t('quota.status.' + view.status));
    if (view?.currency) meta.push(view.currency);
    if (view?.fetched_at) meta.push(t('quota.updated').replace('{time}', fmtTime(view.fetched_at)));
    if (view?.cached) meta.push(t('quota.cached'));
    this.setText('quota-meta', !view && this.loading ? t('data.loading') : meta.join(' · '));
    this.setText('quota-source', view?.source ? t('quota.source.' + view.source) : '');
    this.setText('quota-endpoint', view?.endpoint ? t('quota.endpoint').replace('{url}', view.endpoint) : '');
    const error = document.getElementById('quota-error');
    if (error) {
      error.hidden = !this.accountError;
      error.textContent = this.accountError ? t('quota.loadFail') + ': ' + this.accountError : '';
    }
    const links = document.getElementById('quota-links');
    if (links) {
      const labels = {docs:'commandcode.docs', usage:'commandcode.usage', billing:'commandcode.billing', keys:'commandcode.keys'};
      links.innerHTML = (view?.links || []).filter(link => labels[link.kind] && /^https?:\/\//i.test(link.url)).map(link =>
        `<a class="btn btn-small" href="${escapeHtml(link.url)}" target="_blank" rel="noopener noreferrer">${t(labels[link.kind])}</a>`).join('');
      links.hidden = !links.innerHTML;
    }
    if (!isGo) {
      this.clearSummary();
      this.renderModelLimits({});
      if (isOpenRouter) {
        this.renderOpenRouter(view);
        return;
      }
      if (isBedrock) {
        this.renderBedrockBilling(view);
        return;
      }
      if (isCommandCode) {
        this.renderCommandCode(view);
        this.tickCountdowns();
        return;
      }
      this.setText('quota-unavailable-title', t('quota.notRetrieved').replace('{provider}', PROVIDERS[this.provider]?.name || this.provider));
      this.setText('quota-unavailable-note', !view && this.loading ? t('data.loading')
        : this.accountError ? t('detail.unavailable')
        : view?.reason ? t('quota.reason.' + view.reason) : t('quota.providerUnavailable'));
      return;
    }
    const root = document.getElementById('quota-accounts');
    if (!root) return;
    if (!view) {
      if (this.accountError) this.renderError(this.accountError);
      else {
        root.innerHTML = `<div class="quota-notice">${t(this.loading ? 'data.loading' : 'detail.unavailable')}</div>`;
        this.clearSummary();
        this.renderModelLimits({});
      }
      return;
    }

    if (view.error) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(view.error)}</span></div>`;
      this.clearSummary();
      this.renderModelLimits(view);
      return;
    }
    const accounts = view.accounts || [];
    if (!accounts.length) {
      root.innerHTML = `<div class="quota-notice"><strong>${t('quota.noKey')}</strong><span>${t('quota.noKeyHint')}</span></div>`;
      this.clearSummary();
      this.renderModelLimits(view);
      return;
    }
    root.innerHTML = accounts.map(account => this.renderAccount(account)).join('') +
      `<div class="quota-index-lag-notice">
        <svg width="16" height="16" fill="currentColor" aria-hidden="true"><path d="M8 1a7 7 0 100 14A7 7 0 008 1zm0 3a1 1 0 011 1v3.586l1.707 1.707a1 1 0 01-1.414 1.414l-2-2A1 1 0 017 9V5a1 1 0 011-1z"/></svg>
        <span>${t('quota.indexLagNotice')}</span>
      </div>`;
    this.renderSummary(accounts);
    this.tickCountdowns();
    this.renderModelLimits(view);
  },

  renderError(message) {
    const root = document.getElementById('quota-accounts');
    if (root) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(message)}</span></div>`;
    }
    this.clearSummary();
    this.renderModelLimits({});
  },

  renderBedrockBilling(view) {
    const root = document.getElementById('quota-bedrock-body');
    if (!root) return;
    if (view?.error) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert">${escapeHtml(view.error)}</div>`;
      return;
    }
    const data = view?.bedrock_billing;
    if (!data || data.total_cost == null) {
      root.innerHTML = `<div class="quota-notice">${escapeHtml(t(!view ? (this.loading ? 'data.loading' : 'detail.unavailable')
        : view.reason ? 'quota.reason.' + view.reason : 'quota.reason.aws_billing_no_data'))}</div>`;
      return;
    }
    if (!Number.isFinite(data.total_cost) || !/^[A-Z]{3}$/.test(data.currency || '') || !Array.isArray(data.daily) || !Array.isArray(data.services)) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert">${t('data.invalid')}</div>`;
      return;
    }
    const money = new Intl.NumberFormat(currentLang === 'zh' ? 'zh-CN' : 'en-US', {style:'currency',currency:data.currency,maximumFractionDigits:8});
    const formatCost = value => Number.isFinite(value) ? escapeHtml(money.format(value)) : '—';
    const status = estimated => t(estimated ? 'aws.billingEstimated' : 'aws.billingReported');
    root.innerHTML = `<dl class="quota-figures">
      <div><dt>${t('aws.billingTotal')}</dt><dd>${formatCost(data.total_cost)}</dd></div>
      <div><dt>${t('aws.billingAccount')}</dt><dd>${escapeHtml(data.linked_account_id)}</dd></div>
      <div><dt>${t('aws.billingPeriod')}</dt><dd>${escapeHtml(data.start_date)} → ${escapeHtml(data.end_date)}</dd></div>
    </dl><p class="page-meta">${escapeHtml(status(data.estimated))} · ${escapeHtml(data.currency)}</p>
    <p class="page-meta">${t('aws.billingServices')}: ${data.services.map(escapeHtml).join(' · ')}</p>
    <div class="analytics-table-scroll"><table class="analytics-table"><thead><tr><th>${t('analytics.day')} (UTC)</th><th>${t('analytics.cost')} (${escapeHtml(data.currency)})</th><th>${t('th.status')}</th></tr></thead><tbody>
      ${data.daily.map(row => `<tr><td>${escapeHtml(row.date)}</td><td>${formatCost(row.cost)}</td><td>${escapeHtml(status(row.estimated))}</td></tr>`).join('')}
    </tbody></table></div>`;
  },

  renderOpenRouter(view) {
    const creditsRoot = document.getElementById('quota-openrouter-credits');
    const heading = `<h2 class="section-heading">${t('openrouter.accountCredits')}</h2>`;
    const credits = view?.credits;
    if (creditsRoot) {
      if (view?.credits_status === 'available' && Number.isFinite(credits?.total_credits) && Number.isFinite(credits?.total_usage)) {
        creditsRoot.innerHTML = heading + `<dl class="quota-figures">
          <div><dt>${t('openrouter.totalCredits')}</dt><dd>${fmtCost(credits.total_credits)}</dd></div>
          <div><dt>${t('openrouter.totalUsage')}</dt><dd>${fmtCost(credits.total_usage)}</dd></div>
          <div><dt>${t('openrouter.balance')}</dt><dd>${fmtCost(credits.total_credits - credits.total_usage)}</dd></div>
        </dl><p class="page-meta">${escapeHtml(view.credits_endpoint || '')}</p>`;
      } else if (view?.credits_status === 'error' || view?.credits_status === 'available') {
        creditsRoot.innerHTML = heading + `<div class="quota-notice is-error" role="alert"><strong>${t('openrouter.creditsFail')}</strong><span>${escapeHtml(view.credits_error || t('data.invalid'))}</span></div>`;
      } else {
        creditsRoot.innerHTML = heading + `<div class="quota-notice">${t(!view ? (this.loading ? 'data.loading' : 'detail.unavailable')
          : view.credits_status === 'not_configured' ? 'openrouter.noManagementKey' : 'detail.unavailable')}</div>`;
      }
    }
    const root = document.getElementById('quota-openrouter-accounts');
    if (!root) return;
    if (!view) {
      root.innerHTML = `<div class="quota-notice">${t(this.loading ? 'data.loading' : 'detail.unavailable')}</div>`;
    } else if (view.error) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(view.error)}</span></div>`;
    } else if (!view.accounts?.length) {
      root.innerHTML = `<div class="quota-notice"><strong>${t('quota.noProviderKey').replace('{provider}', 'OpenRouter')}</strong><span>${t('quota.noProviderKeyHint')}</span></div>`;
    } else {
      root.innerHTML = view.accounts.map(account => this.renderOpenRouterAccount(account)).join('');
    }
  },

  renderOpenRouterAccount(account) {
    const head = `<div class="quota-account-head"><span class="quota-key">${t('quota.keyLabel')} <code>${escapeHtml(account.key_hint || '—')}</code></span></div>`;
    const data = account.openrouter;
    if (account.error || !data || !Number.isFinite(data.usage) || (data.limit !== null && !Number.isFinite(data.limit))) {
      return `<section class="quota-account analytics-section">${head}<div class="quota-notice is-error" role="alert"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(account.error || t('data.invalid'))}</span></div></section>`;
    }
    const yesNo = value => value === true ? t('quota.yes') : value === false ? t('quota.no') : '—';
    const fields = [
      ['openrouter.limit', data.limit === null ? t('openrouter.noLimit') : fmtCost(data.limit)],
      ['openrouter.limitRemaining', fmtCost(data.limit_remaining)],
      ['openrouter.limitReset', data.limit_reset ?? '—'],
      ['openrouter.includeByok', yesNo(data.include_byok_in_limit)],
      ['openrouter.freeTier', yesNo(data.is_free_tier)],
      ['openrouter.expires', data.expires_at ?? '—'],
    ];
    const periods = [
      ['perf.allTime', 'usage', 'byok_usage'],
      ['analytics.day', 'usage_daily', 'byok_usage_daily'],
      ['quota.weekly', 'usage_weekly', 'byok_usage_weekly'],
      ['quota.monthly', 'usage_monthly', 'byok_usage_monthly'],
    ];
    return `<section class="quota-account analytics-section">${head}
      <dl class="quota-figures">${fields.map(([label, value]) => `<div><dt>${t(label)}</dt><dd>${escapeHtml(value)}</dd></div>`).join('')}</dl>
      <div class="analytics-table-scroll"><table class="analytics-table"><thead><tr><th>${t('analytics.period')}</th><th>${t('openrouter.usage')}</th><th>${t('openrouter.byokUsage')}</th></tr></thead>
        <tbody>${periods.map(([label, usage, byok]) => `<tr><td>${t(label)}</td><td>${fmtCost(data[usage])}</td><td>${fmtCost(data[byok])}</td></tr>`).join('')}</tbody>
      </table></div>
    </section>`;
  },

  renderCommandCode(view) {
    const root = document.getElementById('quota-commandcode-accounts');
    if (!root) return;
    if (!view) {
      root.innerHTML = `<div class="quota-notice">${t(this.loading ? 'data.loading' : 'detail.unavailable')}</div>`;
    } else if (view.error) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert">${escapeHtml(view.error)}</div>`;
    } else if (!view.accounts?.length) {
      root.innerHTML = `<div class="quota-notice"><strong>${t('quota.noProviderKey').replace('{provider}', 'CommandCode')}</strong><span>${t('quota.noProviderKeyHint')}</span></div>`;
    } else {
      root.innerHTML = view.accounts.map(account => this.renderCommandCodeAccount(account)).join('');
    }
  },

  renderCommandCodeAccount(account) {
    const head = `<div class="quota-account-head"><span class="quota-key">${t('quota.keyLabel')} <code>${escapeHtml(account.key_hint || '—')}</code></span></div>`;
    const data = account.commandcode;
    const error = message => `<div class="quota-notice is-error" role="alert">${escapeHtml(message)}</div>`;
    if (account.error || !data) return `<section class="quota-account analytics-section">${head}${error(account.error || t('data.invalid'))}</section>`;
    const figures = rows => `<dl class="quota-figures">${rows.map(([label, value]) => `<div><dt>${t(label)}</dt><dd>${escapeHtml(value)}</dd></div>`).join('')}</dl>`;
    const section = (title, content, className = '') => `<section class="commandcode-block ${className}"><h3 class="section-heading">${t(title)}</h3>${content}</section>`;
    const utc = value => {
      const date = value ? new Date(value) : null;
      return date && Number.isFinite(date.getTime()) ? date.toISOString().replace('T', ' ').replace('.000Z', ' UTC') : '—';
    };
    let credits = error(data.credits_error || t('detail.unavailable'));
    if (data.credits) {
      const balance = data.credits.credits;
      credits = '<div class="commandcode-credit-values">' + figures([
        ['commandcode.monthlyCredits', fmtCost(balance?.monthlyCredits)],
        ['commandcode.freeCredits', fmtCost(balance?.freeCredits)],
        ['commandcode.purchasedCredits', fmtCost(balance?.purchasedCredits)],
      ]) + `</div><p class="page-meta">${t('commandcode.monthlyLimitUnknown')}</p>`;
      const limits = data.credits.windowLimits;
      const windows = [['quota.rolling5h', limits?.fiveHour], ['quota.weekly', limits?.weekly]].filter(([, window]) => window);
      credits += `<h4 class="section-heading">${t('commandcode.windowLimits')}</h4>`;
      if (windows.length) {
        credits += `<div class="commandcode-windows">${windows.map(([label, window]) => {
          const percent = Number.isFinite(window.used) && Number.isFinite(window.cap) && window.cap > 0 ? window.used / window.cap * 100 : null;
          const level = window.exceeded ? 'crit' : percent == null ? 'ok' : this.levelOf(percent);
          const reset = Number.isFinite(window.resetAt) && window.resetAt > 0 ? window.resetAt : null;
          return `<div class="commandcode-window level-${level}"><div class="commandcode-window-heading"><strong>${t(label)}</strong><span>${fmtCost(window.used)} / ${fmtCost(window.cap)}${percent == null ? '' : ` · ${percent.toFixed(1)}%`}</span></div>
            ${percent == null ? '' : `<progress max="100" value="${Math.min(100, Math.max(0, percent))}" aria-label="${t(label)}"></progress>`}
            ${window.exceeded ? `<span class="quota-badge is-crit">${t('quota.exhausted')}</span>` : ''}
            <span class="quota-reset"${reset == null ? '' : ` data-deadline="${reset}"`}>${t('quota.resetUnknown')}</span></div>`;
        }).join('')}</div>`;
      } else {
        credits += `<p class="page-meta">${t(limits?.limited === false ? 'commandcode.noWindowLimits' : 'detail.unavailable')}</p>`;
      }
    }
    const sub = data.subscription;
    const subscription = data.subscription_error ? error(data.subscription_error) : sub ? figures([
      ['quota.plan', sub.planId], ['th.status', sub.status],
      ['commandcode.billingPeriod', `${utc(sub.currentPeriodStart)} → ${utc(sub.currentPeriodEnd)}`],
      ['commandcode.cancelAtEnd', sub.cancelAtPeriodEnd === true ? t('quota.yes') : sub.cancelAtPeriodEnd === false ? t('quota.no') : '—'],
    ]) : `<p class="page-meta">${t('commandcode.noSubscription')}</p>`;
    const usage = data.usage;
    const summary = data.usage_error ? error(data.usage_error) : usage ? figures([
      ['commandcode.usagePeriod', usage.periodBasis === 'billing-period' ? t('commandcode.billingPeriodScope') : usage.periodBasis || '—'],
      ['analytics.requests', fmt(usage.totalCount)], ['metric.success', fmt(usage.completedCount)], ['metric.failed', fmt(usage.failedCount)],
      ['analytics.inputTokens', fmt(usage.totalTokensIn)], ['analytics.outputTokens', fmt(usage.totalTokensOut)],
      ['analytics.totalTokens', fmt(usage.totalTokens)], ['commandcode.usageCredits', fmtCost(usage.totalCredits)],
      ['commandcode.officialCost', fmtCost(usage.totalCost)],
    ]) : `<p class="page-meta">${t('detail.unavailable')}</p>`;
    // The account's count and this instance's count describe different things,
    // so they are shown as two blocks with their difference named rather than
    // merged into one number.
    const ledger = data.ledger ? figures([
      ['commandcode.ledgerRequests', fmt(data.ledger.requests)],
      ['commandcode.ledgerCost', fmtCost(data.ledger.cost_usd)],
      ...(usage && usage.totalCount != null
        ? [['commandcode.ledgerGap', `${fmt(Math.max(0, usage.totalCount - data.ledger.requests))} · ${t('commandcode.ledgerHint')}`]]
        : []),
    ]) : '';
    return `<section class="quota-account commandcode-account">${head}<div class="commandcode-panels">${section('commandcode.credits', credits, 'commandcode-credits')}${section('quota.plan', subscription)}${section('commandcode.usageSummary', summary)}${ledger ? section('commandcode.ledger', ledger, 'commandcode-ledger') : ''}</div></section>`;
  },

  renderAccount(account) {
    const head = `<div class="quota-account-head">${account.key_hint ? `<span class="quota-key">${t('quota.keyLabel')} <code>${escapeHtml(account.key_hint)}</code></span>` : ''}${account.report?.plan ? `<span class="quota-account-plan">${escapeHtml(account.report.plan)}</span>` : ''}</div>`;
    if (account.error) {
      return `<section class="quota-account">${head}
        <div class="quota-notice is-error" role="alert"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(account.error)}</span></div>
      </section>`;
    }
    const windows = this.windowsOf(account);
    if (!windows.length) {
      return `<section class="quota-account">${head}
        <div class="quota-notice is-error"><strong>${t('quota.loadFail')}</strong><span>${escapeHtml(t('quota.resetUnknown'))}</span></div>
      </section>`;
    }
    return `<section class="quota-account">${head}
      <div class="quota-window-grid">${windows.map(item => this.renderWindow(item)).join('')}</div>
    </section>`;
  },

  renderWindow(item) {
    const w = item.window;
    const used = w.has_percent ? Math.min(100, Math.max(0, Number(w.used_percent))) : null;
    const left = used != null ? 100 - used : null;
    const level = used != null ? ' level-' + this.levelOf(used) : '';
    const offset = left != null ? QUOTA_RING_LENGTH * (1 - left / 100) : QUOTA_RING_LENGTH;
    const deadline = this.deadlineOf(w, item.fetchedAt);
    const limit = w.limit_dollars != null ? Number(w.limit_dollars) : null;
    const usedDollars = w.used_dollars != null ? Number(w.used_dollars) : null;
    const leftDollars = limit != null && usedDollars != null ? Math.max(0, limit - usedDollars) : null;
    const badge = used != null && used >= 100
      ? `<span class="quota-badge is-crit">${t('quota.exhausted')}</span>`
      : (w.status && w.status !== 'ok' ? `<span class="quota-badge is-warn">${escapeHtml(w.status)}</span>` : '');

    return `<article class="quota-card${level}">
      <header class="quota-card-head">
        <span class="quota-card-label">${t(item.label)}</span>
        ${badge}
      </header>
      <div class="quota-gauge">
        <svg viewBox="0 0 128 128" aria-hidden="true">
          <circle class="quota-gauge-track" cx="64" cy="64" r="${QUOTA_RING_RADIUS}"></circle>
          <circle class="quota-gauge-fill" cx="64" cy="64" r="${QUOTA_RING_RADIUS}"
                  stroke-dasharray="${QUOTA_RING_LENGTH.toFixed(2)}" stroke-dashoffset="${offset.toFixed(2)}"></circle>
        </svg>
        <div class="quota-gauge-center">
          <strong>${left != null ? `${left.toFixed(left >= 10 ? 0 : 1)}<span class="quota-unit">%</span>` : '—'}</strong>
          <span>${t('quota.leftShort')}</span>
        </div>
      </div>
      <dl class="quota-figures">
        <div><dt>${t('quota.used')}</dt><dd>${usedDollars != null ? fmtCost(usedDollars) : '—'}</dd></div>
        <div><dt>${t('quota.limit')}</dt><dd>${limit != null ? fmtCost(limit) : '—'}</dd></div>
        <div><dt>${t('quota.left')}</dt><dd class="quota-figure-strong">${leftDollars != null ? fmtCost(leftDollars) : '—'}</dd></div>
      </dl>
      <footer class="quota-card-foot">
        <span class="quota-reset"${deadline != null ? ` data-deadline="${deadline}"` : ''}>${deadline != null ? '' : t('quota.resetUnknown')}</span>
        ${w.percent_derived ? `<span class="quota-derived" title="${t('quota.derived')}">${t('quota.derived')}</span>` : ''}
      </footer>
    </article>`;
  },

  renderSummary(accounts) {
    let tightest = null;
    let nextReset = null;
    let plan = '';
    accounts.forEach(account => {
      if (account.report?.plan) plan = account.report.plan;
      this.windowsOf(account).forEach(item => {
        if (item.window.has_percent) {
          const used = Number(item.window.used_percent);
          if (!tightest || used > Number(tightest.window.used_percent)) tightest = item;
        }
        const deadline = this.deadlineOf(item.window, item.fetchedAt);
        if (deadline != null && (nextReset == null || deadline < nextReset.deadline)) {
          nextReset = { deadline, label: item.label };
        }
      });
    });

    const bottleneck = document.getElementById('quota-bottleneck');
    if (tightest) {
      const used = Math.min(100, Math.max(0, Number(tightest.window.used_percent || 0)));
      const limit = tightest.window.limit_dollars != null ? Number(tightest.window.limit_dollars) : null;
      const usedDollars = tightest.window.used_dollars != null ? Number(tightest.window.used_dollars) : null;
      const leftDollars = limit != null && usedDollars != null ? Math.max(0, limit - usedDollars) : null;
      if (bottleneck) bottleneck.className = 'metric-value quota-level-' + this.levelOf(used);
      this.setText('quota-bottleneck', t(tightest.label));
      this.setText('quota-bottleneck-note', `${used.toFixed(1)}% ${t('quota.used').toLowerCase()}`);
      this.setText('quota-remaining', leftDollars != null ? fmtCost(leftDollars) : `${(100 - used).toFixed(1)}%`);
      this.setText('quota-remaining-note', limit != null ? `${t('quota.limit')} ${fmtCost(limit)}` : '');
    } else {
      this.clearSummary();
    }

    const resetEl = document.getElementById('quota-next-reset');
    if (resetEl) {
      if (nextReset) {
        resetEl.dataset.deadline = String(nextReset.deadline);
        this.setText('quota-next-reset-note', `${t(nextReset.label)} · ${t('quota.resetsAtTime').replace('{time}', new Date(nextReset.deadline).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }))}`);
      } else {
        delete resetEl.dataset.deadline;
        resetEl.textContent = '--';
        this.setText('quota-next-reset-note', t('quota.resetUnknown'));
      }
    }
    this.setText('quota-plan', plan || 'OpenCode Go');
    this.setText('quota-plan-note', t('quota.keyCount').replace('{n}', accounts.length));
  },

  // Model costs come from this instance's Go ledger; account gauges above
  // remain authoritative for account usage. Missing rows are never free usage.
  renderModelLimits(view) {
    const root = document.getElementById('quota-model-limits');
    if (!root) return;
    if (view.model_usage_error) {
      root.innerHTML = `<div class="quota-notice is-error" role="alert"><strong>${t('quota.localUsageFail')}</strong><span>${escapeHtml(view.model_usage_error)}</span></div>`;
      return;
    }
    const ml = view.model_limits;
    const rows = view.model_usage || (ml?.models || []).map(m => ({
      model: m.model,
      used_usd: null,
      allowance_usd: m.allowance_usd,
      percent: null,
    }));
    if (!rows.length) {
      root.innerHTML = '';
      return;
    }
    const body = rows.map(m => `<tr>
        <td>${escapeHtml(m.model)}</td>
        <td class="quota-model-number" title="${escapeHtml(costCoverageNote(m))}">${fmtAggregateCost({...m, cost_usd:m.used_usd})}</td>
        <td class="quota-model-number">${m.allowance_usd != null ? fmtCost(m.allowance_usd) : '—'}</td>
        <td class="quota-model-number">${m.percent != null ? Number(m.percent).toFixed(1) + '%' : '—'}</td>
      </tr>`).join('');
    const totals = {
      requests: rows.reduce((sum, m) => sum + Number(m.requests || 0), 0),
      unknown_cost_requests: rows.reduce((sum, m) => sum + Number(m.unknown_cost_requests || 0), 0),
      cost_usd: rows.every(m => m.used_usd != null) ? rows.reduce((sum, m) => sum + Number(m.used_usd), 0) : null,
    };
    const poolPercent = rows.every(m => m.percent != null) ? rows.reduce((sum, m) => sum + Number(m.percent), 0) : null;
    const accountNotice = (view.accounts || []).length > 1 ? `<p class="quota-disclaimer">${t('quota.multiKeyUsage')}</p>` : '';
    root.innerHTML = `${accountNotice}<div class="quota-model-head">
        <span class="quota-model-title">${t('quota.modelLimits')}</span>
        <span class="quota-model-meta">${t('quota.modelLimitsNote').replace('{time}', ml?.fetched_at ? fmtTime(ml.fetched_at) : '—')}</span>
      </div>
      <div class="analytics-table-scroll quota-model-scroll">
        <table class="analytics-table quota-model-table">
          <thead><tr><th>${t('quota.model')}</th><th>${t('quota.modelUsed')}</th><th>${t('quota.modelAllowance')}</th><th>${t('quota.percent')}</th></tr></thead>
          <tbody>${body}</tbody>
          <tfoot><tr>
            <td>${t('quota.total')}</td>
            <td class="quota-model-number" title="${escapeHtml(costCoverageNote(totals))}">${fmtAggregateCost(totals)}</td>
            <td class="quota-model-number">—</td>
            <td class="quota-model-number" title="${t('quota.poolShare')}">${poolPercent != null ? poolPercent.toFixed(1) + '%' : '—'}</td>
          </tr></tfoot>
        </table>
      </div>`;
  },

  clearSummary() {
    ['quota-bottleneck', 'quota-remaining', 'quota-next-reset', 'quota-plan'].forEach(id => {
      const el = document.getElementById(id);
      if (el) {
        el.textContent = '--';
        delete el.dataset.deadline;
      }
    });
    ['quota-bottleneck-note', 'quota-remaining-note', 'quota-next-reset-note', 'quota-plan-note'].forEach(id => this.setText(id, ''));
    const bottleneck = document.getElementById('quota-bottleneck');
    if (bottleneck) bottleneck.className = 'metric-value';
  },

  // tickCountdowns re-renders only the countdown text, so the gauges are not
  // rebuilt every second.
  tickCountdowns() {
    const now = Date.now();
    document.querySelectorAll('#tab-quota [data-deadline]').forEach(el => {
      const remaining = Number(el.dataset.deadline) - now;
      const text = fmtCountdown(remaining);
      el.textContent = el.classList.contains('metric-value') ? text : t('quota.resetsIn').replace('{d}', text);
    });
  },

  windowsOf(account) {
    const report = account.report;
    if (!report) return [];
    return QUOTA_WINDOWS
      .filter(spec => report[spec.key])
      .map(spec => ({ label: spec.label, window: report[spec.key], fetchedAt: report.fetched_at }));
  },

  // Thresholds mirror ocusage's alert levels: red once a window is nearly gone,
  // amber while it is going fast.
  levelOf(usedPercent) {
    if (usedPercent >= 90) return 'crit';
    if (usedPercent >= 75) return 'warn';
    return 'ok';
  },

  // deadlineOf prefers the absolute reset timestamp; a relative countdown is
  // anchored to when the report was fetched, not to now, so a cached response
  // does not restart the clock.
  deadlineOf(w, fetchedAt) {
    if (w.resets_at) {
      const parsed = Date.parse(w.resets_at);
      if (!Number.isNaN(parsed)) return parsed;
    }
    if (w.resets_in_sec != null) {
      const base = fetchedAt ? Date.parse(fetchedAt) : NaN;
      return (Number.isNaN(base) ? Date.now() : base) + Number(w.resets_in_sec) * 1000;
    }
    return null;
  },

  setText(id, value) {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  },
};

// fmtCountdown renders a live countdown the way "2d 9h" / "3h 20m" / "12m 04s"
// reads naturally, and stays at zero instead of going negative.
function fmtCountdown(ms) {
  const total = Math.max(0, Math.floor((Number(ms) || 0) / 1000));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const mins = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const pad = n => String(n).padStart(2, '0');
  if (currentLang === 'zh') {
    if (days > 0) return `${days} 天 ${hours} 小时`;
    if (hours > 0) return `${hours} 小时 ${pad(mins)} 分`;
    return `${mins} 分 ${pad(secs)} 秒`;
  }
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${pad(mins)}m`;
  return `${mins}m ${pad(secs)}s`;
}

QuotaModule.init();
