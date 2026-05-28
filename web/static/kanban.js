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

// Map any task status to exactly one column. Crucially, EVERY waiting_* status
// (waiting_review/_external/_dependency/...) lands in Waiting, and any unknown
// status falls back to Backlog — so no task can silently vanish from the board.
function _columnIdForStatus(status) {
  if (status === 'done') return 'done';
  if (status === 'in_progress') return 'in_progress';
  if (status && status.indexOf('waiting') === 0) return 'waiting';
  return 'todo';
}

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
let _collapsedColumns = new Set(['done']);
let _doneCollapsed = true; // kept for backward compat with CSS class

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
  _doneCollapsed = _collapsedColumns.has('done');
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

function _toggleColumn(colId) {
  if (_collapsedColumns.has(colId)) {
    _collapsedColumns.delete(colId);
  } else {
    _collapsedColumns.add(colId);
  }
  _drawBoard();
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
        <span class="stat-chip wip ${inProgress >= ((_project && _project.wip_limit) || 5) ? 'wip-over' : ''}" id="stats-wip-chip" title="Click to set WIP limit — controls max items in progress">WIP: ${inProgress}/${(_project && _project.wip_limit) || 5}</span>
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
    .concat(sourceTypes.map(s => `<option value="${escHtml(s)}" ${_filters.sourceTypes.includes(s) ? 'selected' : ''}>${escHtml(s)}</option>`))
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
  const colTasks = tasks.filter(t => _columnIdForStatus(t.status) === col.id);
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

  if (_collapsedColumns.has(col.id)) {
    return `
      <div class="kanban-column kanban-column-collapsed" data-column="${col.id}" id="col-${col.id}">
        <div class="kanban-column-header kanban-column-header-collapsed">
          <button class="col-toggle-btn" data-col-toggle="${col.id}" title="Expand ${col.label}">◂</button>
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
        <span class="kanban-count">${totalCards}${isInProgress ? `/<span class="wip-limit-display" id="wip-limit-display" title="Click to set WIP limit — controls max items in progress">${wipLimit}</span>` : ''}</span>
        ${wip ? '<span class="wip-label" title="WIP limit reached">⚠</span>' : ''}
        <button class="col-toggle-btn" data-col-toggle="${col.id}" title="Collapse ${col.label}">▸</button>
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
  const epicName = _getEpicName(feat);
  const epicHtml = epicName
    ? `<div class="task-card-epic">${escHtml(epicName)}</div>`
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
         aria-label="${displayId}: ${escHtml(feat.title)}">
      <div class="feature-card-header">
        <div class="feature-card-left">
          <div class="feature-card-id">${displayId} ${staleHtml}</div>
          <div class="feature-card-title">${escHtml(title)}</div>
          ${epicHtml}
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
  const epicName = _getEpicName(task);
  const epicHtml = epicName
    ? `<div class="task-card-epic">${escHtml(epicName)}</div>`
    : '';

  return `
    <div class="task-card ${task.blocked ? 'task-blocked' : ''}"
         draggable="true"
         data-task-id="${task.id}"
         data-task-seq="${task.seq}"
         data-status="${task.status}"
         role="button"
         tabindex="0"
         aria-label="${displayId}: ${escHtml(task.title)}">
      <div class="task-card-row">
        <div class="task-card-left">
          <div class="task-card-id">${displayId} ${blockedHtml}${staleHtml}</div>
          <div class="task-card-title">${escHtml(title)}</div>
          ${epicHtml}
        </div>
        <span class="priority-badge task-card-priority ${task.priority}">${task.priority}</span>
      </div>
    </div>
  `;
}

function _renderDetailPanel(task) {
  const displayId = `${_prefix}-${task.seq}`;
  const epicName = _getEpicName(task);
  const featureName = _getFeatureName(task);
  const types = ['epic', 'feature', 'task'];
  const priorities = ['urgent', 'high', 'medium', 'low'];
  const statuses = ['todo', 'in_progress', 'done', 'blocked', 'waiting_external', 'waiting_decision', 'waiting_feedback'];
  const sizes = ['', 'XS', 'S', 'M', 'L', 'XL'];
  return `
    <div class="tt-modal-overlay" id="detail-backdrop">
      <div class="tt-modal" role="dialog" aria-label="Task detail">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px;">
          <span style="font-family:var(--font-mono);font-size:12px;color:var(--muted);">${displayId}</span>
          <button class="modal-close" id="detail-close" aria-label="Close">&times;</button>
        </div>
        <div class="tt-modal-field">
          <label class="tt-modal-label">Title</label>
          <input class="tt-modal-input" id="detail-title" value="${escHtml(task.title)}">
        </div>
        <div class="tt-modal-field">
          <label class="tt-modal-label">Description</label>
          <textarea class="tt-modal-textarea" id="detail-description" rows="8">${escHtml(task.description || '')}</textarea>
        </div>
        <div class="tt-modal-row">
          <div class="tt-modal-field">
            <label class="tt-modal-label">Type</label>
            <select class="tt-modal-input" id="detail-type">${types.map(t => `<option value="${t}" ${(task.type || 'task') === t ? 'selected' : ''}>${t}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Priority</label>
            <select class="tt-modal-input" id="detail-priority">${priorities.map(p => `<option value="${p}" ${task.priority === p ? 'selected' : ''}>${p}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Status</label>
            <select class="tt-modal-input" id="detail-status">${statuses.map(s => `<option value="${s}" ${task.status === s ? 'selected' : ''}>${s.replace(/_/g, ' ')}</option>`).join('')}</select>
          </div>
        </div>
        <div class="tt-modal-row">
          <div class="tt-modal-field">
            <label class="tt-modal-label">Estimate Size</label>
            <select class="tt-modal-input" id="detail-estimate-size">${sizes.map(s => `<option value="${s}" ${(task.estimate_size || '') === s ? 'selected' : ''}>${s || '—'}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Hours</label>
            <input class="tt-modal-input" id="detail-hours" type="number" step="0.25" value="${task.estimate_hours || ''}">
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Due Date</label>
            <input class="tt-modal-input" id="detail-due" type="date" value="${task.due_date || ''}">
          </div>
        </div>
        <div class="tt-modal-field">
          <label class="tt-modal-label">Parent</label>
          <select class="tt-modal-input" id="detail-parent">
            <option value="">(none)</option>
            ${_tasks.filter(t => t.id !== task.id && (t.type === 'epic' || t.type === 'feature')).map(t => `<option value="${t.id}" ${task.parent_id === t.id ? 'selected' : ''}>${_prefix}-${t.seq} ${escHtml(t.title.length > 40 ? t.title.slice(0, 37) + '...' : t.title)}</option>`).join('')}
          </select>
        </div>
        <div class="stat-row mt-8"><span class="stat-label">Source</span><span class="stat-value">${escHtml(task.source_type || '—')}</span></div>
        <div class="stat-row"><span class="stat-label">Created</span><span class="stat-value">${fmtDate(task.created_at)}</span></div>
        <div class="stat-row"><span class="stat-label">Updated</span><span class="stat-value">${fmtDate(task.updated_at)}</span></div>
        <div id="detail-deps-container" class="mt-16"><p class="text-muted">Loading deps…</p></div>
        <div class="tt-modal-actions">
          <button class="tt-modal-btn danger" id="detail-delete">Delete</button>
          <div style="display:flex;gap:8px;">
            <button class="tt-modal-btn" id="detail-cancel">Cancel</button>
            <button class="tt-modal-btn primary" id="detail-save">Save</button>
          </div>
        </div>
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

  // WIP limit click to edit (column header + stats chip)
  document.querySelectorAll('#wip-limit-display, #stats-wip-chip').forEach(el => {
    el.addEventListener('click', (e) => {
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
  });

  // Column collapse toggles
  document.querySelectorAll('.col-toggle-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      _toggleColumn(btn.dataset.colToggle);
    });
  });

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
  const cancelBtn = document.getElementById('detail-cancel');
  if (cancelBtn) cancelBtn.addEventListener('click', _closeDetail);
  const backdrop = document.getElementById('detail-backdrop');
  if (backdrop) backdrop.addEventListener('click', (e) => { if (e.target === backdrop) _closeDetail(); });
  const saveBtn = document.getElementById('detail-save');
  if (saveBtn) saveBtn.addEventListener('click', _saveDetail);
  const deleteBtn = document.getElementById('detail-delete');
  if (deleteBtn) deleteBtn.addEventListener('click', _deleteDetail);

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
      if (deps.length > 0) {
        const depRows = deps.map(d => {
          const depTask = _tasks.find(t => t.id === d.to_task_id);
          const label = depTask
            ? `${_prefix}-${depTask.seq}: ${escHtml(depTask.title.length > 40 ? depTask.title.slice(0, 39) + '…' : depTask.title)}`
            : escHtml(d.to_task_id.slice(-8));
          return `<li class="stat-row"><span class="stat-label">${escHtml(d.dep_type || 'blocks')}</span><span class="stat-value">${label}</span></li>`;
        }).join('');
        container.innerHTML = `<ul style="list-style:none;padding:0">${depRows}</ul>`;
      } else {
        container.innerHTML = '<p class="text-muted">No dependencies.</p>';
      }
    }
  } catch (_) {}
}

function _closeDetail() {
  _detail = null;
  _drawBoard();
}

async function _saveDetail() {
  if (!_detail) return;
  const payload = {};
  const title = document.getElementById('detail-title')?.value.trim();
  const description = document.getElementById('detail-description')?.value;
  const type = document.getElementById('detail-type')?.value;
  const priority = document.getElementById('detail-priority')?.value;
  const status = document.getElementById('detail-status')?.value;
  const estimateSize = document.getElementById('detail-estimate-size')?.value;
  const hours = parseFloat(document.getElementById('detail-hours')?.value) || 0;
  const due = document.getElementById('detail-due')?.value || '';
  const parentId = document.getElementById('detail-parent')?.value || '';

  if (title && title !== _detail.title) payload.title = title;
  if (description !== (_detail.description || '')) payload.description = description;
  if (type !== (_detail.type || 'task')) payload.type = type;
  if (priority !== _detail.priority) payload.priority = priority;
  if (status !== _detail.status) payload.status = status;
  if (estimateSize !== (_detail.estimate_size || '')) payload.estimate_size = estimateSize;
  if (hours !== (_detail.estimate_hours || 0)) payload.estimate_hours = hours;
  if (due !== (_detail.due_date || '')) payload.due_date = due;
  if (parentId !== (_detail.parent_id || '')) payload.parent_id = parentId;

  if (Object.keys(payload).length === 0) { _closeDetail(); return; }

  try {
    await api.patch(`/tasks/${_detail.id}`, payload);
    const task = _tasks.find(t => t.id === _detail.id);
    if (task) Object.assign(task, payload);
    _closeDetail();
  } catch (err) {
    alert('Save failed: ' + (err.message || err));
  }
}

async function _deleteDetail() {
  if (!_detail) return;
  const displayId = `${_prefix}-${_detail.seq}`;
  if (!confirm(`Delete ${displayId}?`)) return;
  try {
    await api.del(`/tasks/${_detail.id}`);
    _tasks = _tasks.filter(t => t.id !== _detail.id);
    _closeDetail();
  } catch (err) {
    alert('Delete failed: ' + (err.message || err));
  }
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

function _getEpicName(task) {
  if (!task.parent_id) return null;
  const parent = _tasks.find(t => t.id === task.parent_id);
  if (!parent) return null;
  if (parent.type === 'epic') return parent.title;
  if (parent.parent_id) {
    const grandparent = _tasks.find(t => t.id === parent.parent_id);
    if (grandparent && grandparent.type === 'epic') return grandparent.title;
  }
  return null;
}

function _getFeatureName(task) {
  if (!task.parent_id || (task.type || 'task') !== 'task') return null;
  const parent = _tasks.find(t => t.id === task.parent_id);
  if (parent && (parent.type || 'task') === 'feature') return parent.title;
  return null;
}

return renderKanban;
})();
