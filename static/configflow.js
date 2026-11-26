// NGINX++ config workflow UI: real snapshot diffs, save → preview → apply & reload,
// live lint "Problems" panel, directive help, and recipe forms.
// Depends on globals from index.html: currentConfigPath, activeSnapshotID, escapeHtml, testSyntax, switchTab.

/* ------------------------------------------------------------------ */
/* Editor adapter: CodeMirror 6 (window.NP from vendor/bundle.js), or  */
/* the plain <textarea> if the bundle failed to load.                  */
/* ------------------------------------------------------------------ */

let cm = null;
const textarea = () => document.getElementById('config-content');
const Editor = {
  get: () => cm ? cm.get() : textarea().value,
  set: v => { if (cm) cm.set(v); else textarea().value = v; scheduleLint(); },
  gotoLine: n => {
    if (cm) return cm.gotoLine(n);
    const ta = textarea(), lines = ta.value.split('\n');
    const start = lines.slice(0, n - 1).reduce((acc, l) => acc + l.length + 1, 0);
    ta.focus(); ta.setSelectionRange(start, start + (lines[n - 1] || '').length);
  },
  insert: t => {
    if (cm) return cm.insert(t);
    const ta = textarea(), pos = ta.selectionStart ?? ta.value.length;
    ta.value = ta.value.slice(0, pos) + t + ta.value.slice(pos);
  },
};

async function fetchLint(text) {
  const { ok, data } = await postJSON('/api/nginx/lint', { content: text });
  const findings = ok ? (data.findings || []) : [];
  renderLint(document.getElementById('editor-problems'), findings);
  document.getElementById('problems-count').innerText = findings.length;
  return findings;
}

function initEditor() {
  if (!window.NP || cm) return;
  const ta = textarea();
  const host = document.createElement('div');
  host.id = 'config-editor';
  host.className = 'flex-1 min-h-0 overflow-hidden';
  ta.after(host);
  ta.classList.add('hidden');
  cm = window.NP.createEditor(host, {
    docs: NGINX_DOCS,
    dark: true, // the editor pane is always dark, like the old textarea
    lint: fetchLint,
    onCursor: ({ line, word }) => showDirectiveHelp(line, word),
    onChange: () => typeof onEditorChanged === 'function' && onEditorChanged(),
    onSave: () => openPreview(),
  });
}

/* ------------------------------------------------------------------ */
/* Shared: diff rendering                                              */
/* ------------------------------------------------------------------ */

function renderDiff(container, diff, { contextLines = 3 } = {}) {
  const lines = diff.lines || [];
  if (!lines.length || (diff.additions === 0 && diff.deletions === 0)) {
    container.innerHTML = '<div class="opacity-60 p-2">No changes. The file is identical.</div>';
    return;
  }
  // Collapse long unchanged runs so the reader sees only the hunks.
  const keep = new Array(lines.length).fill(false);
  lines.forEach((l, i) => {
    if (l.op !== ' ') for (let k = Math.max(0, i - contextLines); k <= Math.min(lines.length - 1, i + contextLines); k++) keep[k] = true;
  });
  let html = '', skipped = 0;
  const flush = () => { if (skipped) { html += `<div class="opacity-40 py-0.5 select-none">··· ${skipped} unchanged line${skipped > 1 ? 's' : ''}</div>`; skipped = 0; } };
  lines.forEach((l, i) => {
    if (!keep[i]) { skipped++; return; }
    flush();
    const num = (l.op === '+' ? l.new : l.old) || '';
    const cls = l.op === '+' ? 'diff-add px-2 font-bold' : l.op === '-' ? 'diff-del px-2 font-bold' : 'opacity-60 px-2';
    const sign = l.op === ' ' ? '&nbsp;' : l.op;
    html += `<div class="${cls} py-0.5 whitespace-pre"><span class="inline-block w-10 opacity-50 select-none">${num}</span>${sign} ${escapeHtml(l.text)}</div>`;
  });
  flush();
  container.innerHTML = html;
}

async function api(url, opts = {}) {
  const res = await fetch(url, opts);
  let data = {};
  try { data = await res.json(); } catch (_) {}
  return { ok: res.ok, status: res.status, data };
}

