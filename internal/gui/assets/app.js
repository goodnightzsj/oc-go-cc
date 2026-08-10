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
    'analytics.costEstimated': 'Estimated',
    'analytics.costProvider': 'Provider-reported',
    'analytics.costMixed': '{provider}/{total} provider-reported',
    'analytics.costSourceProvider': 'Provider-reported cost',
    'analytics.costSourceEstimated': 'Estimated cost',
    'analytics.p95Latency': 'p95 Latency',
    'analytics.latencyUnit': 'request-weighted',
    'analytics.cacheHitShort': 'cache hit',
    'analytics.cacheHitTitle': 'Cache read as a share of all prompt tokens (input + cache read + cache write)',
    'analytics.byModel': 'Tokens by Model',
    'analytics.byProvider': 'Tokens by Provider',
    'analytics.noData': 'No data',
    'analytics.noTrend': 'No trend data',
    'analytics.singleDay': '(single-day data)',
    'analytics.dailyTrend': 'Daily Token Trend',
    'analytics.last7d': 'Last 7 days',
    'analytics.last30d': 'Last 30 days',
    'analytics.last90d': 'Last 90 days',
    'analytics.inputTokens': 'Input tokens',
    'analytics.outputTokens': 'Output tokens',
    'analytics.cacheTokens': 'Cache tokens',
    'analytics.sourceOpenCode': 'Source: OpenCode usage',
    'analytics.sourceLocalFallback': 'Source: local proxy (OpenCode snapshot unavailable)',
    'analytics.byPlan': 'Usage by Plan',
    'analytics.recentUsage': 'Recent Usage',
    'analytics.reasoningTokens': 'Reasoning Tokens',
    'analytics.reasoningNote': 'included in output',
    'analytics.capturedAt': 'Captured {time}',
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
    'filter.dateRange': 'Date range',
    'filter.today': 'Today',
    'filter.clear': 'Clear',
    'filter.apply': 'Apply',
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
    'filter.allCostSources': 'All cost sources',
    'filter.providerCost': 'Provider-reported',
    'filter.estimatedCost': 'Estimated',
    'filter.reset': 'Reset',
    'history.title': 'Request History',
    'history.searchPlaceholder': 'Search ID, model, provider, scenario, or error…',
    'analytics.viewRequests': 'View requests',
    'th.time': 'Time',
    'th.model': 'Model',
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
    'analytics.costEstimated': '预估',
    'analytics.costProvider': '供应商原始费用',
    'analytics.costMixed': '{provider}/{total} 条为供应商原始费用',
    'analytics.costSourceProvider': '供应商原始费用',
    'analytics.costSourceEstimated': '预估费用',
    'analytics.p95Latency': 'p95 延迟',
    'analytics.latencyUnit': '按请求数加权',
    'analytics.cacheHitShort': '缓存命中',
    'analytics.cacheHitTitle': '缓存读取占全部输入 Token 的比例（输入 + 缓存读 + 缓存写）',
    'analytics.byModel': '按模型 Token 用量',
    'analytics.byProvider': '按供应商 Token 用量',
    'analytics.noData': '暂无数据',
    'analytics.noTrend': '暂无趋势数据',
    'analytics.singleDay': '（仅单日数据）',
    'analytics.dailyTrend': '每日 Token 趋势',
    'analytics.last7d': '最近 7 天',
    'analytics.last30d': '最近 30 天',
    'analytics.last90d': '最近 90 天',
    'analytics.inputTokens': '输入 Token',
    'analytics.outputTokens': '输出 Token',
    'analytics.cacheTokens': '缓存 Token',
    'analytics.cacheTokensLegend': '缓存',
    'analytics.sourceOpenCode': '数据源：OpenCode 用量账单',
    'analytics.sourceLocalFallback': '数据源：本地代理（尚未导入 OpenCode 基线）',
    'analytics.byPlan': '按套餐拆分',
    'analytics.recentUsage': '最近使用',
    'analytics.reasoningTokens': '推理 Token',
    'analytics.reasoningNote': '已包含在输出 Token 中',
    'analytics.capturedAt': '采集于 {time}',
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
    'filter.dateRange': '日期范围',
    'filter.today': '今天',
    'filter.clear': '清除',
    'filter.apply': '应用',
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
    'filter.allCostSources': '全部费用来源',
    'filter.providerCost': '供应商原始费用',
    'filter.estimatedCost': '预估费用',
    'filter.reset': '重置',
    'history.title': '请求记录',
    'history.searchPlaceholder': '搜索请求 ID、模型、供应商、场景或错误…',
    'analytics.viewRequests': '查看请求',
    'th.time': '时间',
    'th.model': '模型',
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
  // Analytics charts (trend legend, donuts) carry inline strings; reload them
  // so they pick up the new language instead of keeping stale ones.
  if (activeTab === 'analytics') AnalyticsModule.load(true);
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
// Upper bound on how many history rows are rendered into the DOM at once.
// Keeps long-session history tables fast while the count reflects all rows.
const HISTORY_RENDER_LIMIT = 200;

