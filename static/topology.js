// NGINX++ topology view: the REAL routing graph from /api/nginx/topology,
// auto-laid-out with dagre (window.NP.dagre), rendered as SVG with pan/zoom,
// minimap, hover path highlighting, click-to-source, search, collapse,
// live upstream health colouring and a route tester.
// Depends on globals: escapeHtml, switchTab, currentConfigPath (index.html), Editor (configflow.js).

// Datadog-style service map: round nodes, health ring, label underneath.
const TOPO_STYLE = {
  client:   { color: '#9ca3af', icon: '\uf0ac', r: 20, name: 'Clients' },
  port:     { color: '#60a5fa', icon: '\uf52b', r: 18, name: 'Listener' },
  server:   { color: '#34d399', icon: '\uf233', r: 26, name: 'Server block' },
  location: { color: '#fbbf24', icon: '\uf126', r: 17, name: 'Location' },
  upstream: { color: '#c084fc', icon: '\uf1b3', r: 26, name: 'Upstream' },
  backend:  { color: '#38bdf8', icon: '\uf1b2', r: 20, name: 'Backend' },
  static:   { color: '#94a3b8', icon: '\uf07b', r: 17, name: 'Static files' },
  return:   { color: '#fb7185', icon: '\uf064', r: 17, name: 'Return / redirect' },
};
const STATUS = {
  ok:      { ring: '#2fbf71', label: 'Healthy' },
  warn:    { ring: '#f5a623', label: 'Degraded' },
  error:   { ring: '#e5484d', label: 'Down' },
  unknown: { ring: '#4b5563', label: 'No health data' },
};
const NODE_W = 170, NODE_H = 92; // layout box: circle + two label lines

const topo = {
  raw: null,         // graph from the API
  health: {},        // backend address -> 'healthy' | 'down' | 'drained'
  collapsed: false,
  scale: 1, tx: 0, ty: 0,
  layout: null,      // { nodes: Map(id -> {x,y,node}), edges: [{from,to,points}] , width, height }
  adj: null,         // { out: Map, in: Map }
};

// Called by switchTab('topology') and the Redraw button.
async function renderLangGraphFlow() {
  const status = document.getElementById('topo-status');
  status.textContent = 'Loading…';
  try {
    const [g, h] = await Promise.all([
      fetch('/api/nginx/topology').then(r => r.json()),
      fetch('/api/nginx/upstreams/health').then(r => r.json()).catch(() => ({})),
    ]);
    topo.raw = g;
    topo.health = {};
    (h.upstreams || []).forEach(up => (up.nodes || []).forEach(n => { topo.health[n.address] = n.status; }));
    drawTopology(true);
    const servers = g.nodes.filter(n => n.type === 'server').length;
    status.textContent = `${servers} server block(s) · ${g.nodes.length} nodes`;
  } catch (err) {
    status.textContent = '';
    document.getElementById('topo-svg').innerHTML = '';
    document.getElementById('topo-empty').textContent = 'Could not load topology: ' + err;
    document.getElementById('topo-empty').classList.remove('hidden');
  }
}

function collapsedGraph(g) {
  if (!topo.collapsed) return g;
  // Remove location nodes; connect each server directly to the location's targets.
  const locs = new Set(g.nodes.filter(n => n.type === 'location').map(n => n.id));
  const edges = [];
  g.edges.forEach(e => {
    if (locs.has(e.from)) return;
    if (locs.has(e.to)) g.edges.filter(x => x.from === e.to).forEach(x => edges.push({ from: e.from, to: x.to, type: x.type }));
    else edges.push(e);
  });
  const seen = new Set();
  return { nodes: g.nodes.filter(n => !locs.has(n.id)), edges: edges.filter(e => { const k = e.from + '>' + e.to; if (seen.has(k)) return false; seen.add(k); return true; }) };
}

