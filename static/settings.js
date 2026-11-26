// NGINX++ UI settings (stored per browser in localStorage) and what each one controls.
//   snippets      → show/hide the Snippets tab
//   autoSnapshot  → whether "Apply & Reload" stores an undo snapshot first
//                   (a failed reload is still auto-restored either way)
//   probeInterval → how often the Upstreams tab re-probes while it is open

const SETTINGS_KEY = 'nginxpp-settings';
const SETTINGS_DEFAULTS = { snippets: true, autoSnapshot: true, probeInterval: '5s' };
const Settings = Object.assign({}, SETTINGS_DEFAULTS, (() => {
  try { return JSON.parse(localStorage.getItem(SETTINGS_KEY)) || {}; } catch (_) { return {}; }
})());

const SETTING_ALIASES = { ai: 'snippets', snapshots: 'autoSnapshot', interval: 'probeInterval' };

function saveSetting(key, val) {
  key = SETTING_ALIASES[key] || key;
  Settings[key] = val;
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(Settings));
  applySettings();
  if (typeof toast === 'function') toast('Setting saved');
}

function applySettings() {
  document.querySelectorAll(`[onclick="switchTab('copilot')"]`).forEach(el => el.classList.toggle('hidden', !Settings.snippets));
  if (!Settings.snippets && !document.getElementById('tab-copilot').classList.contains('hidden')) switchTab('overview');
  if (probeTimer) startProbeTimer(); // restart with the new interval
  const active = document.querySelector('#sidebar .nav-item.active');
  if (active && typeof renderTopPills === 'function') renderTopPills(active.dataset.group);
}

let probeTimer = null;
function startProbeTimer() {
  stopProbeTimer();
  const ms = parseInt(Settings.probeInterval, 10) * 1000 || 5000;
  probeTimer = setInterval(() => { if (!document.hidden) fetchUpstreamHealth(); }, ms);
}
function stopProbeTimer() { if (probeTimer) { clearInterval(probeTimer); probeTimer = null; } }

document.addEventListener('DOMContentLoaded', () => {
  const set = (id, prop, v) => { const el = document.getElementById(id); if (el) el[prop] = v; };
  set('setting-ai-enabled', 'checked', Settings.snippets);
  set('setting-snapshots-enabled', 'checked', Settings.autoSnapshot);
  set('setting-polling-interval', 'value', Settings.probeInterval);
  applySettings();
});
