import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import test from 'node:test';

// Exercise the actual standalone page's sizing code without a browser.
const source = readFileSync(new URL('./page.html', import.meta.url), 'utf8');
const sizing = source.slice(source.indexOf('  function directLanguages'), source.indexOf('  function worst'));
function calculate(node) {
  return vm.runInNewContext(sizing + '\n({items: childItems(node)});', { node });
}
function folder(name, languages, children = []) {
  const lines = Object.values(languages).reduce((sum, stat) => sum + stat.lines, 0);
  const directLanguages = Object.fromEntries(Object.entries(languages).map(([name, stat]) => [name, {
    lines: stat.lines - children.reduce((sum, child) => sum + (child.languages[name]?.lines || 0), 0),
    files: (stat.files || 0) - children.reduce((sum, child) => sum + (child.languages[name]?.files || 0), 0),
  }]).filter(([, stat]) => stat.lines > 0));
  return {
    name, path: name, languages, lines, children, directLanguages,
    directLines: lines - children.reduce((sum, child) => sum + child.lines, 0),
  };
}

test('rectangle areas use raw lines and preserve direct-file totals', () => {
  const java = folder('java', { Java: {lines:10000} });
  const html = folder('html', { HTML: {lines:10000} });
  const root = folder('project', { Java: {lines:10000}, HTML: {lines:10000}, Markdown: {lines:500} }, [java, html]);
  const before = JSON.stringify(root);
  const {items} = calculate(root);
  assert.equal(items[0].value, 10000);
  assert.equal(items[1].value, 10000);
  assert.equal(items.find(item => item.direct).value, 500);
  assert.equal(items.reduce((sum, item) => sum + item.value, 0), root.lines);
  assert.equal(JSON.stringify(root), before);
  assert.equal(calculate(folder('empty', {})).items.length, 0);
});

test('file lists show 20 or 50 names, full totals, and escaped filenames', () => {
  const helpers = source.slice(source.indexOf('  function formatNumber'), source.indexOf('  function hueFor'));
  const listing = source.slice(source.indexOf('  function fileListHTML'), source.indexOf('  function renderFileList'));
  const node = { directFiles: 137, fileList: Array.from({length:50}, (_, i) => ({name: i === 0 ? '<img>.go' : 'file-' + i + '.go', lines: 100-i})) };
  const html = limit => vm.runInNewContext(helpers + listing + '\nfileListHTML(node, limit)', {node, limit});
  assert.equal((html(20).match(/class="file-row"/g) || []).length, 20);
  assert.match(html(20), /Showing 20 of 137 files/);
  assert.match(html(20), /&lt;img&gt;.go/);
  assert.equal((html(50).match(/class="file-row"/g) || []).length, 50);
  assert.match(html(50), /Showing 50 of 137 files/);
  delete node.fileList;
  assert.match(html(20), /Regenerate it/);
});

test('language chips rank raw lines, separate names, escape labels, and fit available space', () => {
  const helpers = source.slice(source.indexOf('  function formatNumber'), source.indexOf('  function hueFor'));
  const chips = source.slice(source.indexOf('  function languageStyle'), source.indexOf('  function languageBar'));
  const node = folder('mixed', { HTML: {lines:200}, Go: {lines:150}, SQL: {lines:50}, CSS: {lines:50}, Python: {lines:0} });
  const evaluate = expression => vm.runInNewContext(helpers + sizing + chips + '\n' + expression, {node});
  const html = evaluate('languageChips(node)');
  assert.equal((html.match(/class="language-chip"/g) || []).length, 2);
  assert.match(html, />HTML<.*>Go</);
  assert.match(html, /class="language-more">\+2</);
  assert.doesNotMatch(html, />SQL<|>Python</);
  const single = evaluate('languageChips({languages:{Go:{lines:1}}})');
  assert.match(single, />Go</);
  assert.doesNotMatch(single, /language-more/);
  assert.doesNotMatch(evaluate('languageChips({languages:{}})'), /class="language-chip"/);
  const escaped = evaluate('languageChips({languages:{"<script>":{lines:1}}})');
  assert.match(escaped, /&lt;script&gt;/);
  assert.match(escaped, /--language-saturation:0%/);
  const distinct = evaluate('languageChips({languages:{Java:{lines:2},JavaScript:{lines:1}}})');
  assert.match(distinct, /--language-hue:25.*>Java<.*--language-hue:48.*>JavaScript</);
  assert.equal(evaluate('showTileLanguages(node, {w:140,h:76})'), true);
  assert.equal(evaluate('showTileLanguages(node, {w:139,h:76})'), false);
  assert.equal(evaluate('showTileLanguages(node, {w:200,h:54})'), false);
});

