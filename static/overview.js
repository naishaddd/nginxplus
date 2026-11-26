// Overview KPIs: live values from the metrics, health and SSL endpoints.
// Shows "—" instead of a number whenever data is missing; never a placeholder value.
async function loadOverview() {
  const set = (id, html) => { const el = document.getElementById(id); if (el) el.innerHTML = html; };
  const get = url => fetch(url).then(r => (r.ok ? r.json() : null)).catch(() => null);
  const [m, h, ssl] = await Promise.all([get('/api/nginx/metrics?window=1m'), get('/api/nginx/upstreams/health'), get('/api/nginx/ssl/audit')]);

  if (m && m.traffic) {
    const t = m.traffic;
    set('ov-qps', t.qps.toFixed(2));
    set('ov-total', `${t.total_requests} requests in the last minute`);
    const errs = t.status_4xx + t.status_5xx;
    const rate = t.total_requests ? (100 * errs / t.total_requests) : 0;
    const cls = t.status_5xx > 0 ? 'np-badge--error' : rate > 5 ? 'np-badge--warn' : 'np-badge--ok';
    set('ov-errors', t.total_requests ? `${rate.toFixed(1)}<span class="text-base np-muted">%</span>` : '—');
    set('ov-errors-meta', `<span class="np-badge ${cls}">${t.status_4xx} × 4xx · ${t.status_5xx} × 5xx</span>`);
  }

  if (h && h.upstreams) {
    let healthy = 0, total = 0;
    h.upstreams.forEach(u => (u.nodes || []).forEach(n => { if (n.status !== 'drained') { total++; if (n.status === 'healthy') healthy++; } }));
    if (total) {
      const cls = healthy === total ? 'np-badge--ok' : healthy === 0 ? 'np-badge--error' : 'np-badge--warn';
      set('stat-healthy-upstreams', `${healthy}<span class="text-base np-muted"> / ${total}</span>`);
      set('ov-upstream-meta', `<span class="np-badge ${cls}"><span class="np-dot"></span>${healthy === total ? 'all healthy' : `${total - healthy} down`}</span>`);
    } else {
      set('stat-healthy-upstreams', '—');
      set('ov-upstream-meta', 'no upstream blocks configured');
    }
  }

  if (ssl && ssl.audit && ssl.audit.grade) {
    const g = ssl.audit.grade;
    const cls = g.startsWith('A') ? 'np-badge--ok' : g === 'B' ? 'np-badge--warn' : 'np-badge--error';
    set('stat-sec-grade', `<span class="np-badge ${cls}" style="font-size:18px;padding:4px 14px">${escapeHtml(g)}</span>`);
    const certs = (ssl.certificates || []).length;
    set('ov-sec-meta', `${certs} certificate${certs === 1 ? '' : 's'} scanned`);
  }
}
