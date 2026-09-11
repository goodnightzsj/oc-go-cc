package gui

import (
	"regexp"
	"strings"
	"testing"
)

func TestConsoleRedesignStructure(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	ids := map[string]bool{}
	for _, match := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		if ids[match[1]] {
			t.Errorf("duplicate DOM id: %s", match[1])
		}
		ids[match[1]] = true
	}
	for _, id := range []string{"app-navigation", "main-content", "active-page-title", "btn-theme-toggle", "history-advanced-filters", "history-advanced-count", "overview-empty", "overview-insights", "analytics-empty", "analytics-insights", "analytics-request-trend", "analytics-token-trend"} {
		if !ids[id] {
			t.Errorf("missing console control or content region: %s", id)
		}
	}
	for _, pageID := range []string{"overview", "history", "performance", "fallback", "analytics", "quota", "settings"} {
		if !ids["tab-"+pageID] || !strings.Contains(page, `data-tab="`+pageID+`"`) {
			t.Errorf("missing page or navigation target: %s", pageID)
		}
	}
	if !strings.Contains(page, `<main class="app-main" id="main-content"`) || !strings.Contains(page, `class="app-sidebar"`) {
		t.Error("the console needs a persistent navigation region and a main landmark")
	}
}

func TestConsoleRetainedDataPresentation(t *testing.T) {
	runPlatformBehavior(t, consoleRetainedDataScript)
}

func TestConsoleThemeBehavior(t *testing.T) {
	runPlatformBehavior(t, consoleThemeBehaviorScript)
}

func TestConsoleThemeStyles(t *testing.T) {
	data, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	for _, selector := range []string{`:root[data-theme="light"]`, `:root:not([data-theme="dark"])`} {
		if !strings.Contains(css, selector) {
			t.Errorf("manual theme selection must override the system preference: missing %s", selector)
		}
	}
}

const consoleThemeBehaviorScript = platformBehaviorDOMScript + `
context.getComputedStyle = element => ({colorScheme:element.dataset.theme || 'dark'});
async function checks() {
  const root = document.documentElement;
  root.dataset = {};
  const saved = new Map();
  localStorage.setItem = (key,value) => saved.set(key,value);
  const attributes = {};
  document.getElementById('btn-theme-toggle').setAttribute = (key,value) => attributes[key] = value;
  for (const lang of ['en','zh']) {
    currentLang = lang;
    root.dataset.theme = 'light';
    syncThemeControl();
    assert.equal(attributes['aria-label'],t('theme.dark'),'the action names the next theme');
    toggleTheme();
    assert.equal(root.dataset.theme,'dark');
    assert.equal(saved.get('routatic-proxy-theme'),'dark','the selected theme survives a reload');
    assert.equal(attributes['aria-label'],t('theme.light'));
    assert.equal(document.getElementById('theme-action').textContent,t('theme.light'));
    toggleTheme();
    assert.equal(root.dataset.theme,'light');
    assert.equal(saved.get('routatic-proxy-theme'),'light');
    assert.equal(attributes['data-i18n-aria-label'],'theme.dark','language switching keeps the correct theme action');
  }
}
` + platformBehaviorRunScript

const consoleRetainedDataScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const empty = {total_requests:0,known_requests:0,success_rate:0,input_tokens:0,output_tokens:0,cache_read_tokens:0,cache_creation_tokens:0,est_cost_usd:0,unknown_cost_requests:0};
  for (const lang of ['en','zh']) {
    currentLang = lang;
    renderOverviewUsage({summary:empty,today:empty,retained:empty},[],{});
    assert.equal(get('overview-empty').hidden,false,'a successful empty query needs one explicit local-ledger empty state');
    assert.equal(get('overview-insights').hidden,true,'empty charts must not occupy four blank panels');
    assert.equal(get('m-total').textContent,'0','known empty totals remain zero');
    assert.ok(get('m-total-note').textContent.includes(t('data.retained')),'retained rows are not all-time account activity');
    clearOverviewUsage(true);
    assert.equal(get('overview-empty').hidden,true,'loading must not claim an empty ledger');
    clearOverviewUsage();
    assert.equal(get('overview-empty').hidden,true,'an error must not claim an empty ledger');
    assert.equal(get('m-total').textContent,'—');

    fetch = async raw => ({ok:true,json:async()=>raw.includes('/tokens/trend') ? {trend:[]} : {summary:empty}});
    await AnalyticsModule.load(true);
    assert.equal(get('analytics-empty').hidden,false);
    assert.equal(get('analytics-insights').hidden,true);
    AnalyticsModule.clearView();
    assert.equal(get('analytics-empty').hidden,true);

    allHistory = [];
    resetHistoryFilters(false);
    renderHistory();
    assert.ok(get('history-tbody').innerHTML.includes(t('data.localEmptyHint')),'empty local history must disclose scope and possible retention');
    get('model-filter').value = 'filtered-model';
    syncAdvancedFilters();
    assert.equal(get('history-advanced-filters').open,true,'active advanced filters must be discoverable');
    assert.equal(get('history-advanced-count').textContent,'1');
    assert.equal(historyQueryParams().get('model'),'filtered-model','disclosure must not change the query');
    resetHistoryFilters(false);
    assert.equal(get('history-advanced-count').textContent,'');
    QuotaModule.provider = 'opencode-go';
    const account = {key_hint:'masked-go',report:{plan:'go-plan',weekly:{has_percent:true,used_percent:25,used_dollars:5,limit_dollars:20}}};
    QuotaModule.view = {provider:'opencode-go',accounts:[account]};
    QuotaModule.render();
    assert.equal(get('quota-go-summary').hidden,false,'a single measured account can show its summary');
    QuotaModule.view.accounts = [account,{...account,key_hint:'masked-other',report:{...account.report,plan:'other-plan'}}];
    QuotaModule.render();
    assert.equal(get('quota-go-summary').hidden,true,'multiple accounts must not look like one combined plan');
    assert.ok(get('quota-accounts').innerHTML.includes('go-plan') && get('quota-accounts').innerHTML.includes('other-plan'),'individual plan names remain visible');
    QuotaModule.view.accounts = [{key_hint:'masked-go',error:'HTTP 403'}];
    QuotaModule.render();
    assert.equal(get('quota-go-summary').hidden,true,'an unavailable account must not fill four summary tiles with dashes');
    assert.ok(get('quota-accounts').innerHTML.includes('403'),'the account error remains explicit');
    for (const key of ['data.retained','data.localEmptyHint','shell.workspace','theme.light','theme.dark','history.advanced','history.previous','history.next','perf.p50Hint','setting.platformsHint']) assert.notEqual(t(key),key);
  }
}
` + platformBehaviorRunScript
