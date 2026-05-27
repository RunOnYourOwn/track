// tree.js — Nested card hierarchy view (epic → feature → task)
// Depends on globals: api, render, escHtml, priorityBadge, statusBadge
// Exposes: renderTree(prefix)

var renderTree = (function() {
'use strict';

let _prefix = '';
let _tasks = [];
let _collapsed = {};
let _showDone = false;

async function renderTree(prefix) {
  _prefix = prefix;
  render('<div class="loading"><div class="spinner"></div> Loading tree…</div>');

  try {
    _tasks = await api.get(`/projects/${prefix}/tasks`);
  } catch (err) {
    render(`<div class="alert alert-danger">Failed to load tasks: ${escHtml(err.message)}</div>`);
    return;
  }

  _drawTree();
}

function _drawTree() {
  const tree = _buildHierarchy();
  const doneCount = _tasks.filter(t => t.status === 'done').length;
  const html = `
    <div class="tree-header">
      <div class="tree-title">Hierarchy</div>
      <div class="tree-actions">
        ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="tree-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
        <button class="btn-ghost btn-sm" id="tree-expand-all">Expand all</button>
        <button class="btn-ghost btn-sm" id="tree-collapse-all">Collapse all</button>
      </div>
    </div>
    <div class="tree-root">
      ${tree.length === 0 ? '<div class="empty-state"><div class="empty-state-title">No active tasks</div></div>' : ''}
      ${tree.map(node => _renderCard(node)).join('')}
    </div>
  `;
  render(html);
  _attachListeners();
}

function _buildHierarchy() {
  const visibleTasks = _showDone ? _tasks : _tasks.filter(t => t.status !== 'done');
  const byId = {};
  visibleTasks.forEach(t => { byId[t.id] = { ...t, children: [] }; });

  const roots = [];
  visibleTasks.forEach(t => {
    const node = byId[t.id];
    if (t.parent_id && byId[t.parent_id]) {
      byId[t.parent_id].children.push(node);
    } else {
      roots.push(node);
    }
  });

  const typeOrder = { epic: 0, feature: 1, task: 2 };
  const statusOrder = { in_progress: 0, todo: 1, waiting_external: 2, waiting_decision: 2, waiting_feedback: 2, done: 3 };
  const priOrder = { urgent: 0, high: 1, medium: 2, low: 3 };
  const sortFn = (a, b) => {
    const ta = typeOrder[(a.type || 'task')] ?? 2;
    const tb = typeOrder[(b.type || 'task')] ?? 2;
    if (ta !== tb) return ta - tb;
    const sa = statusOrder[a.status] ?? 2;
    const sb = statusOrder[b.status] ?? 2;
    if (sa !== sb) return sa - sb;
    const pa = priOrder[a.priority] ?? 2;
    const pb = priOrder[b.priority] ?? 2;
    if (pa !== pb) return pa - pb;
    return a.seq - b.seq;
  };

  const sortTree = (nodes) => {
    nodes.sort(sortFn);
    nodes.forEach(n => sortTree(n.children));
  };
  sortTree(roots);
  return roots;
}

function _renderCard(node) {
  const hasChildren = node.children.length > 0;
  const isCollapsed = _collapsed[node.id];
  const typeLabel = node.type || 'task';
  const displayId = `${_prefix}-${node.seq}`;

  if (typeLabel === 'epic') return _renderEpicCard(node, hasChildren, isCollapsed, displayId);
  if (typeLabel === 'feature') return _renderFeatureCard(node, hasChildren, isCollapsed, displayId);
  return _renderTaskRow(node, displayId);
}

function _progressBar(children) {
  if (children.length === 0) return '';
  const done = children.filter(c => c.status === 'done').length;
  const pct = Math.round((done / children.length) * 100);
  return `
    <div class="tree-progress">
      <div class="tree-progress-bar">
        <div class="tree-progress-fill" style="width:${pct}%"></div>
      </div>
      <span class="tree-progress-label">${done}/${children.length}</span>
    </div>
  `;
}

function _agentMinTotal(node) {
  let total = node.estimate_agent_minutes || 0;
  (node.children || []).forEach(c => { total += _agentMinTotal(c); });
  return total;
}

function _renderEpicCard(node, hasChildren, isCollapsed, displayId) {
  const totalMin = _agentMinTotal(node);
  const allDescendants = _flatDescendants(node);
  const doneCount = allDescendants.filter(c => c.status === 'done').length;
  const totalCount = allDescendants.length;
  const pct = totalCount > 0 ? Math.round((doneCount / totalCount) * 100) : 0;

  return `
    <div class="tree-epic-card">
      <div class="tree-epic-header" data-toggle-id="${node.id}">
        <div class="tree-epic-left">
          <span class="tree-toggle ${isCollapsed ? 'collapsed' : ''}">${hasChildren ? '▾' : ''}</span>
          <span class="tree-epic-icon">◆</span>
          <span class="tree-card-id">${displayId}</span>
          <span class="tree-epic-title">${escHtml(node.title)}</span>
        </div>
        <div class="tree-epic-right">
          ${totalMin ? `<span class="tree-min-badge">${totalMin}m</span>` : ''}
          ${statusBadge(node.status)}
          ${priorityBadge(node.priority)}
        </div>
      </div>
      <div class="tree-epic-progress">
        <div class="tree-progress-bar wide">
          <div class="tree-progress-fill" style="width:${pct}%"></div>
        </div>
        <span class="tree-progress-label">${doneCount}/${totalCount} tasks (${pct}%)</span>
      </div>
      ${hasChildren && !isCollapsed ? `
        <div class="tree-epic-children">
          ${node.children.map(c => _renderCard(c)).join('')}
        </div>
      ` : ''}
    </div>
  `;
}

function _renderFeatureCard(node, hasChildren, isCollapsed, displayId) {
  const totalMin = _agentMinTotal(node);

  return `
    <div class="tree-feature-card">
      <div class="tree-feature-header" data-toggle-id="${node.id}">
        <div class="tree-feature-left">
          <span class="tree-toggle ${isCollapsed ? 'collapsed' : ''}">${hasChildren ? '▾' : ''}</span>
          <span class="tree-feature-icon">◇</span>
          <span class="tree-card-id">${displayId}</span>
          <span class="tree-feature-title">${escHtml(node.title)}</span>
        </div>
        <div class="tree-feature-right">
          ${totalMin ? `<span class="tree-min-badge">${totalMin}m</span>` : ''}
          ${statusBadge(node.status)}
          ${priorityBadge(node.priority)}
        </div>
      </div>
      ${hasChildren ? _progressBar(node.children) : ''}
      ${hasChildren && !isCollapsed ? `
        <div class="tree-feature-children">
          ${node.children.map(c => _renderTaskRow(c, `${_prefix}-${c.seq}`)).join('')}
        </div>
      ` : ''}
    </div>
  `;
}

function _renderTaskRow(node, displayId) {
  const isDone = node.status === 'done';
  const agentMin = node.estimate_agent_minutes
    ? `<span class="tree-min-badge small">${node.estimate_agent_minutes}m</span>`
    : '';

  return `
    <div class="tree-task-row ${isDone ? 'done' : ''}">
      <span class="tree-task-check">${isDone ? '✓' : '○'}</span>
      <span class="tree-card-id">${displayId}</span>
      <span class="tree-task-title">${escHtml(node.title)}</span>
      <div class="tree-task-meta">
        ${agentMin}
        ${statusBadge(node.status)}
        ${priorityBadge(node.priority)}
      </div>
    </div>
  `;
}

function _flatDescendants(node) {
  let result = [];
  (node.children || []).forEach(c => {
    result.push(c);
    result = result.concat(_flatDescendants(c));
  });
  return result;
}

function _attachListeners() {
  const showDoneEl = document.getElementById('tree-show-done');
  if (showDoneEl) {
    showDoneEl.addEventListener('change', () => {
      _showDone = showDoneEl.checked;
      _drawTree();
    });
  }

  document.querySelectorAll('[data-toggle-id]').forEach(el => {
    el.addEventListener('click', () => {
      const id = el.dataset.toggleId;
      _collapsed[id] = !_collapsed[id];
      _drawTree();
    });
  });

  const expandBtn = document.getElementById('tree-expand-all');
  const collapseBtn = document.getElementById('tree-collapse-all');

  if (expandBtn) {
    expandBtn.addEventListener('click', () => {
      _collapsed = {};
      _drawTree();
    });
  }
  if (collapseBtn) {
    collapseBtn.addEventListener('click', () => {
      _tasks.forEach(t => {
        const hasChildren = _tasks.some(c => c.parent_id === t.id);
        if (hasChildren) _collapsed[t.id] = true;
      });
      _drawTree();
    });
  }
}

return renderTree;
})();
