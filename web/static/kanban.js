// kanban.js — Kanban board view (Azure DevOps style: features with inline task lists)
// Depends on globals: api, render, fmtDate, fmtHours, priorityColor, escHtml
// Exposes: renderKanban(prefix)

var renderKanban = (function() {
'use strict';

const KANBAN_COLUMNS = [
  { id: 'todo',     label: 'Backlog',    statuses: ['todo'] },
  { id: 'in_progress', label: 'In Progress', statuses: ['in_progress'] },
  { id: 'waiting',  label: 'Waiting',    statuses: ['waiting_external', 'waiting_decision', 'waiting_feedback'] },
  { id: 'done',     label: 'Done',       statuses: ['done'] },
];

const COLUMN_DROP_STATUS = {
  todo:        'todo',
  in_progress: 'in_progress',
  waiting:     'waiting_external',
  done:        'done',
};

const PRIORITY_ORDER = { urgent: 0, high: 1, medium: 2, low: 3 };
const STALE_DAYS = 7;

let _prefix = '';
let _project = null;
let _tasks   = [];
let _filters = { priorities: [], sourceTypes: [], blockedOnly: false };
let _detail  = null;
let _dragTaskId   = null;
let _dragFromCol  = null;
let _expandedFeatures = {};
let _doneCollapsed = true;

async function renderKanban(prefix) {
  _prefix  = prefix;
  _filters = { priorities: [], sourceTypes: [], blockedOnly: false };
  _detail  = null;

  render('<div class="loading"><div class="spinner"></div> Loading board…</div>');

  try {
    const [project, tasks] = await Promise.all([
      api.get(`/projects/${prefix}`),
      api.get(`/projects/${prefix}/tasks`),
    ]);
    _project = project;
    _tasks = tasks;
  } catch (err) {
    render(`<div class="alert alert-danger">Failed to load tasks: ${escHtml(err.message)}</div>`);
    return;
  }

  _drawBoard();
}

function _drawBoard() {
  const filtered = _applyFilters(_tasks);
  const html = `
    ${_statsBar()}
    ${_filterBar()}
    <div class="kanban-board ${_doneCollapsed ? 'done-collapsed' : ''}" id="kanban-board">
      ${KANBAN_COLUMNS.map(col => _renderColumn(col, filtered)).join('')}
    </div>
    ${_detail ? _renderDetailPanel(_detail) : ''}
  `;
  render(html);
  _attachBoardListeners();
}

function _statsBar() {
  const boardTasks = _tasks.filter(t => (t.type || 'task') !== 'epic');
  const total = boardTasks.length;
  if (total === 0) return '';
  const done = boardTasks.filter(t => t.status === 'done').length;
  const inProgress = boardTasks.filter(t => t.status === 'in_progress').length;
  const waiting = boardTasks.filter(t => t.status.startsWith('waiting')).length;
  const blocked = boardTasks.filter(t => t.blocked).length;
  const todo = total - done - inProgress - waiting;
  const pct = Math.round((done / total) * 100);

  return `
    <div class="board-stats">
      <div class="board-stats-progress">
        <div class="board-stats-bar">
          <div class="bar-done" style="width:${pct}%"></div>
          <div class="bar-inprogress" style="width:${Math.round((inProgress / total) * 100)}%"></div>
        </div>
        <span class="board-stats-pct">${pct}%</span>
      </div>
      <div class="board-stats-counts">
        <span class="stat-chip done">${done} done</span>
        <span class="stat-chip inprogress">${inProgress} active</span>
        <span class="stat-chip waiting">${waiting} waiting</span>
        ${blocked > 0 ? `<span class="stat-chip blocked">${blocked} blocked</span>` : ''}
        <span class="stat-chip todo">${todo} backlog</span>
        <span class="stat-chip total">${total} total</span>
      </div>
    </div>
  `;
}

function _filterBar() {
  const priorities = ['urgent', 'high', 'medium', 'low'];
  const sourceTypes = [...new Set(_tasks.map(t => t.source_type).filter(Boolean))];

  const priorityCheckboxes = priorities.map(p => `
    <label class="filter-checkbox">
      <input type="checkbox" data-filter-priority="${p}"
             ${_filters.priorities.includes(p) ? 'checked' : ''}>
      <span class="priority-badge ${priorityColor(p)}">${p}</span>
    </label>
  `).join('');

  const sourceOptions = ['<option value="">All sources</option>']
    .concat(sourceTypes.map(s => `<option value="${s}" ${_filters.sourceTypes.includes(s) ? 'selected' : ''}>${s}</option>`))
    .join('');

  const blockedCount = _tasks.filter(t => t.blocked).length;

  return `
    <div class="filter-bar">
      <span class="filter-label">Priority:</span>
      ${priorityCheckboxes}
      ${sourceTypes.length > 0 ? `
        <span class="filter-label filter-sep">Source:</span>
        <select id="filter-source" class="filter-select">
          ${sourceOptions}
        </select>
      ` : ''}
      ${blockedCount > 0 ? `
        <label class="filter-checkbox filter-sep">
          <input type="checkbox" id="filter-blocked" ${_filters.blockedOnly ? 'checked' : ''}>
          <span class="text-danger">Blocked (${blockedCount})</span>
        </label>
      ` : ''}
      <button id="filter-clear" class="btn-ghost btn-sm">Clear</button>
    </div>
  `;
}

function _renderColumn(col, tasks) {
  const colTasks = tasks.filter(t => col.statuses.includes(t.status));
  const epicIds = new Set(_tasks.filter(t => t.type === 'epic').map(t => t.id));
  const features = colTasks.filter(t => (t.type || 'task') === 'feature');
  const orphanTasks = colTasks.filter(t => (t.type || 'task') === 'task' && (!t.parent_id || epicIds.has(t.parent_id)));

  // Also find tasks that belong to features in THIS column
  // (tasks may be in a different status than their parent feature)
  const allTasks = _tasks.filter(t => (t.type || 'task') === 'task');
  const tasksByParent = {};
  allTasks.forEach(t => {
    if (t.parent_id && !epicIds.has(t.parent_id)) {
      if (!tasksByParent[t.parent_id]) tasksByParent[t.parent_id] = [];
      tasksByParent[t.parent_id].push(t);
    }
  });

  const isInProgress = col.id === 'in_progress';
  const totalCards = features.length + orphanTasks.length;
  const wipLimit = (_project && _project.wip_limit) || 5;
  const wip = isInProgress && totalCards >= wipLimit;
  const classes = ['kanban-column', wip ? 'wip-warning' : ''].filter(Boolean).join(' ');

  // Sort features by priority
  const sortedFeatures = [...features].sort((a, b) => {
    const pa = PRIORITY_ORDER[a.priority] ?? 99;
    const pb = PRIORITY_ORDER[b.priority] ?? 99;
    if (pa !== pb) return pa - pb;
    return (a.seq ?? 0) - (b.seq ?? 0);
  });

  // Sort orphan tasks by priority
  const sortedOrphans = [...orphanTasks].sort((a, b) => {
    const pa = PRIORITY_ORDER[a.priority] ?? 99;
    const pb = PRIORITY_ORDER[b.priority] ?? 99;
    if (pa !== pb) return pa - pb;
    return (a.seq ?? 0) - (b.seq ?? 0);
  });

  let cards = '';
  sortedFeatures.forEach(feat => {
    let childTasks = tasksByParent[feat.id] || [];
    if (_filters.blockedOnly) childTasks = childTasks.filter(t => t.blocked);
    cards += _renderFeatureCard(feat, childTasks);
  });

  if (sortedOrphans.length > 0) {
    sortedOrphans.forEach(t => {
      cards += _renderTaskCard(t);
    });
  }

  if (col.id === 'done' && _doneCollapsed) {
    return `
      <div class="kanban-column kanban-column-collapsed" data-column="${col.id}" id="col-${col.id}">
        <div class="kanban-column-header kanban-column-header-collapsed">
          <button class="done-toggle-btn" id="done-toggle" title="Expand Done column">◂</button>
          <span class="kanban-column-title-rotated">${col.label}</span>
          <span class="kanban-count">${totalCards}</span>
        </div>
      </div>
    `;
  }

  return `
    <div class="${classes}" data-column="${col.id}" id="col-${col.id}">
      <div class="kanban-column-header">
        <span class="kanban-column-title">${col.label}</span>
        <span class="kanban-count">${totalCards}${isInProgress ? `/<span class="wip-limit-display" id="wip-limit-display" title="Click to change WIP limit">${wipLimit}</span>` : ''}</span>
        ${wip ? '<span class="wip-label" title="WIP limit reached">⚠</span>' : ''}
        ${col.id === 'done' ? '<button class="done-toggle-btn" id="done-toggle" title="Collapse Done column">▸</button>' : ''}
      </div>
      <div class="kanban-cards" data-column="${col.id}">
        ${cards || '<div style="padding:8px;color:var(--muted);font-size:11px">No items</div>'}
      </div>
    </div>
  `;
}

function _renderFeatureCard(feat, childTasks) {
  const displayId = `${_prefix}-${feat.seq}`;
  const title = feat.title && feat.title.length > 60 ? feat.title.slice(0, 59) + '…' : (feat.title || '');
  const expanded = !!_expandedFeatures[feat.id];
  const doneCount = childTasks.filter(t => t.status === 'done').length;
  const totalCount = childTasks.length;
  const pctDone = totalCount > 0 ? Math.round((doneCount / totalCount) * 100) : 0;
  const stale = _staleDays(feat.updated_at);
  const staleHtml = stale > STALE_DAYS
    ? `<span class="stale-indicator" title="Last updated ${stale} days ago">⏱ ${stale}d</span>`
    : '';

  let taskListHtml = '';
  if (expanded && childTasks.length > 0) {
    const sorted = [...childTasks].sort((a, b) => {
      const pa = PRIORITY_ORDER[a.priority] ?? 99;
      const pb = PRIORITY_ORDER[b.priority] ?? 99;
      if (pa !== pb) return pa - pb;
      return (a.seq ?? 0) - (b.seq ?? 0);
    });
    taskListHtml = `
      <div class="feature-task-list">
        ${sorted.map(t => _renderInlineTask(t)).join('')}
      </div>
    `;
  }

  return `
    <div class="feature-card"
         draggable="true"
         data-task-id="${feat.id}"
         data-task-seq="${feat.seq}"
         data-status="${feat.status}"
         role="button"
         tabindex="0"
         aria-label="${displayId}: ${feat.title}">
      <div class="feature-card-header">
        <div class="feature-card-left">
          <div class="feature-card-id">${displayId} ${staleHtml}</div>
          <div class="feature-card-title">${escHtml(title)}</div>
        </div>
        <span class="priority-badge feature-card-priority ${feat.priority}">${feat.priority}</span>
      </div>
      ${totalCount > 0 ? `
        <div class="feature-card-progress">
          <div class="feature-progress-bar">
            <div class="feature-progress-fill" style="width:${pctDone}%"></div>
          </div>
          <span class="feature-progress-text">${doneCount}/${totalCount}</span>
          <button class="feature-expand-btn" data-feature-id="${feat.id}" title="${expanded ? 'Collapse' : 'Expand'} tasks">
            ${expanded ? '▾' : '▸'}
          </button>
        </div>
        ${taskListHtml}
      ` : ''}
    </div>
  `;
}

function _renderInlineTask(task) {
  const displayId = `${_prefix}-${task.seq}`;
  const isDone = task.status === 'done';
  const isBlocked = task.status.startsWith('waiting');
  const title = task.title && task.title.length > 40 ? task.title.slice(0, 39) + '…' : (task.title || '');
  const statusClass = isDone ? 'done' : isBlocked ? 'blocked' : task.status === 'in_progress' ? 'active' : '';

  return `
    <div class="inline-task ${statusClass}" data-task-id="${task.id}" role="button" tabindex="0">
      <span class="inline-task-check">${isDone ? '✓' : isBlocked ? '⏸' : '○'}</span>
      <span class="inline-task-id">${displayId}</span>
      <span class="inline-task-title">${escHtml(title)}</span>
      <span class="priority-dot ${task.priority}" title="${task.priority}"></span>
    </div>
  `;
}

function _renderTaskCard(task) {
  const displayId = `${_prefix}-${task.seq}`;
  const title = task.title && task.title.length > 60 ? task.title.slice(0, 59) + '…' : (task.title || '');
  const stale = _staleDays(task.updated_at);
  const staleHtml = stale > STALE_DAYS
    ? `<span class="stale-indicator" title="Last updated ${stale} days ago">⏱ ${stale}d</span>`
    : '';
  const blockedHtml = task.blocked
    ? `<span class="blocked-indicator" title="Blocked by dependency">●</span>`
    : '';

  return `
    <div class="task-card ${task.blocked ? 'task-blocked' : ''}"
         draggable="true"
         data-task-id="${task.id}"
         data-task-seq="${task.seq}"
         data-status="${task.status}"
         role="button"
         tabindex="0"
         aria-label="${displayId}: ${task.title}">
      <div class="task-card-row">
        <div class="task-card-left">
          <div class="task-card-id">${displayId} ${blockedHtml}${staleHtml}</div>
          <div class="task-card-title">${escHtml(title)}</div>
        </div>
        <span class="priority-badge task-card-priority ${task.priority}">${task.priority}</span>
      </div>
    </div>
  `;
}

function _renderDetailPanel(task) {
  const displayId = `${_prefix}-${task.seq}`;
  return `
    <div class="modal-backdrop" id="detail-backdrop">
      <div class="modal" role="dialog" aria-label="Task detail">
        <div class="modal-header">
          <span class="modal-title">${displayId} — ${escHtml(task.title)}</span>
          <button class="modal-close" id="detail-close" aria-label="Close">&times;</button>
        </div>
        <div class="detail-badges mb-16">
          <span class="priority-badge ${task.priority}">${task.priority}</span>
          <span class="status-badge ${task.status}">${task.status.replace(/_/g, ' ')}</span>
          ${task.type && task.type !== 'task' ? `<span class="type-badge type-${task.type}">${task.type}</span>` : ''}
          ${task.estimate_size ? `<span class="badge">${task.estimate_size}</span>` : ''}
        </div>
        ${task.description ? `<p style="color:var(--muted);margin-bottom:12px">${escHtml(task.description)}</p>` : ''}
        <div class="stat-row"><span class="stat-label">Type</span><span class="stat-value">${task.type || 'task'}</span></div>
        <div class="stat-row"><span class="stat-label">Source</span><span class="stat-value">${task.source_type || '—'}</span></div>
        <div class="stat-row"><span class="stat-label">Created</span><span class="stat-value">${fmtDate(task.created_at)}</span></div>
        <div class="stat-row"><span class="stat-label">Updated</span><span class="stat-value">${fmtDate(task.updated_at)}</span></div>
        ${task.due_date ? `<div class="stat-row"><span class="stat-label">Due</span><span class="stat-value">${fmtDate(task.due_date)}</span></div>` : ''}
        <div id="detail-deps-container" class="mt-16"><p class="text-muted">Loading deps…</p></div>
      </div>
    </div>
  `;
}

function _attachBoardListeners() {
  // Filter checkboxes
  document.querySelectorAll('[data-filter-priority]').forEach(cb => {
    cb.addEventListener('change', () => {
      const p = cb.dataset.filterPriority;
      if (cb.checked) {
        if (!_filters.priorities.includes(p)) _filters.priorities.push(p);
      } else {
        _filters.priorities = _filters.priorities.filter(x => x !== p);
      }
      _drawBoard();
    });
  });

  const srcSelect = document.getElementById('filter-source');
  if (srcSelect) {
    srcSelect.addEventListener('change', () => {
      _filters.sourceTypes = srcSelect.value ? [srcSelect.value] : [];
      _drawBoard();
    });
  }

  const blockedCb = document.getElementById('filter-blocked');
  if (blockedCb) {
    blockedCb.addEventListener('change', () => {
      _filters.blockedOnly = blockedCb.checked;
      _drawBoard();
    });
  }

  const clearBtn = document.getElementById('filter-clear');
  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      _filters = { priorities: [], sourceTypes: [], blockedOnly: false };
      _drawBoard();
    });
  }

  // WIP limit click to edit
  const wipDisplay = document.getElementById('wip-limit-display');
  if (wipDisplay) {
    wipDisplay.addEventListener('click', (e) => {
      e.stopPropagation();
      const current = (_project && _project.wip_limit) || 5;
      const input = prompt('WIP limit for In Progress:', current);
      if (input === null) return;
      const val = parseInt(input, 10);
      if (isNaN(val) || val < 1) return;
      _project.wip_limit = val;
      api.patch(`/projects/${_prefix}`, { wip_limit: val }).catch(() => {});
      _drawBoard();
    });
  }

  // Done column toggle
  const doneToggle = document.getElementById('done-toggle');
  if (doneToggle) {
    doneToggle.addEventListener('click', (e) => {
      e.stopPropagation();
      _doneCollapsed = !_doneCollapsed;
      _drawBoard();
    });
  }

  // Feature expand toggles
  document.querySelectorAll('.feature-expand-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      const fid = btn.dataset.featureId;
      _expandedFeatures[fid] = !_expandedFeatures[fid];
      _drawBoard();
    });
  });

  // Card click → detail
  document.querySelectorAll('.feature-card, .task-card').forEach(card => {
    card.addEventListener('click', (e) => {
      if (e.target.closest('.feature-expand-btn')) return;
      _openDetail(card.dataset.taskId);
    });
    card.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); _openDetail(card.dataset.taskId); }
    });
  });

  // Inline task click → detail
  document.querySelectorAll('.inline-task').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      _openDetail(el.dataset.taskId);
    });
  });

  const closeBtn = document.getElementById('detail-close');
  if (closeBtn) closeBtn.addEventListener('click', _closeDetail);
  const backdrop = document.getElementById('detail-backdrop');
  if (backdrop) backdrop.addEventListener('click', (e) => { if (e.target === backdrop) _closeDetail(); });

  _attachDragListeners();
}

