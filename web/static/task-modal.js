// task-modal.js — shared editable task-detail modal, reused by the board and focus views.
// Depends on globals: api, escHtml, fmtDate.
// Exposes: openTaskModal(task, { prefix, allTasks, onSaved, onDeleted })
//   - prefix:    project prefix for display IDs (e.g. "WEB")
//   - allTasks:  array of the project's tasks (for the parent dropdown + dep labels)
//   - onSaved:   called after a successful save (caller re-fetches/redraws)
//   - onDeleted: called with the task id after a successful delete
// Renders an overlay into document.body so it is independent of any view's render loop.

var openTaskModal = (function () {
  'use strict';

  const TYPES = ['epic', 'feature', 'task'];
  const PRIORITIES = ['urgent', 'high', 'medium', 'low'];
  const STATUSES = ['todo', 'in_progress', 'done', 'blocked', 'waiting_external', 'waiting_decision', 'waiting_feedback'];
  const SIZES = ['', 'XS', 'S', 'M', 'L', 'XL'];

  let _overlay = null;
  let _task = null;
  let _opts = null;

  function _close() {
    document.removeEventListener('keydown', _onKey);
    if (_overlay) { _overlay.remove(); _overlay = null; }
    _task = null;
    _opts = null;
  }

  function _onKey(e) {
    if (e.key === 'Escape') _close();
  }

  // IDs of a task's own subtree (the task itself excluded), so the parent
  // dropdown never offers a descendant — which would create a cycle.
  function _descendantIds(rootId, all) {
    const kids = {};
    all.forEach(x => { if (x.parent_id) (kids[x.parent_id] = kids[x.parent_id] || []).push(x.id); });
    const out = new Set();
    const stack = [rootId];
    while (stack.length) {
      const id = stack.pop();
      (kids[id] || []).forEach(c => { if (!out.has(c)) { out.add(c); stack.push(c); } });
    }
    return out;
  }

  function _render() {
    const prefix = _opts.prefix || '';
    const allTasks = _opts.allTasks || [];
    const t = _task;
    const displayId = `${prefix}-${t.seq}`;

    // A task can be parented under an epic, feature, OR another task (subtask
    // decomposition). Exclude the task itself and its descendants to avoid cycles.
    const blockedParents = _descendantIds(t.id, allTasks);
    const typeOrder = { epic: 0, feature: 1, task: 2 };
    const parentOptions = allTasks
      .filter(x => x.id !== t.id && !blockedParents.has(x.id))
      .sort((a, b) => {
        const ta = typeOrder[a.type || 'task'] ?? 2;
        const tb = typeOrder[b.type || 'task'] ?? 2;
        if (ta !== tb) return ta - tb;
        return (a.seq || 0) - (b.seq || 0);
      })
      .map(x => `<option value="${x.id}" ${t.parent_id === x.id ? 'selected' : ''}>${prefix}-${x.seq} · ${x.type || 'task'} · ${escHtml(x.title.length > 40 ? x.title.slice(0, 37) + '...' : x.title)}</option>`)
      .join('');

    // Parent → grandparent breadcrumb for context.
    let breadcrumb = '';
    if (t.parent_id) {
      const parent = allTasks.find(x => x.id === t.parent_id);
      if (parent) {
        breadcrumb = escHtml(parent.title);
        if (parent.parent_id) {
          const gp = allTasks.find(x => x.id === parent.parent_id);
          if (gp) breadcrumb = escHtml(gp.title) + ' → ' + breadcrumb;
        }
      }
    }

    _overlay = document.createElement('div');
    _overlay.className = 'tt-modal-overlay';
    _overlay.id = 'detail-backdrop';
    _overlay.innerHTML = `
      <div class="tt-modal" role="dialog" aria-label="Task detail">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px;">
          <span style="font-family:var(--font-mono);font-size:12px;color:var(--muted);">${escHtml(displayId)}</span>
          <button class="modal-close" id="detail-close" aria-label="Close">&times;</button>
        </div>
        ${breadcrumb ? `<div style="font-size:11px;color:var(--muted);margin:-8px 0 12px;">\u{1F4C2} ${breadcrumb}</div>` : ''}
        <div class="tt-modal-field">
          <label class="tt-modal-label">Title</label>
          <input class="tt-modal-input" id="detail-title" value="${escHtml(t.title)}">
        </div>
        <div class="tt-modal-field">
          <label class="tt-modal-label">Description</label>
          <textarea class="tt-modal-textarea" id="detail-description" rows="8">${escHtml(t.description || '')}</textarea>
        </div>
        <div class="tt-modal-row">
          <div class="tt-modal-field">
            <label class="tt-modal-label">Type</label>
            <select class="tt-modal-input" id="detail-type">${TYPES.map(x => `<option value="${x}" ${(t.type || 'task') === x ? 'selected' : ''}>${x}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Priority</label>
            <select class="tt-modal-input" id="detail-priority">${PRIORITIES.map(x => `<option value="${x}" ${t.priority === x ? 'selected' : ''}>${x}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Status</label>
            <select class="tt-modal-input" id="detail-status">${STATUSES.map(x => `<option value="${x}" ${t.status === x ? 'selected' : ''}>${x.replace(/_/g, ' ')}</option>`).join('')}</select>
          </div>
        </div>
        <div class="tt-modal-row">
          <div class="tt-modal-field">
            <label class="tt-modal-label">Estimate Size</label>
            <select class="tt-modal-input" id="detail-estimate-size">${SIZES.map(x => `<option value="${x}" ${(t.estimate_size || '') === x ? 'selected' : ''}>${x || '—'}</option>`).join('')}</select>
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Hours</label>
            <input class="tt-modal-input" id="detail-hours" type="number" step="0.25" value="${t.estimate_hours || ''}">
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Agent min</label>
            <input class="tt-modal-input" id="detail-agent-min" type="number" min="0" value="${t.estimate_agent_minutes || ''}">
          </div>
        </div>
        <div class="tt-modal-row">
          <div class="tt-modal-field">
            <label class="tt-modal-label">Start Date</label>
            <input class="tt-modal-input" id="detail-start" type="date" value="${t.start_date || ''}">
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Due Date</label>
            <input class="tt-modal-input" id="detail-due" type="date" value="${t.due_date || ''}">
          </div>
          <div class="tt-modal-field">
            <label class="tt-modal-label">Tags</label>
            <input class="tt-modal-input" id="detail-tags" value="${escHtml(t.tags === '[]' ? '' : (t.tags || ''))}">
          </div>
        </div>
        <div class="tt-modal-field">
          <label class="tt-modal-label">Parent</label>
          <select class="tt-modal-input" id="detail-parent">
            <option value="">(none)</option>
            ${parentOptions}
          </select>
        </div>
        <div class="stat-row mt-8"><span class="stat-label">Source</span><span class="stat-value">${escHtml(t.source_type || '—')}</span></div>
        <div class="stat-row"><span class="stat-label">Created</span><span class="stat-value">${fmtDate(t.created_at)}</span></div>
        <div class="stat-row"><span class="stat-label">Updated</span><span class="stat-value">${fmtDate(t.updated_at)}</span></div>
        <div id="detail-deps-container" class="mt-16"><p class="text-muted">Loading deps…</p></div>
        <div class="tt-modal-actions">
          <button class="tt-modal-btn danger" id="detail-delete">Delete</button>
          <div style="display:flex;gap:8px;">
            <button class="tt-modal-btn" id="detail-cancel">Cancel</button>
            <button class="tt-modal-btn primary" id="detail-save">Save</button>
          </div>
        </div>
      </div>`;

    document.body.appendChild(_overlay);

    _overlay.addEventListener('click', e => { if (e.target === _overlay) _close(); });
    _overlay.querySelector('#detail-close').addEventListener('click', _close);
    _overlay.querySelector('#detail-cancel').addEventListener('click', _close);
    _overlay.querySelector('#detail-save').addEventListener('click', _save);
    _overlay.querySelector('#detail-delete').addEventListener('click', _delete);
    document.addEventListener('keydown', _onKey);
  }

  function _loadDeps() {
    const t = _task;
    const prefix = _opts.prefix || '';
    const allTasks = _opts.allTasks || [];
    api.get(`/tasks/${t.id}/deps`).then(deps => {
      if (!_overlay || !_task || _task.id !== t.id) return;
      const container = _overlay.querySelector('#detail-deps-container');
      if (!container) return;
      if (deps.length > 0) {
        const rows = deps.map(d => {
          const dep = allTasks.find(x => x.id === d.to_task_id);
          const label = dep
            ? `${prefix}-${dep.seq}: ${escHtml(dep.title.length > 40 ? dep.title.slice(0, 39) + '…' : dep.title)}`
            : escHtml(d.to_task_id.slice(-8));
          return `<li class="stat-row"><span class="stat-label">${escHtml(d.dep_type || 'blocks')}</span><span class="stat-value">${label}</span></li>`;
        }).join('');
        container.innerHTML = `<ul style="list-style:none;padding:0">${rows}</ul>`;
      } else {
        container.innerHTML = '<p class="text-muted">No dependencies.</p>';
      }
    }).catch(() => {});
  }

  function _val(id) {
    const el = _overlay.querySelector(`#${id}`);
    return el ? el.value : undefined;
  }

  async function _save() {
    if (!_task) return;
    const t = _task;
    const payload = {};
    const title = (_val('detail-title') || '').trim();
    const description = _val('detail-description');
    const type = _val('detail-type');
    const priority = _val('detail-priority');
    const status = _val('detail-status');
    const estimateSize = _val('detail-estimate-size');
    const hours = parseFloat(_val('detail-hours')) || 0;
    const agentMin = parseInt(_val('detail-agent-min'), 10) || 0;
    const start = _val('detail-start') || '';
    const due = _val('detail-due') || '';
    const tags = _val('detail-tags');
    const parentId = _val('detail-parent') || '';
    // Treat the default "[]" tags as empty so saving an untouched task doesn't churn it.
    const origTags = t.tags === '[]' ? '' : (t.tags || '');

    if (title && title !== t.title) payload.title = title;
    if (description !== (t.description || '')) payload.description = description;
    if (type !== (t.type || 'task')) payload.type = type;
    if (priority !== t.priority) payload.priority = priority;
    if (status !== t.status) payload.status = status;
    if (estimateSize !== (t.estimate_size || '')) payload.estimate_size = estimateSize;
    if (hours !== (t.estimate_hours || 0)) payload.estimate_hours = hours;
    if (agentMin !== (t.estimate_agent_minutes || 0)) payload.estimate_agent_minutes = agentMin;
    if (start !== (t.start_date || '')) payload.start_date = start;
    if (due !== (t.due_date || '')) payload.due_date = due;
    if (tags !== origTags) payload.tags = tags;
    if (parentId !== (t.parent_id || '')) payload.parent_id = parentId;

    if (Object.keys(payload).length === 0) { _close(); return; }

    const onSaved = _opts.onSaved;
    try {
      await api.patch(`/tasks/${t.id}`, payload);
      _close();
      if (onSaved) onSaved();
    } catch (err) {
      alert('Save failed: ' + (err.message || err));
    }
  }

  async function _delete() {
    if (!_task) return;
    const t = _task;
    const displayId = `${_opts.prefix || ''}-${t.seq}`;
    if (!confirm(`Delete ${displayId}?`)) return;
    const onDeleted = _opts.onDeleted;
    try {
      await api.del(`/tasks/${t.id}`);
      _close();
      if (onDeleted) onDeleted(t.id);
    } catch (err) {
      alert('Delete failed: ' + (err.message || err));
    }
  }

  return function openTaskModal(task, opts) {
    if (!task) return;
    if (_overlay) _close();
    _task = task;
    _opts = opts || {};
    _render();
    _loadDeps();
  };
})();
