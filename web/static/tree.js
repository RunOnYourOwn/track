// tree.js — Editable hierarchical task table (epic → feature → task)
// Depends on globals: api, render, escHtml, priorityBadge, statusBadge
// Exposes: renderTree(prefix)

var renderTree = (function() {
'use strict';

let _prefix = '';
let _tasks = [];
let _collapsed = {};
let _showDone = false;
let _selected = new Set();
let _editing = null; // { id, field }
let _dragId = null;
let _menuOpen = null; // task id with open action menu

const TYPES = ['epic', 'feature', 'task'];
const PRIORITIES = ['urgent', 'high', 'medium', 'low'];
// STATUSES and PRIORITY_ORDER are shared globals from app.js.
const SIZES = ['', 'XS', 'S', 'M', 'L', 'XL'];

async function renderTree(prefix) {
  _prefix = prefix;
  render('<div class="loading"><div class="spinner"></div> Loading…</div>');
  try {
    _tasks = await api.get(`/projects/${prefix}/tasks`);
  } catch (err) {
    render(`<div class="alert alert-danger">Failed to load: ${escHtml(err.message)}</div>`);
    return;
  }
  _drawTable();
}

function _drawTable() {
  const rows = _buildRows();
  const doneCount = _tasks.filter(t => t.status === 'done').length;
  const allChecked = rows.length > 0 && rows.every(r => _selected.has(r.task.id));

  let html = `
    <div class="tt-toolbar">
      <div class="tt-toolbar-left">
        <span style="font-weight:600;font-size:14px;">Tasks</span>
        ${doneCount > 0 ? `<label class="filter-checkbox" style="margin-left:12px;"><input type="checkbox" id="tt-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted" style="font-size:12px;">Show done (${doneCount})</span></label>` : ''}
      </div>
      <div class="tt-toolbar-right">
        <button class="tt-btn-add" id="tt-add-root">+ New task</button>
        <button class="btn-ghost btn-sm" id="tt-expand-all">Expand all</button>
        <button class="btn-ghost btn-sm" id="tt-collapse-all">Collapse all</button>
      </div>
    </div>
    <div class="tt-container">
      <table class="tt-table">
        <thead>
          <tr>
            <th class="tt-col-check"><input type="checkbox" class="tt-checkbox" id="tt-check-all" ${allChecked ? 'checked' : ''}></th>
            <th class="tt-col-drag"></th>
            <th class="tt-col-id">ID</th>
            <th class="tt-col-title">Title</th>
            <th class="tt-col-type">Type</th>
            <th class="tt-col-priority">Priority</th>
            <th class="tt-col-status">Status</th>
            <th class="tt-col-est">Est</th>
            <th class="tt-col-due">Due</th>
            <th class="tt-col-actions"></th>
          </tr>
        </thead>
        <tbody>
          ${rows.length === 0 ? '<tr><td colspan="10" class="tt-empty">No tasks found</td></tr>' : ''}
          ${rows.map(r => _renderRow(r.task, r.depth)).join('')}
        </tbody>
      </table>
    </div>
    <div class="tt-bulk-bar ${_selected.size > 0 ? 'visible' : ''}" id="tt-bulk-bar">
      <span class="tt-bulk-count">${_selected.size} selected</span>
      <select class="tt-bulk-btn" id="tt-bulk-status"><option value="">Set status…</option>${STATUSES.map(s => `<option value="${s}">${s}</option>`).join('')}</select>
      <select class="tt-bulk-btn" id="tt-bulk-priority"><option value="">Set priority…</option>${PRIORITIES.map(p => `<option value="${p}">${p}</option>`).join('')}</select>
      <button class="tt-bulk-btn danger" id="tt-bulk-delete">Delete</button>
      <button class="tt-bulk-btn" id="tt-bulk-clear">Clear</button>
    </div>
  `;
  render(html);
  _attachListeners();
}

function _buildRows() {
  const visible = _showDone ? _tasks : _tasks.filter(t => t.status !== 'done');
  const byId = {};
  visible.forEach(t => { byId[t.id] = { ...t, children: [] }; });

  const roots = [];
  visible.forEach(t => {
    const node = byId[t.id];
    if (t.parent_id && byId[t.parent_id]) {
      byId[t.parent_id].children.push(node);
    } else {
      roots.push(node);
    }
  });

  const typeOrder = { epic: 0, feature: 1, task: 2 };
  const sortFn = (a, b) => {
    if ((a.sort_order || 0) !== (b.sort_order || 0)) return (a.sort_order || 0) - (b.sort_order || 0);
    const ta = typeOrder[(a.type || 'task')] ?? 2;
    const tb = typeOrder[(b.type || 'task')] ?? 2;
    if (ta !== tb) return ta - tb;
    const pa = PRIORITY_ORDER[a.priority] ?? 2;
    const pb = PRIORITY_ORDER[b.priority] ?? 2;
    if (pa !== pb) return pa - pb;
    return (a.seq || 0) - (b.seq || 0);
  };
  const sortTree = (nodes) => { nodes.sort(sortFn); nodes.forEach(n => sortTree(n.children)); };
  sortTree(roots);

  const flat = [];
  const walk = (nodes, depth) => {
    nodes.forEach(node => {
      flat.push({ task: node, depth });
      if (node.children.length > 0 && !_collapsed[node.id]) {
        walk(node.children, depth + 1);
      }
    });
  };
  walk(roots, 0);
  return flat;
}

function _renderRow(task, depth) {
  const t = task;
  const type = t.type || 'task';
  const checked = _selected.has(t.id) ? 'checked' : '';
  const isDone = t.status === 'done';
  const hasChildren = (t.children && t.children.length > 0);
  const isCollapsed = _collapsed[t.id];
  const toggleIcon = hasChildren ? (isCollapsed ? '▸' : '▾') : '<span style="width:16px;display:inline-block"></span>';
  const indent = depth * 20;
  const displayId = `${_prefix}-${t.seq}`;
  const estDisplay = t.estimate_hours ? t.estimate_hours + 'h' : (t.estimate_size || '');
  const dueDisplay = t.due_date ? t.due_date.slice(5) : '';

  return `
    <tr class="tt-row ${type} ${isDone ? 'done' : ''}" data-id="${t.id}" data-type="${type}" draggable="false">
      <td class="tt-col-check"><input type="checkbox" class="tt-checkbox tt-row-check" data-id="${t.id}" ${checked}></td>
      <td class="tt-col-drag"><span class="tt-drag-handle" draggable="true" data-id="${t.id}">⠿</span></td>
      <td class="tt-col-id" style="font-family:var(--font-mono);font-size:11px;color:var(--muted);">${displayId}</td>
      <td class="tt-col-title">
        <div class="tt-cell-title" style="padding-left:${indent}px;">
          <span class="tt-toggle" data-toggle-id="${t.id}">${toggleIcon}</span>
          <span class="tt-title-text" data-edit="title" data-id="${t.id}">${escHtml(t.title)}</span>
        </div>
      </td>
      <td class="tt-col-type" data-edit="type" data-id="${t.id}">${type}</td>
      <td class="tt-col-priority" data-edit="priority" data-id="${t.id}">${priorityBadge(t.priority)}</td>
      <td class="tt-col-status" data-edit="status" data-id="${t.id}">${statusBadge(t.status)}</td>
      <td class="tt-col-est" data-edit="estimate_hours" data-id="${t.id}">${estDisplay}</td>
      <td class="tt-col-due" data-edit="due_date" data-id="${t.id}">${dueDisplay}</td>
      <td class="tt-col-actions" style="position:relative;">
        <div class="tt-row-actions">
          <button class="tt-action-btn" data-menu-id="${t.id}" title="Actions">⋯</button>
        </div>
        ${_menuOpen === t.id ? _renderMenu(t) : ''}
      </td>
    </tr>
  `;
}

function _renderMenu(t) {
  const type = t.type || 'task';
  return `
    <div class="tt-actions-menu" id="tt-menu-${t.id}">
      <button class="tt-actions-menu-item" data-action="detail" data-id="${t.id}">Edit details</button>
      <button class="tt-actions-menu-item" data-action="add-child" data-id="${t.id}" data-child-type="task">Add child task</button>
      ${type === 'epic' ? `<button class="tt-actions-menu-item" data-action="add-child" data-id="${t.id}" data-child-type="feature">Add child feature</button>` : ''}
      <button class="tt-actions-menu-item danger" data-action="delete" data-id="${t.id}">Delete</button>
    </div>
  `;
}

function _attachListeners() {
  // Show done toggle
  const showDone = document.getElementById('tt-show-done');
  if (showDone) showDone.addEventListener('change', () => { _showDone = showDone.checked; _drawTable(); });

  // Expand/collapse all
  const expandAll = document.getElementById('tt-expand-all');
  const collapseAll = document.getElementById('tt-collapse-all');
  if (expandAll) expandAll.addEventListener('click', () => { _collapsed = {}; _drawTable(); });
  if (collapseAll) collapseAll.addEventListener('click', () => {
    _tasks.forEach(t => { if (_tasks.some(c => c.parent_id === t.id)) _collapsed[t.id] = true; });
    _drawTable();
  });

  // Toggle expand/collapse per row
  document.querySelectorAll('.tt-toggle[data-toggle-id]').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      const id = el.dataset.toggleId;
      _collapsed[id] = !_collapsed[id];
      _drawTable();
    });
  });

  // Inline editing — click cells
  document.querySelectorAll('[data-edit]').forEach(el => {
    el.addEventListener('click', (e) => {
      if (e.target.closest('.tt-toggle')) return;
      const id = el.dataset.id;
      const field = el.dataset.edit;
      _startEdit(id, field, el);
    });
  });

  // Checkboxes
  const checkAll = document.getElementById('tt-check-all');
  if (checkAll) checkAll.addEventListener('change', () => {
    const rows = _buildRows();
    if (checkAll.checked) { rows.forEach(r => _selected.add(r.task.id)); }
    else { _selected.clear(); }
    _drawTable();
  });
  document.querySelectorAll('.tt-row-check').forEach(el => {
    el.addEventListener('change', () => {
      const id = el.dataset.id;
      if (el.checked) _selected.add(id); else _selected.delete(id);
      _updateBulkBar();
    });
  });

  // Bulk actions
  const bulkStatus = document.getElementById('tt-bulk-status');
  const bulkPriority = document.getElementById('tt-bulk-priority');
  const bulkDelete = document.getElementById('tt-bulk-delete');
  const bulkClear = document.getElementById('tt-bulk-clear');
  if (bulkStatus) bulkStatus.addEventListener('change', () => { if (bulkStatus.value) _bulkAction('status', bulkStatus.value); });
  if (bulkPriority) bulkPriority.addEventListener('change', () => { if (bulkPriority.value) _bulkAction('priority', bulkPriority.value); });
  if (bulkDelete) bulkDelete.addEventListener('click', _bulkDelete);
  if (bulkClear) bulkClear.addEventListener('click', () => { _selected.clear(); _drawTable(); });

  // Action menus
  document.querySelectorAll('[data-menu-id]').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      const id = el.dataset.menuId;
      _menuOpen = _menuOpen === id ? null : id;
      _drawTable();
    });
  });
  document.querySelectorAll('[data-action]').forEach(el => {
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      const action = el.dataset.action;
      const id = el.dataset.id;
      _menuOpen = null;
      if (action === 'detail') _openDetail(id);
      else if (action === 'add-child') _createTask(id, el.dataset.childType || 'task');
      else if (action === 'delete') _deleteTask(id);
    });
  });

  // Position the open menu at its button (fixed → not clipped by table/container overflow).
  if (_menuOpen) {
    const menu = document.querySelector('.tt-actions-menu');
    const btn = document.querySelector(`[data-menu-id="${_menuOpen}"]`);
    if (menu && btn) {
      const r = btn.getBoundingClientRect();
      menu.style.top = (r.bottom + 2) + 'px';
      menu.style.left = Math.max(8, r.right - menu.offsetWidth) + 'px';
    }
  }

  // Close menu on outside click
  document.addEventListener('click', () => { if (_menuOpen) { _menuOpen = null; _drawTable(); } }, { once: true });

  // Add root task
  const addRoot = document.getElementById('tt-add-root');
  if (addRoot) addRoot.addEventListener('click', () => _createTask('', 'task'));

  // Drag and drop
  document.querySelectorAll('.tt-drag-handle').forEach(el => {
    el.addEventListener('dragstart', (e) => {
      _dragId = el.dataset.id;
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', _dragId);
      setTimeout(() => { const row = document.querySelector(`tr[data-id="${_dragId}"]`); if (row) row.classList.add('dragging'); }, 0);
    });
    el.addEventListener('dragend', () => {
      _dragId = null;
      document.querySelectorAll('.tt-row').forEach(r => r.classList.remove('dragging', 'tt-drop-above', 'tt-drop-below', 'tt-drop-child'));
    });
  });

  document.querySelectorAll('.tt-row').forEach(row => {
    row.addEventListener('dragover', (e) => {
      e.preventDefault();
      if (!_dragId || row.dataset.id === _dragId) return;
      row.classList.remove('tt-drop-above', 'tt-drop-below', 'tt-drop-child');
      const rect = row.getBoundingClientRect();
      const y = e.clientY - rect.top;
      const pct = y / rect.height;
      const type = row.dataset.type;
      if (pct < 0.25) row.classList.add('tt-drop-above');
      else if (pct > 0.75) row.classList.add('tt-drop-below');
      else if (type === 'epic' || type === 'feature') row.classList.add('tt-drop-child');
      else row.classList.add('tt-drop-below');
    });
    row.addEventListener('dragleave', () => {
      row.classList.remove('tt-drop-above', 'tt-drop-below', 'tt-drop-child');
    });
    row.addEventListener('drop', (e) => {
      e.preventDefault();
      if (!_dragId || row.dataset.id === _dragId) return;
      const targetId = row.dataset.id;
      const rect = row.getBoundingClientRect();
      const y = e.clientY - rect.top;
      const pct = y / rect.height;
      const type = row.dataset.type;
      let position;
      if (pct < 0.25) position = 'above';
      else if (pct > 0.75) position = 'below';
      else if (type === 'epic' || type === 'feature') position = 'child';
      else position = 'below';
      _handleDrop(_dragId, targetId, position);
    });
  });
}

