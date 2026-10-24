package gui

import "testing"

func TestStreamTimingPresentationAndCSV(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const rec = {id:'synthetic', model:'alpha', provider:'workbuddy', details_known:true,
    success:true, streaming:true, duration_ms:16000, output_tokens:600,
    first_event_ms:0, first_content_ms:4000, message_end_ms:14000,
    content_tokens_per_second:60, event_tokens_per_second:37.5};
  for (const field of ['first_event_ms','first_content_ms','message_end_ms','content_tokens_per_second','event_tokens_per_second']) {
    const column = HISTORY_CSV_COLUMNS.find(c => c[0] === field);
    assert.ok(column, 'CSV missing '+field);
    assert.equal(column[1](rec), rec[field]);
    assert.equal(escapeCSV(column[1]({})), '', 'unknown CSV '+field);
  }
  assert.equal(escapeCSV(HISTORY_CSV_COLUMNS.find(c => c[0] === 'first_event_ms')[1](rec)), '0');
  const grade = gradeDuration(rec, 'duration');
  assert.equal(gradeDuration({...rec, content_tokens_per_second:0.1}, 'duration'), grade, 'new metrics must not change grading');
  assert.equal(requestThroughput(rec), 37.5);
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const key of ['analytics.contentThroughput','analytics.contentThroughputHint','analytics.contentSamples',
      'detail.firstEvent','detail.firstContent','detail.messageEnd','detail.eventThroughput','detail.streamTimingNote']) {
      assert.notEqual(t(key), key, 'translation missing '+key);
    }
    showHistoryDetail(rec);
    let html = get('modal-body').innerHTML;
    assert.ok(html.includes('0 ms'), 'measured zero event offset is visible');
    assert.ok(html.includes('4.0 s') && html.includes('14.0 s'));
    assert.ok(html.includes('60.0') && html.includes('37.5'));
    assert.ok(html.includes(t('detail.streamTimingNote')), 'measurement caveat must be visible, not tooltip-only');
    showHistoryDetail({...rec, first_content_ms:0});
    assert.ok((get('modal-body').innerHTML.match(/0 ms/g)||[]).length >= 2, 'zero content offset is visible');
    showHistoryDetail({model:'legacy', details_known:false});
    html = get('modal-body').innerHTML;
    assert.ok(!html.includes('0 ms') && !html.includes('60.0'), 'old records cannot inherit observations');
    AnalyticsModule.renderModelTable([
      {model:'known', provider:'workbuddy', tokens_per_second:12.3, content_tokens_per_second:61.2, content_samples:7},
      {model:'legacy', provider:'workbuddy', content_samples:0}
    ]);
    html = get('analytics-model-tbody').innerHTML;
    const rows = [...html.matchAll(/<tr[^>]*>(.*?)<[/]tr>/gs)]
      .map(m => [...m[1].matchAll(/<td[^>]*>(.*?)<[/]td>/gs)].map(c => c[1]));
    assert.equal(rows[0].length, 7);
    assert.ok(rows[0][4].includes('12.3') && rows[0][5].includes('61.2'));
    assert.ok(rows[0][5].includes(t('analytics.contentSamples').replace('{n}','7')));
    assert.ok(rows[1][5].includes('—') && rows[1][5].includes(t('analytics.contentSamples').replace('{n}','0')));
    AnalyticsModule.renderPlatformHealth([{provider:'workbuddy', requests:7, known_requests:7,
      success_rate:1, content_tokens_per_second:61.2, content_samples:7}], null);
    html = get('platform-health-rows').innerHTML;
    assert.ok(html.includes('61.2') && html.includes(t('analytics.contentSamples').replace('{n}','7')));
    AnalyticsModule.renderModelTable([]);
    assert.ok(get('analytics-model-tbody').innerHTML.includes('colspan="7"'));
  }
  currentLang = 'en';
  assert.equal(t('analytics.throughput'), 'Execution Tok/s');
  assert.equal(t('th.tokensPerSecond'), 'Execution Tok/s');
  assert.ok(t('workbuddy.tps').includes('gateway'), 'upstream statistic must have a separate scope label');
  assert.equal((page.match(/data-i18n="analytics.contentThroughput"/g)||[]).length, 2, 'both model tables need the new column');
  assert.ok(page.includes('data-sort="tokens_per_second"'), 'existing sort field is retained');
}
`+platformBehaviorRunScript)
}