test('direct-file labels exclude descendant languages without mutating totals', () => {
  const child = folder('child', { Go: {lines:100,files:2}, HTML: {lines:200,files:1} });
  const root = folder('project', { Go: {lines:150,files:3}, HTML: {lines:200,files:1}, SQL: {lines:25,files:1} }, [child]);
  const before = JSON.stringify(root);
  const direct = calculate(root).items.find(item => item.direct).node;
  assert.deepEqual(JSON.parse(JSON.stringify(direct.languages)), {Go:{lines:50,files:1}, SQL:{lines:25,files:1}});
  assert.equal(JSON.stringify(root), before);
});

test('shares compare selection to project and focused view without duplicating root totals', () => {
  const helper = source.slice(source.indexOf('  function shareHTML'), source.indexOf('  function renderDetails'));
  const root = {lines:1000};
  const render = focus => vm.runInNewContext(helper + '\nshareHTML({lines:100})', {
    root, focus, percent: (value, total) => value / total * 100 + '%'
  });
  assert.match(render({lines:200}), /50% of lines in the current view/);
  assert.match(render(root), /10% of project lines/);
  assert.doesNotMatch(render(root), /current view/);
});


function functions(first, next) {
  const start = source.indexOf('  function ' + first);
  const end = next ? source.indexOf('  function ' + next, start) : source.indexOf('\n  const repositoryStats', start);
  assert.ok(start >= 0 && end > start, `missing page functions ${first}/${next}`);
  return source.slice(start, end);
}
const helpers = functions('formatNumber', 'hueFor');
const language = functions('languageStyle', 'shareHTML');

test('complete embedded script has valid JavaScript syntax', () => {
  const script = source.slice(source.indexOf('<script>') + '<script>'.length, source.indexOf('</script>', source.indexOf('<script>')));
  assert.doesNotThrow(() => new vm.Script(script));
});

test('blank descendants cannot leak file counts into direct languages', () => {
  const root = folder('project', {Go:{lines:10,files:8}});
  root.directFiles = 2;
  root.directLanguages = {Go:{lines:10,files:2}};
  const before = JSON.stringify(root);
  const direct = calculate(root).items[0].node;
  assert.equal(direct.files, 2);
  assert.equal(direct.languages.Go.files, 2);
  assert.equal(JSON.stringify(root), before);
});

test('squarified areas conserve raw proportions and stay in bounds', () => {
  const items = [100, 50, 20, 10, 1].map(value => ({value}));
  const box = {x:3,y:5,w:900,h:480};
  const layouts = vm.runInNewContext(functions('worst', 'tileKey') + '\nsquarify(items, box)', {items,box});
  assert.equal(layouts.length, items.length);
  for (const item of layouts) {
    assert.ok(item.box.w > 0 && item.box.h > 0);
    assert.ok(item.box.x >= box.x - 1e-8 && item.box.y >= box.y - 1e-8);
    assert.ok(item.box.x + item.box.w <= box.x + box.w + 1e-8);
    assert.ok(item.box.y + item.box.h <= box.y + box.h + 1e-8);
    assert.ok(Math.abs(item.box.w * item.box.h / (box.w * box.h) - item.value / 181) < 1e-8);
  }
});

test('branch hues stay anchored below repository root and separate language colors', () => {
  const leaf = {path:'src/deep/leaf'}, deep = {path:'src/deep'}, branch = {path:'src'};
  const root = {path:'.', children:[branch, {path:'other'}]};
  const parents = new Map([[branch.path,root],[deep.path,branch],[leaf.path,deep]]);
  const context = vm.createContext({root,parents,branch,deep,leaf});
  vm.runInContext(functions('hueFor','directLanguages') + language, context);
  const hues = vm.runInContext('[branchHue(branch),branchHue(deep),branchHue(leaf)]',context);
  assert.equal(hues[0], hues[1]); assert.equal(hues[1], hues[2]);
  assert.match(vm.runInContext('languageBar("Go",25,100)',context), /--language-hue:185.*width:25%/);
  assert.match(vm.runInContext('languageBar("unknown",200,100)',context), /--language-saturation:0%.*width:100%/);
});

function element() {
  const classes = new Set();
  return {
    innerHTML:'', style:{setProperty() {}}, attributes:{}, events:{},
    focus(options) {this.focused=true; this.focusOptions=options;},
    classList:{add(...names) {names.forEach(name => classes.add(name));}, remove(name) {classes.delete(name);}, contains(name) {return classes.has(name);}},
    setAttribute(name, value) {this.attributes[name] = value;},
    addEventListener(name, callback) {this.events[name] = callback;},
    appendChild() {},
  };
}

