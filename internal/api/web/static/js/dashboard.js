function requireAuth() {
  if (!localStorage.getItem('token')) {
    window.location.href = '/login';
  }
}
function logout() {
  localStorage.removeItem('token');
  window.location.href = '/login';
}
function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
function daysUntil(dateStr) {
  if (!dateStr) return 999;
  const now = new Date();
  const then = new Date(dateStr);
  return Math.ceil((then - now) / (1000 * 60 * 60 * 24));
}
function statusClass(status) {
  if (!status) return '';
  const s = status.toLowerCase();
  if (s === 'valid' || s === 'healthy') return 'status-valid';
  if (s === 'warning' || s === 'expiring') return 'status-warning';
  return 'status-error';
}
async function apiFetch(url, opts) {
  opts = opts || {};
  const headers = opts.headers || {};
  headers['Authorization'] = 'Bearer ' + localStorage.getItem('token');
  if (opts.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  try {
    const res = await fetch(url, {...opts, headers});
    if (res.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
      return null;
    }
    const text = await res.text();
    if (!text) return res.ok ? {} : null;
    try { return JSON.parse(text); }
    catch { return text; }
  } catch (err) {
    console.error('API fetch error:', err);
    return null;
  }
}