function drawTopology(fit) {
  const svg = document.getElementById('topo-svg');
  const empty = document.getElementById('topo-empty');
  const g = collapsedGraph(topo.raw);
  if (!g.nodes.some(n => n.type === 'server')) {
    svg.innerHTML = '';
    empty.textContent = 'No server blocks found in the nginx configuration.';
    empty.classList.remove('hidden');
    return;
  }
  empty.classList.add('hidden');

  // Layout
  const dg = new window.NP.dagre.graphlib.Graph();
  dg.setGraph({ rankdir: 'LR', nodesep: 14, ranksep: 70, marginx: 20, marginy: 20 });
  dg.setDefaultEdgeLabel(() => ({}));
  g.nodes.forEach(n => dg.setNode(n.id, { width: NODE_W, height: NODE_H }));
  g.edges.forEach(e => dg.setEdge(e.from, e.to));
  window.NP.dagre.layout(dg);

  const nodes = new Map();
  g.nodes.forEach(n => { const p = dg.node(n.id); nodes.set(n.id, { x: p.x, y: p.y, node: n }); });
  const edges = g.edges.map(e => ({ ...e, points: dg.edge(e.from, e.to).points }));
  const info = dg.graph();
  topo.layout = { nodes, edges, width: info.width, height: info.height };
  topo.adj = { out: new Map(), in: new Map() };
  edges.forEach(e => {
    (topo.adj.out.get(e.from) || topo.adj.out.set(e.from, []).get(e.from)).push(e.to);
    (topo.adj.in.get(e.to) || topo.adj.in.set(e.to, []).get(e.to)).push(e.from);
  });

  computeStatuses();

  const R = id => (TOPO_STYLE[nodes.get(id)?.node.type] || TOPO_STYLE.static).r;
  const cy = p => p.y - 14; // circle sits above its labels
  const curve = e => {
    const f = nodes.get(e.from), t = nodes.get(e.to);
    const x1 = f.x + R(e.from), y1 = cy(f), x2 = t.x - R(e.to) - 4, y2 = cy(t);
    const dx = Math.max(30, (x2 - x1) / 2);
    return `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`;
  };

  let html = `<defs>
      <pattern id="topo-grid" width="22" height="22" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r="1" fill="#1f2a33"/></pattern>
      <marker id="topo-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="5" markerHeight="5" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="#4b5563"/></marker>
      <filter id="topo-glow" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="3" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
    </defs>
    <rect width="100%" height="100%" fill="url(#topo-grid)"/>
    <g id="topo-viewport">`;
  edges.forEach(e => {
    const st = topo.status.get(e.to) || 'unknown';
    const color = st === 'error' ? '#e5484d' : st === 'warn' ? '#f5a623' : '#3b4552';
    const d = curve(e);
    html += `<g class="topo-edge-g" data-from="${escapeHtml(e.from)}" data-to="${escapeHtml(e.to)}">
      <path class="topo-edge" d="${d}" stroke="${color}" marker-end="url(#topo-arrow)" data-from="${escapeHtml(e.from)}" data-to="${escapeHtml(e.to)}"/>
      ${st !== 'error' ? `<path class="topo-flow" d="${d}"/>` : ''}
    </g>`;
  });
  nodes.forEach(({ x, y, node: n }) => {
    const st = TOPO_STYLE[n.type] || TOPO_STYLE.static;
    const status = STATUS[topo.status.get(n.id) || 'unknown'];
    const meta = n.metadata || {};
    const label = n.label.length > 22 ? n.label.slice(0, 21) + '…' : n.label;
    const sub = String(meta.sub || st.name);
    const c = y - 14;
    html += `<g class="topo-node" data-id="${escapeHtml(n.id)}" tabindex="0" role="button"
        aria-label="${escapeHtml(st.name + ' ' + n.label + ', ' + status.label)}" style="cursor:pointer">
      <title>${escapeHtml(n.label + ' (' + status.label + ')')}</title>
      <circle class="topo-halo" cx="${x}" cy="${c}" r="${st.r + 7}" fill="${status.ring}" opacity="0"/>
      <circle cx="${x}" cy="${c}" r="${st.r}" fill="#0f1720" stroke="${status.ring}" stroke-width="3"/>
      <circle cx="${x}" cy="${c}" r="${st.r - 5}" fill="${st.color}" opacity=".13"/>
      <text x="${x}" y="${c + 5}" text-anchor="middle" font-family="'Font Awesome 6 Free'" font-weight="900" font-size="${Math.round(st.r * 0.62)}" fill="${st.color}">${st.icon}</text>
      <text x="${x}" y="${c + st.r + 16}" text-anchor="middle" font-size="12" font-weight="600" fill="#e5e7eb" font-family="Inter, sans-serif">${escapeHtml(label)}</text>
      <text x="${x}" y="${c + st.r + 30}" text-anchor="middle" font-size="10" fill="#8b949e" font-family="JetBrains Mono, monospace">${escapeHtml(sub.length > 26 ? sub.slice(0, 25) + '…' : sub)}</text>
    </g>`;
  });
  html += '</g>';
  svg.innerHTML = html;

  svg.querySelectorAll('.topo-node').forEach(el => {
    const id = el.dataset.id;
    el.addEventListener('mouseenter', () => highlightPath(id));
    el.addEventListener('mouseleave', () => topo.selected ? highlightPath(topo.selected) : applySearch());
    el.addEventListener('focus', () => highlightPath(id));
    el.addEventListener('blur', () => applySearch());
    el.addEventListener('click', () => showNodePanel(id));
    el.addEventListener('keydown', ev => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); showNodePanel(id); } });
  });

  drawMinimap();
  if (fit) fitTopology(); else applyTransform();
  applySearch();
}

