package gui

import "testing"

func TestWorkBuddyCostPresentation(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  for (const lang of ['en', 'zh']) {
    currentLang = lang;
    for (const provider of ['workbuddy', 'commandcode', '']) {
      const partial = {provider,model:'synthetic',requests:2,total_requests:2,unknown_cost_requests:1,cost_usd:1.25,est_cost_usd:1.25};
      const expected = provider === 'workbuddy' ? '$1.25' : t('analytics.knownSubtotal').replace('{value}', '$1.25');
      assert.equal(fmtAggregateCost(partial), expected);
      assert.equal(fmtAggregateCost({...partial,unknown_cost_requests:2}), '—');
      assert.equal(fmtAggregateCost({...partial,unknown_cost_requests:0,cost_usd:0}), '$0.00');
      AnalyticsModule.renderKPIs({provider,summary:{...partial,provider:undefined}});
      assert.equal(get('kpi-cost').textContent, expected);
      assert.equal(get('kpi-cost-note').textContent, costCoverageNote(partial));
      AnalyticsModule.renderModelTable([partial]);
      assert.ok(get('analytics-model-tbody').innerHTML.includes('>' + expected + '</td>'));
      AnalyticsModule.renderPeriodTable([{...partial,date:'2026-10-25'}]);
      assert.ok(get('analytics-period-tbody').innerHTML.includes('>' + expected + '</td>'));
      assert.ok(get('analytics-period-tbody').innerHTML.includes(costCoverageNote(partial)));
      AnalyticsModule.renderDistribution('provider-distribution',[partial],'cost_usd','provider');
      assert.ok(get('provider-distribution').innerHTML.includes('<strong>' + expected + '</strong>'));
      QuotaModule.provider = provider;
      QuotaModule.localView = {provider,summary:{...partial,provider:undefined},models:[partial]};
      QuotaModule.renderLocalUsage();
      assert.equal(get('quota-local-cost').textContent,expected);
      get('analytics-provider').value = provider;
      get('analytics-start').value = get('analytics-end').value = '2026-10-25';
      fetch = async raw => {
        const url = new URL(raw,'http://synthetic.invalid');
        if (url.pathname === '/api/prices') return {ok:true,json:async()=>({tables:{}})};
        assert.equal(url.searchParams.get('provider') || '',provider);
        if (url.pathname === '/api/analytics/summary') return {ok:true,json:async()=>({provider,summary:partial,models:[partial],providers:[partial]})};
        assert.equal(url.pathname,'/api/analytics/tokens/trend');
        return {ok:true,json:async()=>({provider,trend:[{...partial,provider:undefined,date:'2026-10-25'}]})};
      };
      await AnalyticsModule.load(true);
      assert.equal(get('analytics-error').hidden,true);
      assert.equal(get('kpi-cost').textContent,expected);
      assert.ok(get('analytics-period-tbody').innerHTML.includes('>' + expected + '</td>'));
      assert.equal(fmtAggregateCost(AnalyticsModule.currentTrend[0]),expected,'trend tooltips keep the response provider');
      const anonymous = {...partial,provider:undefined};
      renderOverviewUsage({provider,summary:anonymous,today:anonymous,retained:anonymous},[{...anonymous,date:utcDateInputValue(new Date())}],{});
      assert.equal(get('m-cost').textContent,expected,'overview amount follows response scope');
      assert.ok(get('daily-spend-note').textContent.includes(expected),'daily spend amount follows response scope');
      assert.equal(fmtAggregateCost(AnalyticsModule.chartData['overview-request-trend'].points.find(point=>point.requests>0)),expected);
      get('provider-filter').value = provider;
      fetch = async raw => {
        const url = new URL(raw,'http://synthetic.invalid');
        if (url.pathname === '/api/history/channels') return {ok:true,json:async()=>({channels:[]})};
        assert.equal(url.searchParams.get('provider') || '',provider);
        if (url.pathname === '/api/history') return {ok:true,json:async()=>({items:[],total:0})};
        assert.equal(url.pathname,'/api/history/summary');
        return {ok:true,json:async()=>({...anonymous,models:[{...partial,name:'synthetic'}],providers:[{...anonymous,name:provider}],scenarios:[{...anonymous,name:'default'}]})};
      };
      await refreshHistory();
      assert.equal(get('history-error').hidden,true);
      assert.equal(get('history-summary-cost').textContent,expected,'history summary follows the captured request scope');
      for (const id of ['history-model-breakdown','history-provider-breakdown','history-scenario-breakdown']) {
        assert.ok(get(id).innerHTML.includes('<strong>' + expected + '</strong>'),id);
      }
      renderHistorySummary(lastHistorySummary);
      assert.equal(get('history-summary-cost').textContent,expected,'rerenders retain history scope');
    }
    renderHistorySummary({providers:[{name:'workbuddy',requests:2,unknown_cost_requests:1,cost_usd:1.25},{name:'commandcode',requests:2,unknown_cost_requests:1,cost_usd:2.5}]});
    const mixed = get('history-provider-breakdown').innerHTML;
    assert.ok(mixed.includes('<strong>$1.25</strong>'),'WorkBuddy row in all-platform view');
    assert.ok(mixed.includes('<strong>' + t('analytics.knownSubtotal').replace('{value}','$2.50') + '</strong>'),'other provider retains prefix');
  }
}
`+platformBehaviorRunScript)
}