function _startEdit(id, field, el) {
  if (_editing && _editing.id === id && _editing.field === field) return;
  const task = _tasks.find(t => t.id === id);
  if (!task) return;

  _editing = { id, field };

  if (field === 'title') {
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'tt-input';
    input.value = task.title;
    el.innerHTML = '';
    el.appendChild(input);
    input.focus();
    input.select();
    input.addEventListener('blur', () => _commitEdit(id, field, input.value));
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') input.blur();
      if (e.key === 'Escape') { _editing = null; _drawTable(); }
    });
  } else if (field === 'type') {
    const select = document.createElement('select');
    select.className = 'tt-select';
    TYPES.forEach(t => { const o = document.createElement('option'); o.value = t; o.textContent = t; if (t === (task.type || 'task')) o.selected = true; select.appendChild(o); });
    el.innerHTML = '';
    el.appendChild(select);
    select.focus();
    select.addEventListener('change', () => _commitEdit(id, field, select.value));
    select.addEventListener('blur', () => { _editing = null; _drawTable(); });
  } else if (field === 'priority') {
    const select = document.createElement('select');
    select.className = 'tt-select';
    PRIORITIES.forEach(p => { const o = document.createElement('option'); o.value = p; o.textContent = p; if (p === task.priority) o.selected = true; select.appendChild(o); });
    el.innerHTML = '';
    el.appendChild(select);
    select.focus();
    select.addEventListener('change', () => _commitEdit(id, field, select.value));
    select.addEventListener('blur', () => { _editing = null; _drawTable(); });
  } else if (field === 'status') {
    const select = document.createElement('select');
    select.className = 'tt-select';
    STATUSES.forEach(s => { const o = document.createElement('option'); o.value = s; o.textContent = s.replace(/_/g, ' '); if (s === task.status) o.selected = true; select.appendChild(o); });
    el.innerHTML = '';
    el.appendChild(select);
    select.focus();
    select.addEventListener('change', () => _commitEdit(id, 'status', select.value));
    select.addEventListener('blur', () => { _editing = null; _drawTable(); });
  } else if (field === 'estimate_hours') {
    const input = document.createElement('input');
    input.type = 'number';
    input.className = 'tt-input';
    input.value = task.estimate_hours || '';
    input.step = '0.5';
    input.min = '0';
    el.innerHTML = '';
    el.appendChild(input);
    input.focus();
    input.select();
    input.addEventListener('blur', () => _commitEdit(id, field, input.value));
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') input.blur();
      if (e.key === 'Escape') { _editing = null; _drawTable(); }
    });
  } else if (field === 'due_date') {
    const input = document.createElement('input');
    input.type = 'date';
    input.className = 'tt-input';
    input.value = task.due_date || '';
    el.innerHTML = '';
    el.appendChild(input);
    input.focus();
    input.addEventListener('change', () => _commitEdit(id, field, input.value));
    input.addEventListener('blur', () => { _editing = null; _drawTable(); });
  }
}