/* ---------- highlighting ---------- */

function reachable(id) {
  const keep = new Set([id]);
  const walk = (start, dir) => {
    const stack = [start];
    while (stack.length) {
      const cur = stack.pop();
      (topo.adj[dir].get(cur) || []).forEach(n => { if (!keep.has(n)) { keep.add(n); stack.push(n); } });
    }
  };
  walk(id, 'out'); walk(id, 'in');
  return keep;
}

function setDim(keepNodes, hitNodes = new Set(), hitEdges = new Set()) {
  document.querySelectorAll('#topo-svg .topo-node').forEach(el => {
    el.classList.toggle('topo-dim', keepNodes && !keepNodes.has(el.dataset.id));
    el.classList.toggle('topo-hit', hitNodes.has(el.dataset.id));
  });
  document.querySelectorAll('#topo-svg .topo-edge-g').forEach(el => {
    const k = el.dataset.from + '>' + el.dataset.to;
    el.classList.toggle('topo-dim', keepNodes && !(keepNodes.has(el.dataset.from) && keepNodes.has(el.dataset.to)));
    el.classList.toggle('topo-hit-edge', hitEdges.has(k));
  });
}

function highlightPath(id) { setDim(reachable(id)); }

function applySearch() {
  if (topo.route) return showRouteHighlight(topo.route);
  const q = (document.getElementById('topo-search')?.value || '').trim().toLowerCase();
  if (!q || !topo.layout) return setDim(null);
  const keep = new Set();
  topo.layout.nodes.forEach(({ node }) => {
    if (node.label.toLowerCase().includes(q) || String(node.metadata?.sub || '').toLowerCase().includes(q)) reachable(node.id).forEach(x => keep.add(x));
  });
  setDim(keep);
}

/* ---------- click to source ---------- */

let pendingGotoLine = null;

function openNodeSource(id) { // kept for keyboard shortcuts / external callers
  const n = topo.layout.nodes.get(id)?.node;
  const meta = n?.metadata || {};
  if (!meta.file) return;
  openConfigAt(meta.file, meta.line);
}

function openConfigAt(file, line) {
  currentConfigPath = file;
  pendingGotoLine = line;
  switchTab('editor'); // loads currentConfigPath, then consumePendingGoto() jumps to the line
}

function consumePendingGoto() {
  if (pendingGotoLine) { const n = pendingGotoLine; pendingGotoLine = null; setTimeout(() => Editor.gotoLine(n), 0); }
}

/* ---------- pan / zoom / fit / minimap ---------- */

function applyTransform() {
  const vp = document.getElementById('topo-viewport');
  if (vp) vp.setAttribute('transform', `translate(${topo.tx},${topo.ty}) scale(${topo.scale})`);
  updateMinimapViewport();
}

function fitTopology() {
  const svg = document.getElementById('topo-svg');
  if (!topo.layout) return;
  const { width, height } = topo.layout;
  const W = svg.clientWidth || 800, H = svg.clientHeight || 500;
  topo.scale = Math.min(1.25, Math.max(0.15, Math.min(W / (width + 20), H / (height + 20))));
  topo.tx = (W - width * topo.scale) / 2;
  topo.ty = (H - height * topo.scale) / 2;
  applyTransform();
}

function zoomTopology(factor, cx, cy) {
  const svg = document.getElementById('topo-svg');
  if (cx === undefined) { cx = svg.clientWidth / 2; cy = svg.clientHeight / 2; }
  const ns = Math.min(3, Math.max(0.1, topo.scale * factor));
  topo.tx = cx - (cx - topo.tx) * (ns / topo.scale);
  topo.ty = cy - (cy - topo.ty) * (ns / topo.scale);
  topo.scale = ns;
  applyTransform();
}