function _attachDragListeners() {
  document.querySelectorAll('.feature-card[draggable], .task-card[draggable]').forEach(card => {
    card.addEventListener('dragstart', _onDragStart);
    card.addEventListener('dragend', _onDragEnd);
  });
  document.querySelectorAll('.kanban-cards').forEach(list => {
    list.addEventListener('dragover', e => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; });
    list.addEventListener('dragenter', e => { e.preventDefault(); e.currentTarget.classList.add('drag-over'); });
    list.addEventListener('dragleave', e => { if (!e.currentTarget.contains(e.relatedTarget)) e.currentTarget.classList.remove('drag-over'); });
    list.addEventListener('drop', _onDrop);
  });
}

function _onDragStart(e) {
  _dragTaskId  = e.currentTarget.dataset.taskId;
  _dragFromCol = e.currentTarget.closest('[data-column]')?.dataset.column;
  e.currentTarget.classList.add('dragging');
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', _dragTaskId);
}

function _onDragEnd(e) {
  e.currentTarget.classList.remove('dragging');
  document.querySelectorAll('.kanban-cards.drag-over').forEach(el => el.classList.remove('drag-over'));
}

async function _onDrop(e) {
  e.preventDefault();
  e.currentTarget.classList.remove('drag-over');
  const targetCol = e.currentTarget.dataset.column;
  if (!_dragTaskId || targetCol === _dragFromCol) return;
  const newStatus = COLUMN_DROP_STATUS[targetCol];
  if (!newStatus) return;

  // Enforce WIP limit on In Progress
  if (targetCol === 'in_progress') {
    const wipLimit = (_project && _project.wip_limit) || 5;
    const currentWip = _tasks.filter(t => t.status === 'in_progress').length;
    if (currentWip >= wipLimit) {
      alert(`WIP limit reached (${wipLimit}). Complete or move an item before starting new work.`);
      return;
    }
  }

  const task = _tasks.find(t => t.id === _dragTaskId);
  if (!task) return;
  const prevStatus = task.status;
  task.status = newStatus;
  _drawBoard();

  try {
    await api.patch(`/tasks/${_dragTaskId}`, { status: newStatus });
  } catch (err) {
    task.status = prevStatus;
    _drawBoard();
  }
}