async function _commitEdit(id, field, value) {
  _editing = null;
  const task = _tasks.find(t => t.id === id);
  if (!task) return;

  const oldValue = field === 'status' ? task.status : (task[field] ?? '');
  if (String(value) === String(oldValue)) { _drawTable(); return; }

  try {
    let body = {};
    if (field === 'status') {
      body.status = value;
    } else if (field === 'estimate_hours') {
      body.estimate_hours = parseFloat(value) || 0;
    } else {
      body[field] = value;
    }
    const updated = await api.patch(`/tasks/${id}`, body);
    const idx = _tasks.findIndex(t => t.id === id);
    if (idx >= 0) _tasks[idx] = updated;
  } catch (err) {
    alert('Update failed: ' + err.message);
  }
  _drawTable();
}

async function _createTask(parentId, type) {
  try {
    const body = { title: 'New ' + type, type: type, priority: 'medium' };
    if (parentId) body.parent_id = parentId;
    const created = await api.post(`/projects/${_prefix}/tasks`, body);
    _tasks.push(created);
    if (parentId && _collapsed[parentId]) _collapsed[parentId] = false;
    _drawTable();
    // Enter edit mode on the new task's title
    setTimeout(() => {
      const el = document.querySelector(`[data-edit="title"][data-id="${created.id}"]`);
      if (el) _startEdit(created.id, 'title', el);
    }, 50);
  } catch (err) {
    alert('Create failed: ' + err.message);
  }
}

