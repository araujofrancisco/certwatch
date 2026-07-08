function getUserEmail() {
  try {
    var token = localStorage.getItem('token');
    if (!token) return '';
    var payload = JSON.parse(atob(token.split('.')[1]));
    return payload.email || '';
  } catch (e) {
    return '';
  }
}

function updateUserDisplay() {
  var el = document.getElementById('userEmail');
  if (el) el.textContent = getUserEmail();
}
updateUserDisplay();

function initTheme() {
  var theme = localStorage.getItem('theme') || 'light';
  document.documentElement.setAttribute('data-bs-theme', theme);
  updateThemeIcon(theme);
}
function toggleTheme() {
  var current = document.documentElement.getAttribute('data-bs-theme');
  var next = current === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-bs-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcon(next);
}
function updateThemeIcon(theme) {
  var el = document.getElementById('themeToggle');
  if (el) el.textContent = theme === 'dark' ? '☀️' : '🌙';
}
initTheme();
initUserDropdown();
initGlobalSearch();
updateNotificationBadge();
setInterval(updateNotificationBadge, 60000);

function initGlobalSearch() {
  var el = document.getElementById('globalSearch');
  if (!el) return;
  el.addEventListener('keydown', function(e) {
    if (e.key === 'Enter') {
      var q = el.value.trim();
      if (q) {
        window.location.href = '/domains?q=' + encodeURIComponent(q);
      }
    }
  });
}

function updateNotificationBadge() {
  fetch('/api/dashboard', {
    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
  }).then(function(r) { return r.json(); }).then(function(data) {
    if (!data || typeof data.warning === 'undefined') return;
    var badge = document.getElementById('notifBadge');
    if (!badge) return;
    if (data.warning > 0) {
      badge.textContent = data.warning;
      badge.classList.remove('d-none');
    } else {
      badge.classList.add('d-none');
    }
  }).catch(function() {});
}

function initUserDropdown() {
  var email = getUserEmail();
  var avatar = document.getElementById('userAvatar');
  var ddEmail = document.getElementById('userEmailDD');
  if (avatar && email) {
    avatar.textContent = email.charAt(0).toUpperCase();
  }
  if (ddEmail && email) {
    ddEmail.textContent = email;
  }
}

function closeOffcanvas() {
  var el = document.getElementById('offcanvasSidebar');
  if (el && el.classList.contains('show')) {
    var offcanvas = bootstrap.Offcanvas.getInstance(el);
    if (offcanvas) offcanvas.hide();
  }
}

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
function makeSortable(tableId) {
  var table = document.getElementById(tableId);
  if (!table) return;
  var headers = table.querySelectorAll('thead th');
  var tbody = table.querySelector('tbody');
  if (!tbody) return;
  headers.forEach(function(th, col) {
    th.style.cursor = 'pointer';
    th.addEventListener('click', function() {
      var direction = th.getAttribute('data-sort') === 'asc' ? 'desc' : 'asc';
      headers.forEach(function(h) { h.removeAttribute('data-sort'); });
      th.setAttribute('data-sort', direction);
      var rows = Array.from(tbody.querySelectorAll('tr'));
      if (rows.length === 0 || rows[0].classList.contains('skeleton-row')) return;
      var multiplier = direction === 'asc' ? 1 : -1;
      rows.sort(function(a, b) {
        var aText = (a.children[col] ? a.children[col].textContent.trim() : '');
        var bText = (b.children[col] ? b.children[col].textContent.trim() : '');
        var aNum = parseFloat(aText), bNum = parseFloat(bText);
        if (!isNaN(aNum) && !isNaN(bNum)) return (aNum - bNum) * multiplier;
        if (aText === '-' || aText === '') return 1 * multiplier;
        if (bText === '-' || bText === '') return -1 * multiplier;
        return aText.localeCompare(bText) * multiplier;
      });
      rows.forEach(function(r) { tbody.appendChild(r); });
    });
  });
}

function showToast(message, type) {
  type = type || 'info';
  const colors = { info: 'bg-info text-dark', success: 'bg-success text-white', warning: 'bg-warning text-dark', error: 'bg-danger text-white' };
  const cls = colors[type] || colors.info;
  const el = document.createElement('div');
  el.className = 'toast align-items-center border-0 ' + cls;
  el.role = 'alert';
  el.innerHTML = '<div class="d-flex"><div class="toast-body">' + escapeHtml(message) + '</div><button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button></div>';
  document.getElementById('toastContainer').appendChild(el);
  const toast = bootstrap.Toast.getOrCreateInstance(el, { autohide: true, delay: 4000 });
  toast.show();
  el.addEventListener('hidden.bs.toast', function () { el.remove(); });
}

function showConfirm(title, message) {
  return new Promise(function (resolve) {
    document.getElementById('confirmTitle').textContent = title;
    document.getElementById('confirmBody').textContent = message;
    var btn = document.getElementById('confirmBtn');
    var modal = new bootstrap.Modal(document.getElementById('confirmModal'));
    var resolved = false;
    function cleanup() { if (resolved) return; resolved = true; modal.hide(); btn.removeEventListener('click', onConfirm); }
    function onConfirm() { cleanup(); resolve(true); }
    btn.addEventListener('click', onConfirm);
    document.getElementById('confirmModal').addEventListener('hidden.bs.modal', function () { if (!resolved) { resolved = true; resolve(false); } }, { once: true });
    modal.show();
  });
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