test('tile labels progressively disappear; activation selects or explores', () => {
  const node = folder('<src>', {Go:{lines:25,files:1}});
  const selected = {path:'.'}, visibleTiles = new Map();
  let inspected, explored;
  const context = vm.createContext({
    document:{createElement:element}, map:{appendChild() {}}, selected, visibleTiles,
    showTooltip() {}, hideTooltip() {}, selectNode(node) {inspected=node;}, focusOn(node) {explored=node;},
    node,
  });
  vm.runInContext(helpers + sizing + language + functions('tileKey','renderChildren'), context);
  const tile = (w,h,direct=false) => vm.runInContext(`makeTile({node,value:25,direct:${direct}}, {x:0,y:0,w:${w},h:${h}}, 0, 158)`,context);
  assert.match(tile(200,90).innerHTML, /&lt;src&gt;.*25 lines.*language-chip/);
  assert.doesNotMatch(tile(139,75).innerHTML, /language-chip/);
  assert.match(tile(139,75).innerHTML, /25 lines/);
  assert.doesNotMatch(tile(89,47).innerHTML, /tile-lines/);
  assert.match(tile(89,47).innerHTML, /&lt;src&gt;/);
  assert.equal(tile(63,29).innerHTML, '');
  const active = tile(200,90);
  active.events.keydown({key:'Enter',preventDefault() {}});
  assert.equal(inspected,node); assert.equal(explored,undefined);
  active.events.keydown({key:'Enter',shiftKey:true,preventDefault() {}});
  assert.equal(explored,node);
  explored=undefined;
  node.owner={path:'owner'};
  const direct = tile(200,90,true);
  direct.events.dblclick({stopPropagation() {}});
  assert.equal(explored,undefined);
  direct.events.keydown({key:'Enter',shiftKey:true,preventDefault() {}});
  assert.equal(inspected,node); assert.equal(explored,undefined);
});

test('direct-region selection cannot collide with unusual real folder paths and survives redraw', () => {
  const key = node => vm.runInNewContext(functions('tileKey','makeTile') + '\ntileKey(node)',{node});
  const direct = {path:'src::direct',owner:{path:'src'}};
  assert.notEqual(key(direct),key({path:'src::direct'}));
  assert.equal(key(direct),key({path:'src::direct',owner:{path:'src'}}));
});

test('adaptive nesting respects selected levels and avoids small interiors', () => {
  const child = folder('child',{Go:{lines:100}},[folder('grandchild',{Go:{lines:100}})]);
  const root = folder('root',{Go:{lines:100}},[child]);
  let tiles=[];
  const depth={value:'2'};
  const context=vm.createContext({document:{getElementById() {return depth;}},root,
    makeTile(item,box,level) {tiles.push({item,box,level}); return element();}, branchHue() {return 158;}, showTileLanguages() {return true;},
  });
  vm.runInContext(sizing + functions('worst','tileKey') + functions('renderChildren','showTooltip'),context);
  const render = box => {tiles=[]; context.box=box; vm.runInContext('renderChildren(root,box,0,158)',context); return tiles;};
  assert.equal(render({x:0,y:0,w:500,h:500}).length,2);
  depth.value='1'; assert.equal(render({x:0,y:0,w:500,h:500}).length,1);
  depth.value='4'; assert.equal(render({x:0,y:0,w:100,h:80}).length,1);
  assert.equal(render({x:0,y:0,w:20,h:20}).length,0);
});

test('file-list expansion toggles between 20 and 50 and restores keyboard focus', () => {
  const node={directFiles:80,fileList:Array.from({length:50},(_,i)=>({name:`file${i}`,lines:50-i}))};
  const button={addEventListener(name,callback) {this.click=callback;},focus() {this.focused=true;}};
  const host={querySelector() {return button;}};
  vm.runInNewContext(helpers + functions('fileListHTML','languageStyle') + '\nrenderFileList(host,node)',{host,node});
  assert.match(host.innerHTML,/Showing 20 of 80/);
  button.click(); assert.match(host.innerHTML,/Showing 50 of 80/); assert.equal(button.focused,true);
  button.click(); assert.match(host.innerHTML,/Showing 20 of 80/);
});