async function _deleteTask(id) {
  const task = _tasks.find(t => t.id === id);
  const displayId = task ? `${_prefix}-${task.seq}` : id.slice(-8);
  if (!confirm(`Delete ${displayId}? This cannot be undone.`)) return;
  try {
    await api.del(`/tasks/${id}`);
    _tasks = _tasks.filter(t => t.id !== id);
    _selected.delete(id);
    _drawTable();
  } catch (err) {
    alert('Delete failed: ' + err.message);
  }
}

async function _handleDrop(dragId, targetId, position) {
  const target = _tasks.find(t => t.id === targetId);
  if (!target) return;

  try {
    if (position === 'child') {
      await api.patch(`/tasks/${dragId}`, { parent_id: targetId });
      const idx = _tasks.findIndex(t => t.id === dragId);
      if (idx >= 0) _tasks[idx].parent_id = targetId;
    } else {
      // Reorder: set parent to target's parent, adjust sort_order
      const newParent = target.parent_id || '';
      await api.patch(`/tasks/${dragId}`, { parent_id: newParent });
      const idx = _tasks.findIndex(t => t.id === dragId);
      if (idx >= 0) _tasks[idx].parent_id = newParent;
    }
  } catch (err) {
    alert('Move failed: ' + err.message);
  }
  _drawTable();
}

