import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';

// Supply the client of the already-connected edge-debug-attach driver.
// This verifier never launches a browser or attaches a new CDP connection.
if (!process.env.EDGE_CLIENT || !process.env.CONSOLE_EVIDENCE_DIR) throw Error('EDGE_CLIENT and CONSOLE_EVIDENCE_DIR are required');
const {call, evaluate, cdp} = await import(pathToFileURL(process.env.EDGE_CLIENT).href);
const output = process.env.CONSOLE_EVIDENCE_DIR;
await fs.mkdir(output, {recursive:true, mode:0o700});
const providers = ['opencode-go','opencode-zen','aws-bedrock','openrouter','commandcode'];
const tabs = ['overview','history','performance','fallback','analytics','quota','settings'];
const report = {startedAt:new Date().toISOString(),checks:0,failures:[],scopes:[],layouts:[],accounts:[],screenshots:[]};
const check = (ok,label,detail) => { report.checks++; if (!ok) report.failures.push({label,detail}); };
const page = (fn,arg=null) => evaluate('(' + fn.toString() + ')(' + JSON.stringify(arg) + ')');

async function waitFor(expr,label) {
  const deadline = Date.now()+35000;
  while (Date.now()<deadline) {
    const value = await evaluate(expr);
    if (value) return value;
    await delay(150);
  }
  throw Error('Timed out: '+label);
}
async function settle() { await waitFor('window.__consoleCheck.pending === 0','read-only requests'); }
async function openTab(tab) {
  const result = await page(name => {
    document.querySelector('.tab[data-tab="'+name+'"]').click();
    return {active:document.querySelector('.tab.active')?.dataset.tab,current:document.querySelector('.tab[aria-current=page]')?.dataset.tab,title:document.getElementById('active-page-title').textContent,expected:t('tab.'+name)};
  },tab);
  check(result.active===tab && result.current===tab && result.title===result.expected,'navigation '+tab,result);
  await settle();
}
async function select(id,value) {
  return page(({id,value}) => {
    const select=document.getElementById(id);
    const index=[...select.options].findIndex(option=>option.value===value);
    if(index<0) throw Error('Missing option for '+id);
    const wrapper=select.closest('.theme-select');
    wrapper.querySelector('.theme-select-trigger').click();
    wrapper.querySelector('[role=option][data-index="'+index+'"]').click();
    return select.value;
  },{id,value});
}
async function key(key,code,keyCode) {
  await cdp('Input.dispatchKeyEvent',{type:'keyDown',key,code,windowsVirtualKeyCode:keyCode,...(key==='Enter'?{text:'\r',unmodifiedText:'\r'}:{})});
  await cdp('Input.dispatchKeyEvent',{type:'keyUp',key,code,windowsVirtualKeyCode:keyCode});
  await delay(100);
}
async function screenshot(name) {
  await page(() => {
    const style=document.createElement('style');
    style.id='console-evidence-mask';
    style.textContent='input[type=password],input[id*="key"],.quota-key code{visibility:hidden!important}';
    document.head.append(style);
  });
  try {
    const result=await call({op:'screenshot',name:'console-'+name});
    const destination=path.join(output,path.basename(result.screenshot));
    await fs.copyFile(result.screenshot,destination);
    report.screenshots.push(destination);
  } finally { await page(()=>document.getElementById('console-evidence-mask')?.remove()); }
}
const original=await page(() => ({
  origin:location.origin,hash:location.hash,tab:document.querySelector('.tab.active')?.dataset.tab,
  lang:document.documentElement.lang,theme:document.documentElement.dataset.theme||null,
  savedTheme:localStorage.getItem('routatic-proxy-theme'),savedLang:localStorage.getItem('routatic-proxy-lang'),
  build:document.getElementById('ui-build')?.dataset.build,
  historySort:{...currentSort},performanceSort:{field:PerfModule.sortField,dir:PerfModule.sortDir},
  headers:[...document.querySelectorAll('th.sortable')].map(element=>({panel:element.closest('.tab-content').id,key:element.dataset.sort,value:element.getAttribute('aria-sort')})),
  details:[...document.querySelectorAll('details')].map(element=>element.open),
  values:Object.fromEntries(['overview-provider','provider-filter','perf-provider','analytics-provider','quota-provider','settings-provider-jump','model-filter','scenario-filter','streaming-filter','cost-source-filter','history-search','status-filter','history-start','history-end'].map(id=>[id,document.getElementById(id).value])),
  scrolls:[...document.querySelectorAll('.tab-content')].map(element=>({id:element.id,top:element.scrollTop})),
}));
report.origin=original.origin;
report.build=original.build;
await page(() => {
  if(window.__consoleCheck) throw Error('An existing verifier is active');
  const state=window.__consoleCheck={events:[],pending:0,blocked:[],errors:[],fetch:window.fetch};
  const allowed=new Set(['/api/metrics','/api/config','/api/proxy/config','/api/catalog/lock','/api/history','/api/history/summary','/api/analytics/summary','/api/analytics/tokens/trend','/api/perf/models','/api/perf/aggregate','/api/quota']);
  state.onError=event=>state.errors.push(String(event.message||'Browser error').replace(/(?:sk-|sk_)[a-zA-Z0-9_-]+/g,'[redacted]'));
  window.addEventListener('error',state.onError);
  window.fetch=async function(resource,init) {
    const url=new URL(typeof resource==='string'?resource:resource.url,location.href);
    const method=String(init?.method||resource?.method||'GET').toUpperCase();
    const event={path:url.pathname,provider:url.searchParams.get('provider')||'',method};
    if(url.origin!==location.origin || !allowed.has(url.pathname) || !['GET','HEAD'].includes(method) || url.searchParams.has('billing_refresh')) {
      state.blocked.push(event);
      throw Error('Blocked unexpected, mutating or paid request');
    }
    state.pending++;
    try {
      const response=await Reflect.apply(state.fetch,this,[resource,init]);
      event.status=response.status;
      // Configuration, key hints and request details are never exported.
      if(['/api/analytics/summary','/api/analytics/tokens/trend','/api/history','/api/history/summary','/api/perf/models','/api/quota'].includes(event.path)) {
        const data=await response.clone().json();
        event.providerEcho=data?.provider;
        if(Array.isArray(data)) event.rowProviders=data.map(row=>row.provider);
        if(data && typeof data==='object') {
          for(const field of ['items','providers','models']) if(Array.isArray(data[field])) event[field+'Providers']=data[field].map(row=>row.provider);
          if('trend' in data) event.trendArray=Array.isArray(data.trend);
          if(data.summary) event.totalRequests=data.summary.total_requests;
          if(event.path==='/api/history/summary') event.totalRequests=data.total_requests;
          if(event.path==='/api/quota') event.account={provider:data.provider,status:data.status,reason:data.reason,currency:data.currency,blocks:(data.accounts||[]).map(account=>({error:!!account.error,credits:!!account.commandcode?.credits,subscription:!!account.commandcode?.subscription,usage:!!account.commandcode?.usage}))};
        }
      }
      state.events.push(event);
      return response;
    } catch(error) {
      event.failed=true;
      state.events.push(event);
      throw error;
    } finally { state.pending--; }
  };
});
try {
  for(const [view,id,paths] of [
    ['overview','overview-provider',['/api/analytics/summary','/api/analytics/tokens/trend','/api/perf/aggregate']],
    ['history','provider-filter',['/api/history','/api/history/summary']],
    ['performance','perf-provider',['/api/perf/models']],
    ['analytics','analytics-provider',['/api/analytics/summary','/api/analytics/tokens/trend']],
    ['quota','quota-provider',['/api/quota','/api/analytics/summary']],
  ]) {
    await openTab(view);
    for(const provider of view==='quota'?providers:[...providers,'']) {
      const marker=await evaluate('window.__consoleCheck.events.length');
      check(await select(id,provider)===provider,'platform selection '+view+'/'+provider);
      await waitFor('('+JSON.stringify(paths)+').every(path=>window.__consoleCheck.events.slice('+marker+').some(event=>event.path===path&&event.provider==='+JSON.stringify(provider)+'))',view+'/'+provider);
      await settle();
      const events=await page(({marker,provider,paths})=>window.__consoleCheck.events.slice(marker).filter(event=>event.provider===provider&&paths.includes(event.path)),{marker,provider,paths});
      report.scopes.push({view,provider:provider||'all',events});
      for(const event of events) {
        check(event.status===200&&!event.failed,'HTTP '+view+'/'+provider+event.path,event);
        if(event.path.startsWith('/api/analytics/')) check(event.providerEcho===provider,'analytics scope echo',event);
        for(const field of ['rowProviders','itemsProviders','providersProviders','modelsProviders']) if(event[field]) check(!provider||event[field].every(value=>value===provider),'platform row isolation '+view,event);
        if(event.path==='/api/analytics/tokens/trend') check(event.trendArray,'trend array',event);
        if(event.account) {
          report.accounts.push(event.account);
          check(event.account.provider===provider,'account identity',event.account);
          if(provider==='commandcode') check(event.account.status==='available'&&event.account.blocks.length>0&&event.account.blocks.every(block=>block.credits&&block.subscription&&block.usage&&!block.error),'live CommandCode account blocks',event.account);
        }
      }
      if(view==='quota') {
        const accountUI=await page(() => ({
          visible:['quota-go','quota-openrouter','quota-bedrock','quota-commandcode','quota-unavailable'].filter(id=>!document.getElementById(id).hidden),
          localNote:document.getElementById('quota-local-note').textContent,
          awsDisabled:document.getElementById('btn-fetch-bedrock-billing').disabled,
          commandPanels:document.querySelectorAll('.commandcode-panels .commandcode-block').length,
        }));
        check(accountUI.visible.length===1&&(provider==='opencode-go'||!accountUI.visible.includes('quota-go')),'single account surface '+provider,accountUI);
        check(/不等同于账户账单或余额|not an account bill or balance/.test(accountUI.localNote),'local ledger boundary '+provider);
        if(provider==='aws-bedrock') check(accountUI.awsDisabled,'AWS paid query remains disabled');
        if(provider==='commandcode') check(accountUI.commandPanels===3,'three distinct CommandCode panels',accountUI);
      }
    }
    console.log(JSON.stringify({phase:'scopes',view,failures:report.failures.length}));
  }

  await openTab('history');
  await page(()=>document.getElementById('nav-history').focus());
  for(const [pressed,code,keyCode,expected] of [['ArrowDown','ArrowDown',40,'performance'],['End','End',35,'settings'],['Home','Home',36,'overview']]) {
    await key(pressed,code,keyCode);
    const state=await page(()=>({tab:document.querySelector('.tab.active')?.dataset.tab,focus:document.activeElement?.id}));
    check(state.tab===expected&&state.focus==='nav-'+expected,'keyboard navigation '+pressed,state);
    await settle();
  }
  await openTab('history');
  await openTab('analytics');
  await page(()=>history.back());
  await waitFor('document.querySelector(".tab.active")?.dataset.tab === "history"','hash back');
  await page(()=>history.forward());
  await waitFor('document.querySelector(".tab.active")?.dataset.tab === "analytics"','hash forward');
  check(true,'hash back/forward');

  for(const [tab,column] of [['history','start_time'],['performance','avg_ms']]) {
    await openTab(tab);
    const before=await page(({tab,column})=>{
      const th=document.querySelector('#tab-'+tab+' th[data-sort="'+column+'"]');
      th.querySelector('button').focus();
      return th.getAttribute('aria-sort');
    },{tab,column});
    await key('Enter','Enter',13);
    const after=await page(({tab,column})=>{
      const th=document.querySelector('#tab-'+tab+' th[data-sort="'+column+'"]'), button=th.querySelector('button'), css=getComputedStyle(button);
      return {sort:th.getAttribute('aria-sort'),focus:button.matches(':focus-visible'),outline:css.outlineStyle,outlineWidth:css.outlineWidth,shadow:css.boxShadow};
    },{tab,column});
    check(after.sort!==before,'keyboard sorting '+tab,{before,after});
    check(after.focus&&((after.outline!=='none'&&after.outlineWidth!=='0px')||after.shadow!=='none'),'visible keyboard focus '+tab,after);
    await key('Enter','Enter',13);
  }

  await openTab('history');
  const advanced=await page(()=>{
    document.getElementById('history-advanced-filters').open=true;
    const input=document.getElementById('model-filter');
    input.value='__read_only_console_probe__';
    input.dispatchEvent(new Event('input',{bubbles:true}));
    return {count:document.getElementById('history-advanced-count').textContent,query:historyQueryParams().get('model')};
  });
  check(Number(advanced.count)>0&&advanced.query==='__read_only_console_probe__','advanced filter identity and count',advanced);
  await page(()=>document.getElementById('history-reset').click());
  await settle();
  check(await evaluate('document.getElementById("history-advanced-count").textContent === ""'),'reset advanced filter count');

  await openTab('settings');
  for(const provider of providers) {
    await select('settings-provider-jump',provider);
    const state=await page(provider=>{
      const section=document.querySelector('details[data-settings-provider="'+provider+'"]');
      return {open:section.open,fields:section.querySelectorAll('input').length,focused:section.contains(document.activeElement)};
    },provider);
    check(state.open&&state.fields>0,'reachable settings '+provider,state);
  }
  await page(()=>document.querySelector('.settings-runtime').open=true);
  const {root}=await cdp('DOM.getDocument',{depth:0});
  for(const id of ['toggle-proxy','toggle-autostart','toggle-notify']) {
    const {nodeId}=await cdp('DOM.querySelector',{nodeId:root.nodeId,selector:'#'+id});
    const {nodes}=await cdp('Accessibility.getPartialAXTree',{nodeId,fetchRelatives:false});
    check(nodes.some(node=>!node.ignored&&['checkbox','switch'].includes(node.role?.value)&&node.name?.value?.trim()),'runtime control accessible name '+id);
  }
  await page(details=>document.querySelectorAll('details').forEach((element,index)=>element.open=details[index]),original.details);

  const combinations=[];
  for(const lang of ['zh','en']) for(const theme of ['light','dark']) for(const width of [1440,390]) combinations.push({lang,theme,width});
  combinations.push({lang:'zh',theme:'light',width:768},{lang:'zh',theme:'light',width:320});
  for(const combination of combinations) {
    const {lang,theme,width}=combination;
    await cdp('Emulation.setDeviceMetricsOverride',{width,height:900,deviceScaleFactor:1,mobile:width<768});
    await cdp('Emulation.setTouchEmulationEnabled',{enabled:width<768,maxTouchPoints:1});
    // A manual theme must work even when the OS preference is the opposite.
    await cdp('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:theme==='light'?'dark':'light'}]});
    await page(({lang,theme})=>{
      if(document.documentElement.lang!==lang) document.getElementById('btn-lang-toggle').click();
      if(getComputedStyle(document.documentElement).colorScheme!==theme) document.getElementById('btn-theme-toggle').click();
    },{lang,theme});
    for(const tab of tabs) {
      await openTab(tab);
      const layout=await page(tab=>{
        const panel=document.getElementById('tab-'+tab);
        panel.scrollTop=0;
        const visible=element=>element.checkVisibility({checkOpacity:true,checkVisibilityCSS:true})&&!element.closest('[aria-hidden=true]');
        const selectors='button:not(:disabled),summary';
        return {viewport:innerWidth,pageWidth:document.documentElement.scrollWidth,panelWidth:panel.clientWidth,panelScroll:panel.scrollWidth,
          theme:getComputedStyle(document.documentElement).colorScheme,savedTheme:localStorage.getItem('routatic-proxy-theme'),
          untranslated:[...panel.querySelectorAll('[data-i18n]')].filter(visible).filter(element=>element.textContent===element.dataset.i18n).map(element=>element.dataset.i18n),
          smallTargets:[...document.querySelectorAll('.app-header '+selectors+',.app-nav button,#tab-'+tab+' '+selectors)].filter(visible).filter(element=>element.getBoundingClientRect().height<43.5).map(element=>element.id||element.className),
          empty:tab==='overview'?!document.getElementById('overview-empty').hidden:tab==='analytics'?!document.getElementById('analytics-empty').hidden:null,
        };
      },tab);
      report.layouts.push({...combination,tab,...layout});
      check(layout.pageWidth<=layout.viewport+1&&layout.panelScroll<=layout.panelWidth+1,'layout width '+[lang,theme,width,tab].join('/'),layout);
      check(layout.theme===theme&&layout.savedTheme===theme,'manual theme '+[lang,theme,width,tab].join('/'),layout);
      check(layout.untranslated.length===0,'translations '+[lang,theme,width,tab].join('/'),layout.untranslated);
      if(width<768) check(layout.smallTargets.length===0,'touch target height '+[lang,theme,width,tab].join('/'),layout.smallTargets);
      if(lang==='zh'&&(width===1440||(width===390&&theme==='light'))) await screenshot([theme,width,tab].join('-'));
    }
    console.log(JSON.stringify({phase:'layouts',...combination,failures:report.failures.length}));
  }
} catch(error) {
  report.failure=error.message;
  console.log(JSON.stringify({phase:'failure',message:error.message}));
} finally {
  try {
    for(const id of ['overview-provider','provider-filter','perf-provider','analytics-provider','quota-provider']) {
      if(await page(id=>document.getElementById(id).value,id)!==original.values[id]) await select(id,original.values[id]);
    }
    await settle();
    await page(original=>{
      for(const [id,value] of Object.entries(original.values)) document.getElementById(id).value=value;
      window.CustomSelect.syncAll();
      window.HistoryDateRange.syncFromHidden();
      syncAdvancedFilters(false);
      Object.assign(currentSort,original.historySort);
      PerfModule.sortField=original.performanceSort.field;
      PerfModule.sortDir=original.performanceSort.dir;
      for(const header of original.headers) {
        const element=document.querySelector('#'+header.panel+' th[data-sort="'+header.key+'"]');
        element.setAttribute('aria-sort',header.value);
        element.classList.toggle('asc',header.value==='ascending');
        element.classList.toggle('desc',header.value==='descending');
      }
      PerfModule.render();
      document.querySelectorAll('details').forEach((element,index)=>element.open=original.details[index]);
      if(document.documentElement.lang!==original.lang) document.getElementById('btn-lang-toggle').click();
      for(const [key,value] of [['routatic-proxy-lang',original.savedLang],['routatic-proxy-theme',original.savedTheme]]) {
        if(value===null) localStorage.removeItem(key); else localStorage.setItem(key,value);
      }
      if(original.theme===null) delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme=original.theme;
      syncThemeControl();
    },original);
    await cdp('Emulation.clearDeviceMetricsOverride');
    await cdp('Emulation.setTouchEmulationEnabled',{enabled:false});
    await cdp('Emulation.setEmulatedMedia',{features:[]});
    await openTab(original.tab);
    await page(original=>{
      original.scrolls.forEach(scroll=>document.getElementById(scroll.id).scrollTop=scroll.top);
      history.replaceState(history.state,'',location.pathname+location.search+original.hash);
    },original);
    await settle();
    const observations=await page(()=>{
      const state=window.__consoleCheck;
      window.fetch=state.fetch;
      window.removeEventListener('error',state.onError);
      delete window.__consoleCheck;
      return {network:state.events,blocked:state.blocked,errors:state.errors};
    });
    Object.assign(report,observations);
    check(!report.blocked.length,'no unexpected, mutating or paid requests',report.blocked);
    check(!report.errors.length,'no browser exceptions',report.errors);
    report.restored=true;
  } catch(error) { report.restoreError=error.message; }
  report.finishedAt=new Date().toISOString();
  report.ok=!report.failure&&!report.restoreError&&!report.failures.length;
  const evidence=path.join(output,'console-verification.json');
  await fs.writeFile(evidence,JSON.stringify(report,null,2)+'\n',{mode:0o600});
  console.log(JSON.stringify({ok:report.ok,checks:report.checks,scopes:report.scopes.length,layouts:report.layouts.length,failures:report.failures,failure:report.failure,restoreError:report.restoreError,evidence}));
  if(!report.ok) process.exitCode=1;
}
