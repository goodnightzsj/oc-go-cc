package gui

import "testing"

func TestStreamTimingPresentationAndCSV(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const rec = {id:'synthetic', model:'alpha', provider:'workbuddy', details_known:true,
    success:true, streaming:true, duration_ms:16000, output_tokens:600,
    first_event_ms:0, first_content_ms:4000, message_end_ms:14000,
    content_tokens_per_second:60, event_tokens_per_second:37.5,
    generation_sample:{first_output_ms:0,duration_ms:10000,output_tokens:800},
    generation_tokens_per_second:80};
  for (const field of ['first_event_ms','first_content_ms','message_end_ms','content_tokens_per_second','event_tokens_per_second']) {
    const column = HISTORY_CSV_COLUMNS.find(c => c[0] === field);
    assert.ok(column, 'CSV missing '+field);
    assert.equal(column[1](rec), rec[field]);
    assert.equal(escapeCSV(column[1]({})), '', 'unknown CSV '+field);
  }
  assert.equal(escapeCSV(HISTORY_CSV_COLUMNS.find(c => c[0] === 'first_event_ms')[1](rec)), '0');
  const grade = gradeDuration(rec, 'duration');
  assert.equal(gradeDuration({...rec, generation_tokens_per_second:0.1}, 'duration'), grade, 'new metric must not change duration grading');
  assert.equal(requestThroughput(rec), 37.5);
  for (const [field,value] of [['generation_first_output_ms',0],['generation_duration_ms',10000],['generation_output_tokens',800],['generation_tokens_per_second',80]]) {
    const column = HISTORY_CSV_COLUMNS.find(c => c[0] === field);
    assert.ok(column, 'missing source CSV field '+field);
    assert.equal(column[1](rec), value);
    assert.equal(escapeCSV(column[1]({})), '');
  }
  assert.equal(fmtGenerationThroughput(0), '0.0 tok/s');
  assert.equal(fmtGenerationThroughput(235), '235.0 tok/s');
  assert.equal(fmtGenerationThroughput(1234.56), '1,234.6 tok/s');
  for (const value of [undefined,null,-1,NaN,Infinity,'80']) assert.equal(fmtGenerationThroughput(value), '—');
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const key of ['analytics.generationThroughput','analytics.generationThroughputHint','analytics.generationSamples',
      'detail.generationFirstOutput','detail.generationDuration','detail.generationOutput','detail.streamTimingNote']) {
      assert.notEqual(t(key), key, 'translation missing '+key);
    }
    showHistoryDetail(rec);
    let html = get('modal-body').innerHTML;
    assert.ok(html.includes('0 ms'), 'measured zero source offset is visible');
    assert.ok(html.includes('10.0 s') && html.includes('800'));
    assert.ok(html.includes('80.0 tok/s') && html.includes('37.5'));
    assert.ok(!html.includes('60.0'), 'old application-output rate is no longer primary UI');
    assert.ok(html.includes(t('detail.streamTimingNote')), 'measurement caveat must be visible, not tooltip-only');
    showHistoryDetail({...rec, generation_tokens_per_second:undefined, generation_sample:{first_output_ms:0,duration_ms:99,output_tokens:800}});
    html = get('modal-body').innerHTML;
    assert.ok(html.includes('99 ms') && !html.includes('80.0 tok/s'), 'short sample stays inspectable, never backfilled');
    showHistoryDetail({model:'legacy', details_known:false});
    html = get('modal-body').innerHTML;
    assert.ok(!html.includes('>0 ms<') && !html.includes('80.0 tok/s'), 'old records cannot inherit observations');
    AnalyticsModule.renderModelTable([
      {model:'known', provider:'workbuddy', tokens_per_second:12.3, generation_tokens_per_second:235, generation_samples:7},
      {model:'legacy', provider:'workbuddy', tokens_per_second:12.3, generation_samples:0},
      {model:'zero', provider:'workbuddy', generation_tokens_per_second:0, generation_samples:1}
    ]);
    html = get('analytics-model-tbody').innerHTML;
    const rows = [...html.matchAll(/<tr[^>]*>(.*?)<[/]tr>/gs)]
      .map(m => [...m[1].matchAll(/<td[^>]*>(.*?)<[/]td>/gs)].map(c => c[1]));
    assert.equal(rows[0].length, 6);
    assert.equal(rows[0][4], '235.0 tok/s', 'exactly one speed, without a prefix or sample counter');
    assert.equal(rows[1][4], '—', 'no fallback to old average');
    assert.equal(rows[2][4], '0.0 tok/s');
    assert.ok(html.includes('12.3') && html.includes(t('analytics.generationSamples').replace('{n}','7')), 'average and sample count remain in tooltip');
    AnalyticsModule.renderPlatformHealth([{provider:'workbuddy', requests:7, known_requests:7,
      success_rate:1, tokens_per_second:12.3, generation_tokens_per_second:235, generation_samples:7}], null);
    html = get('platform-health-rows').innerHTML;
    const visible = html.replace(/<[^>]*>/g,'');
    assert.ok(visible.includes('235.0 tok/s') && !visible.includes('12.3'), 'platform main text shows generation only');
    assert.ok(html.includes(t('analytics.generationSamples').replace('{n}','7')));
    AnalyticsModule.renderModelTable([]);
    assert.ok(get('analytics-model-tbody').innerHTML.includes('colspan="6"'));
  }
  currentLang = 'en';
  assert.equal(t('analytics.throughput'), 'Average throughput');
  assert.equal(t('th.tokensPerSecond'), 'Tok/s');
  assert.ok(t('workbuddy.tps').includes('gateway'), 'upstream statistic must have a separate scope label');
  assert.equal((page.match(/data-i18n="th.tokensPerSecond"/g)||[]).length, 3, 'all three tables use the plain Tok/s heading');
  assert.ok(!page.includes('data-i18n="analytics.contentThroughput"'));
  assert.ok(page.includes('data-sort="generation_tokens_per_second"'), 'sort matches the displayed generation speed');
}
`+platformBehaviorRunScript)
}
