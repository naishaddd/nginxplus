// Login + first-run setup pages. Talks to /api/auth/*.
(function () {
  const $ = id => document.getElementById(id);
  const form = $('auth-form');
  const mode = form.dataset.mode; // "login" | "setup"
  const errorBox = $('auth-error');

  function showError(msg) {
    errorBox.textContent = msg;
    errorBox.hidden = !msg;
  }

  fetch('/api/auth/state').then(r => r.json()).then(s => {
    if (s.authenticated) location.replace('/');
    else if (mode === 'login' && !s.setup) location.replace('/setup.html');
    else if (mode === 'setup' && s.setup) location.replace('/login.html');
  }).catch(() => {});

  const pw = $('password'), pw2 = $('password2'), meter = $('pw-meter');
  if (pw && meter) pw.addEventListener('input', () => {
    const v = pw.value, n = v.length;
    const score = (n >= 10) + (n >= 14) + (/[A-Z]/.test(v) && /[a-z]/.test(v)) + /\d/.test(v) + /[^A-Za-z0-9]/.test(v);
    meter.value = n === 0 ? 0 : Math.max(1, score);
    $('pw-hint').textContent = n < 10 ? `${10 - n} more character${10 - n === 1 ? '' : 's'} needed` : ['Weak', 'Weak', 'Fair', 'Good', 'Strong', 'Strong'][score];
  });

  form.addEventListener('submit', async e => {
    e.preventDefault();
    showError('');
    if (mode === 'setup' && pw.value !== pw2.value) {
      pw2.setAttribute('aria-invalid', 'true');
      return showError('Passwords do not match.');
    }
    const btn = form.querySelector('button[type=submit]');
    btn.disabled = true;
    const body = { username: $('username').value, password: pw.value };
    if (mode === 'setup') body.setupCode = $('setup-code').value;
    try {
      const res = await fetch(mode === 'setup' ? '/api/auth/setup' : '/api/auth/login', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
      const next = new URLSearchParams(location.search).get('next');
      location.replace(next && next.startsWith('/') && !next.startsWith('//') ? next : '/');
    } catch (err) {
      showError(err.message);
      btn.disabled = false;
    }
  });
})();
