const $ = selector => document.querySelector(selector);
const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, character => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character]));
const state = {principal:null,status:[],operations:[],events:[],clients:[]};

async function api(path, options = {}) {
  const response = await fetch(path, {...options, headers:{Accept:'application/json', ...(options.headers || {})}});
  if (response.status === 401) throw new Error('AUTH_REQUIRED');
  if (!response.ok) throw new Error(`${response.status} ${String(await response.text()).trim()}`);
  if (response.status === 204) return null;
  return response.json();
}

function time(value) {
  if (!value) return 'Never';
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? String(value) : parsed.toLocaleString();
}

function toast(message, error = false) {
  const element = $('#toast');
  element.textContent = message;
  element.className = error ? 'visible error' : 'visible';
  window.clearTimeout(toast.timer);
  toast.timer = window.setTimeout(() => { element.className = ''; }, 5000);
}

function table(columns, rows) {
  if (!rows?.length) return '<div class="empty">No records.</div>';
  return `<div class="table-wrap"><table class="table"><thead><tr>${columns.map(column => `<th>${escapeHTML(column[0])}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${columns.map(column => `<td>${column[1](row)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
}

function render() {
  const failed = state.status.filter(item => !item.success);
  const healthy = state.status.length - failed.length;
  const running = state.operations.filter(item => item.state === 'pending' || item.state === 'running');
  const failedOperations = state.operations.filter(item => item.state === 'failed');
  const safe = state.status.length > 0 && failed.length === 0;

  $('#control-state').textContent = safe ? 'Ready' : failed.length ? 'Attention' : 'Waiting';
  $('#control-detail').textContent = failed.length ? `${failed.length} reconciler${failed.length === 1 ? '' : 's'} failed` : safe ? 'All domains report success' : 'No reconciler evidence';
  $('#reconciler-count').textContent = state.status.length;
  $('#reconciler-detail').textContent = `${healthy} healthy · ${failed.length} failed`;
  $('#operation-count').textContent = state.operations.length;
  $('#operation-detail').textContent = `${running.length} active · ${failedOperations.length} failed`;
  $('#client-count').textContent = state.clients.length;
  $('#health-pill').textContent = safe ? 'Healthy' : failed.length ? 'Needs attention' : 'Waiting';
  $('#health-pill').className = `status-pill${safe ? '' : failed.length ? ' failed' : ' warning'}`;
  $('#running-pill').textContent = `${running.length} running`;
  $('#running-pill').className = `status-pill${running.length ? ' warning' : ''}`;

  $('#reconcilers').innerHTML = state.status.length ? state.status.map(item => `<div class="reconciler-row"><div><strong>${escapeHTML(item.name)}</strong><small>Last run ${escapeHTML(time(item.last_run))} · ${escapeHTML(item.duration || 'duration unavailable')}</small></div><span class="badge ${item.success ? '' : 'failed'}">${item.success ? 'Healthy' : 'Failed'}</span></div>`).join('') : '<div class="empty">No reconciler status returned.</div>';
  $('#operations').innerHTML = table([
    ['Operation', item => escapeHTML(item.kind)],
    ['Actor', item => escapeHTML(item.actor)],
    ['State', item => `<span class="badge ${item.state === 'failed' ? 'failed' : item.state === 'running' || item.state === 'pending' ? 'running' : ''}">${escapeHTML(item.state)}</span>`],
    ['Started', item => `<span class="muted-cell">${escapeHTML(time(item.createdAt))}</span>`]
  ], state.operations.slice(0, 30));
  $('#events').innerHTML = table([
    ['Event', item => escapeHTML(item.type)],
    ['Actor', item => escapeHTML(item.actor)],
    ['Message', item => escapeHTML(item.message)],
    ['Time', item => `<span class="muted-cell">${escapeHTML(time(item.createdAt))}</span>`]
  ], state.events.slice(0, 40));

  const confidential = state.clients.filter(client => !client.isPublic);
  $('#client-select').innerHTML = '<option value="">Select a client</option>' + confidential.map(client => `<option value="${escapeHTML(client.id)}" data-name="${escapeHTML(client.name)}">${escapeHTML(client.name)}</option>`).join('');
  const canOperate = state.principal.role === 'operator' || state.principal.role === 'admin';
  const isAdmin = state.principal.role === 'admin';
  $('#run-reconcile').disabled = !canOperate;
  $('#rotate-client').disabled = !isAdmin;
}

async function refresh() {
  try {
    state.principal = await api('/api/v1/me');
    const [status, operations, events, clients] = await Promise.all([
      api('/api/v1/status'), api('/api/v1/operations'), api('/api/v1/events'), api('/api/v1/identity/clients')
    ]);
    Object.assign(state, {status, operations, events, clients});
    $('#signed-out').hidden = true;
    $('#console').hidden = false;
    $('#login').hidden = true;
    $('#logout').hidden = false;
    $('#actor').textContent = state.principal.email || state.principal.subject;
    $('#role').textContent = `${state.principal.role} · Pocket ID`;
    $('#avatar').textContent = (state.principal.email || state.principal.subject || '?').slice(0, 1).toUpperCase();
    $('#last-refresh').textContent = `Updated ${new Date().toLocaleTimeString()}`;
    render();
  } catch (error) {
    if (error.message === 'AUTH_REQUIRED') {
      $('#signed-out').hidden = false;
      $('#console').hidden = true;
      return;
    }
    toast(`Refresh failed: ${error.message}`, true);
  }
}

async function reconcile() {
  if ($('#reconcile-confirmation').value !== 'RECONCILE') return toast('Type RECONCILE before queuing the operation.', true);
  try {
    const operation = await api('/api/v1/reconcile', {method:'POST'});
    $('#reconcile-confirmation').value = '';
    toast(`Reconciliation queued as ${operation.id}.`);
    window.setTimeout(refresh, 700);
  } catch (error) { toast(`Reconciliation failed: ${error.message}`, true); }
}

async function rotateClient() {
  const select = $('#client-select');
  const selected = select.options[select.selectedIndex];
  const name = selected?.dataset.name || '';
  if (!selected?.value) return toast('Select a confidential client.', true);
  if ($('#rotate-confirmation').value !== name) return toast(`Type ${name} exactly before rotating.`, true);
  try {
    await api(`/api/v1/identity/clients/${encodeURIComponent(selected.value)}/rotate`, {method:'POST'});
    $('#rotate-confirmation').value = '';
    toast(`${name} rotated; the replacement was persisted to Vault before retirement of the previous secret.`);
    await refresh();
  } catch (error) { toast(`Rotation failed: ${error.message}`, true); }
}

const views = {
  overview:['Control posture','Is Butler safe and ready to perform management work?'],
  actions:['Exceptional actions','High-impact operations that belong to Butler rather than another application.'],
  activity:['Operations & events','Bounded, audit-safe evidence for control-plane changes.']
};

document.querySelectorAll('.nav').forEach(button => button.addEventListener('click', () => {
  document.querySelectorAll('.nav').forEach(item => item.classList.toggle('active', item === button));
  document.querySelectorAll('main section').forEach(section => { section.hidden = section.id !== button.dataset.view; });
  $('#page-title').textContent = views[button.dataset.view][0];
  $('#page-description').textContent = views[button.dataset.view][1];
}));
$('#refresh').addEventListener('click', refresh);
$('#run-reconcile').addEventListener('click', reconcile);
$('#rotate-client').addEventListener('click', rotateClient);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });
window.setInterval(() => { if (!document.hidden && state.principal) refresh(); }, 30000);
refresh();