async function _openDetail(taskId) {
  const task = _tasks.find(t => t.id === taskId);
  if (!task) return;
  _detail = { ...task, _deps: null };
  _drawBoard();

  try {
    const deps = await api.get(`/tasks/${taskId}/deps`);
    if (!_detail || _detail.id !== taskId) return;
    const container = document.getElementById('detail-deps-container');
    if (container) {
      container.innerHTML = deps.length > 0
        ? `<ul style="list-style:none;padding:0">${deps.map(d => `<li class="stat-row"><span class="stat-label">${escHtml(d.dep_type || 'blocks')}</span><span class="stat-value mono">${escHtml(d.to_task_id.slice(-8))}</span></li>`).join('')}</ul>`
        : '<p class="text-muted">No dependencies.</p>';
    }
  } catch (_) {}
}

function _closeDetail() {
  _detail = null;
  _drawBoard();
}

function _applyFilters(tasks) {
  let result = tasks;
  if (_filters.priorities.length > 0) result = result.filter(t => _filters.priorities.includes(t.priority));
  if (_filters.sourceTypes.length > 0) result = result.filter(t => _filters.sourceTypes.includes(t.source_type));
  if (_filters.blockedOnly) {
    const blockedParentIds = new Set(tasks.filter(t => t.blocked && t.parent_id).map(t => t.parent_id));
    result = result.filter(t => t.blocked || blockedParentIds.has(t.id));
  }
  return result;
}

function _staleDays(isoDate) {
  if (!isoDate) return 0;
  return Math.floor((Date.now() - new Date(isoDate).getTime()) / 86400000);
}

return renderKanban;
})();
