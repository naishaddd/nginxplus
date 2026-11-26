// NGINX++ terminal workspace: one shell, or split into two panes.
// The second pane can be another shell or a live log tail (access / error).
// Each shell is its own WebSocket session (capped server-side by NGINXPP_MAX_TERMINALS).

class ShellSession {
  constructor(el, label) {
    this.el = el;
    this.label = label;
    this.connect();
    // Refit whenever the pane changes size (window resize, split drag, sidebar collapse).
    this.ro = new ResizeObserver(() => this.fit());
    this.ro.observe(el);
  }

  connect() {
    this.el.innerHTML = '';
    this.term = new Terminal({
      cursorBlink: true, fontSize: 13, fontFamily: 'JetBrains Mono, monospace',
      theme: { background: '#0A1013', foreground: '#E8EBE6', cursor: '#34D399', selectionBackground: 'rgba(52,211,153,.25)' },
    });
    this.fitAddon = new FitAddon.FitAddon();
    this.term.loadAddon(this.fitAddon);
    this.term.open(this.el);
    this.fit();
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.ws = new WebSocket(`${proto}//${location.host}/ws/terminal`);
    this.ws.onopen = () => {
      this.term.write(`\x1b[32m[${this.label}: connected to /bin/sh in the nginx container]\x1b[0m\r\n\r\n`);
      this.sendSize();
    };
    this.ws.onmessage = e => this.term.write(e.data);
    this.ws.onclose = () => this.term.write('\r\n\x1b[33m[session closed. Click "Reconnect" to start a new one]\x1b[0m\r\n');
    this.term.onData(d => this.ws.readyState === WebSocket.OPEN && this.ws.send(d));
  }

  fit() {
    if (!this.el.offsetParent) return; // hidden tab
    try { this.fitAddon.fit(); this.sendSize(); } catch (_) {}
  }

  sendSize() {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify({ resize: { cols: this.term.cols, rows: this.term.rows } }));
  }

  reconnect() { this.dispose(false); this.connect(); }

  dispose(full = true) {
    try { this.ws && this.ws.close(); } catch (_) {}
    try { this.term && this.term.dispose(); } catch (_) {}
    if (full && this.ro) this.ro.disconnect();
  }
}

const termUI = { main: null, second: null, split: false, secondKind: 'logs-access', logTimer: null };

function initTerminalWorkspace() {
  if (!termUI.main) termUI.main = new ShellSession(document.getElementById('term-pane-1'), 'Shell 1');
  setTimeout(() => termUI.main.fit(), 50);
  if (termUI.split) startSecondPane();
}

function setTerminalLayout(split) {
  termUI.split = split;
  document.getElementById('term-workspace').classList.toggle('is-split', split);
  document.querySelectorAll('[data-term-layout]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.termLayout === (split ? 'split' : 'single'))));
  document.getElementById('term-second-kind').disabled = !split;
  if (split) startSecondPane(); else stopSecondPane();
  setTimeout(() => termUI.main && termUI.main.fit(), 60);
}

function setSecondPaneKind(kind) {
  termUI.secondKind = kind;
  if (termUI.split) { stopSecondPane(); startSecondPane(); }
}

function startSecondPane() {
  const host = document.getElementById('term-pane-2');
  const title = document.getElementById('term-pane-2-title');
  if (termUI.secondKind === 'shell') {
    title.textContent = 'Shell 2';
    host.className = 'term-pane flex-1 min-h-0';
    if (!termUI.second) termUI.second = new ShellSession(host, 'Shell 2');
    setTimeout(() => termUI.second.fit(), 60);
  } else {
    const type = termUI.secondKind === 'logs-error' ? 'error' : 'access';
    title.textContent = `Live ${type} log`;
    host.className = 'term-pane flex-1 min-h-0 overflow-y-auto p-3 font-mono text-[12px] leading-relaxed';
    host.innerHTML = '<div class="opacity-60">Loading…</div>';
    const load = async () => {
      if (document.hidden) return;
      try {
        const res = await fetch(`/api/nginx/logs?type=${type}&lines=200`);
        const data = await res.json();
        const atBottom = host.scrollTop + host.clientHeight >= host.scrollHeight - 30;
        host.innerHTML = (data.lines || []).map(l => {
          const m = l.match(/" (\d{3}) /);
          const code = m ? +m[1] : 0;
          const color = code >= 500 ? 'var(--np-error)' : code >= 400 ? 'var(--np-warn)' : '';
          return `<div style="${color ? 'color:' + color : ''}">${escapeHtml(l)}</div>`;
        }).join('') || '<div class="opacity-60">No log lines yet. Send some traffic, e.g. <code>curl -s localhost/</code> in the shell.</div>';
        if (atBottom) host.scrollTop = host.scrollHeight;
      } catch (err) { host.innerHTML = `<div style="color:var(--np-error)">${escapeHtml(err)}</div>`; }
    };
    load();
    termUI.logTimer = setInterval(load, 2000);
  }
}

function stopSecondPane() {
  clearInterval(termUI.logTimer);
  termUI.logTimer = null;
  if (termUI.second) { termUI.second.dispose(); termUI.second = null; }
  const host = document.getElementById('term-pane-2');
  if (host) host.innerHTML = '';
}

function reconnectTerminals() {
  if (termUI.main) termUI.main.reconnect();
  if (termUI.second) termUI.second.reconnect();
}

// Stop log polling when leaving the tab; shells stay connected.
function pauseTerminalWorkspace() { clearInterval(termUI.logTimer); termUI.logTimer = null; }

// Drag the divider to resize panes.
document.addEventListener('DOMContentLoaded', () => {
  const div = document.getElementById('term-divider');
  if (!div) return;
  div.addEventListener('pointerdown', e => {
    const ws = document.getElementById('term-workspace');
    const p1 = document.getElementById('term-col-1');
    div.setPointerCapture(e.pointerId);
    const move = ev => {
      const r = ws.getBoundingClientRect();
      const pct = Math.min(80, Math.max(20, ((ev.clientX - r.left) / r.width) * 100));
      p1.style.flexBasis = pct + '%';
    };
    const up = () => { div.removeEventListener('pointermove', move); div.removeEventListener('pointerup', up); };
    div.addEventListener('pointermove', move);
    div.addEventListener('pointerup', up);
  });
  div.addEventListener('keydown', e => {
    const p1 = document.getElementById('term-col-1');
    const cur = parseFloat(p1.style.flexBasis) || 50;
    if (e.key === 'ArrowLeft') p1.style.flexBasis = Math.max(20, cur - 5) + '%';
    if (e.key === 'ArrowRight') p1.style.flexBasis = Math.min(80, cur + 5) + '%';
  });
});
