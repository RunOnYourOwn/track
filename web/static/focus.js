// focus.js — Compact "Current Work" view for narrow split-tab usage
// Depends on globals: api, render, escHtml, priorityBadge, openTaskModal
// Exposes: renderFocus(prefix)

var renderFocus = (function() {
'use strict';

// PRIORITY_ORDER, byPriority, isWaiting are shared globals from app.js.

let _prefix = '';
let _tasks = [];
let _blockers = [];
let _timerInterval = null;

async function renderFocus(prefix) {
  _prefix = prefix;
  render('<div class="loading"><div class="spinner"></div> Loading…</div>');

  try {
    const [tasks, blockers] = await Promise.all([
      api.get(`/projects/${prefix}/tasks`),
      api.get(`/projects/${prefix}/blockers?open=true`).catch(() => []),
    ]);
    _tasks = tasks;
    _blockers = blockers || [];
  } catch (err) {
    render(`<div class="alert alert-danger">Failed to load: ${escHtml(err.message)}</div>`);
    return;
  }

  _draw();
}

function _draw() {
  if (_timerInterval) { clearInterval(_timerInterval); _timerInterval = null; }

  // Focus is about the actual unit of work — leaf tasks (incl. subtasks), not
  // the feature/epic containers they roll up into.
  const isWorkTask = t => (t.type || 'task') === 'task';

  const inProgress = _tasks
    .filter(t => t.status === 'in_progress' && isWorkTask(t))
    .sort(byPriority);

  const queue = _tasks
    .filter(t => t.status === 'todo' && !t.blocked && isWorkTask(t))
    .sort(byPriority)
    .slice(0, 3);

  const recentDone = _tasks
    .filter(t => t.status === 'done' && t.completed_at && isWorkTask(t))
    .sort((a, b) => b.completed_at.localeCompare(a.completed_at))
    .slice(0, 3);

  let html = '<div class="focus-page">';

  // WIP summary
  const wipCount = inProgress.length;
  const waiting = _tasks.filter(t => isWaiting(t.status) && isWorkTask(t)).length;
  html += `<div class="focus-wip-bar">
    <span class="focus-wip-active">${wipCount} active</span>
    ${waiting > 0 ? `<span class="focus-wip-waiting">${waiting} waiting</span>` : ''}
    <span class="focus-wip-queue">${queue.length} next</span>
  </div>`;

  // Active work with task timer
  if (inProgress.length > 0) {
    html += '<div class="focus-section">';
    html += '<div class="focus-section-label">Working on</div>';
    inProgress.forEach(t => { html += _renderActiveCard(t); });
    html += '</div>';
  } else {
    html += `<div class="focus-section">
      <div class="focus-empty">No active task — pick one below to start</div>
    </div>`;
  }

  // Up next
  if (queue.length > 0) {
    html += '<div class="focus-section">';
    html += '<div class="focus-section-label">Up next</div>';
    queue.forEach(t => { html += _renderQueueCard(t); });
    html += '</div>';
  }

  // Blockers
  if (_blockers.length > 0) {
    html += '<div class="focus-section">';
    html += `<div class="focus-section-label focus-label-danger">Blockers (${_blockers.length})</div>`;
    _blockers.slice(0, 3).forEach(b => {
      html += `<div class="focus-blocker">
        <span class="focus-blocker-type">${escHtml(b.blocker_type || '●')}</span>
        <span class="focus-blocker-title">${escHtml(b.title)}</span>
      </div>`;
    });
    html += '</div>';
  }

  // Recently done
  if (recentDone.length > 0) {
    html += '<div class="focus-section focus-section-muted">';
    html += '<div class="focus-section-label">Done recently</div>';
    recentDone.forEach(t => {
      html += `<div class="focus-done-item" data-task-id="${t.id}" role="button" tabindex="0">
        <span class="focus-done-check">✓</span>
        <span class="focus-done-id">${_prefix}-${t.seq}</span>
        <span class="focus-done-title">${escHtml(_trunc(t.title, 30))}</span>
      </div>`;
    });
    html += '</div>';
  }

  html += '</div>';
  render(html);
  _attachListeners();
  _startTaskTimer(inProgress);
}

function _renderActiveCard(task) {
  const displayId = `${_prefix}-${task.seq}`;
  const elapsed = _elapsed(task.updated_at);
  const estimate = task.estimate_hours > 0 ? `<span class="focus-estimate">${task.estimate_hours}h est</span>` : '';
  const desc = task.description ? `<div class="focus-active-desc">${escHtml(_trunc(task.description, 60))}</div>` : '';
  return `<div class="focus-active-card" data-task-id="${task.id}" role="button" tabindex="0">
    <div class="focus-active-header">
      <span class="focus-active-id">${displayId}</span>
      ${estimate}
      <span class="priority-badge ${task.priority}">${task.priority}</span>
    </div>
    <div class="focus-active-title">${escHtml(task.title)}</div>
    ${desc}
    <div class="focus-active-timer">
      <span class="focus-timer-dot"></span>
      <span class="focus-timer-value" data-timer-start="${task.updated_at}">${elapsed}</span>
      <span class="focus-timer-label">on task</span>
    </div>
    <div class="focus-active-actions">
      <button class="focus-btn focus-btn-done" data-action="done" data-task-id="${task.id}">✓ Done</button>
    </div>
  </div>`;
}

function _renderQueueCard(task) {
  const displayId = `${_prefix}-${task.seq}`;
  return `<div class="focus-queue-card" data-task-id="${task.id}" role="button" tabindex="0">
    <div class="focus-queue-left">
      <span class="focus-queue-id">${displayId}</span>
      <span class="focus-queue-title">${escHtml(_trunc(task.title, 36))}</span>
    </div>
    <button class="focus-btn focus-btn-start" data-action="start" data-task-id="${task.id}">▶</button>
  </div>`;
}

function _attachListeners() {
  document.querySelectorAll('[data-action="done"]').forEach(btn => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation(); // don't open the card modal
      const id = btn.dataset.taskId;
      btn.disabled = true;
      btn.textContent = '…';
      try {
        await api.patch(`/tasks/${id}`, { status: 'done' });
        _tasks = await api.get(`/projects/${_prefix}/tasks`);
        _draw();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '✓ Done';
      }
    });
  });

  document.querySelectorAll('[data-action="start"]').forEach(btn => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation(); // don't open the card modal
      const id = btn.dataset.taskId;
      btn.disabled = true;
      try {
        await api.patch(`/tasks/${id}`, { status: 'in_progress' });
        _tasks = await api.get(`/projects/${_prefix}/tasks`);
        _draw();
      } catch (err) {
        btn.disabled = false;
      }
    });
  });

  // Clicking a task card/item opens the shared editable detail modal.
  document.querySelectorAll('[data-task-id][role="button"]').forEach(card => {
    card.addEventListener('click', () => _openModal(card.dataset.taskId));
    card.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); _openModal(card.dataset.taskId); }
    });
  });
}

function _openModal(taskId) {
  const task = _tasks.find(t => t.id === taskId);
  if (!task) return;
  const refresh = async () => {
    _tasks = await api.get(`/projects/${_prefix}/tasks`);
    _draw();
  };
  openTaskModal(task, { prefix: _prefix, allTasks: _tasks, onSaved: refresh, onDeleted: refresh });
}

function _startTaskTimer(inProgress) {
  if (inProgress.length === 0) return;
  _timerInterval = setInterval(() => {
    document.querySelectorAll('[data-timer-start]').forEach(el => {
      el.textContent = _elapsed(el.dataset.timerStart);
    });
  }, 60000); // update every minute (not seconds — less distracting)
}

function _elapsed(isoStart) {
  const ms = Date.now() - new Date(isoStart).getTime();
  const totalMin = Math.floor(ms / 60000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function _trunc(str, max) {
  if (!str) return '';
  return str.length > max ? str.slice(0, max - 1) + '…' : str;
}

return renderFocus;
})();