function startPolling() {
  refreshAll();
  PerfModule.init();
  PerfModule.refresh();
  // Core metrics always refresh so overview badges stay warm; history,
  // config and catalog-age are only refreshed when their tab is active so we
  // don't rebuild DOM or hammer SQLite on views the user isn't looking at.
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

// Refresh only what the current tab needs. Turns expensive/rare refreshes
// (config, catalog-age) off when the user is on Overview/History/Analytics,
// and gates history's DOM-heavy render to the History tab.
async function refreshCurrentTab() {
  switch (activeTab) {
    case 'history':
      await Promise.all([refreshMetrics(), refreshHistory(), refreshCatalogAge()]);
      break;
    case 'settings':
      await Promise.all([refreshMetrics(), refreshConfig(), refreshCatalogAge()]);
      break;
    case 'analytics':
    case 'performance':
    case 'fallback':
      await refreshMetrics();
      break;
    case 'overview':
    default:
      await refreshMetrics();
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
    const r = await fetch('/api/metrics');
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

    // metric cards
    document.getElementById('m-total').textContent   = fmt(d.requests_received);
    document.getElementById('m-success').textContent = fmt(d.requests_success);
    document.getElementById('m-failed').textContent  = fmt(d.requests_failed);
    document.getElementById('m-streamed').textContent = fmt(d.requests_streamed);

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
    cost_source: document.getElementById('cost-source-filter')?.value,
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

function renderHistorySummary(summary) {
  const set = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };
  set('history-summary-requests', Number(summary.total_requests || 0).toLocaleString());
  set('history-summary-success', `${(Number(summary.success_rate || 0) * 100).toFixed(1)}%`);
  set('history-summary-tokens', fmtTok(Number(summary.total_tokens || 0)));
  set('history-summary-cost', fmtCost(Number(summary.cost_usd || 0)));
  renderCompactBreakdown('history-model-breakdown', summary.models || []);
  renderCompactBreakdown('history-provider-breakdown', summary.providers || []);
  renderCompactBreakdown('history-scenario-breakdown', summary.scenarios || []);
  renderHistoryMiniTrend(summary.trend || []);
}

function renderCompactBreakdown(id, items) {
  const root = document.getElementById(id);
  if (!root) return;
  const top = [...items].sort((a, b) => (b.tokens || 0) - (a.tokens || 0)).slice(0, 4);
  if (!top.length) {
    root.innerHTML = `<span class="compact-breakdown-value">${t('analytics.noData')}</span>`;
    return;
  }
  const max = Math.max(1, ...top.map(item => Number(item.tokens || 0)));
  root.innerHTML = top.map(item => `
    <div class="compact-breakdown-row" title="${escapeHtml(item.name)} · ${Number(item.requests || 0).toLocaleString()} ${t('analytics.requests')}">
      <span class="compact-breakdown-name">${escapeHtml(item.name)}</span>
      <span class="compact-breakdown-track"><i class="compact-breakdown-fill" style="width:${Math.max(3, Number(item.tokens || 0) / max * 100).toFixed(1)}%"></i></span>
      <span class="compact-breakdown-value">${fmtTok(Number(item.tokens || 0))}</span>
    </div>`).join('');
}

function renderHistoryMiniTrend(points) {
  const root = document.getElementById('history-filter-trend');
  if (!root) return;
  if (!points.length) {
    root.innerHTML = '';
    return;
  }
  const width = 760, height = 72, pad = 4;
  const max = Math.max(1, ...points.map(point => Number(point.tokens || 0)));
  const slot = (width - pad * 2) / points.length;
  const bars = points.map((point, index) => {
    const barHeight = Math.max(2, Number(point.tokens || 0) / max * (height - 16));
    const x = pad + index * slot + slot * .16;
    const y = height - barHeight - 4;
    return `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${Math.max(2, slot * .68).toFixed(1)}" height="${barHeight.toFixed(1)}" rx="1"><title>${escapeHtml(point.date)} · ${Number(point.tokens || 0).toLocaleString()} Token</title></rect>`;
  }).join('');
  root.innerHTML = `<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img">${bars}</svg>`;
}

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
    tbody.innerHTML = '<tr><td colspan="8" class="empty-state">' + t('empty.noHistory') + '</td></tr>';
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
    const costSource = h.cost_source === 'provider' ? 'provider' : 'estimated';
    const costSourceLabel = t(costSource === 'provider' ? 'analytics.costSourceProvider' : 'analytics.costSourceEstimated');
    const cost = h.cost_usd != null
      ? `<span class="cost-value"><span class="cost-source-dot ${costSource === 'provider' ? 'provider' : ''}" title="${escapeHtml(costSourceLabel)}" aria-label="${escapeHtml(costSourceLabel)}"></span>${fmtCost(h.cost_usd)}</span>`
      : '—';
    return `
    <tr data-id="${escapeHtml(rowId)}" tabindex="0" aria-haspopup="dialog" style="cursor: pointer;">
      <td>${fmtTime(h.start_time)}</td>
      <td><span title="${escapeHtml(h.provider || '')}">${escapeHtml(h.model) || '—'}</span></td>
      <td><span class="badge badge-scene">${escapeHtml(h.scenario) || '—'}</span></td>
      <td>${h.prompt_tokens != null ? h.prompt_tokens.toLocaleString() : '—'}</td>
      <td>${h.output_tokens != null ? h.output_tokens.toLocaleString() : '—'}</td>
      <td>${cost}</td>
      <td>${fmtDuration(h.duration_ms)}</td>
      <td><span class="badge ${h.success ? 'badge-success' : 'badge-error'}">${h.success ? t('badge.success') : t('badge.fail')}</span></td>
    </tr>
  `}).join('');

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
  ['cost_source', r => r.cost_source || ''],
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
['history-start', 'history-end', 'status-filter', 'streaming-filter', 'cost-source-filter'].forEach(id => {
  document.getElementById(id)?.addEventListener('change', scheduleHistoryRefresh);
});
document.getElementById('history-reset')?.addEventListener('click', () => resetHistoryFilters());

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
  const costSource = record.cost_source === 'provider' ? 'provider' : 'estimated';
  const costSourceLabel = t(costSource === 'provider' ? 'analytics.costSourceProvider' : 'analytics.costSourceEstimated');
  const tokenValue = value => value != null ? Number(value).toLocaleString() : '—';
  const statusLabel = record.success ? t('detail.success') : t('detail.failed');
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
        <span class="detail-status ${record.success ? 'success' : ''}">${statusLabel}</span>
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
      <div class="detail-row"><span class="detail-label">${t('detail.requestType')}</span><span class="detail-value">${t(record.streaming ? 'detail.streaming' : 'detail.nonStreaming')}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.attempt')}</span><span class="detail-value">${record.attempt || 1}</span></div>
      <div class="detail-row"><span class="detail-label">${t('detail.duration')}</span><span class="detail-value">${fmtDuration(record.duration_ms)}</span></div>
      <div class="detail-row"><span class="detail-label">${t('analytics.cost')}</span><span class="detail-value"><span class="cost-source-badge ${costSource === 'provider' ? 'provider' : ''}">${escapeHtml(costSourceLabel)}</span></span></div>
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
  refreshStorageKey: 'routatic-analytics-refresh-interval',
  refreshTimer: null,
  refreshInterval: 30,
  loadSeq: 0,
  ready: false,

  init() {
    const daysSel = document.getElementById('analytics-days');
    const refreshBtn = document.getElementById('btn-refresh-analytics');
    const intervalSel = document.getElementById('analytics-refresh-interval');
    if (daysSel) daysSel.addEventListener('change', () => this.load(true));
    if (refreshBtn) refreshBtn.addEventListener('click', () => this.load(true));
    this.refreshInterval = this.readRefreshInterval();
    if (intervalSel) {
      intervalSel.value = String(this.refreshInterval);
      intervalSel.addEventListener('change', () => {
        const value = Number(intervalSel.value);
        this.refreshInterval = [0, 5, 30, 60].includes(value) ? value : 30;
        try {
          localStorage.setItem(this.refreshStorageKey, String(this.refreshInterval));
        } catch (_) {}
        this.scheduleRefresh();
      });
    }
    document.addEventListener('visibilitychange', () => this.scheduleRefresh());
    this.scheduleRefresh();
  },

  readRefreshInterval() {
    try {
      const stored = localStorage.getItem(this.refreshStorageKey);
      const value = Number(stored);
      if (stored !== null && [0, 5, 30, 60].includes(value)) return value;
    } catch (_) {}
    return 30;
  },

  refreshPaused() {
    return activeTab !== 'analytics' || document.hidden ||
      !!document.querySelector('.modal-overlay.visible, dialog[open]');
  },

  scheduleRefresh() {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
      this.refreshTimer = null;
    }
    if (!this.refreshInterval) return;
    const delay = this.refreshPaused() ? 1000 : this.refreshInterval * 1000;
    this.refreshTimer = setTimeout(async () => {
      this.refreshTimer = null;
      if (!this.refreshPaused()) await this.load(false);
      this.scheduleRefresh();
    }, delay);
  },

  loadingHtml() {
    return '<div class="flex items-center justify-center py-12"><svg class="animate-spin h-6 w-6 text-[#98989d]" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path></svg><span class="ml-2 text-xs text-[#98989d]">Loading analytics…</span></div>';
  },

  async load(force) {
    const seq = ++this.loadSeq;
    const daysEl = document.getElementById('analytics-days');
    const days = daysEl ? daysEl.value : 30;
    const genEl = document.getElementById('analytics-generated');
    if (!this.ready) {
      if (genEl) genEl.textContent = '';
      ['kpi-requests','kpi-tokens','kpi-tokens-in','kpi-tokens-out','kpi-cost','kpi-reasoning'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.textContent = '…';
      });
      this.showLoading();
    }

    try {
      const [summaryRes, trendRes] = await Promise.all([
        fetch(`/api/analytics/summary?days=${days}`),
        fetch(`/api/analytics/tokens/trend?days=${days}`)
      ]);
      if (!summaryRes.ok) throw new Error('summary fetch failed');
      const summary = await summaryRes.json();
      const trend = trendRes.ok ? await trendRes.json() : { trend: [] };

      if (seq !== this.loadSeq) return;
      const account = summary.account || {};
      const hasAccount = Number(account.summary?.total_requests || 0) > 0;
      const view = hasAccount ? this.accountView(account) : summary;
      this.renderSource(account.summary, hasAccount);
      this.renderKPIs(view, hasAccount);
      this.renderDonuts(view);
      this.renderTrend(hasAccount ? (account.trend || []) : (trend.trend || []));
      this.renderRecent(hasAccount ? (account.recent || []) : []);
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
    } finally {
      if (seq === this.loadSeq) this.scheduleRefresh();
    }
  },

  showLoading() {
    ['model-donut','provider-donut','plan-donut','token-trend','analytics-recent'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = this.loadingHtml();
    });
  },

  accountView(account) {
    const normalize = (items, field) => (items || []).map(item => ({
      ...item,
      [field]: item.name,
      est_cost_usd: item.cost_usd,
      account_usage: true,
    }));
    return {
      summary: {
        ...(account.summary || {}),
        est_cost_usd: account.summary?.cost_usd || 0,
        provider_cost_rows: account.summary?.total_requests || 0,
        estimated_cost_rows: 0,
      },
      models: normalize(account.models, 'model'),
      providers: normalize(account.providers, 'provider'),
      plans: normalize(account.plans, 'plan'),
    };
  },

  renderSource(summary, hasAccount) {
    const source = document.getElementById('analytics-source');
    if (!source) return;
    source.classList.toggle('local', !hasAccount);
    if (!hasAccount) {
      source.textContent = t('analytics.sourceLocalFallback');
      return;
    }
    const captured = summary?.snapshot_at ? fmtTime(summary.snapshot_at) : '';
    source.textContent = `${t('analytics.sourceOpenCode')}${captured ? ` · ${t('analytics.capturedAt').replace('{time}', captured)}` : ''}`;
  },

  renderKPIs(data, hasAccount) {
    const s = data.summary || {};
    const fmt = (n) => n != null ? Number(n).toLocaleString() : '—';
    document.getElementById('kpi-requests').textContent = fmt(s.total_requests);
    // Total tokens = input (non-cached new) + output + cache (read+creation).
    // This matches the billing structure and avoids underreporting when a large
    // fraction of input is served from cache at lower cost.
    const totTok = (s.input_tokens||0) + (s.output_tokens||0)
      + (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    document.getElementById('kpi-tokens').textContent = fmt(totTok);
    document.getElementById('kpi-tokens-in').textContent = fmt(s.input_tokens);
    const cacheTok = (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    const cacheEl = document.getElementById('kpi-tokens-cache');
    if (cacheEl) cacheEl.textContent = fmt(cacheTok);
    document.getElementById('kpi-tokens-out').textContent = fmt(s.output_tokens);

    // Cache hit rate is an input-side ratio: the denominator is everything that
    // arrived as prompt (fresh input + cache read + cache creation). Dividing by
    // total tokens instead would fold output in and drift with response length.
    const promptTok = (s.input_tokens||0) + (s.cache_read_tokens||0) + (s.cache_creation_tokens||0);
    const hitWrap = document.getElementById('kpi-cache-hit-wrap');
    const hitEl = document.getElementById('kpi-cache-hit');
    if (hitWrap && hitEl) {
      if (promptTok > 0) {
        const pct = ((s.cache_read_tokens||0) / promptTok) * 100;
        hitEl.textContent = (pct >= 10 ? pct.toFixed(0) : pct.toFixed(1)) + '%';
        hitWrap.hidden = false;
        hitWrap.title = t('analytics.cacheHitTitle');
      } else {
        hitWrap.hidden = true;
      }
    }
    document.getElementById('kpi-cost').textContent = fmtCost(s.est_cost_usd);
    const providerCostRows = Number(s.provider_cost_rows || 0);
    const estimatedCostRows = Number(s.estimated_cost_rows || 0);
    const knownCostRows = providerCostRows + estimatedCostRows;
    const sourceEl = document.getElementById('kpi-cost-source');
    if (sourceEl) {
      sourceEl.classList.remove('provider', 'mixed');
      if (providerCostRows > 0 && estimatedCostRows === 0) {
        sourceEl.textContent = t('analytics.costProvider');
        sourceEl.classList.add('provider');
      } else if (providerCostRows > 0) {
        sourceEl.textContent = t('analytics.costMixed')
          .replace('{provider}', providerCostRows.toLocaleString())
          .replace('{total}', knownCostRows.toLocaleString());
        sourceEl.classList.add('mixed');
      } else {
        sourceEl.textContent = t('analytics.costEstimated');
      }
    }

    document.getElementById('kpi-reasoning').textContent = hasAccount ? fmt(s.reasoning_tokens || 0) : '—';
  },

  renderDonuts(summary) {
    // Weight both ring charts by total tokens consumed (input + output +
    // cache) rather than request count, which better reflects real usage.
    const withTotal = (items) => (items || []).map(it => ({
      ...it,
      total_tokens: (it.input_tokens||0) + (it.output_tokens||0)
        + (it.cache_read_tokens||0) + (it.cache_creation_tokens||0),
    }));
    this.renderDonutChart('model-donut', withTotal(summary.models), 'total_tokens');
    this.renderDonutChart('provider-donut', withTotal(summary.providers), 'total_tokens');
    this.renderDonutChart('plan-donut', withTotal(summary.plans), 'total_tokens');
  },

  renderDonutChart(containerId, items, valKey) {
    const wrap = document.getElementById(containerId);
    if (!wrap) return;
    wrap.innerHTML = '';

    if (!items.length) {
      wrap.innerHTML = '<div class="empty-state">' + t('analytics.noData') + '</div>';
      return;
    }

    const sorted = [...items].sort((a,b) => (b[valKey]||0) - (a[valKey]||0));
    let top = sorted.slice(0,5);
    const rest = sorted.slice(5);
    const otherVal = rest.reduce((sum,i) => sum + (i[valKey]||0), 0);
    if (otherVal > 0) top.push({name: 'Other', [valKey]: otherVal});
    const total = top.reduce((sum,i) => sum + (i[valKey]||0), 0) || 1;

    // Donut drawn with stroke-dasharray on <circle> arcs rather than <path>
    // wedges: a 100% share is just a full-circumference dash, so it renders
    // correctly instead of degenerating (coincident arc endpoints draw nothing).
    const C = 120, R = 82, STROKE = 34;
    const CIRC = 2 * Math.PI * R;
    const fmtTok = (n) => n >= 1_000_000 ? (n/1_000_000).toFixed(1)+'M'
      : n >= 1000 ? (n/1000).toFixed(1)+'K' : String(n);

    const tooltipId = containerId + '-tip';
    const legend = [];
    // Floor tiny-but-nonzero shares so a <0.5% sector stays visible/hoverable.
    const MIN_FRAC = 0.005;
    let offset = 0;
    const arcs = top.map((it, idx) => {
      const v = it[valKey] || 0;
      const frac = v / total;
      const drawFrac = Math.min(1, Math.max(frac, v > 0 ? MIN_FRAC : 0));
      const col = this.palette[idx % this.palette.length];
      const label = it.model || it.provider || it.plan || it.name || 'Unknown';
      const pctTxt = (frac * 100).toFixed(1) + '%';
      const tip = `${this.escapeHtml(label)} · ${v.toLocaleString()} (${pctTxt})`;
      const metrics = [
        [t('analytics.totalTokens'), v, fmt],
        [t('analytics.requests'), it.requests, fmt],
        [t('analytics.inputTokens'), it.input_tokens, fmt],
        [t('detail.cacheRead'), it.cache_read_tokens, fmt],
        [t('detail.cacheCreation'), it.cache_creation_tokens, fmt],
        [t('analytics.outputTokens'), it.output_tokens, fmt],
        [t('analytics.reasoningTokens'), it.reasoning_tokens, fmt],
        [t('analytics.avgLatency'), it.avg_latency_ms, n => `${Number(n).toFixed(0)} ms`],
        [t('analytics.successRate'), it.success_rate, n => `${(Number(n) * 100).toFixed(1)}%`],
        [t('analytics.fallbackRate'), it.fallback_rate, n => `${Number(n).toFixed(1)}%`],
        [t('analytics.cost'), it.est_cost_usd, fmtCost],
      ];
      const detailId = `${containerId}-legend-details-${idx}`;
      const detailRows = metrics.filter(([, value]) => value != null)
        .map(([name, value, format]) =>
          `<div class="legend-detail"><span>${name}</span><strong>${format(value)}</strong></div>`)
        .join('');
      const filterKind = containerId === 'model-donut' ? 'model' : containerId === 'provider-donut' ? 'provider' : '';
      const drilldown = label === 'Other' || !filterKind || it.account_usage ? '' :
        `<button type="button" class="legend-drilldown" data-filter-kind="${filterKind}" data-filter-value="${this.escapeHtml(label)}">${t('analytics.viewRequests')}</button>`;
      legend.push(
        `<div class="legend-entry">` +
          `<button type="button" class="legend-item" data-slice="${idx}" aria-expanded="false" aria-controls="${detailId}">` +
          `<span class="legend-swatch" style="background:${col}"></span>` +
          `<span class="legend-label">${this.escapeHtml(label)}</span>` +
          `<span class="legend-value">${fmtTok(v)}</span>` +
          `<span class="legend-pct">${pctTxt}</span>` +
          `</button>` +
          `<div class="legend-details" id="${detailId}" hidden>${detailRows}${drilldown}</div>` +
        `</div>`);
      const dash = `${(drawFrac * CIRC).toFixed(2)} ${((1 - drawFrac) * CIRC).toFixed(2)}`;
      // Negative dashoffset advances clockwise from 12 o'clock (rotate -90).
      const arc = `<circle class="donut-arc" cx="${C}" cy="${C}" r="${R}" fill="none"` +
        ` stroke="${col}" stroke-width="${STROKE}"` +
        ` stroke-dasharray="${dash}" stroke-dashoffset="${(-offset * CIRC).toFixed(2)}"` +
        ` data-tip="${tip}" id="slice-${containerId}-${idx}"></circle>`;
      offset += drawFrac;
      return arc;
    }).join('');

    const html = `
      <div class="donut-layout">
        <div class="donut-svg-wrap">
          <svg viewBox="0 0 240 240" class="donut-svg" role="img">
            <circle cx="${C}" cy="${C}" r="${R}" fill="none" stroke="#343a41" stroke-width="${STROKE}" opacity=".55"></circle>
            <g transform="rotate(-90 ${C} ${C})">${arcs}</g>
            <text x="${C}" y="${C - 4}" text-anchor="middle" class="donut-center-value">${fmtTok(total)}</text>
            <text x="${C}" y="${C + 16}" text-anchor="middle" class="donut-center-label">Token</text>
          </svg>
          <div id="${tooltipId}" class="chart-tip"></div>
        </div>
        <div class="donut-legend">${legend.join('')}</div>
      </div>`;
    wrap.innerHTML = html;

    // Hover wiring (JS listeners instead of inline handlers so the tooltip can
    // follow the cursor and the matching legend row can highlight in sync).
    const tipEl = wrap.querySelector('#' + CSS.escape(tooltipId));
    const wrapEl = wrap.querySelector('.donut-svg-wrap');
    wrap.querySelectorAll('.donut-arc').forEach((arc, idx) => {
      const row = wrap.querySelector(`.legend-item[data-slice="${idx}"]`);
      const show = (e) => {
        tipEl.textContent = arc.getAttribute('data-tip');
        tipEl.style.display = 'block';
        arc.style.opacity = '0.78';
        if (row) row.classList.add('is-active');
        move(e);
      };
      const move = (e) => {
        const r = wrapEl.getBoundingClientRect();
        tipEl.style.left = (e.clientX - r.left + 12) + 'px';
        tipEl.style.top = (e.clientY - r.top - 8) + 'px';
      };
      const hide = () => {
        tipEl.style.display = 'none';
        arc.style.opacity = '1';
        if (row) row.classList.remove('is-active');
      };
      arc.addEventListener('mouseenter', show);
      arc.addEventListener('mousemove', move);
      arc.addEventListener('mouseleave', hide);
    });
    wrap.querySelectorAll('.legend-item').forEach(row => {
      const details = document.getElementById(row.getAttribute('aria-controls'));
      row.addEventListener('click', () => {
        const expanded = row.getAttribute('aria-expanded') === 'true';
        row.setAttribute('aria-expanded', String(!expanded));
        if (details) details.hidden = expanded;
      });
    });
    wrap.querySelectorAll('.legend-drilldown').forEach(button => {
      button.addEventListener('click', () => {
        this.drillDown(button.dataset.filterKind, button.dataset.filterValue);
      });
    });
  },

  drillDown(kind, value) {
    resetHistoryFilters(false);
    const days = Number(document.getElementById('analytics-days')?.value || 30);
    const end = new Date();
    const start = new Date(end);
    start.setDate(start.getDate() - Math.max(0, days - 1));
    document.getElementById('history-start').value = dateInputValue(start);
    document.getElementById('history-end').value = dateInputValue(end);
    const target = document.getElementById(kind === 'provider' ? 'provider-filter' : 'model-filter');
    if (target) target.value = value;
    historyPage = 1;
    if (location.hash !== '#history') location.hash = 'history';
    else activateTab('history');
  },

  renderTrend(points) {
    const wrap = document.getElementById('token-trend');
    if (!wrap) return;
    wrap.innerHTML = '';
    if (!points.length) {
      wrap.innerHTML = '<div class="empty-state">' + t('analytics.noTrend') + '</div>';
      return;
    }

    // Layout: leave room for a y-axis label column (left) and an x-axis date
    // row (bottom) so the chart reads as a real axes plot even with few points.
    const w = 648, h = 224;
    const ml = 56, mb = 22, mt = 10, mr = 12;
    const plotW = w - ml - mr;
    const plotH = h - mt - mb;
    const AXIS = '#49515b', GRID = '#2a3036', TXT = '#9aa3ad';

    // Bars are stacked, so the axis must span the stacked TOTAL of a day, not
    // the largest single series — otherwise tall columns overflow the plot.
    const dayTotal = (p) => (p.input_tokens||0) + (p.output_tokens||0)
      + (p.cache_read_tokens||0) + (p.cache_creation_tokens||0);
    const maxV = Math.max(1, ...points.map(dayTotal));
    const stepX = plotW / Math.max(1, points.length - 1);
    const x = (i) => ml + i * stepX;
    const y = (v) => mt + plotH - (v || 0) / maxV * plotH;

    const fmt = (n) => {
      if (n >= 1_000_000) return (n/1_000_000).toFixed(n>=10_000_000?0:1) + 'M';
      if (n >= 1000) return (n/1000).toFixed(n>=100_000?0:1) + 'K';
      return String(n);
    };
    const fmtDay = (d) => { const parts=String(d).split('-'); return parts.length>=2 ? parts[1]+'-'+parts[2] : d; };

    // Y gridlines + labels (0..maxV, 4 steps)
    let yGrid = '';
    for (let s = 0; s <= 4; s++) {
      const v = maxV * s / 4;
      const yy = y(v).toFixed(1);
      yGrid += `<line x1="${ml}" y1="${yy}" x2="${w-mr}" y2="${yy}" stroke="${GRID}" stroke-width="1"/>`;
      yGrid += `<text x="${ml-8}" y="${(+yy+3).toFixed(1)}" text-anchor="end" font-size="9.5" fill="${TXT}">${fmt(Math.round(v))}</text>`;
    }
    // X date labels + vertical gridlines (stride so labels don't collide)
    const maxLabels = 7;
    const stride = Math.max(1, Math.ceil(points.length / maxLabels));
    let xGrid = '';
    for (let i = 0; i < points.length; i += stride) {
      const lx = x(i).toFixed(1);
      xGrid += `<line x1="${lx}" y1="${mt}" x2="${lx}" y2="${h-mb}" stroke="${GRID}" stroke-width="1"/>`;
      xGrid += `<text x="${lx}" y="${h-mb+14}" text-anchor="middle" font-size="9.5" fill="${TXT}">${fmtDay(points[i].date||'')}</text>`;
    }
    // last point label included even if skipped
    if ((points.length-1) % stride !== 0) {
      const lx = x(points.length-1).toFixed(1);
      xGrid += `<text x="${lx}" y="${h-mb+14}" text-anchor="middle" font-size="9.5" fill="${TXT}">${fmtDay(points[points.length-1].date||'')}</text>`;
    }

    // Deepseek-style STACKED daily bars: one column per day, segmented by
    // output (bottom), cache (middle), and input (top) in distinct colors.
    // A single day renders a full-height stacked column; each segment has its
    // own native hover tooltip.
    const baseY = (h - mb);
    const colW = Math.max(10, (plotW / points.length) * 0.55);
    const bars = points.map((p, i) => {
      const cx = ml + i * (plotW / points.length) + (plotW / points.length) * 0.225;
      const inV = p.input_tokens || 0;
      const cacheV = (p.cache_read_tokens || 0) + (p.cache_creation_tokens || 0);
      const outV = p.output_tokens || 0;
      const segs = [
        { v: outV, col: '#34d399', label: t('analytics.outputTokens') },
        { v: cacheV, col: '#fbbf24', label: t('analytics.cacheTokens') },
        { v: inV, col: '#818cf8', label: t('analytics.inputTokens') },
      ];
      let yy = baseY;
      const rects = segs.filter(s => (s.v || 0) > 0).map(s => {
        // Enforce a ~2px floor so a tiny-but-real segment (e.g. output is 0.2%
        // of a huge input day) stays visible instead of collapsing to a
        // sub-pixel sliver that reads as "only one color". Auto-adjusts yy so
        // the stacked column still sits on the baseline.
        const rawH = (s.v || 0) / maxV * plotH;
        const segH = Math.max(rawH, 2);
        const yTop = yy - segH;
        const r = `<rect x="${(cx).toFixed(1)}" y="${yTop.toFixed(1)}" width="${colW.toFixed(1)}" height="${segH.toFixed(1)}" rx="1" fill="${s.col}"></rect>`;
        yy = yTop;
        return r;
      }).join('');
      // Full-height transparent hit area: hovering anywhere in the day's column
      // (not just an individual segment) opens one tooltip listing all three
      // series, matching how the deepseek usage page behaves.
      const tipRows = [
        { v: inV, col: '#818cf8', label: t('analytics.inputTokens') },
        { v: cacheV, col: '#fbbf24', label: t('analytics.cacheTokens') },
        { v: outV, col: '#34d399', label: t('analytics.outputTokens') },
      ].map(s => `<div class="tip-row"><span class="tip-dot" style="background:${s.col}"></span>` +
        `<span class="tip-label">${s.label}</span><span class="tip-val">${(s.v||0).toLocaleString()}</span></div>`).join('');
      const tipHtml = `<div class="tip-title">${p.date || ''}</div>${tipRows}`;
      const hit = `<rect class="trend-hit" x="${(cx - colW * 0.35).toFixed(1)}" y="${mt}"` +
        ` width="${(colW * 1.7).toFixed(1)}" height="${plotH.toFixed(1)}"` +
        ` fill="transparent" data-tip="${this.escapeHtml(tipHtml)}"></rect>`;
      return rects + hit;
    }).join('');

    const svg = `
      <div style="position:relative;">
        <svg class="trend-svg" viewBox="0 0 ${w} ${h}" preserveAspectRatio="xMidYMid meet">
          ${yGrid}
          ${xGrid}
          <line x1="${ml}" y1="${mt}" x2="${ml}" y2="${h-mb}" stroke="${AXIS}" stroke-width="1"/>
          <line x1="${ml}" y1="${h-mb}" x2="${w-mr}" y2="${h-mb}" stroke="${AXIS}" stroke-width="1"/>
          ${bars}
        </svg>
        <div class="chart-tip" id="trend-tip"></div>
      </div>
      <div class="trend-legend">
        <span><span class="swatch" style="background:#818cf8;height:10px;width:14px;display:inline-block;border-radius:3px;margin-right:4px;"></span>${t('analytics.inputTokens')}</span>
        <span><span class="swatch" style="background:#fbbf24;height:10px;width:14px;display:inline-block;border-radius:3px;margin-right:4px;"></span>${t('analytics.cacheTokens')}</span>
        <span><span class="swatch" style="background:#34d399;height:10px;width:14px;display:inline-block;border-radius:3px;margin-right:4px;"></span>${t('analytics.outputTokens')}</span>
      </div>`;
    wrap.innerHTML = svg;

    // Hover tooltip wiring (HTML floating tip, matching donut style)
    const tipEl = wrap.querySelector('#trend-tip');
    const svgWrap = wrap.querySelector('svg').parentElement;
    wrap.querySelectorAll('.trend-hit').forEach(hit => {
      const show = (e) => {
        tipEl.innerHTML = hit.getAttribute('data-tip');
        tipEl.style.display = 'block';
        move(e);
      };
      const move = (e) => {
        const r = svgWrap.getBoundingClientRect();
        tipEl.style.left = (e.clientX - r.left + 12) + 'px';
        tipEl.style.top = (e.clientY - r.top - 8) + 'px';
      };
      const hide = () => { tipEl.style.display = 'none'; };
      hit.addEventListener('mouseenter', show);
      hit.addEventListener('mousemove', move);
      hit.addEventListener('mouseleave', hide);
    });
  },

  renderRecent(items) {
    const root = document.getElementById('analytics-recent');
    if (!root) return;
    if (!items.length) {
      root.innerHTML = `<div class="empty-state">${t('analytics.noData')}</div>`;
      return;
    }
    root.innerHTML = items.map(item => {
      const cache = Number(item.cache_read_tokens || 0) + Number(item.cache_write_5m_tokens || 0) + Number(item.cache_write_1h_tokens || 0);
      const tokens = Number(item.input_tokens || 0) + Number(item.output_tokens || 0) + cache;
      const cost = Number(item.cost_units || 0) / 1e8 || Number(item.cost_usd || 0);
      return `<div class="recent-usage-row">
        <div class="recent-usage-model"><strong>${this.escapeHtml(item.model || '—')}</strong><span>${this.escapeHtml(item.provider || '—')} · ${this.escapeHtml(item.plan || '—')}</span></div>
        <div class="recent-usage-meta">${fmtTime(item.time)}</div>
        <div class="recent-usage-tokens">${fmtTok(tokens)} Token</div>
        <div class="recent-usage-cost">${fmtCost(cost)}</div>
      </div>`;
    }).join('');
  },

  renderEmpty(msg = 'No usage data yet. Run some requests or configure a model to see analytics.') {
    ['model-donut','provider-donut','plan-donut','token-trend','analytics-recent'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.innerHTML = `<div class="empty-state">${msg}</div>`;
    });
  },

  escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
  }
};

// Boot analytics module (listeners only; data loads when tab is clicked)
setTimeout(() => {
  AnalyticsModule.init();
}, 250);