function _updateBulkBar() {
  const bar = document.getElementById('tt-bulk-bar');
  if (bar) {
    bar.classList.toggle('visible', _selected.size > 0);
    const count = bar.querySelector('.tt-bulk-count');
    if (count) count.textContent = _selected.size + ' selected';
  }
}

async function _bulkAction(field, value) {
  const ids = [..._selected];
  for (const id of ids) {
    try {
      if (field === 'status') {
        await api.patch(`/tasks/${id}`, { status: value });
        const idx = _tasks.findIndex(t => t.id === id);
        if (idx >= 0) _tasks[idx].status = value;
      } else {
        await api.patch(`/tasks/${id}`, { [field]: value });
        const idx = _tasks.findIndex(t => t.id === id);
        if (idx >= 0) _tasks[idx][field] = value;
      }
    } catch (err) { /* continue with others */ }
  }
  _selected.clear();
  _drawTable();
}

async function _bulkDelete() {
  if (!confirm(`Delete ${_selected.size} tasks? This cannot be undone.`)) return;
  const ids = [..._selected];
  for (const id of ids) {
    try {
      await api.del(`/tasks/${id}`);
      _tasks = _tasks.filter(t => t.id !== id);
    } catch (err) { /* continue */ }
  }
  _selected.clear();
  _drawTable();
}

// --- Detail Modal ---

function _openDetail(id) {
  const task = _tasks.find(t => t.id === id);
  if (!task) return;
  // Use the shared task modal (same editor as the board/focus/timeline). It
  // includes the Parent dropdown, so the tree can re-parent from here too.
  const refresh = async () => {
    _tasks = await api.get(`/projects/${_prefix}/tasks`);
    _drawTable();
  };
  openTaskModal(task, {
    prefix: _prefix,
    allTasks: _tasks,
    onSaved: refresh,
    onDeleted: () => { _selected.delete(id); refresh(); },
  });
}

return renderTree;
})();