test('folder and direct-file details use exact totals, shares, escaped paths, and ordered subfolders', () => {
  const small=folder('small',{Go:{lines:10,files:1}}), large=folder('<large>',{Go:{lines:50,files:2}});
  const root=folder('project',{Go:{lines:100,files:5}},[small,large]);
  root.files=5; root.directFiles=2; root.fileList=[];
  const focus={lines:200};
  const hosts=new Map();
  const details={innerHTML:'',querySelector(selector) {
    if (!hosts.has(selector)) hosts.set(selector,{addEventListener() {},insertAdjacentHTML(where,text) {this.html=(this.html||'')+text;}});
    return hosts.get(selector);
  },querySelectorAll() {return [];}};
  const context=vm.createContext({root,focus,selected:root,details,byPath:new Map(),focusOn() {},renderFileList() {}});
  vm.runInContext(helpers+sizing+language+functions('shareHTML','selectNode'),context);
  vm.runInContext('renderDetails()',context);
  assert.match(details.innerHTML, /100<\/strong>nonblank lines/);
  assert.ok(details.innerHTML.indexOf('&lt;large&gt;') < details.innerHTML.indexOf('>small<'));
  assert.match(details.innerHTML,/100% of project lines.*50% of lines in the current view/);
  context.selected=calculate(root).items.find(item=>item.direct).node;
  vm.runInContext('renderDetails()',context);
  assert.match(details.innerHTML,/40<\/strong>nonblank lines.*2<\/strong>recognized files/);
  assert.match(hosts.get('.summary').html,/40% of project lines.*20% of lines in the current view/);
  assert.match(hosts.get('.file-list').html,/40 lines/);
});

test('focus, breadcrumbs, leaf summaries, and Up/Repository state preserve folder context', () => {
  const root={name:'project',path:'.',children:[],lines:20,files:4};
  const leaf={name:'<leaf>',path:'src/<leaf>',children:[],lines:10,files:3,directFiles:1};
  root.children=[leaf];
  const parents=new Map([[leaf.path,root]]);
  const elements=new Map(['depth','up','reset','leaf-back'].map(id=>[id,element()]));
  const caption={}, map={innerHTML:'',clientWidth:600,clientHeight:500,querySelector() {return {};}}, details={scrollTop:8};
  const crumbs={buttons:[],appendChild(button) {this.buttons.push(button);},
    set innerHTML(value) {this.buttons=[];},
    querySelector() {return this.buttons.find(button=>button.attributes['aria-current']==='location');},
  };
  let listed;
  const context=vm.createContext({root,focus:root,selected:root,parents,map,details,crumbs,visibleTiles:new Map(),
    document:{getElementById(id) {return elements.get(id);},querySelector() {return caption;},createElement:element},
    hideTooltip() {},renderDetails() {},renderChildren() {},hueFor() {return 158;},childItems() {return [];},
    renderFileList(host,node) {listed=node;},escapeHTML: value=>String(value).replaceAll('<','&lt;').replaceAll('>','&gt;'),formatNumber:String,
  });
  vm.runInContext(functions('ancestry'),context);
  vm.runInContext('focusOn(root.children[0])',context);
  assert.equal(context.focus,leaf); assert.equal(context.selected,leaf); assert.equal(details.scrollTop,0);
  assert.match(map.innerHTML,/No subfolders with nonblank source lines/);
  assert.match(map.innerHTML,/Totals include recognized files in subfolders; the list below shows only files directly/);
  assert.match(map.innerHTML,/&lt;leaf&gt;/);
  assert.equal(listed,leaf);
  assert.equal(elements.get('depth').disabled,true); assert.equal(caption.hidden,true);
  assert.equal(elements.get('up').disabled,false); assert.equal(elements.get('reset').disabled,false);
  const destination = crumbs.querySelector();
  assert.equal(destination.attributes['aria-current'],'location');
  assert.equal(destination.focused,true);
  assert.equal(destination.focusOptions.preventScroll,true);
  elements.get('leaf-back').events.click();
  assert.equal(context.focus,root);
  assert.equal(crumbs.querySelector().focused,true);
  assert.notEqual(crumbs.querySelector(),destination);
  assert.equal(elements.get('up').disabled,true); assert.equal(elements.get('reset').disabled,true);
});

