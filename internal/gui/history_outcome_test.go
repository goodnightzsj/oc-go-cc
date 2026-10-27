package gui

import "testing"

func TestHistoryMissingUsageShowsObservedFailure(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const rec = {id:'truncated', model:'test', details_known:false, outcome_known:true,
    success:false, streaming:true, duration_ms:3000, attempt:1, error_msg:'unexpected EOF'};
  for (const lang of ['en','zh']) {
    currentLang = lang;
    allHistory = [rec];
    renderHistory();
    let html = document.getElementById('history-tbody').innerHTML;
    assert.ok(html.includes('badge-error') && html.includes(t('history.streaming')));
    assert.ok(html.includes('3.0 s') && !html.includes('$0'));
    showHistoryDetail(rec);
    html = document.getElementById('modal-body').innerHTML;
    assert.ok(html.includes(t('detail.failed')) && !html.includes('detail-status unknown'));
    for (const field of ['duration_ms','streaming','success','attempt','outcome_known']) {
      assert.equal(HISTORY_CSV_COLUMNS.find(c => c[0] === field)[1](rec), rec[field]);
    }
    const imported = {...rec, outcome_known:false, streaming:false};
    allHistory = [imported];
    renderHistory();
    assert.ok(document.getElementById('history-tbody').innerHTML.includes('badge-unknown'));
    assert.equal(HISTORY_CSV_COLUMNS.find(c => c[0] === 'success')[1](imported), null);
  }
}
`+platformBehaviorRunScript)
}