function drawMinimap() {
  const mm = document.getElementById('topo-minimap');
  if (!mm || !topo.layout) return;
  const { width, height, nodes } = topo.layout;
  mm.setAttribute('viewBox', `0 0 ${width} ${height}`);
  let html = '';
  html += `<rect width="${width}" height="${height}" fill="#0b1216"/>`;
  nodes.forEach(({ x, y, node }) => { html += `<circle cx="${x}" cy="${y - 14}" r="${(TOPO_STYLE[node.type] || TOPO_STYLE.static).r + 6}" fill="${STATUS[topo.status.get(node.id) || 'unknown'].ring}" opacity=".8"/>`; });
  html += '<rect id="topo-mm-view" fill="rgba(52,211,153,.08)" stroke="#34d399" stroke-width="6"/>';
  mm.innerHTML = html;
}

function updateMinimapViewport() {
  const r = document.getElementById('topo-mm-view');
  const svg = document.getElementById('topo-svg');
  if (!r || !svg) return;
  r.setAttribute('x', -topo.tx / topo.scale);
  r.setAttribute('y', -topo.ty / topo.scale);
  r.setAttribute('width', svg.clientWidth / topo.scale);
  r.setAttribute('height', svg.clientHeight / topo.scale);
}

function initTopologyInteractions() {
  const svg = document.getElementById('topo-svg');
  if (!svg || svg.dataset.ready) return;
  svg.dataset.ready = '1';
  svg.addEventListener('wheel', e => {
    e.preventDefault();
    const r = svg.getBoundingClientRect();
    zoomTopology(e.deltaY < 0 ? 1.12 : 1 / 1.12, e.clientX - r.left, e.clientY - r.top);
  }, { passive: false });
  let drag = null;
  svg.addEventListener('pointerdown', e => {
    if (e.target.closest('.topo-node')) return;
    drag = { x: e.clientX, y: e.clientY, tx: topo.tx, ty: topo.ty };
    svg.setPointerCapture(e.pointerId);
    svg.classList.add('dragging');
  });
  svg.addEventListener('pointermove', e => {
    if (!drag) return;
    topo.tx = drag.tx + e.clientX - drag.x;
    topo.ty = drag.ty + e.clientY - drag.y;
    applyTransform();
  });
  const end = () => { drag = null; svg.classList.remove('dragging'); };
  svg.addEventListener('pointerup', end);
  svg.addEventListener('pointercancel', end);

  const mm = document.getElementById('topo-minimap');
  mm.addEventListener('click', e => {
    if (!topo.layout) return;
    const r = mm.getBoundingClientRect();
    const gx = (e.clientX - r.left) / r.width * topo.layout.width;
    const gy = (e.clientY - r.top) / r.height * topo.layout.height;
    topo.tx = svg.clientWidth / 2 - gx * topo.scale;
    topo.ty = svg.clientHeight / 2 - gy * topo.scale;
    applyTransform();
  });
  window.addEventListener('resize', () => { if (!document.getElementById('tab-topology').classList.contains('hidden')) fitTopology(); });
}

function toggleCollapseLocations(btn) {
  topo.collapsed = !topo.collapsed;
  btn.setAttribute('aria-pressed', String(topo.collapsed));
  btn.querySelector('span').textContent = topo.collapsed ? 'Show locations' : 'Collapse locations';
  if (topo.raw) drawTopology(true);
}

/* ---------- route tester ---------- */