test('global search finds paths case-insensitively, ranks by lines, caps results, and escapes labels', () => {
  const nodes=Array.from({length:25},(_,i)=>({name:'<src>',path:'src/'+i,lines:i+1,files:1}));
  const search={value:'SRC',focus() {this.focused=true;}}, results={innerHTML:'',buttons:[],appendChild(button) {this.buttons.push(button);}};
  let focused;
  const context=vm.createContext({search,results,byPath:new Map(nodes.map(node=>[node.path,node])),document:{createElement:element},
    focusOn(node) {focused=node;},
  });
  const start=source.indexOf('  function searchFolders');
  const end=source.indexOf('  search.addEventListener',start);
  vm.runInContext(helpers+source.slice(start,end)+'\nsearchFolders()',context);
  assert.equal(results.buttons.length,20);
  assert.match(results.buttons[0].innerHTML,/src\/24/);
  results.buttons[0].events.click(); assert.equal(focused,nodes[24]); assert.equal(search.value,''); assert.equal(search.focused,true);
  search.value='nothing'; vm.runInContext('searchFolders()',context);
  assert.match(results.innerHTML,/role="status">No folders found/);
  nodes[0].path='src/<unsafe>'; search.value='unsafe'; results.buttons=[];
  vm.runInContext('searchFolders()',context); assert.match(results.buttons[0].innerHTML,/&lt;unsafe&gt;/);
});

test('repository totals retain wrapping and shrinkable CSS with large counts at narrow breakpoints', () => {
  const styles=source.slice(source.indexOf('<style>')+7,source.indexOf('</style>'));
  const statsRule=styles.match(/^#repository-stats \{([^}]+)\}/m)[1];
  assert.match(statsRule,/flex:0 1 auto/);
  assert.match(statsRule,/min-width:0/);
  assert.match(statsRule,/max-width:100%/);
  assert.match(statsRule,/white-space:normal/);
  assert.match(statsRule,/overflow-wrap:anywhere/);
  for (const rule of styles.matchAll(/(?:#repository-stats|\.repository-counts|\.repository-loc)\s*\{([^}]+)\}/g)) {
    assert.doesNotMatch(rule[1],/white-space:nowrap|flex-shrink:0/);
  }
  assert.match(styles,/@media\(max-width:1100px\).*#repository-stats \{[^}]*flex-basis:100%/);
  assert.match(styles,/@media\(max-width:680px\)/);
  const stats=element();
  const root={lines:999999999999,files:999999999999};
  const start=source.indexOf('  const repositoryStats');
  const end=source.indexOf('  document.getElementById("up")',start);
  vm.runInNewContext(helpers+functions('percent','selectNode')+source.slice(start,end), {
    root,byPath:{size:999999999999},document:{getElementById() {return stats;}},
  });
  assert.match(stats.innerHTML,/999,999,999,999 files · 999,999,999,999 folders/);
  assert.doesNotMatch(stats.innerHTML,/&nbsp;|\u00a0|nowrap/);
});

test('Shift+Enter, Explore, and subfolder navigation focus the newly rendered destination breadcrumb', () => {
  const child=folder('child',{Go:{lines:10,files:1}});
  child.files=1;
  const root=folder('root',{Go:{lines:20,files:2}},[child]);
  root.files=2;
  let active, destination;
  const crumbHost={querySelector() {return destination;}};
  const explore=element(), row=element(); row.dataset={path:child.path};
  const summary={insertAdjacentHTML() {}}, fileList={insertAdjacentHTML() {}};
  const details={scrollTop:10,querySelector(selector) {
    return selector==='.focus-button' ? explore : selector==='.summary' ? summary : fileList;
  },querySelectorAll() {return [row];}};
  const context=vm.createContext({root,focus:root,selected:child,details,crumbs:crumbHost,
    byPath:new Map([[child.path,child]]),visibleTiles:new Map(),map:{appendChild() {}},
    document:{createElement:element},showTooltip() {},hideTooltip() {},selectNode() {},renderFileList() {},
    render() {
      active=null; // Old tiles, detail actions, and breadcrumb buttons are removed.
      destination={focus(options) {active=this; this.options=options;}};
    },
  });
  vm.runInContext(helpers+sizing+language+functions('shareHTML','selectNode')+
    functions('tileKey','renderChildren')+functions('focusOn','render'),context);
  const tile=vm.runInContext('makeTile({node:selected,direct:false},{x:0,y:0,w:200,h:100},0,158)',context);
  active=tile;
  tile.events.keydown({key:'Enter',shiftKey:true,preventDefault() {}});
  assert.equal(active,destination); assert.equal(context.focus,child);
  context.focus=root; context.selected=child;
  vm.runInContext('renderDetails()',context); active=explore;
  explore.events.click(); assert.equal(active,destination); assert.equal(context.focus,child);
  context.focus=root; context.selected=root;
  vm.runInContext('renderDetails()',context); active=row;
  row.events.click(); assert.equal(active,destination); assert.equal(context.focus,child);
  assert.equal(destination.options.preventScroll,true);
});
