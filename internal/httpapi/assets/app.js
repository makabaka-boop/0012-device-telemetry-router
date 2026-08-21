// Frontend application: real calls to the Go API for all four views.
const api = {
  async get(path) {
    const r = await fetch(path);
    return r.json();
  },
  async post(path, body) {
    const r = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await r.json().catch(() => ({}));
    if (!r.ok) { return { _error: data, _status: r.status }; }
    return data;
  },
  async put(path, body) {
    const r = await fetch(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await r.json().catch(() => ({}));
    if (!r.ok) { return { _error: data, _status: r.status }; }
    return data;
  },
  async del(path) {
    const r = await fetch(path, { method: 'DELETE' });
    return r.json();
  },
};

function fmtError(d) {
  if (d && d.error) {
    const e = d.error;
    return `[${e.code}] ${e.message}${e.details ? ' ' + JSON.stringify(e.details) : ''}`;
  }
  return JSON.stringify(d);
}

function switchView(name) {
  document.querySelectorAll('.view').forEach(v => v.hidden = true);
  const el = document.getElementById('view-' + name);
  if (el) el.hidden = false;
  document.querySelectorAll('nav a').forEach(a => a.classList.toggle('active', a.dataset.view === name));
}

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
}

async function loadHome() {
  try {
    const h = await api.get('/healthz');
    document.getElementById('stat-health').textContent = h.status || '-';
  } catch (e) { document.getElementById('stat-health').textContent = 'down'; }
  try {
    const s = await api.get('/api/v1/stats');
    document.getElementById('stat-devices').textContent = s.devices ?? '-';
    document.getElementById('stat-events').textContent = s.events ?? '-';
    document.getElementById('stat-pending').textContent = s.pending_deliveries ?? '-';
  } catch (e) {}
  try {
    const ev = await api.get('/api/v1/events?page=1&size=10');
    const tbody = document.querySelector('#recent-events tbody');
    tbody.innerHTML = '';
    (ev.items || []).forEach(e => {
      const tr = document.createElement('tr');
      tr.innerHTML = `<td>${esc(e.event_id)}</td><td>${esc(e.device_id)}</td><td>${esc(e.metric)}</td><td>${e.value}</td><td>${esc(e.status)}</td><td>${esc(e.ts)}</td>`;
      tbody.appendChild(tr);
    });
  } catch (e) {}
}

async function loadDevices() {
  const res = await api.get('/api/v1/devices');
  const tbody = document.querySelector('#device-table tbody');
  tbody.innerHTML = '';
  (res.items || []).forEach(d => {
    const tr = document.createElement('tr');
    const cls = { active: 'active', suspended: 'suspended', deleted: 'deleted' }[d.status] || '';
    tr.innerHTML = `<td>${esc(d.device_id)}</td><td>${esc(d.name)}</td>` +
      `<td><span class="badge ${cls}">${esc(d.status)}</span></td>` +
      `<td>${esc(d.protocol_version)}</td>` +
      `<td>${d.status === 'active' ? `<button class="danger" data-suspend="${esc(d.device_id)}">停用</button>` : ''}</td>`;
    tbody.appendChild(tr);
  });
  tbody.querySelectorAll('[data-suspend]').forEach(btn => {
    btn.addEventListener('click', async () => {
      await api.post(`/api/v1/devices/${btn.dataset.suspend}/status`, { status: 'suspended' });
      loadDevices();
    });
  });
}

async function submitTelemetry() {
  const raw = document.getElementById('telemetry-raw').value;
  const box = document.getElementById('telemetry-result');
  const res = await api.post('/api/v1/telemetry', { raw_text: raw });
  if (res._error) {
    box.textContent = '错误：' + fmtError(res._error);
    return;
  }
  box.textContent = '上报成功\n' + JSON.stringify(res, null, 2);
}

