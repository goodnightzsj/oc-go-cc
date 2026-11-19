package gui

import "testing"

func TestAttemptIdentityPresentationAndCSV(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const record = {id:'attempt', model:'alpha', provider:'cline-pass', outcome_known:true,
    success:true, streaming:false, duration_ms:100, attempt:1, candidate_position:3};
  const candidate = HISTORY_CSV_COLUMNS.find(column => column[0] === 'candidate_position');
  assert.ok(candidate, 'CSV must include the candidate position');
  assert.equal(HISTORY_CSV_COLUMNS.at(-1), candidate, 'append without moving existing columns');
  assert.equal(candidate[1](record), 3);
  assert.equal(HISTORY_CSV_COLUMNS.find(column => column[0] === 'attempt')[1](record), 1);
  assert.equal(escapeCSV(candidate[1]({...record, candidate_position:undefined})), '');
  for (const lang of ['en','zh']) {
    currentLang = lang;
    showHistoryDetail(record);
    const row = key => get('modal-body').innerHTML.split('<div class="detail-row">')
      .find(part => part.includes('>' + t(key) + '</span>'))?.split('</div>')[0];
    assert.ok(row('detail.attempt')?.includes('>1</span>'), 'first actual execution');
    assert.ok(row('detail.candidatePosition')?.includes('>3</span>'), 'third eligible candidate');
    showHistoryDetail({...record, candidate_position:undefined});
    assert.ok(row('detail.candidatePosition')?.includes('>' + t('detail.unavailable') + '</span>'), 'old records must not invent a candidate position');
    for (const key of ['detail.attemptHint','detail.candidatePositionHint']) assert.notEqual(t(key), key);

    // A failed attempt followed by a successful fallback is still two ledger rows.
    renderOverviewUsage({summary:{total_requests:2,known_requests:2,success_rate:0.5},
      last_minute:{total_requests:2}});
    assert.equal(get('m-total').textContent, '2');
    assert.equal(get('m-success').textContent, '50.0%');
    assert.equal(get('m-throughput').textContent, t('metric.perMinute').replace('{v}','2'));
    assert.ok(t('metric.total').includes(lang === 'zh' ? '尝试' : 'attempt'));
    assert.ok(t('analytics.requests').includes(lang === 'zh' ? '尝试' : 'attempt'));
    assert.ok(t('history.successRate').includes(lang === 'zh' ? '尝试' : 'Attempt'));
    assert.ok(t('analytics.platformHealthNote').includes('50%'));
    assert.equal(t('commandcode.requests'), lang === 'zh' ? '请求数' : 'Requests', 'account usage keeps its provider definition');
  }
}
`+platformBehaviorRunScript)
}
