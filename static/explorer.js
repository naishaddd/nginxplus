// Config editor workspace (VS Code-style): explorer tree on the left,
// open-file tabs with unsaved markers on top, one shared editor below.
// Depends on: Editor / scheduleLint / openPreview (configflow.js), consumePendingGoto (topology.js),
// currentConfigPath + escapeHtml (index.html).

const ROOT = '/etc/nginx/';
const explorer = {
  files: [],
  open: [],            // [{ path, original, content }]
  activePath: '',      // the tab whose content is in the editor (currentConfigPath may change first)
  expanded: new Set(JSON.parse(localStorage.getItem('np-explorer-open') || '["conf.d"]')),
  filter: '',
};

function persistExpanded() { localStorage.setItem('np-explorer-open', JSON.stringify([...explorer.expanded])); }

// Called by switchTab('editor').
async function fetchConfigFiles() {
  try {
    const res = await fetch('/api/nginx/files');
    const data = await res.json();
    explorer.files = (data.files || []).filter(f => f.startsWith(ROOT)).sort();
    renderExplorer();
  } catch (err) {
    document.getElementById('explorer-tree').innerHTML = `<div class="p-3 text-xs" style="color:var(--np-error)">${escapeHtml(err)}</div>`;
  }
  if (currentConfigPath) await loadConfigFile(currentConfigPath);
  else renderTabs();
}

function buildTree(paths) {
  const root = { dirs: {}, files: [] };
  paths.forEach(p => {
    const parts = p.slice(ROOT.length).split('/');
    let node = root;
    parts.slice(0, -1).forEach((d, i) => {
      node.dirs[d] = node.dirs[d] || { dirs: {}, files: [], path: parts.slice(0, i + 1).join('/') };
      node = node.dirs[d];
    });
    node.files.push(p);
  });
  return root;
}

const FILE_ICON = name =>
  name.endsWith('.conf') ? ['fa-gear', 'var(--np-accent)'] :
  /mime|types/.test(name) ? ['fa-list', 'var(--np-info)'] :
  /\.(crt|pem|key)$/.test(name) ? ['fa-key', 'var(--np-warn)'] :
  ['fa-file-lines', 'var(--np-ink-faint)'];

function renderExplorer() {
  const q = explorer.filter.toLowerCase();
  const paths = q ? explorer.files.filter(f => f.toLowerCase().includes(q)) : explorer.files;
  const tree = buildTree(paths);
  const dirty = new Set(explorer.open.filter(isDirty).map(t => t.path));
  const row = (depth, inner, attrs) => `<button ${attrs} class="explorer-row" style="padding-left:${8 + depth * 14}px">${inner}</button>`;

  const walk = (node, depth) => {
    let html = '';
    Object.keys(node.dirs).sort().forEach(d => {
      const dir = node.dirs[d];
      const open = q || explorer.expanded.has(dir.path);
      html += row(depth, `<i class="fa-solid fa-chevron-${open ? 'down' : 'right'} fa-fw text-[9px] opacity-50"></i><i class="fa-solid fa-folder${open ? '-open' : ''} fa-fw" style="color:var(--np-warn)"></i><span class="truncate">${escapeHtml(d)}</span>`,
        `data-dir="${escapeHtml(dir.path)}" aria-expanded="${open}"`);
      if (open) html += walk(dir, depth + 1);
    });
    node.files.forEach(f => {
      const name = f.split('/').pop();
      const [icon, color] = FILE_ICON(name);
      const active = f === explorer.activePath ? ' is-active' : '';
      html += row(depth, `<i class="fa-fw opacity-0 text-[9px]"></i><i class="fa-solid ${icon} fa-fw" style="color:${color}"></i><span class="truncate">${escapeHtml(name)}</span>${dirty.has(f) ? '<span class="explorer-dirty" title="Unsaved changes"></span>' : ''}`,
        `data-file="${escapeHtml(f)}" class="explorer-row${active}" title="${escapeHtml(f)}"`).replace('class="explorer-row" ', '');
    });
    return html;
  };
  const el = document.getElementById('explorer-tree');
  el.innerHTML = walk(tree, 0) || '<div class="p-3 text-xs np-muted">No files match.</div>';
  el.querySelectorAll('[data-dir]').forEach(b => b.onclick = () => {
    const d = b.dataset.dir;
    explorer.expanded.has(d) ? explorer.expanded.delete(d) : explorer.expanded.add(d);
    persistExpanded();
    renderExplorer();
  });
  el.querySelectorAll('[data-file]').forEach(b => b.onclick = () => loadConfigFile(b.dataset.file));
}

