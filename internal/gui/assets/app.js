/* ── i18n ────────────────────────────────────────────────────────── */
const TRANSLATIONS = {
  en: {
    'lang.toggle': '中文',
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
    'tab.settings': 'Settings',
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
    'analytics.p95Latency': 'p95 Latency',
    'analytics.latencyUnit': 'request-weighted',
    'analytics.cacheHitShort': 'cache hit',
    'analytics.cacheHitTitle': 'Cache read as a share of all prompt tokens (input + cache read + cache write)',
    'analytics.byModel': 'Model distribution',
    'analytics.byProvider': 'Platform distribution',
    'analytics.noData': 'No data',
    'analytics.noTrend': 'No trend data',
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
    'analytics.retainedRange': 'Request records {from} - {to}',
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
    'perf.lastHour': 'Last Hour',
    'perf.last24h': 'Last 24 Hours',
    'perf.last7d': 'Last 7 Days',
    'perf.allTime': 'All Time',
    'perf.th.model': 'Model',
    'perf.th.count': 'Count',
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
    'analytics.p95Latency': 'p95 延迟',
    'analytics.latencyUnit': '按请求数加权',
    'analytics.cacheHitShort': '缓存命中',
    'analytics.cacheHitTitle': '缓存读取占全部输入 Token 的比例（输入 + 缓存读 + 缓存写）',
    'analytics.byModel': '模型分布',
    'analytics.byProvider': '平台分布',
    'analytics.noData': '暂无数据',
    'analytics.noTrend': '暂无趋势数据',
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
    'analytics.retainedRange': '请求记录 {from} - {to}',
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
    'perf.lastHour': '最近 1 小时',
    'perf.last24h': '最近 24 小时',
    'perf.last7d': '最近 7 天',
    'perf.allTime': '全部时间',
    'perf.th.model': '模型',
    'perf.th.count': '请求数',
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
  PerfModule.render();
  // Analytics charts and distributions carry inline strings; reload them
  // so they pick up the new language instead of keeping stale ones.
  if (activeTab === 'analytics') AnalyticsModule.load(true);
  if (lastOverviewView) renderOverviewUsage(lastOverviewView.data, lastOverviewView.trend, lastOverviewView.latency);
}

// Apply translations on load
document.addEventListener('DOMContentLoaded', () => {
  document.documentElement.lang = currentLang;
  applyTranslations();
  const exportBtn = document.getElementById('history-export');
  if (exportBtn) exportBtn.addEventListener('click', exportHistoryCSV);
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
  data: [],
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
    try {
      const r = await fetch('/api/perf/models?range=' + encodeURIComponent(this.timeRange));
      if (!r.ok) return;
      this.data = await r.json() || [];
      this.render();
    } catch (e) {
      console.error('PerfModule refresh failed:', e);
    }
  },

  render() {
    const tbody = document.getElementById('perf-tbody');
    if (!tbody) return;

    if (this.data.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + t('empty.noData') + '</td></tr>';
      return;
    }

    const sorted = [...this.data].sort((a, b) => {
      let aVal = a[this.sortField];
      let bVal = b[this.sortField];
      if (aVal == null) aVal = 0;
      if (bVal == null) bVal = 0;
      if (typeof aVal === 'string') aVal = aVal.toLowerCase();
      if (typeof bVal === 'string') bVal = bVal.toLowerCase();
      if (aVal < bVal) return this.sortDir === 'asc' ? -1 : 1;
      if (aVal > bVal) return this.sortDir === 'asc' ? 1 : -1;
      return 0;
    });

    tbody.innerHTML = sorted.map(row => {
      const successRate = row.count > 0 ? (row.success / row.count * 100).toFixed(1) : 0;
      const successClass = successRate >= 99 ? 'success-rate' : (successRate >= 95 ? '' : 'error-rate');
      return `
        <tr>
          <td class="perf-model">${escapeHtml(row.model)}</td>
          <td>${fmt(row.count)}</td>
          <td class="${successClass}">${successRate}%</td>
          <td class="${this.getLatencyClass(row.avg_ms)}">${fmt(row.avg_ms)}</td>
          <td class="${this.getLatencyClass(row.p50_ms)}">${fmt(row.p50_ms)}</td>
          <td class="${this.getLatencyClass(row.p90_ms)}">${fmt(row.p90_ms)}</td>
          <td class="${this.getLatencyClass(row.p99_ms)}">${fmt(row.p99_ms)}</td>
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
  if (!name) return;
  document.querySelectorAll('.chart-tip').forEach(tip => { tip.style.display = 'none'; });
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
  const tabEl = document.querySelector('.tab[data-tab="' + name + '"]');
  const panel = document.getElementById('tab-' + name);
  if (tabEl) tabEl.classList.add('active');
  if (panel) panel.classList.add('active');
  activeTab = name;
  if (name === 'analytics' && AnalyticsModule && AnalyticsModule.load) {
    AnalyticsModule.load(true);
  }
  refreshCurrentTab();
}

document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    // Update the hash (also fires hashchange -> activateTab), but avoid a
    // duplicate activation by calling directly with the desired name.
    if (location.hash !== '#' + tab.dataset.tab) {
      location.hash = tab.dataset.tab;
    }
    activateTab(tab.dataset.tab);
  });
});

// Respond to back/forward and manual hash edits.
window.addEventListener('hashchange', () => {
  const name = (location.hash || '').replace(/^#/, '') || 'overview';
  activateTab(name);
});

/* ── Polling ───────────────────────────────────────────────────── */
let perfPollTimer = null;
let perfPollCounter = 0;
let activeTab = 'overview';
let overviewDays = 7;
let overviewBreakdownMetric = 'requests';
let lastOverviewView = null;
// Upper bound on how many history rows are rendered into the DOM at once.
// Keeps long-session history tables fast while the count reflects all rows.
const HISTORY_RENDER_LIMIT = 200;

function startPolling() {
  refreshAll();
  PerfModule.init();
  PerfModule.refresh();
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
  try {
    const latencyRange = overviewDays === 7 ? '7d' : overviewDays === 30 ? '30d' : 'all';
    const [r, usageResponse, trendResponse, latencyResponse] = await Promise.all([
      fetch('/api/metrics'),
      fetch(`/api/analytics/summary?days=${overviewDays}&compare=1`),
      fetch(`/api/analytics/tokens/trend?days=${overviewDays}`),
      fetch(`/api/perf/aggregate?range=${latencyRange}`),
    ]);
    if (!r.ok) { markPollFail(); return; }
    const d = await r.json();
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

    document.getElementById('m-total').textContent = fmt(d.requests_received);

    if (usageResponse.ok) {
      const usage = await usageResponse.json();
      const trend = trendResponse.ok ? await trendResponse.json() : {trend: []};
      const latency = latencyResponse.ok ? await latencyResponse.json() : null;
      if (activeTab === 'overview') renderOverviewUsage(usage, trend.trend || [], latency);
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

function renderOverviewUsage(data, trend, latency) {
  const summary = data.summary || {};
  const input = Number(summary.input_tokens || 0);
  const output = Number(summary.output_tokens || 0);
  const cacheRead = Number(summary.cache_read_tokens || 0);
  const cacheWrite = Number(summary.cache_creation_tokens || 0);
  const total = input + output + cacheRead + cacheWrite;
  const prompt = input + cacheRead + cacheWrite;
  const today = data.today || {};
  const retained = data.retained || {};
  const lastMinute = data.last_minute || {};
  const compactTotal = value => fmtTok(Number(value || 0));
  const set = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };
  set('m-total', Number(summary.total_requests || 0).toLocaleString());
  set('m-cost', fmtCost(summary.cost_usd ?? summary.est_cost_usd ?? 0));
  set('m-tokens', fmtTok(total));
  set('m-cache-hit', prompt > 0 ? `${(cacheRead / prompt * 100).toFixed(1)}%` : '—');
  set('m-total-note', today.total_requests != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} ${fmt(Number(today.total_requests || 0))} · ${currentLang === 'zh' ? '保留' : 'Retained'} ${fmt(Number(retained.total_requests || 0))}`
    : `${overviewDays} ${currentLang === 'zh' ? '天' : 'days'}`);
  set('m-tokens-note', today.input_tokens != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} ${compactTotal(totalUsageTokens(today))} · ${currentLang === 'zh' ? '保留' : 'Retained'} ${compactTotal(totalUsageTokens(retained))}`
    : `${fmtTok(input)} ${currentLang === 'zh' ? '输入' : 'input'} · ${fmtTok(output)} ${currentLang === 'zh' ? '输出' : 'output'}`);
  set('m-cache-hit-note', `${fmtTok(cacheRead)} ${currentLang === 'zh' ? '读取' : 'read'}`);
  set('m-cost-note', today.est_cost_usd != null
    ? `${currentLang === 'zh' ? '今日' : 'Today'} ${fmtCost(today.est_cost_usd)} · ${currentLang === 'zh' ? '保留' : 'Retained'} ${fmtCost(retained.est_cost_usd)}`
    : t('analytics.currencyUSD'));
  set('m-throughput', `${Number(lastMinute.total_requests || 0).toLocaleString()} RPM`);
  set('m-throughput-note', `${fmtTok(totalUsageTokens(lastMinute))} TPM`);
  const known = Number(summary.known_requests || 0);
  set('m-success', known > 0 ? `${(Number(summary.success_rate || 0) * 100).toFixed(1)}%` : '—');
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

function dateInputValue(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
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
    'scenario-filter', 'status-filter', 'streaming-filter']
    .some(id => document.getElementById(id)?.value);
}

function resetHistoryFilters(refresh = true) {
  ['history-search', 'history-start', 'history-end', 'model-filter', 'provider-filter',
    'scenario-filter', 'status-filter', 'streaming-filter'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.value = '';
  });
  window.CustomSelect?.syncAll();
  window.HistoryDateRange?.syncFromHidden();
  historyPage = 1;
  if (refresh) refreshHistory();
}

async function refreshHistory() {
  try {
    const params = historyQueryParams();
    const [r, summaryResponse] = await Promise.all([
      fetch(`/api/history?${params}`),
      fetch(`/api/history/summary?${params}`),
    ]);
    if (!r.ok) return;
    const data = await r.json();
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
    if (summaryResponse.ok) renderHistorySummary(await summaryResponse.json());
  } catch(e) {}
}

let lastHistorySummary = null;
let historyBreakdownMetric = 'tokens';

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
  set('history-summary-cost', fmtCost(Number(summary.cost_usd || 0)));
  renderCompactBreakdown('history-model-breakdown', summary.models || []);
  renderCompactBreakdown('history-provider-breakdown', summary.providers || []);
  renderCompactBreakdown('history-scenario-breakdown', summary.scenarios || []);
}

function renderCompactBreakdown(id, items) {
  const root = document.getElementById(id);
  if (!root) return;
  const metricKey = historyBreakdownMetric === 'cost' ? 'cost_usd' : historyBreakdownMetric;
  const top = [...items].sort((a, b) => Number(b[metricKey] || 0) - Number(a[metricKey] || 0)).slice(0, 5);
  if (!top.length) {
    root.innerHTML = `<span class="compact-breakdown-value">${t('analytics.noData')}</span>`;
    return;
  }
  const total = Math.max(1, top.reduce((sum, item) => sum + Number(item[metricKey] || 0), 0));
  root.innerHTML = top.map(item => {
    const label = !item.name || item.name === 'unknown' ? 'override' : item.name;
    return `
    <div class="compact-breakdown-row">
      <i class="compact-breakdown-fill" style="width:${Math.max(2, Number(item[metricKey] || 0) / total * 100).toFixed(1)}%"></i>
      <span class="compact-breakdown-name">${escapeHtml(label)}</span>
      <span class="compact-breakdown-stat"><strong>${Number(item.requests || 0).toLocaleString()}</strong><small>${t('analytics.requests')}</small></span>
      <span class="compact-breakdown-stat"><strong>${fmtTok(Number(item.tokens || 0))}</strong><small>Token</small></span>
      <span class="compact-breakdown-stat"><strong>${fmtCost(Number(item.cost_usd || 0))}</strong><small>${t('analytics.cost')}</small></span>
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
}

function renderHistory() {
  const tbody = document.getElementById('history-tbody');
  document.getElementById('history-count').textContent =
    historyTotal + t('status.count') + (historyHasFilters() ? t('status.filtered') : '');

  if (allHistory.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + t('empty.noHistory') + '</td></tr>';
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
    const detailsKnown = h.details_known !== false;
    const streamLabel = detailsKnown
      ? (h.streaming ? t('history.streaming') : t('history.nonStreaming'))
      : t('detail.unknown');
    const totalTokens = Number(h.input_tokens || 0) + Number(h.output_tokens || 0)
      + Number(h.cache_read_tokens || 0) + Number(h.cache_creation_tokens || 0);
    return `
    <tr data-id="${escapeHtml(rowId)}" tabindex="0" aria-haspopup="dialog" style="cursor: pointer;">
      <td>${fmtTime(h.start_time)}</td>
      <td><div class="history-status-stack">${detailsKnown ? `<span class="badge ${h.success ? 'badge-success' : 'badge-error'}">${h.success ? t('badge.success') : t('badge.fail')}</span>` : `<span class="badge badge-unknown">${t('detail.unknown')}</span>`}<small class="history-stream-state">${streamLabel}</small></div></td>
      <td><div class="history-model-cell"><strong>${escapeHtml(h.model) || '—'}</strong><small>${escapeHtml(h.provider) || '—'}</small></div></td>
      <td><span class="badge badge-scene">${escapeHtml(h.scenario) || '—'}</span></td>
      <td><button type="button" class="history-token-trigger" data-token-id="${escapeHtml(rowId)}" aria-label="${t('detail.title')}">${totalTokens.toLocaleString()}</button></td>
      <td>${cost}</td>
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
  ['duration_ms', r => r.duration_ms],
  ['streaming', r => r.streaming],
  ['attempt', r => r.attempt],
  ['success', r => r.success],
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
    const rows = [];
    for (let page = 1; ; page++) {
      const r = await fetch(`/api/history?${historyQueryParams(page, size)}`);
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

function fillRecentDailyTrend(points, days) {
  const byDate = new Map((points || []).map(point => [point.date, point]));
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - Math.max(0, days - 1));
  const result = [];
  while (start <= end) {
    const date = dateInputValue(start);
    result.push(byDate.get(date) || {date, requests:0, known_requests:0, error_requests:0, input_tokens:0, output_tokens:0, cache_read_tokens:0, cache_creation_tokens:0, cost_usd:0});
    start.setDate(start.getDate() + 1);
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
    target.setAttribute('aria-valuetext', options.labelForIndex(currentIndex));
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

// Percentiles cannot be averaged across models: a model with one request would
// otherwise weigh as much as one with 100k. Weighting each model's p95 by its
// request count is still an approximation (the true p95 needs the merged
// distribution), so the worst single-model p95 is surfaced alongside it.
function aggregateP95(stats) {
  const rows = (stats || []).filter(st => st && (st.p95_ms || 0) > 0);
  if (!rows.length) return '—';

  let weighted = 0;
  let totalCount = 0;
  let worst = 0;
  for (const st of rows) {
    const count = Number(st.count) || 0;
    const p95 = Number(st.p95_ms) || 0;
    weighted += p95 * count;
    totalCount += count;
    if (p95 > worst) worst = p95;
  }

  // No usable counts (older rows) — fall back to the worst case rather than a
  // misleading equal-weight mean.
  if (totalCount <= 0) return Math.round(worst) + ' ms';

  const val = Math.round(weighted / totalCount);
  if (rows.length > 1 && worst > val) {
    return val + ' ms · max ' + Math.round(worst) + ' ms';
  }
  return val + ' ms';
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
const CONFIG_FIELDS = [
  // Server
  ['host', 'cfg-host', 'string'],
  ['port', 'cfg-port', 'int'],
  ['api_key', 'cfg-global-key', 'string'],
  ['hot_reload', 'cfg-hot-reload', 'bool'],

  // OpenCode Go
  ['opencode_go.base_url', 'cfg-go-base-url', 'string'],
  ['opencode_go.anthropic_base_url', 'cfg-go-anthropic-url', 'string'],
  ['opencode_go.api_key', 'cfg-go-api-key', 'string'],
  ['opencode_go.timeout_ms', 'cfg-go-timeout', 'int'],
  ['opencode_go.stream_timeout_ms', 'cfg-go-stream-timeout', 'int'],

  // OpenCode Zen
  ['opencode_zen.base_url', 'cfg-zen-base-url', 'string'],
  ['opencode_zen.anthropic_base_url', 'cfg-zen-anthropic-url', 'string'],
  ['opencode_zen.responses_base_url', 'cfg-zen-responses-url', 'string'],
  ['opencode_zen.gemini_base_url', 'cfg-zen-gemini-url', 'string'],
  ['opencode_zen.api_key', 'cfg-zen-api-key', 'string'],
  ['opencode_zen.timeout_ms', 'cfg-zen-timeout', 'int'],
  ['opencode_zen.stream_timeout_ms', 'cfg-zen-stream-timeout', 'int'],

  // AWS Bedrock
  ['aws_bedrock.base_url', 'cfg-bedrock-base-url', 'string'],
  ['aws_bedrock.anthropic_base_url', 'cfg-bedrock-anthropic-url', 'string'],
  ['aws_bedrock.api_key', 'cfg-bedrock-api-key', 'string'],
  ['aws_bedrock.project_id', 'cfg-bedrock-project-id', 'string'],
  ['aws_bedrock.timeout_ms', 'cfg-bedrock-timeout', 'int'],
  ['aws_bedrock.stream_timeout_ms', 'cfg-bedrock-stream-timeout', 'int'],

  // Logging
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
    const v = raw.trim() === '' ? undefined : parseInt(raw, 10);
    const current = deepGet(currentProxyConfig, field[0]);
    return v === current ? undefined : v;
  }
  // string
  const v = raw;
  const current = deepGet(currentProxyConfig, field[0]);
  return v === (current || '') ? undefined : v;
}

async function loadProxyConfig() {
  try {
    const r = await fetch('/api/proxy/config');
    if (!r.ok) return;
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
      } else {
        el.value = val || '';
      }
    }
  } catch (e) {
    console.error('Failed to load proxy config:', e);
  }
}

async function saveProxyConfig() {
  if (!currentProxyConfig) {
    showSaveStatus('Config not loaded, cannot save', 'error');
    return;
  }

  const saveBtn = document.getElementById('btn-save-cfg');
  saveBtn.disabled = true;
  saveBtn.textContent = 'Saving...';

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
    showSaveStatus('No changes to save', 'success');
    saveBtn.disabled = false;
    saveBtn.textContent = 'Save & Apply Config';
    return;
  }

  try {
    const r = await fetch('/api/proxy/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    });

    if (r.ok) {
      showSaveStatus('Config saved successfully!', 'success');
      // Reload the full config from the server to stay in sync.
      await loadProxyConfig();
    } else {
      const txt = await r.text();
      showSaveStatus('Save failed: ' + txt, 'error');
    }
  } catch (e) {
    showSaveStatus('Network error, save failed', 'error');
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = 'Save & Apply Config';
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
  historyPage = 1;
  if (historyRefreshTimer) clearTimeout(historyRefreshTimer);
  historyRefreshTimer = setTimeout(() => {
    historyRefreshTimer = null;
    refreshHistory();
  }, 250);
}

['history-search', 'model-filter', 'provider-filter', 'scenario-filter'].forEach(id => {
  document.getElementById(id)?.addEventListener('input', scheduleHistoryRefresh);
});
['history-start', 'history-end', 'status-filter', 'streaming-filter'].forEach(id => {
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
    refreshHistory();
  });
});

/* ── History Detail Modal ──────────────────────────────────────── */
const modal = document.getElementById('history-modal');
const modalBody = document.getElementById('modal-body');
const modalClose = document.getElementById('modal-close');
let modalReturnFocus = null;

function showHistoryDetail(record) {
  const tokenValue = value => value != null ? Number(value).toLocaleString() : '—';
  const detailsKnown = record.details_known !== false;
  const statusLabel = detailsKnown ? (record.success ? t('detail.success') : t('detail.failed')) : t('detail.unknown');
  modalBody.innerHTML = `
    <div class="detail-summary">
      <div>
        <div class="detail-model">${escapeHtml(record.model || '—')}</div>
        <div class="detail-context">
          <span>${escapeHtml(record.provider || '—')}</span>
          <span>${escapeHtml(record.scenario || '—')}</span>
          <span>${fmtTime(record.start_time)}</span>
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
      <div class="detail-row"><span class="detail-label">${t('detail.requestType')}</span><span class="detail-value">${detailsKnown ? t(record.streaming ? 'detail.streaming' : 'detail.nonStreaming') : t('detail.unavailable')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.attempt')}</span><span class="detail-value">${detailsKnown ? (record.attempt || 1) : t('detail.unavailable')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.duration')}</span><span class="detail-value">${detailsKnown ? fmtDuration(record.duration_ms) : t('detail.unavailable')}</span></div>
    </div>
    ${record.error_msg ? `<div class="detail-error"><strong>${t('detail.error')}</strong><br>${escapeHtml(record.error_msg)}</div>` : ''}
  `;
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
  // Tab shortcuts: Cmd/Ctrl + 1/2/3/4/5/6
  if ((e.metaKey || e.ctrlKey) && ['1', '2', '3', '4', '5', '6'].includes(e.key)) {
    e.preventDefault();
    const tabs = ['overview', 'history', 'performance', 'fallback', 'analytics', 'settings'];
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
  const anonymize = document.getElementById('export-anonymize').checked;
  const btn = document.getElementById('btn-export-config');
  btn.disabled = true;
  btn.textContent = t('status.exporting');

  try {
    const url = '/api/config/export?anonymize=' + anonymize;
    const response = await fetch(url);
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
  if (!file || file.type !== 'application/json') {
    showSaveStatus(t('status.importInvalid'), 'error');
    return;
  }

  const btn = document.getElementById('btn-import-config');
  btn.disabled = true;
  btn.textContent = t('status.importing');

  try {
    const content = await file.text();
    const config = JSON.parse(content);

    const previewHtml = `
      <div class="detail-row">
        <span class="detail-label">${t('modal.importConfirm')}</span>
      </div>
      <pre style="max-height: 300px; overflow: auto; background: #3a3a3c; padding: 12px; border-radius: 4px; font-size: 11px; white-space: pre-wrap; word-break: break-all;">${escapeHtml(JSON.stringify(config, null, 2))}</pre>
    `;

    modalBody.innerHTML = previewHtml;
    document.getElementById('modal-title').textContent = t('modal.importPreview');

    const footerHtml = `
      <div style="padding: 12px 16px; display: flex; gap: 8px; justify-content: flex-end; border-top: 1px solid #48484a;">
        <button class="btn btn-small" id="btn-import-cancel">${t('btn.cancel')}</button>
        <button class="btn btn-small btn-primary" id="btn-import-apply">${t('btn.apply')}</button>
      </div>
    `;

    const existingFooter = modal.querySelector('.modal-footer');
    if (existingFooter) existingFooter.remove();

    modal.querySelector('.modal-content').insertAdjacentHTML('beforeend', footerHtml);

    modal.classList.add('visible');

    document.getElementById('btn-import-cancel').onclick = () => {
      modal.classList.remove('visible');
      const footer = modal.querySelector('.modal-footer');
      if (footer) footer.remove();
    };

    document.getElementById('btn-import-apply').onclick = async () => {
      try {
        const response = await fetch('/api/config/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ config: config, apply: true })
        });

        if (!response.ok) {
          throw new Error(await response.text());
        }

        modal.classList.remove('visible');
        const footer = modal.querySelector('.modal-footer');
        if (footer) footer.remove();

        showSaveStatus(t('status.importOk'), 'success');
        await loadProxyConfig();
      } catch (e) {
        showSaveStatus(t('status.importFail') + e.message, 'error');
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

      // Build model list from scenario models + fallback entries
      const modelMap = new Map();
      if (config.models) {
        for (const [, m] of Object.entries(config.models)) {
          if (m.model_id && !modelMap.has(m.model_id)) {
            modelMap.set(m.model_id, { id: m.model_id, display_name: m.model_id, provider: m.provider || 'unknown' });
          }
        }
      }
      if (config.fallbacks) {
        for (const models of Object.values(config.fallbacks)) {
          for (const m of models) {
            if (m.model_id && !modelMap.has(m.model_id)) {
              modelMap.set(m.model_id, { id: m.model_id, display_name: m.model_id, provider: m.provider || 'unknown' });
            }
          }
        }
      }
      this.availableModels = [...modelMap.values()];

      // Discover all scenario keys from config.models
      const scenarioKeys = Object.keys(config.models || {});
      this.chains = {};
      for (const key of scenarioKeys) {
        this.chains[key] = this.parseFallbackChain(config, key);
      }

      // Populate scenario dropdown
      const sel = document.getElementById('fallback-scenario');
      if (sel) {
        sel.innerHTML = scenarioKeys.map(k =>
          `<option value="${k}">${k.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}</option>`
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
      .filter(m => !chain.some(e => (e.model_id || e) === m.id));
    addSel.innerHTML = '<option value="">' + t('fallback.selectModel') + '</option>' +
      available.map(m =>
        `<option value="${escapeHtml(m.id)}">${escapeHtml(m.display_name || m.id)} (${escapeHtml(m.provider)})</option>`
      ).join('');
    addSel.disabled = available.length === 0;
  },

  onAddSelectChange() {
    const addSel = document.getElementById('fallback-add-model');
    const modelId = addSel.value;
    if (!modelId) return;
    const model = this.availableModels.find(m => m.id === modelId);
    if (model) {
      (this.chains[this.currentScenario] || []).push({ model_id: modelId, provider: model.provider, temperature: 0, max_tokens: 0 });
      this.renderChain();
    }
    addSel.value = '';
    this.populateAddSelect();
  },

  renderChain() {
    const list = document.getElementById('fallback-chain');
    const chain = this.chains[this.currentScenario];

    if (!chain || chain.length === 0) {
      list.innerHTML = '<li class="empty-state">' + t('fallback.empty') + '</li>';
      list.classList.remove('has-items');
      this.populateAddSelect();
      return;
    }

    list.classList.add('has-items');
    list.innerHTML = chain.map((entry, index) => {
      const modelId = entry.model_id || entry;
      const model = this.availableModels.find(m => m.id === modelId);
      const displayName = model ? (model.display_name || model.id) : modelId;
      const provider = entry.provider || (model ? model.provider : '');
      return `
        <li class="fallback-item" draggable="true" data-index="${index}" role="option">
          <span class="handle">⋮⋮</span>
          <span class="model-name">${escapeHtml(displayName)}</span>
          ${provider ? '<span class="model-meta">' + escapeHtml(provider) + '</span>' : ''}
          <button class="remove-btn" onclick="FallbackModule.removeModel(${index})" title="Remove model" aria-label="Remove ${escapeHtml(displayName)}">×</button>
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
    const fromIndex = parseInt(e.dataTransfer.getData('text/plain'), 10);
    const toIndex = parseInt(e.currentTarget.dataset.index, 10);

    e.currentTarget.classList.remove('drag-over');

    if (fromIndex !== toIndex) {
      const chain = this.chains[this.currentScenario];
      const [removed] = chain.splice(fromIndex, 1);
      chain.splice(toIndex, 0, removed);
      this.renderChain();
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
    }
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
          const model = this.availableModels.find(m => m.id === modelId);
          const displayName = model ? (model.display_name || model.id) : modelId;
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
      showSaveStatus(t('fallback.noChanges'), 'success');
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
        showSaveStatus(t('fallback.saved'), 'success');
        this.originalChains = JSON.parse(JSON.stringify(this.chains));
        await loadProxyConfig();
      } else {
        const txt = await r.text();
        showSaveStatus(t('fallback.saveFailed') + ': ' + txt, 'error');
      }
    } catch (e) {
      showSaveStatus(t('fallback.saveFailed'), 'error');
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
  activateTab((location.hash || '').replace(/^#/, '') || 'overview');
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
      const modelIds = new Set();
      // Only collect model_overrides and model_family_overrides — the
      // top-level "models" keys are routing scenarios (fast, default,
      // long_context, etc.), not real model IDs.
      Object.keys(data.model_overrides || {}).forEach(k => modelIds.add(k));
      Object.keys(data.model_family_overrides || {}).forEach(k => modelIds.add(k));
      [...modelIds].sort().forEach(id => {
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = id;
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
  palette: ['#818cf8', '#34d399', '#fbbf24', '#fb7185', '#22d3ee', '#c084fc'],
  loadSeq: 0,
  ready: false,
  breakdownMetric: 'tokens',
  granularity: 'day',
  currentView: null,
  currentTrend: [],
  seriesVisibility: {},

  init() {
    const refreshBtn = document.getElementById('btn-refresh-analytics');
    if (refreshBtn) refreshBtn.addEventListener('click', () => this.load(true));
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
        refreshMetrics();
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
    start.setDate(start.getDate() - 6);
    document.getElementById('analytics-start').value = dateInputValue(start);
    document.getElementById('analytics-end').value = dateInputValue(end);
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
    if (label) label.textContent = start && end ? `${start} → ${end}` : t('filter.dateRange');
  },

  presetDateRange(days) {
    const end = new Date();
    const start = new Date(end);
    start.setDate(start.getDate() - Math.max(0, days - 1));
    document.getElementById('analytics-start-display').value = dateInputValue(start);
    document.getElementById('analytics-end-display').value = dateInputValue(end);
  },

  applyDateRange() {
    const start = document.getElementById('analytics-start-display');
    const end = document.getElementById('analytics-end-display');
    const valid = value => /^\d{4}-\d{2}-\d{2}$/.test(value) && dateInputValue(new Date(`${value}T00:00:00`)) === value;
    start.classList.toggle('invalid', !valid(start.value));
    end.classList.toggle('invalid', !valid(end.value));
    if (!valid(start.value) || !valid(end.value) || start.value > end.value) return;
    const span = (new Date(`${end.value}T00:00:00`) - new Date(`${start.value}T00:00:00`)) / 86400000 + 1;
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
    const from = new Date(`${start}T00:00:00`);
    const to = new Date(`${end}T00:00:00`);
    to.setDate(to.getDate() + 1);
    return new URLSearchParams({
      from: from.toISOString(),
      to: to.toISOString(),
      granularity: this.granularity,
    });
  },

  loadingHtml() {
    return '<div class="flex items-center justify-center py-12"><svg class="animate-spin h-6 w-6 text-[#98989d]" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg><span class="ml-2 text-xs text-[#98989d]">Loading analytics…</span></div>';
  },

  async load(force) {
    const seq = ++this.loadSeq;
    const params = this.queryParams();
    const genEl = document.getElementById('analytics-generated');
    if (!this.ready) {
      if (genEl) genEl.textContent = '';
      ['kpi-requests','kpi-tokens','kpi-cost','kpi-input','kpi-cache-rate','kpi-output','kpi-cache-read','kpi-cache-write'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.textContent = '…';
      });
      this.showLoading();
    }

    try {
      const [summaryRes, trendRes] = await Promise.all([
        fetch(`/api/analytics/summary?${params}`),
        fetch(`/api/analytics/tokens/trend?${params}`)
      ]);
      if (!summaryRes.ok) throw new Error('summary fetch failed');
      const summary = await summaryRes.json();
      const trend = trendRes.ok ? await trendRes.json() : { trend: [] };

      if (seq !== this.loadSeq) return;
      this.currentView = summary;
      this.currentTrend = this.fillTrend(trend.trend || []);
      this.renderKPIs(summary);
      this.renderDistributions(summary);
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
      if (!this.ready) this.renderEmpty('Failed to load analytics');
      if (genEl) genEl.textContent = 'Error';
    }
  },

  showLoading() {
    ['provider-distribution'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = this.loadingHtml();
    });
  },

  renderKPIs(data) {
    const s = data.summary || {};
    const fmt = (n) => n != null ? Number(n).toLocaleString() : '—';
    document.getElementById('kpi-requests').textContent = fmt(s.total_requests);
    // Total tokens = input (non-cached new) + output + cache (read+creation).
    // This matches the billing structure and avoids underreporting when a large
    // fraction of input is served from cache at lower cost.
    const totTok = (s.input_tokens||0) + (s.output_tokens||0)
      + (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    document.getElementById('kpi-tokens').textContent = fmtTok(totTok);
    document.getElementById('kpi-tokens-note').textContent = t('analytics.knownRecords').replace('{n}', Number(s.known_requests || 0).toLocaleString());
    document.getElementById('kpi-input').textContent = fmtTok(s.input_tokens);
    document.getElementById('kpi-output').textContent = fmtTok(s.output_tokens);

    // Cache hit rate is an input-side ratio: the denominator is everything that
    // arrived as prompt (fresh input + cache read + cache creation). Dividing by
    // total tokens instead would fold output in and drift with response length.
    const promptTok = (s.input_tokens||0) + (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    document.getElementById('kpi-cost').textContent = fmtCost(s.cost_usd ?? s.est_cost_usd ?? 0);
    document.getElementById('kpi-cache-read').textContent = fmtTok(s.cache_read_tokens || 0);
    document.getElementById('kpi-cache-rate').textContent = promptTok > 0
      ? `${((s.cache_read_tokens || 0) / promptTok * 100).toFixed(1)}%`
      : '—';
    document.getElementById('kpi-cache-rate-note').textContent = `${fmtTok(s.cache_read_tokens || 0)} ${currentLang === 'zh' ? '读取' : 'read'}`;
    document.getElementById('kpi-cache-write').textContent = fmtTok(s.cache_creation_tokens || 0);
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
    })).sort((a, b) => Number(b[valueKey] || 0) - Number(a[valueKey] || 0)).slice(0, 12);
    root.classList.toggle('is-single', normalized.length === 1);
    if (!normalized.length) {
      root.innerHTML = `<div class="empty-state">${t('analytics.noData')}</div>`;
      return;
    }
    const max = Math.max(1, ...normalized.map(item => Number(item[valueKey] || 0)));
    const total = Math.max(1, normalized.reduce((sum, item) => sum + Number(item[valueKey] || 0), 0));
    const formatValue = value => valueKey === 'cost_usd' ? fmtCost(value)
      : valueKey === 'requests' ? Number(value || 0).toLocaleString() : fmtTok(value);
    root.innerHTML = normalized.map((item, index) => {
      const rawLabel = dimension === 'model' ? item.model : item.provider;
      const label = !rawLabel || rawLabel === 'unknown' ? t('detail.unknown') : rawLabel;
      const value = Number(item[valueKey] || 0);
      const share = value / total * 100;
      const meta = `${Number(item.requests || 0).toLocaleString()} ${t('analytics.requests')} · ${fmtTok(item.total_tokens)} Token · ${fmtCost(item.cost_usd)}`;
      return `<div class="analytics-distribution-row">
        <div class="analytics-distribution-label"><span title="${this.escapeHtml(label)}">${this.escapeHtml(label)}</span><strong>${formatValue(value)}</strong></div>
        <div class="analytics-distribution-track"><span style="width:${Math.max(value > 0 ? 2 : 0, value / max * 100).toFixed(1)}%;--distribution-color:${this.palette[index % this.palette.length]}"></span></div>
        <small>${meta} · ${share.toFixed(1)}%</small>
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
      const cursor = new Date(`${startValue}T00:00:00`);
      const end = new Date(`${endValue}T00:00:00`);
      end.setDate(end.getDate() + 1);
      while (cursor < end) {
        const key = cursor.toISOString().slice(0, 13) + ':00:00Z';
        rows.push(byKey.get(key) || blank(key));
        cursor.setHours(cursor.getHours() + 1);
      }
      return rows;
    }
    const cursor = new Date(`${startValue}T00:00:00`);
    const end = new Date(`${endValue}T00:00:00`);
    while (cursor <= end) {
      const key = dateInputValue(cursor);
      rows.push(byKey.get(key) || blank(key));
      cursor.setDate(cursor.getDate() + 1);
    }
    return rows;
  },

  trendLabel(value) {
    const date = String(value || '');
    if (date.includes('T')) {
      return new Date(date).toLocaleString(undefined, {month:'2-digit', day:'2-digit', hour:'2-digit'});
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
      labelForIndex: index => this.trendLabel(points[index].date),
      markersForIndex: index => visible.map(item => ({x:chart.x(index),y:chart.y(points[index][item.key])})),
      contentForIndex: index => {
      const point=points[index];
      return chartTooltipMarkup(this.trendLabel(point.date), visible.map(item => ({label:item.label,value:Number(point[item.key]||0).toLocaleString(),color:item.color})), [{label:t('analytics.cost'),value:fmtCost(point.cost_usd||0)}]);
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
      labelForIndex: index => this.trendLabel(points[index].date),
      markersForIndex: index => visible.map(item => ({x:chart.x(index),y:item.rate ? rateY(rate(points[index])) : chart.y(points[index][item.key])})),
      contentForIndex: index => {
      const point=points[index];
      const footer = [
        {label:t('analytics.requests'),value:Number(point.requests||0).toLocaleString()},
        {label:t('analytics.cost'),value:fmtCost(point.cost_usd||0)},
      ];
      if (showRate) footer.unshift({label:t('analytics.cacheHitShort'),value:`${rate(point).toFixed(1)}%`});
      return chartTooltipMarkup(this.trendLabel(point.date),visibleTokens.map(item=>({label:item.label,value:Number(point[item.key]||0).toLocaleString(),color:item.color})),footer);
      },
    });
  },

  renderPeriodTable(points) {
    const body=document.getElementById('analytics-period-tbody');
    if (!body) return;
    body.innerHTML=points.map(point=>`<tr><td>${this.escapeHtml(this.trendLabel(point.date))}</td><td>${Number(point.requests||0).toLocaleString()}</td><td>${Number(point.known_requests||0)>0?Number(point.error_requests||0).toLocaleString():'—'}</td><td>${Number(point.input_tokens||0).toLocaleString()}</td><td>${Number(point.output_tokens||0).toLocaleString()}</td><td>${Number(point.cache_read_tokens||0).toLocaleString()}</td><td>${Number(point.cache_creation_tokens||0).toLocaleString()}</td><td><strong>${totalUsageTokens(point).toLocaleString()}</strong></td><td>${fmtCost(point.cost_usd||0)}</td></tr>`).join('');
    const count=document.getElementById('analytics-period-count');
    if (count) count.textContent=`${points.length} ${this.granularity==='hour'?t('analytics.hour'):t('analytics.day')}`;
  },

  renderModelTable(models) {
    const body=document.getElementById('analytics-model-tbody');
    if (!body) return;
    body.innerHTML=(models||[]).map(item=>{
      const prompt=Number(item.input_tokens||0)+Number(item.cache_read_tokens||0)+Number(item.cache_creation_tokens||0);
      const rate=prompt>0?Number(item.cache_read_tokens||0)/prompt*100:0;
      return `<tr><td><code>${this.escapeHtml(item.model||t('detail.unknown'))}</code></td><td>${Number(item.requests||0).toLocaleString()}</td><td>${prompt>0?rate.toFixed(1)+'%':'—'}</td><td>${totalUsageTokens(item).toLocaleString()}</td><td>${fmtCost(item.est_cost_usd||0)}</td></tr>`;
    }).join('') || `<tr><td colspan="5" class="empty-state">${t('analytics.noData')}</td></tr>`;
  },

  renderRetainedRange(summary) {
    const root=document.getElementById('analytics-retained-range');
    if (!root) return;
    const from=document.getElementById('analytics-start')?.value||'';
    const to=document.getElementById('analytics-end')?.value||'';
    root.textContent=t('analytics.retainedRange').replace('{from}',from).replace('{to}',to);
  },

  renderEmpty(msg = 'No usage data yet. Run some requests or configure a model to see analytics.') {
    ['provider-distribution'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = `<div class="empty-state">${msg}</div>`;
    });
  },

  escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
  }
};

// Initialize dates and listeners before the queued hash activation runs.
AnalyticsModule.init();
