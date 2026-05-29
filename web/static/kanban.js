// kanban.js — Kanban board view (task-centric: every leaf task is a card placed
// in the column matching its OWN status; features/epics are shown as a label on
// each card, not as cards). A single task moves independently, and subtasks at
// any nesting depth always surface.
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
  if (status === 'done' || status === 'cancelled') return 'done'; // 'closed' column
  if (status === 'in_progress') return 'in_progress';
  if (isWaiting(status)) return 'waiting';
  return 'todo';
}

// PRIORITY_ORDER / byPriority / isWaiting are shared globals from app.js.
const STALE_DAYS = 7;

let _prefix = '';
let _project = null;
let _tasks   = [];
let _filters = { priorities: [], sourceTypes: [], blockedOnly: false };
let _showCancelled = false;
let _dragTaskId   = null;
let _dragFromCol  = null;
let _collapsedColumns = new Set(['done']);
let _doneCollapsed = true; // kept for backward compat with CSS class

async function renderKanban(prefix) {
  _prefix  = prefix;
  _filters = { priorities: [], sourceTypes: [], blockedOnly: false };

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
  const boardTasks = _tasks.filter(t => (t.type || 'task') === 'task');
  const total = boardTasks.length;
  if (total === 0) return '';
  const done = boardTasks.filter(t => t.status === 'done').length;
  const inProgress = boardTasks.filter(t => t.status === 'in_progress').length;
  const waiting = boardTasks.filter(t => isWaiting(t.status)).length;
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
        <span class="stat-chip wip ${inProgress >= ((_project && _project.wip_limit) || 3) ? 'wip-over' : ''}" id="stats-wip-chip" title="Project settings — WIP limit, sort, phase">WIP: ${inProgress}/${(_project && _project.wip_limit) || 3}</span>
        <span class="stat-chip" id="stats-sort-chip" title="Task sort order — click for project settings">Sort: ${TASK_SORT_LABELS[(_project && _project.task_sort) || 'priority'] || 'Priority'}</span>
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
  const cancelledCount = _tasks.filter(t => t.status === 'cancelled').length;

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
      ${cancelledCount > 0 ? `
        <label class="filter-checkbox filter-sep">
          <input type="checkbox" id="filter-cancelled" ${_showCancelled ? 'checked' : ''}>
          <span class="text-muted">Show cancelled (${cancelledCount})</span>
        </label>
      ` : ''}
      <button id="filter-clear" class="btn-ghost btn-sm">Clear</button>
    </div>
  `;
}

function _renderColumn(col, tasks) {
  // Every leaf task (any nesting depth) is a card, placed by its OWN status.
  // Order is preserved from the API, which returns tasks in the project's
  // configured task_sort order (server-side, the one source of truth) — the board
  // must not re-sort or it would override the chosen mode.
  const colTasks = tasks
    .filter(t => (t.type || 'task') === 'task' && _columnIdForStatus(t.status) === col.id
      && (_showCancelled || t.status !== 'cancelled')); // cancelled hidden unless toggled

  const isInProgress = col.id === 'in_progress';
  const totalCards = colTasks.length;
  const wipLimit = (_project && _project.wip_limit) || 3;
  const wip = isInProgress && totalCards >= wipLimit;
  const classes = ['kanban-column', wip ? 'wip-warning' : ''].filter(Boolean).join(' ');

  const cards = colTasks.map(t => _renderTaskCard(t)).join('');

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
  const parent = _parentLabel(task);
  const epicHtml = parent
    ? `<div class="task-card-epic" title="${escHtml(parent.full)}">${escHtml(parent.label)}</div>`
    : '';

  return `
    <div class="task-card ${task.blocked ? 'task-blocked' : ''} ${task.status === 'cancelled' ? 'task-cancelled' : ''}"
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

  const cancelledCb = document.getElementById('filter-cancelled');
  if (cancelledCb) {
    cancelledCb.addEventListener('change', () => {
      _showCancelled = cancelledCb.checked;
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

  // WIP / sort chips (and the in-column WIP display) open the project settings
  // panel — the single home for WIP limit, task sort, phase, and name.
  document.querySelectorAll('#wip-limit-display, #stats-wip-chip, #stats-sort-chip').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      _openProjectSettings(_prefix, _project, () => renderKanban(_prefix));
    });
  });

  // Column collapse toggles
  document.querySelectorAll('.col-toggle-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      e.stopPropagation();
      _toggleColumn(btn.dataset.colToggle);
    });
  });

  // Card click → detail
  document.querySelectorAll('.task-card').forEach(card => {
    card.addEventListener('click', () => _openDetail(card.dataset.taskId));
    card.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); _openDetail(card.dataset.taskId); }
    });
  });

  _attachDragListeners();
}

function _attachDragListeners() {
  document.querySelectorAll('.task-card[draggable]').forEach(card => {
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
    const wipLimit = (_project && _project.wip_limit) || 3;
    const currentWip = _tasks.filter(t => t.status === 'in_progress' && (t.type || 'task') === 'task').length;
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

function _openDetail(taskId) {
  const task = _tasks.find(t => t.id === taskId);
  if (!task) return;
  const refresh = async () => {
    _tasks = await api.get(`/projects/${_prefix}/tasks`);
    _drawBoard();
  };
  openTaskModal(task, { prefix: _prefix, allTasks: _tasks, onSaved: refresh, onDeleted: refresh });
}

function _applyFilters(tasks) {
  let result = tasks;
  if (_filters.priorities.length > 0) result = result.filter(t => _filters.priorities.includes(t.priority));
  if (_filters.sourceTypes.length > 0) result = result.filter(t => _filters.sourceTypes.includes(t.source_type));
  if (_filters.blockedOnly) result = result.filter(t => t.blocked);
  return result;
}

function _staleDays(isoDate) {
  if (!isoDate) return 0;
  return Math.floor((Date.now() - new Date(isoDate).getTime()) / 86400000);
}

// Immediate-parent context label for a task card: the feature/epic it belongs
// to, or — for a subtask — the parent task. Lets a card show where it sits at
// any nesting depth without the board having to nest the cards themselves.
function _parentLabel(task) {
  if (!task.parent_id) return null;
  const parent = _tasks.find(t => t.id === task.parent_id);
  if (!parent) return null;
  const pid = `${_prefix}-${parent.seq}`;
  const title = parent.title || '';
  const short = title.length > 32 ? title.slice(0, 31) + '…' : title;
  return { label: `${short} ▸`, full: `${pid}: ${title}` };
}

function _getFeatureName(task) {
  if (!task.parent_id || (task.type || 'task') !== 'task') return null;
  const parent = _tasks.find(t => t.id === task.parent_id);
  if (parent && (parent.type || 'task') === 'feature') return parent.title;
  return null;
}

return renderKanban;
})();