async function loadRules() {
  const res = await api.get('/api/v1/rules');
  const tbody = document.querySelector('#rule-table tbody');
  tbody.innerHTML = '';
  (res.items || []).forEach(r => {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${esc(r.rule_id)}</td><td>${esc(r.name)}</td><td>${esc(r.topic)}</td>` +
      `<td>${r.priority}</td><td>${r.enabled}</td>` +
      `<td><button data-changes="${esc(r.rule_id)}">留痕</button> ` +
      `<button class="danger" data-del="${esc(r.rule_id)}">删除</button></td>`;
    tbody.appendChild(tr);
  });
  tbody.querySelectorAll('[data-changes]').forEach(btn => {
    btn.addEventListener('click', async () => {
      const c = await api.get(`/api/v1/rules/${btn.dataset.changes}/changes`);
      document.getElementById('rule-changes').textContent = JSON.stringify(c, null, 2);
    });
  });
  tbody.querySelectorAll('[data-del]').forEach(btn => {
    btn.addEventListener('click', async () => {
      await api.del(`/api/v1/rules/${btn.dataset.del}`);
      loadRules();
    });
  });
}

async function submitRule() {
  const metrics = document.getElementById('rule-metric').value;
  const matcher = {
    device_pattern: document.getElementById('rule-device-pattern').value,
    metrics: metrics ? metrics.split(',').map(s => s.trim()).filter(Boolean) : [],
  };
  const body = {
    rule_id: document.getElementById('rule-id').value || undefined,
    name: document.getElementById('rule-name').value,
    matcher,
    topic: document.getElementById('rule-topic').value,
    priority: parseInt(document.getElementById('rule-priority').value, 10) || 0,
    enabled: document.getElementById('rule-enabled').checked,
  };
  const res = await api.post('/api/v1/rules', body);
  if (res._error) { document.getElementById('rule-changes').textContent = '错误：' + fmtError(res._error); return; }
  loadRules();
}

async function loadEvents() {
  const p = new URLSearchParams();
  const g = (id) => document.getElementById(id).value;
  if (g('f-device')) p.set('device_id', g('f-device'));
  if (g('f-metric')) p.set('metric', g('f-metric'));
  if (g('f-status')) p.set('status', g('f-status'));
  if (g('f-from')) p.set('from', g('f-from'));
  if (g('f-to')) p.set('to', g('f-to'));
  p.set('page', '1'); p.set('size', '50');
  const res = await api.get('/api/v1/events?' + p.toString());
  const tbody = document.querySelector('#event-table tbody');
  tbody.innerHTML = '';
  (res.items || []).forEach(e => {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${esc(e.event_id)}</td><td>${esc(e.device_id)}</td><td>${esc(e.metric)}</td>` +
      `<td>${e.value}</td><td>${esc(e.unit)}</td><td>${esc(e.status)}</td><td>${esc(e.ts)}</td>`;
    tbody.appendChild(tr);
  });
}

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('nav a').forEach(a => {
    a.addEventListener('click', (e) => {
      e.preventDefault();
      history.pushState({}, '', a.getAttribute('href'));
      const view = a.dataset.view;
      switchView(view);
      if (view === 'home') loadHome();
      if (view === 'devices') loadDevices();
      if (view === 'rules') loadRules();
      if (view === 'events') loadEvents();
    });
  });
  document.getElementById('device-form').addEventListener('submit', (e) => {
    e.preventDefault();
    api.post('/api/v1/devices', {
      device_id: document.getElementById('device-id').value || undefined,
      name: document.getElementById('device-name').value,
      protocol_version: document.getElementById('device-version').value || 'v1',
      metadata: {},
    }).then(() => loadDevices());
  });
  document.getElementById('telemetry-submit').addEventListener('click', submitTelemetry);
  document.getElementById('rule-form').addEventListener('submit', (e) => { e.preventDefault(); submitRule(); });
  document.getElementById('event-filter').addEventListener('submit', (e) => { e.preventDefault(); loadEvents(); });

  const path = location.pathname;
  const view = path === '/' ? 'home' : path.slice(1);
  switchView(view);
  if (view === 'home') loadHome();
  if (view === 'devices') loadDevices();
  if (view === 'rules') loadRules();
  if (view === 'events') loadEvents();
});
