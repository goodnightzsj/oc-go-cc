package gui

import "testing"

func TestClearViewsDiscardDerivedDataAndChartCache(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const point = {date:utcDateInputValue(new Date()),requests:3,known_requests:3,input_tokens:10,output_tokens:20,cost_usd:4};
  const data = {summary:{...point,total_requests:3,est_cost_usd:4,cache_read_tokens:0,cache_creation_tokens:0},
    last_minute:{total_requests:3},providers:[{...point,provider:'workbuddy'}]};
  for (const loading of [true,false]) {
    renderOverviewUsage(data,[point],{});
    AnalyticsModule.renderRequestTrend([point],'analytics-request-trend');
    AnalyticsModule.renderTokenLines([point],'analytics-token-trend');
    assert.ok(get('m-total-avg').textContent && get('m-cost-avg').textContent);
    clearOverviewUsage(loading);
    for (const id of ['m-total-avg','m-tokens-avg','m-cost-avg','m-success-avg','daily-spend-note']) {
      assert.equal(get(id).textContent,'','stale derived value '+id);
    }
    for (const id of ['platform-health','daily-spend']) assert.equal(get(id).hidden,true,id);
    for (const id of ['platform-health-rows','daily-spend-bars']) assert.equal(get(id).innerHTML,'',id);
    for (const id of ['overview-request-trend','overview-token-trend']) {
      assert.equal(AnalyticsModule.chartData[id],undefined,'stale cache '+id);
      get(id).clientWidth = 600;
    }
    assert.ok(AnalyticsModule.chartData['analytics-request-trend'],'clearing overview must not clear analytics');
    AnalyticsModule.redrawCharts();
    assert.ok(!get('overview-request-trend').innerHTML.includes('<svg'),'resize revived old overview');
    renderOverviewUsage(data,[point],{});
    AnalyticsModule.clearView(loading);
    for (const id of ['analytics-request-trend','analytics-token-trend']) {
      assert.equal(AnalyticsModule.chartData[id],undefined,'stale cache '+id);
      get(id).clientWidth = 600;
    }
    assert.ok(AnalyticsModule.chartData['overview-request-trend'],'clearing analytics must not clear overview');
    AnalyticsModule.redrawCharts();
    assert.ok(!get('analytics-request-trend').innerHTML.includes('<svg'),'resize revived old analytics');
  }
  const models = Array.from({length:7},(_,i)=>({...point,model:'m'+i}));
  AnalyticsModule.renderDistribution('overview-model-distribution',models,'requests','model');
  assert.ok(get('overview-model-distribution-count').textContent);
  AnalyticsModule.renderDistribution('overview-model-distribution',[],'requests','model');
  assert.equal(get('overview-model-distribution-count').textContent,'','empty data left old top-N label');
}
`+platformBehaviorRunScript)
}
