package gui

import "testing"

func TestHistoryResponseModelDisplayAndExport(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const rec = {id:'observed', model:'cline-pass/route', provider:'cline-pass',
    response_model:'vmc/served', upstream_model:'deepseek/canonical', upstream_channel:'deepseek',
    requested_model:'client-model', start_time:'2026-10-27T00:00:00Z'};
  for (const lang of ['en','zh']) {
    currentLang = lang;
    for (const [row, expected] of [[rec, 'vmc/served'], [{...rec,upstream_channel:'private'}, 'vmc/served'], [{...rec,response_model:''}, 'deepseek/canonical'],
      [{...rec,response_model:'',upstream_model:''}, 'cline-pass/route']]) {
      allHistory = [row];
      renderHistory();
      const html = document.getElementById('history-tbody').innerHTML;
      assert.ok(html.includes('<strong title="' + expected + '">' + expected + '</strong>'));
      assert.ok(html.includes(providerLabel('cline-pass')) && html.includes('>' + row.upstream_channel + '</span>'));
      showHistoryDetail(row);
      const detail = document.getElementById('modal-body').innerHTML;
      assert.ok(detail.includes('<div class="detail-model">' + expected + '</div>'));
      for (const value of [providerLabel('cline-pass'), row.model, row.upstream_model, row.upstream_channel, row.requested_model]) {
        if (value) assert.ok(detail.includes(value), 'missing attribution ' + value);
      }
    }
    for (const field of ['model','requested_model','provider','response_model','upstream_model','upstream_channel']) {
      assert.equal(HISTORY_CSV_COLUMNS.find(c => c[0] === field)?.[1](rec), rec[field]);
    }
    const malicious = {...rec, response_model:'<img src=x>', upstream_channel:''};
    allHistory = [malicious];
    renderHistory();
    const html = document.getElementById('history-tbody').innerHTML;
    assert.ok(html.includes('&lt;img src=x&gt;') && !html.includes('<img'));
    assert.ok(!html.includes('badge-channel'));
    showHistoryDetail(malicious);
    assert.ok(!document.getElementById('modal-body').innerHTML.includes('<img'));
  }
}
`+platformBehaviorRunScript)
}