async function testRoute(ev) {
  if (ev) ev.preventDefault();
  const host = document.getElementById('rt-host').value.trim();
  const port = document.getElementById('rt-port').value.trim() || '80';
  let path = document.getElementById('rt-path').value.trim() || '/';
  if (!path.startsWith('/')) path = '/' + path;
  const out = document.getElementById('rt-result');
  const res = await fetch(`/api/nginx/route?${new URLSearchParams({ host, port, path })}`);
  const m = await res.json();
  if (!res.ok) { out.innerHTML = `<span class="text-rose-400">${escapeHtml(m.error || 'failed')}</span>`; return; }

  out.innerHTML = `
    <div class="font-bold ${m.found ? 'text-emerald-400' : 'text-rose-400'}">${m.found ? '→ ' + escapeHtml(m.target) : 'No server handles this request'}</div>
    <ol class="list-decimal ml-4 mt-1 space-y-0.5 opacity-80">${m.steps.map(s => `<li>${escapeHtml(s)}</li>`).join('')}</ol>
    ${m.location ? `<button class="underline mt-1" onclick="openConfigAt(${escapeHtml(JSON.stringify(m.location.file))}, ${Number(m.location.line)})">Open location in editor (${escapeHtml(m.location.file.split('/').pop())}:${Number(m.location.line)})</button>` : ''}
    ${m.found ? '<button class="underline mt-1 ml-3 opacity-70" onclick="clearRoute()">Clear highlight</button>' : ''}`;
  topo.route = m.found ? { m, port } : null;
  if (topo.collapsed && m.location) { topo.collapsed = false; drawTopology(false); }
  applySearch();
}

function showRouteHighlight({ m, port }) {
  const ids = ['client', 'port:' + port, 'server:' + m.server_index];
  if (m.location_index >= 0) ids.push(`location:${m.server_index}:${m.location_index}`);
  const last = ids[ids.length - 1];
  // include what the final node leads to (upstream → backends, static, return)
  const tail = []; const stack = [last];
  while (stack.length) (topo.adj.out.get(stack.pop()) || []).forEach(n => { if (!tail.includes(n) && !n.startsWith('location:')) { tail.push(n); stack.push(n); } });
  const all = new Set([...ids, ...tail]);
  const hitEdges = new Set();
  for (let i = 0; i + 1 < ids.length; i++) hitEdges.add(ids[i] + '>' + ids[i + 1]);
  topo.layout.edges.forEach(e => { if (all.has(e.from) && all.has(e.to) && (e.from === last || tail.includes(e.from))) hitEdges.add(e.from + '>' + e.to); });
  setDim(all, all, hitEdges);
}

function clearRoute() {
  topo.route = null;
  document.getElementById('rt-result').innerHTML = '';
  applySearch();
}

document.addEventListener('DOMContentLoaded', initTopologyInteractions);

/* ---------- health roll-up (backends → upstreams → locations → servers → ports) ---------- */

function computeStatuses() {
  const st = new Map();
  const rank = { unknown: 0, ok: 1, warn: 2, error: 3 };
  topo.layout.nodes.forEach(({ node }) => {
    if (node.type === 'backend') {
      const h = topo.health[node.metadata?.address || node.label];
      st.set(node.id, h === 'healthy' ? 'ok' : h === 'down' ? 'error' : h === 'drained' ? 'warn' : 'unknown');
    }
    if (node.type === 'static' || node.type === 'return') st.set(node.id, 'ok');
  });
  // upstream: all ok → ok, some down → warn, all down → error
  const resolve = (id, seen = new Set()) => {
    if (st.has(id)) return st.get(id);
    if (seen.has(id)) return 'unknown';
    seen.add(id);
    const kids = (topo.adj.out.get(id) || []).map(k => resolve(k, seen)).filter(x => x !== 'unknown');
    let v = 'unknown';
    if (kids.length) {
      const errs = kids.filter(k => k === 'error').length;
      if (errs === kids.length) v = 'error';
      else if (errs || kids.includes('warn')) v = 'warn';
      else v = 'ok';
    }
    st.set(id, v);
    return v;
  };
  topo.layout.nodes.forEach((_, id) => resolve(id));
  topo.status = st;
  void rank;
}

/* ---------- details side panel ---------- */