function postJSON(url, body) {
  return api(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
}

/* ------------------------------------------------------------------ */
/* Snapshots tab                                                       */
/* ------------------------------------------------------------------ */

let snapshotFiles = {};

async function fetchSnapshots() {
  const sidebar = document.getElementById('snapshots-sidebar-list');
  const { data } = await api('/api/nginx/snapshots');
  const snaps = data.snapshots || [];
  document.getElementById('snap-count').innerText = `${snaps.length} snapshot${snaps.length === 1 ? '' : 's'}`;
  if (!snaps.length) {
    sidebar.innerHTML = '<div class="p-2 opacity-60 text-xs">No snapshots yet. Every save creates one automatically, or click Create Snapshot above.</div>';
    document.getElementById('diff-content-view').innerHTML = '';
    return;
  }
  snapshotFiles = {};
  sidebar.innerHTML = snaps.map(s => {
    snapshotFiles[s.id] = s.file_path;
    return `
    <button data-snap="${escapeHtml(s.id)}" onclick="selectSnapshot(this.dataset.snap)" class="snap-item w-full text-left p-2 rounded-lg transition hover:bg-white/5 flex items-center justify-between">
      <div class="truncate">
        <div class="font-bold truncate font-sans">${escapeHtml(s.note || s.id)}</div>
        <div class="text-[10px] opacity-60">${escapeHtml(s.timestamp.slice(0, 10))} ${escapeHtml(s.timestamp.slice(11, 19))} • ${escapeHtml(s.file_path.split('/').pop())}</div>
      </div>
      <i class="fa-solid fa-chevron-right text-[10px] opacity-60"></i>
    </button>`;
  }).join('');
  selectSnapshot(snaps[0].id);
}

async function selectSnapshot(id) {
  activeSnapshotID = id;
  document.querySelectorAll('.snap-item').forEach(b => {
    const on = b.dataset.snap === id;
    b.classList.toggle('bg-emerald-500/15', on);
    b.classList.toggle('text-emerald-400', on);
  });
  const view = document.getElementById('diff-content-view');
  view.innerHTML = '<div class="opacity-60">Loading diff…</div>';
  const { ok, data } = await api('/api/nginx/snapshots/diff?id=' + encodeURIComponent(id));
  if (!ok) { view.innerHTML = `<div class="text-rose-400">${escapeHtml(data.error || 'Failed to load diff')}</div>`; return; }
  document.getElementById('diff-file-header').innerText = `${data.snapshot.file_path}  (snapshot → current)`;
  // Rolling back REMOVES the "+" lines and RESTORES the "-" lines.
  document.getElementById('diff-del-count').innerText = `- ${data.diff.deletions} only in snapshot`;
  document.getElementById('diff-add-count').innerText = `+ ${data.diff.additions} only in current`;
  renderDiff(view, data.diff);
}

async function takeSnapshotNow() {
  const msg = document.getElementById('snapshot-commit-msg').value.trim() || 'Manual checkpoint';
  const { ok, data } = await postJSON('/api/nginx/snapshots', { path: currentConfigPath, message: msg });
  if (!ok) return alert('Snapshot failed: ' + (data.error || 'unknown error'));
  document.getElementById('snapshot-commit-msg').value = '';
  fetchSnapshots();
}

async function rollbackActiveSnapshot() {
  if (!activeSnapshotID) return;
  const file = snapshotFiles[activeSnapshotID] || 'the file';
  if (!confirm(`Restore ${file} from this snapshot and reload nginx?\n\nThe current version is snapshotted first, so you can undo this.`)) return;
  const { data } = await api('/api/nginx/snapshots/rollback?id=' + encodeURIComponent(activeSnapshotID), { method: 'POST' });
  alert((data.message || data.error || 'Done') + (data.status === 'ok' ? '' : '\n\n' + (data.output || data.reload_output || '')));
  fetchSnapshots();
  testSyntax();
}

/* ------------------------------------------------------------------ */
/* Editor: preview → apply & reload                                    */
/* ------------------------------------------------------------------ */

function saveConfigFile() { openPreview(); }

async function openPreview() {
  const content = Editor.get();
  const modal = document.getElementById('preview-modal');
  modal.classList.remove('hidden');
  document.getElementById('preview-file').innerText = currentConfigPath;
  document.getElementById('preview-diff').innerHTML = '<div class="opacity-60">Computing diff and running nginx -t (dry run)…</div>';
  document.getElementById('preview-test').innerText = '';
  document.getElementById('preview-lint').innerHTML = '';
  const applyBtn = document.getElementById('preview-apply');
  applyBtn.disabled = true;

  const { ok, data } = await postJSON('/api/nginx/file/preview', { path: currentConfigPath, content });
  if (!ok) { document.getElementById('preview-diff').innerHTML = `<div class="text-rose-400">${escapeHtml(data.error || 'Preview failed')}</div>`; return; }

  renderDiff(document.getElementById('preview-diff'), data.diff);
  const badge = document.getElementById('preview-badge');
  badge.innerText = data.valid ? 'nginx -t passed' : 'nginx -t FAILED';
  badge.className = 'np-pill text-xs ' + (data.valid ? 'bg-emerald-500/20 text-emerald-400' : 'bg-rose-500/20 text-rose-400');
  document.getElementById('preview-test').innerText = data.output || '';
  renderLint(document.getElementById('preview-lint'), data.lint);
  applyBtn.disabled = !data.valid || (data.diff.additions === 0 && data.diff.deletions === 0);
}

function closePreview() { document.getElementById('preview-modal').classList.add('hidden'); }

async function applyPreview() {
  const btn = document.getElementById('preview-apply');
  btn.disabled = true;
  btn.innerText = 'Applying…';
  const { data } = await postJSON('/api/nginx/file', {
    path: currentConfigPath,
    content: Editor.get(),
    message: document.getElementById('preview-message').value.trim(),
    reload: true,
    snapshot: typeof Settings === 'undefined' ? true : Settings.autoSnapshot,
  });
  btn.innerText = 'Apply & Reload';
  if (data.status === 'ok') {
    if (typeof markActiveSaved === 'function') markActiveSaved();
    closePreview();
    document.getElementById('preview-message').value = '';
    toast('Saved, validated and reloaded' + (data.snapshot ? ` (undo point: ${data.snapshot})` : ''), 'ok');
    testSyntax();
  } else {
    document.getElementById('preview-test').innerText = (data.message || data.error || 'Failed') + '\n\n' + (data.output || '') + (data.reload_output || '');
    toast(data.message || data.error || 'Apply failed', 'error');
  }
}

/* ------------------------------------------------------------------ */
/* Toasts (replacing alert() progressively)                            */
/* ------------------------------------------------------------------ */

function toast(msg, kind = 'ok') {
  const host = document.getElementById('toast-host');
  const el = document.createElement('div');
  el.setAttribute('role', 'status');
  el.className = 'np-pill text-xs shadow-lg px-4 py-2 ' + (kind === 'ok' ? 'bg-emerald-600 text-white' : 'bg-rose-600 text-white');
  el.textContent = msg;
  host.appendChild(el);
  setTimeout(() => el.remove(), 5000);
}

/* ------------------------------------------------------------------ */
/* Lint "Problems" panel                                               */
/* ------------------------------------------------------------------ */

const SEV_STYLE = { error: 'text-rose-400 fa-circle-xmark', warning: 'text-amber-400 fa-triangle-exclamation', info: 'text-sky-400 fa-circle-info' };

function renderLint(container, findings) {
  findings = findings || [];
  if (!findings.length) { container.innerHTML = '<div class="opacity-60"><i class="fa-solid fa-circle-check text-emerald-500"></i> No problems found</div>'; return; }
  container.innerHTML = findings.map(f => {
    const [color, icon] = (SEV_STYLE[f.severity] || SEV_STYLE.info).split(' ');
    return `<button class="w-full text-left flex items-start gap-2 py-1 hover:bg-white/5 rounded" onclick="gotoLine(${Number(f.line)})">
      <i class="fa-solid ${icon} ${color} mt-0.5" aria-label="${escapeHtml(f.severity)}"></i>
      <span class="flex-1"><span class="opacity-60 font-mono">L${Number(f.line)}</span> ${escapeHtml(f.message)}
      ${f.doc ? `<a href="${escapeHtml(f.doc)}" target="_blank" rel="noopener" class="underline opacity-70" onclick="event.stopPropagation()">docs</a>` : ''}</span>
    </button>`;
  }).join('');
}

let lintTimer = null;
function scheduleLint() {
  if (cm) { cm.relint(); return; }
  clearTimeout(lintTimer);
  lintTimer = setTimeout(() => fetchLint(Editor.get()), 600);
}

function gotoLine(n) { Editor.gotoLine(n); }

/* ------------------------------------------------------------------ */
/* Directive help for the line under the cursor                        */
/* ------------------------------------------------------------------ */

const NGINX_DOCS = {
  listen: ['listen address[:port] [ssl] [http2] [default_server];', 'Port/address this server accepts connections on. Add `ssl` for HTTPS.', 'http/ngx_http_core_module.html#listen'],
  server_name: ['server_name name ...;', 'Host names this server block answers for. Supports wildcards (*.example.com) and regex (~^...).', 'http/ngx_http_core_module.html#server_name'],
  location: ['location [=|~|~*|^~] uri { ... }', 'Matches request URIs. Exact (=) wins, then longest prefix (^~ stops regex search), then the first matching regex in file order.', 'http/ngx_http_core_module.html#location'],
  root: ['root path;', 'Files are served from path + full URI. Prefer setting it at server level.', 'http/ngx_http_core_module.html#root'],
  alias: ['alias path;', 'Replaces the matched location prefix with path. Mind trailing slashes: `location /img/ { alias /data/images/; }`.', 'http/ngx_http_core_module.html#alias'],
  index: ['index file ...;', 'Files to try when a directory is requested.', 'http/ngx_http_index_module.html#index'],
  try_files: ['try_files file ... uri|=code;', 'Checks files in order; the last argument is an internal redirect or error code. SPA pattern: `try_files $uri $uri/ /index.html;`', 'http/ngx_http_core_module.html#try_files'],
  return: ['return code [text|URL];', 'Stops processing and returns a status/redirect. Cheapest way to redirect: `return 301 https://$host$request_uri;`', 'http/ngx_http_rewrite_module.html#return'],
  rewrite: ['rewrite regex replacement [last|break|redirect|permanent];', 'Rewrites the URI with a regex. Prefer `return` for plain redirects.', 'http/ngx_http_rewrite_module.html#rewrite'],
  proxy_pass: ['proxy_pass URL;', 'Forwards requests to an upstream. If URL has a path (even "/"), the matched location prefix is REPLACED by it; without a path the original URI is passed unchanged.', 'http/ngx_http_proxy_module.html#proxy_pass'],
  proxy_set_header: ['proxy_set_header field value;', 'Sets a request header sent upstream. Defining ANY of these in a block drops all inherited ones from parent blocks.', 'http/ngx_http_proxy_module.html#proxy_set_header'],
  proxy_http_version: ['proxy_http_version 1.0|1.1;', 'Use 1.1 for keepalive upstreams and WebSockets.', 'http/ngx_http_proxy_module.html#proxy_http_version'],
  proxy_read_timeout: ['proxy_read_timeout time;', 'Max time between two reads from the upstream (default 60s). Raise for long-polling/SSE.', 'http/ngx_http_proxy_module.html#proxy_read_timeout'],
  proxy_connect_timeout: ['proxy_connect_timeout time;', 'Time to establish an upstream connection (default 60s; usually should be much lower, e.g. 5s).', 'http/ngx_http_proxy_module.html#proxy_connect_timeout'],
  proxy_buffering: ['proxy_buffering on|off;', 'Turn off for streaming responses (SSE) so data is sent immediately.', 'http/ngx_http_proxy_module.html#proxy_buffering'],
  proxy_cache: ['proxy_cache zone|off;', 'Enables caching with a zone defined by proxy_cache_path (http level).', 'http/ngx_http_proxy_module.html#proxy_cache'],
  upstream: ['upstream name { server addr [weight=n] [backup] [down]; }', 'A named pool of backends for proxy_pass http://name. Default balancing is round-robin.', 'http/ngx_http_upstream_module.html#upstream'],
  server: ['server { ... }  |  server address [params];', 'In http{}: a virtual host. In upstream{}: one backend server.', 'http/ngx_http_core_module.html#server'],
  add_header: ['add_header name value [always];', 'Adds a response header (only for 2xx/3xx unless `always`). Defining it in a block discards all add_header from parent blocks.', 'http/ngx_http_headers_module.html#add_header'],
  expires: ['expires time|off|epoch|max;', 'Sets Expires and Cache-Control max-age response headers.', 'http/ngx_http_headers_module.html#expires'],
  limit_req_zone: ['limit_req_zone key zone=name:size rate=rate;', 'http-level: defines a rate-limit zone, e.g. `$binary_remote_addr zone=api:10m rate=10r/s`.', 'http/ngx_http_limit_req_module.html#limit_req_zone'],
  limit_req: ['limit_req zone=name [burst=n] [nodelay];', 'Applies a rate limit. burst allows short spikes; nodelay serves the burst immediately.', 'http/ngx_http_limit_req_module.html#limit_req'],
  ssl_certificate: ['ssl_certificate file;', 'PEM certificate (with intermediates) for this server.', 'http/ngx_http_ssl_module.html#ssl_certificate'],
  ssl_certificate_key: ['ssl_certificate_key file;', 'Private key matching ssl_certificate.', 'http/ngx_http_ssl_module.html#ssl_certificate_key'],
  ssl_protocols: ['ssl_protocols TLSv1.2 TLSv1.3;', 'Enabled TLS versions. Anything older than TLSv1.2 is insecure.', 'http/ngx_http_ssl_module.html#ssl_protocols'],
  client_max_body_size: ['client_max_body_size size;', 'Max request body (default 1m). Uploads above it get 413.', 'http/ngx_http_core_module.html#client_max_body_size'],
  gzip: ['gzip on|off;', 'Compresses responses. Pair with gzip_types for non-HTML content.', 'http/ngx_http_gzip_module.html#gzip'],
  access_log: ['access_log path [format] | off;', 'Where requests are logged. NGINX++ analytics read this log.', 'http/ngx_http_log_module.html#access_log'],
  error_log: ['error_log file [level];', 'Error log destination and minimum level (debug, info, notice, warn, error…).', 'ngx_core_module.html#error_log'],
  include: ['include file|mask;', 'Inlines other files, e.g. `include /etc/nginx/conf.d/*.conf;`.', 'ngx_core_module.html#include'],
  if: ['if (condition) { ... }', 'Inside location only `return`/`rewrite` are fully safe ("if is evil"). Prefer map or separate locations.', 'http/ngx_http_rewrite_module.html#if'],
  map: ['map $source $result { ... }', 'http-level: derives a variable from another. The safe alternative to many `if`s.', 'http/ngx_http_map_module.html#map'],
  set: ['set $variable value;', 'Sets a variable.', 'http/ngx_http_rewrite_module.html#set'],
  auth_basic: ['auth_basic "realm" | off;', 'Enables HTTP basic auth; pair with auth_basic_user_file.', 'http/ngx_http_auth_basic_module.html#auth_basic'],
  worker_processes: ['worker_processes auto|n;', 'Number of worker processes. `auto` = one per CPU core.', 'ngx_core_module.html#worker_processes'],
  worker_connections: ['worker_connections n;', 'Max simultaneous connections per worker (clients + upstreams).', 'ngx_core_module.html#worker_connections'],
  keepalive_timeout: ['keepalive_timeout time;', 'How long an idle client keep-alive connection stays open.', 'http/ngx_http_core_module.html#keepalive_timeout'],
  server_tokens: ['server_tokens on|off;', 'Whether the nginx version appears in headers/error pages. Use off.', 'http/ngx_http_core_module.html#server_tokens'],
};

function updateDirectiveHelp() { // textarea fallback
  if (cm) return;
  const ta = textarea();
  const before = ta.value.slice(0, ta.selectionStart);
  const lineStart = before.lastIndexOf('\n') + 1;
  const lineEnd = ta.value.indexOf('\n', ta.selectionStart);
  const line = ta.value.slice(lineStart, lineEnd === -1 ? undefined : lineEnd).trim();
  showDirectiveHelp(before.split('\n').length, (line.match(/^([a-z_0-9]+)/) || [])[1]);
}

function showDirectiveHelp(lineNo, word) {
  const box = document.getElementById('directive-help');
  document.getElementById('cursor-pos').innerText = `Ln ${lineNo}`;
  const doc = word && NGINX_DOCS[word];
  if (!doc) { box.innerHTML = '<span class="opacity-50">Place the cursor on a directive to see its reference.</span>'; return; }
  box.innerHTML = `<code class="text-emerald-400">${escapeHtml(doc[0])}</code> <span class="opacity-80">${escapeHtml(doc[1])}</span>
    <a class="underline opacity-60" target="_blank" rel="noopener" href="https://nginx.org/en/docs/${doc[2]}">nginx.org</a>`;
}

/* ------------------------------------------------------------------ */
/* Recipe forms                                                        */
/* ------------------------------------------------------------------ */

const RECIPES = {
  ratelimit: {
    label: 'Rate limit',
    fields: [['location', 'Location', '/login'], ['zone', 'Zone name', 'login_limit'], ['rate', 'Requests / second', '5'], ['burst', 'Burst', '10']],
    build: f => ({
      http: `limit_req_zone $binary_remote_addr zone=${f.zone}:10m rate=${f.rate}r/s;`,
      server: `location ${f.location} {\n    limit_req zone=${f.zone} burst=${f.burst} nodelay;\n    limit_req_status 429;\n}`,
    }),
  },
  https: {
    label: 'HTTP → HTTPS redirect',
    fields: [['server_name', 'Server names', 'example.com www.example.com']],
    build: f => ({ http: `server {\n    listen 80;\n    server_name ${f.server_name};\n    return 301 https://$host$request_uri;\n}`, server: '' }),
  },
  cors: {
    label: 'CORS',
    fields: [['location', 'Location', '/api/'], ['origin', 'Allowed origin', 'https://app.example.com'], ['methods', 'Methods', 'GET, POST, PUT, DELETE, OPTIONS']],
    build: f => ({ http: '', server: `location ${f.location} {\n    if ($request_method = OPTIONS) {\n        add_header Access-Control-Allow-Origin "${f.origin}" always;\n        add_header Access-Control-Allow-Methods "${f.methods}" always;\n        add_header Access-Control-Allow-Headers "Authorization, Content-Type" always;\n        add_header Access-Control-Max-Age 86400;\n        return 204;\n    }\n    add_header Access-Control-Allow-Origin "${f.origin}" always;\n    add_header Vary Origin always;\n}` }),
  },
  cache: {
    label: 'Static asset caching',
    fields: [['ext', 'Extensions (|-separated)', 'css|js|png|jpg|svg|woff2'], ['days', 'Cache days', '30']],
    build: f => ({ http: '', server: `location ~* \\.(${f.ext})$ {\n    expires ${f.days}d;\n    add_header Cache-Control "public, immutable";\n    access_log off;\n}` }),
  },
  websocket: {
    label: 'WebSocket proxy',
    fields: [['location', 'Location', '/ws/'], ['upstream', 'Upstream URL', 'http://127.0.0.1:3000']],
    build: f => ({ http: '', server: `location ${f.location} {\n    proxy_pass ${f.upstream};\n    proxy_http_version 1.1;\n    proxy_set_header Upgrade $http_upgrade;\n    proxy_set_header Connection "upgrade";\n    proxy_set_header Host $host;\n    proxy_read_timeout 3600s;\n}` }),
  },
  reverseproxy: {
    label: 'Reverse proxy',
    fields: [['location', 'Location', '/api/'], ['upstream', 'Upstream URL (keep trailing / to strip prefix)', 'http://127.0.0.1:8080/']],
    build: f => ({ http: '', server: `location ${f.location} {\n    proxy_pass ${f.upstream};\n    proxy_set_header Host $host;\n    proxy_set_header X-Real-IP $remote_addr;\n    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n    proxy_set_header X-Forwarded-Proto $scheme;\n}` }),
  },
  basicauth: {
    label: 'Basic auth',
    fields: [['location', 'Location', '/admin/'], ['realm', 'Realm', 'Restricted'], ['file', 'htpasswd file', '/etc/nginx/.htpasswd']],
    build: f => ({ http: '', server: `location ${f.location} {\n    auth_basic "${f.realm}";\n    auth_basic_user_file ${f.file};\n}` }),
  },
};

let activeRecipe = 'ratelimit';

function initRecipes() {
  const sel = document.getElementById('recipe-select');
  if (!sel || sel.options.length) return;
  sel.innerHTML = Object.entries(RECIPES).map(([k, r]) => `<option value="${k}">${escapeHtml(r.label)}</option>`).join('');
  selectRecipe(activeRecipe);
}

function selectRecipe(key) {
  activeRecipe = key;
  document.getElementById('recipe-select').value = key;
  document.getElementById('recipe-fields').innerHTML = RECIPES[key].fields.map(([name, label, def]) => `
    <label class="block text-[11px] opacity-80">${escapeHtml(label)}
      <input data-field="${name}" value="${escapeHtml(def)}" oninput="buildRecipe()" class="mt-1 w-full theme-surface-elevated px-3 py-2 rounded-lg text-xs font-mono border outline-none focus:ring-1 focus:ring-emerald-500">
    </label>`).join('');
  buildRecipe();
}

function currentRecipe() {
  const f = {};
  document.querySelectorAll('#recipe-fields [data-field]').forEach(i => { f[i.dataset.field] = i.value.trim(); });
  return RECIPES[activeRecipe].build(f);
}

function recipeText(r) {
  return [r.http && `# --- http level (e.g. a conf.d file, outside server {}) ---\n${r.http}`,
          r.server && `# --- inside your server { } block ---\n${r.server}`].filter(Boolean).join('\n\n');
}

function buildRecipe() {
  document.getElementById('recipe-output').textContent = recipeText(currentRecipe());
  const b = document.getElementById('recipe-badge');
  b.innerText = 'not validated';
  b.className = 'np-pill text-xs bg-white/10';
}

async function validateRecipe() {
  const r = currentRecipe();
  const b = document.getElementById('recipe-badge');
  b.innerText = 'validating…';
  const { data } = await postJSON('/api/nginx/snippets/validate', { http: r.http, server: r.server });
  b.innerText = data.syntax_valid ? 'nginx -t passed' : 'nginx -t failed';
  b.className = 'np-pill text-xs ' + (data.syntax_valid ? 'bg-emerald-500/20 text-emerald-400' : 'bg-rose-500/20 text-rose-400');
  document.getElementById('recipe-test').textContent = data.syntax_test || '';
}

function copyRecipe() {
  navigator.clipboard.writeText(recipeText(currentRecipe())).then(() => toast('Copied to clipboard'));
}

function insertRecipeIntoEditor() {
  const text = recipeText(currentRecipe());
  switchTab('editor');
  Editor.insert('\n' + text + '\n');
  scheduleLint();
  toast('Inserted at cursor. Review, then Save → Preview to apply');
}

/* ------------------------------------------------------------------ */
/* Wiring                                                              */
/* ------------------------------------------------------------------ */

document.addEventListener('DOMContentLoaded', () => {
  initEditor();
  const ta = textarea();
  if (ta && !cm) {
    ta.addEventListener('input', () => { scheduleLint(); updateDirectiveHelp(); if (typeof onEditorChanged === 'function') onEditorChanged(); });
    ['click', 'keyup'].forEach(ev => ta.addEventListener(ev, updateDirectiveHelp));
  }
  document.addEventListener('keydown', e => {
    if (!cm && (e.metaKey || e.ctrlKey) && e.key === 's' && !document.getElementById('tab-editor').classList.contains('hidden')) {
      e.preventDefault();
      openPreview();
    }
    if (e.key === 'Escape') closePreview();
  });
  initRecipes();
});