function filterExplorer(v) { explorer.filter = v; renderExplorer(); }

/* ---------- tabs ---------- */

const isDirty = t => t.content !== t.original;
const activeEditorTab = () => explorer.open.find(t => t.path === explorer.activePath);

function stashActive() {
  const t = activeEditorTab();
  if (t) t.content = Editor.get();
}

// Open (or focus) a file in a tab. Name kept for existing callers.
async function loadConfigFile(path) {
  stashActive();
  let tab = explorer.open.find(t => t.path === path);
  if (!tab) {
    try {
      const res = await fetch('/api/nginx/file?path=' + encodeURIComponent(path));
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || res.status);
      tab = { path, original: data.content || '', content: data.content || '' };
      explorer.open.push(tab);
    } catch (err) {
      toast(`Could not open ${path}: ${err.message}`, 'error');
      return;
    }
  }
  currentConfigPath = path;
  explorer.activePath = path;
  suppressDirty = true;
  Editor.set(tab.content);
  suppressDirty = false;
  renderTabs();
  renderExplorer();
  consumePendingGoto();
}

function closeTab(path, ev) {
  if (ev) ev.stopPropagation();
  stashActive();
  const i = explorer.open.findIndex(t => t.path === path);
  if (i < 0) return;
  if (isDirty(explorer.open[i]) && !confirm(`Discard unsaved changes to ${path.split('/').pop()}?`)) return;
  explorer.open.splice(i, 1);
  if (path === explorer.activePath) {
    const next = explorer.open[Math.max(0, i - 1)];
    currentConfigPath = explorer.activePath = next ? next.path : '';
    suppressDirty = true; Editor.set(next ? next.content : ''); suppressDirty = false;
  }
  renderTabs();
  renderExplorer();
}

function renderTabs() {
  const bar = document.getElementById('editor-tabs');
  bar.innerHTML = explorer.open.map(t => {
    const name = t.path.split('/').pop();
    const active = t.path === explorer.activePath;
    return `<div role="tab" aria-selected="${active}" tabindex="0" class="editor-tab${active ? ' is-active' : ''}" title="${escapeHtml(t.path)}"
        data-path="${escapeHtml(t.path)}">
      <i class="fa-solid fa-gear text-[10px]" style="color:var(--np-accent)"></i><span>${escapeHtml(name)}</span>
      <button class="editor-tab-close${isDirty(t) ? ' is-dirty' : ''}" aria-label="Close ${escapeHtml(name)}" data-close="${escapeHtml(t.path)}"><i class="fa-solid fa-xmark"></i></button>
    </div>`;
  }).join('');
  bar.querySelectorAll('[data-path]').forEach(el => {
    el.onclick = () => loadConfigFile(el.dataset.path);
    el.onauxclick = e => { if (e.button === 1) closeTab(el.dataset.path, e); };
    el.onkeydown = e => { if (e.key === 'Enter') loadConfigFile(el.dataset.path); };
  });
  bar.querySelectorAll('[data-close]').forEach(b => b.onclick = e => closeTab(b.dataset.close, e));
  const t = activeEditorTab();
  document.getElementById('active-filename').textContent = t ? t.path : 'No file open';
  document.getElementById('editor-empty').hidden = !!t;
}

// Called on every edit (configflow.js wires CodeMirror / textarea changes here).
let suppressDirty = false;
let dirtyTimer = null;
function onEditorChanged() {
  if (suppressDirty) return;
  clearTimeout(dirtyTimer);
  dirtyTimer = setTimeout(() => { stashActive(); renderTabs(); renderExplorer(); }, 150);
}

// After a successful apply, the editor content becomes the new baseline.
function markActiveSaved() {
  const t = activeEditorTab();
  if (t) { t.content = Editor.get(); t.original = t.content; renderTabs(); renderExplorer(); }
}

window.addEventListener('beforeunload', e => {
  stashActive();
  if (explorer.open.some(isDirty)) { e.preventDefault(); e.returnValue = ''; }
});