function showNodePanel(id) {
  const n = topo.layout.nodes.get(id)?.node;
  if (!n) return;
  const panel = document.getElementById('topo-panel');
  const meta = n.metadata || {};
  const st = TOPO_STYLE[n.type] || TOPO_STYLE.static;
  const status = STATUS[topo.status.get(id) || 'unknown'];
  const ins = (topo.adj.in.get(id) || []).map(x => topo.layout.nodes.get(x)?.node).filter(Boolean);
  const outs = (topo.adj.out.get(id) || []).map(x => topo.layout.nodes.get(x)?.node).filter(Boolean);
  const chip = x => {
    const s2 = STATUS[topo.status.get(x.id) || 'unknown'];
    return `<button class="flex items-center gap-2 w-full text-left px-2 py-1 rounded hover:bg-white/5" onclick="showNodePanel(${escapeHtml(JSON.stringify(x.id))})">
      <span class="w-2 h-2 rounded-full shrink-0" style="background:${s2.ring}"></span>
      <span class="truncate font-mono">${escapeHtml(x.label)}</span><span class="ml-auto opacity-50 text-[10px]">${escapeHtml((TOPO_STYLE[x.type] || {}).name || x.type)}</span></button>`;
  };
  const actions = [];
  if (meta.file) actions.push(`<button class="np-btn-primary text-xs px-3 py-1.5" onclick="openConfigAt(${escapeHtml(JSON.stringify(meta.file))}, ${Number(meta.line)})"><i class="fa-solid fa-file-code mr-1"></i>Open in editor</button>`);
  if (n.type === 'location' && meta.modifier !== '~' && meta.modifier !== '~*') actions.push(`<button class="np-btn-secondary text-xs px-3 py-1.5" onclick="prefillRoute(${escapeHtml(JSON.stringify(id))})"><i class="fa-solid fa-route mr-1"></i>Test this route</button>`);
  panel.innerHTML = `
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <div class="text-[10px] uppercase tracking-wider opacity-60">${escapeHtml(st.name)}</div>
        <div class="font-bold font-mono text-sm break-all">${escapeHtml(n.label)}</div>
      </div>
      <button onclick="closeNodePanel()" class="opacity-60 hover:opacity-100" aria-label="Close details"><i class="fa-solid fa-xmark"></i></button>
    </div>
    <div class="flex items-center gap-2 text-xs"><span class="w-2.5 h-2.5 rounded-full" style="background:${status.ring}"></span>${status.label}</div>
    <dl class="text-xs grid grid-cols-[auto,1fr] gap-x-3 gap-y-1">
      ${meta.sub ? `<dt class="opacity-60">Detail</dt><dd class="font-mono break-all">${escapeHtml(meta.sub)}</dd>` : ''}
      ${meta.file ? `<dt class="opacity-60">Source</dt><dd class="font-mono break-all">${escapeHtml(meta.file)}:${Number(meta.line)}</dd>` : ''}
      ${meta.address ? `<dt class="opacity-60">Address</dt><dd class="font-mono">${escapeHtml(meta.address)}</dd>` : ''}
    </dl>
    <div class="flex flex-wrap gap-2">${actions.join('')}</div>
    ${ins.length ? `<div><div class="text-[10px] uppercase tracking-wider opacity-60 mb-1">Receives from (${ins.length})</div>${ins.map(chip).join('')}</div>` : ''}
    ${outs.length ? `<div><div class="text-[10px] uppercase tracking-wider opacity-60 mb-1">Sends to (${outs.length})</div>${outs.map(chip).join('')}</div>` : ''}`;
  panel.classList.remove('hidden');
  topo.selected = id;
  highlightPath(id);
}

function closeNodePanel() {
  document.getElementById('topo-panel').classList.add('hidden');
  topo.selected = null;
  applySearch();
}

function prefillRoute(id) {
  const n = topo.layout.nodes.get(id)?.node;
  const srv = (topo.adj.in.get(id) || []).map(x => topo.layout.nodes.get(x)?.node).find(x => x && x.type === 'server');
  if (srv) {
    const host = srv.label.split(' ').find(h => !h.includes('*') && !h.startsWith('~') && h !== '(catch-all)');
    if (host) document.getElementById('rt-host').value = host;
    const port = (topo.adj.in.get(srv.id) || []).find(x => x.startsWith('port:'));
    if (port) document.getElementById('rt-port').value = port.slice(5);
  }
  document.getElementById('rt-path').value = n.metadata?.match || '/';
  const p = document.getElementById('rt-panel'); if (p.hidden) toggleRouteTester();
  testRoute();
}

function toggleRouteTester() {
  const p = document.getElementById('rt-panel');
  p.hidden = !p.hidden;
  document.getElementById('rt-toggle').setAttribute('aria-expanded', String(!p.hidden));
  if (!p.hidden) document.getElementById('rt-host').focus();
}

function toggleMapFullscreen() {
  const el = document.getElementById('topo-map');
  if (document.fullscreenElement) document.exitFullscreen();
  else el.requestFullscreen?.().then(() => setTimeout(fitTopology, 100));
}
document.addEventListener('fullscreenchange', () => setTimeout(() => topo.layout && fitTopology(), 100));
