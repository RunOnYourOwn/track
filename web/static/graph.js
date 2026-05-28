// graph.js — DAG dependency graph with layered layout
// Requires: d3 (v7) global, api global, render global, escHtml global

var renderGraph = (function() {
'use strict';

let _prefix = '';
let _allTasks = [];
let _allDeps = [];
let _showDone = false;

async function renderGraph(prefix) {
  _prefix = prefix;
  render(`
    <div class="page-graph">
      <div id="graph-container" style="position:relative;">
        <div id="graph-loading" class="loading">
          <div class="spinner"></div> Loading dependency graph…
        </div>
      </div>
    </div>
  `);

  try {
    _allTasks = await api.get(`/projects/${prefix}/tasks`);
  } catch (e) {
    document.getElementById('graph-container').innerHTML =
      `<div class="empty-state">Failed to load tasks: ${escHtml((e).message)}</div>`;
    return;
  }

  if (_allTasks.length === 0) {
    document.getElementById('graph-container').innerHTML =
      `<div class="empty-state">No tasks found for this project.</div>`;
    return;
  }

  const depResults = await Promise.allSettled(
    _allTasks.map(t => api.get(`/tasks/${t.id}/deps`))
  );
  _allDeps = [];
  depResults.forEach(r => {
    if (r.status === 'fulfilled' && Array.isArray(r.value)) {
      _allDeps.push(...r.value);
    }
  });

  _drawGraph();
}

function _drawGraph() {
  const prefix = _prefix;
  const tasks = _showDone ? _allTasks : _allTasks.filter(t => t.status !== 'done');

  if (tasks.length === 0) {
    const doneCount = _allTasks.filter(t => t.status === 'done').length;
    render(`<div class="page-graph">
      <div class="timeline-toolbar">
        ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="graph-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
      </div>
      <div id="graph-container"><div class="empty-state">No active tasks with dependencies.</div></div>
    </div>`);
    const showDoneEl = document.getElementById('graph-show-done');
    if (showDoneEl) showDoneEl.addEventListener('change', () => { _showDone = showDoneEl.checked; _drawGraph(); });
    return;
  }

  const edgeKey = d => `${d.from_task_id}→${d.to_task_id}`;
  const seen = new Set();
  const edges = _allDeps.filter(d => {
    const k = edgeKey(d);
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });

  const taskById = new Map(tasks.map(t => [t.id, t]));
  const validEdges = edges.filter(
    d => taskById.has(d.from_task_id) && taskById.has(d.to_task_id)
  );

  const doneCount = _allTasks.filter(t => t.status === 'done').length;

  if (validEdges.length === 0) {
    render(`<div class="page-graph">
      <div class="timeline-toolbar">
        ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="graph-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
      </div>
      <div id="graph-container"><div class="empty-state">
        No active tasks with dependencies.<br>
        Link tasks with <code>track task link</code> to see the graph.
      </div></div>
    </div>`);
    const showDoneEl = document.getElementById('graph-show-done');
    if (showDoneEl) showDoneEl.addEventListener('change', () => { _showDone = showDoneEl.checked; _drawGraph(); });
    return;
  }

  // Only show connected tasks (nodes that appear in at least one edge)
  const connectedIds = new Set();
  validEdges.forEach(e => { connectedIds.add(e.from_task_id); connectedIds.add(e.to_task_id); });
  const connectedTasks = tasks.filter(t => connectedIds.has(t.id));

  // Build adjacency for topological layering
  const adj = new Map();      // id -> [successor ids]
  const inDeg = new Map();    // id -> number of incoming edges
  connectedTasks.forEach(t => { adj.set(t.id, []); inDeg.set(t.id, 0); });
  validEdges.forEach(e => {
    adj.get(e.from_task_id).push(e.to_task_id);
    inDeg.set(e.to_task_id, (inDeg.get(e.to_task_id) || 0) + 1);
  });

  // Assign layers via longest-path (gives better spread than simple topo sort)
  const layer = new Map();
  const visiting = new Set();
  function longestPath(id) {
    if (layer.has(id)) return layer.get(id);
    if (visiting.has(id)) { layer.set(id, 0); return 0; }
    visiting.add(id);
    const preds = validEdges.filter(e => e.to_task_id === id).map(e => e.from_task_id);
    if (preds.length === 0) { layer.set(id, 0); visiting.delete(id); return 0; }
    const maxPred = Math.max(...preds.map(p => longestPath(p)));
    layer.set(id, maxPred + 1);
    visiting.delete(id);
    return maxPred + 1;
  }
  connectedTasks.forEach(t => longestPath(t.id));

  // Group tasks by parent feature for swimlane ordering within each layer
  const featureOrder = {};
  tasks.filter(t => (t.type || 'task') === 'feature').forEach((f, i) => { featureOrder[f.id] = i; });

  // Sort nodes within each layer by: feature group, then seq
  const maxLayer = Math.max(...Array.from(layer.values()));
  const layerNodes = [];
  for (let l = 0; l <= maxLayer; l++) {
    const nodesInLayer = connectedTasks
      .filter(t => layer.get(t.id) === l)
      .sort((a, b) => {
        const fa = featureOrder[a.parent_id] ?? 99;
        const fb = featureOrder[b.parent_id] ?? 99;
        if (fa !== fb) return fa - fb;
        return (a.seq || 0) - (b.seq || 0);
      });
    layerNodes.push(nodesInLayer);
  }

  // Layout parameters
  const NODE_W = 220;
  const NODE_H = 58;
  const LAYER_GAP = 80;
  const NODE_GAP = 14;
  const PAD_X = 40;
  const PAD_Y = 40;

  // Compute positions — center each layer's nodes vertically
  const pos = new Map();
  const maxNodesInAnyLayer = Math.max(...layerNodes.map(l => l.length));
  const naturalHeight = maxNodesInAnyLayer * (NODE_H + NODE_GAP) - NODE_GAP + PAD_Y * 2;
  let totalHeight = Math.max(naturalHeight, 500);

  layerNodes.forEach((nodesInLayer, l) => {
    const x = PAD_X + l * (NODE_W + LAYER_GAP);
    const layerH = nodesInLayer.length * (NODE_H + NODE_GAP) - NODE_GAP;
    const startY = Math.max(PAD_Y, (totalHeight - layerH) / 2);
    nodesInLayer.forEach((t, i) => {
      pos.set(t.id, { x, y: startY + i * (NODE_H + NODE_GAP) });
    });
  });

  const totalWidth = PAD_X * 2 + (maxLayer + 1) * (NODE_W + LAYER_GAP) - LAYER_GAP;

  // Status colors
  const STATUS = {
    todo:              { bg: '#1c1c1c', border: '#484f58', text: '#8b949e', label: 'To Do' },
    in_progress:       { bg: '#0d2240', border: '#58a6ff', text: '#58a6ff', label: 'In Progress' },
    done:              { bg: '#0d2d1a', border: '#3fb950', text: '#3fb950', label: 'Done' },
    waiting_external:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
    waiting_decision:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
    waiting_feedback:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
  };

  const PRIORITY_COLORS = { urgent: '#f85149', high: '#d29922', medium: '#58a6ff', low: '#484f58' };

  // Render with toolbar
  render(`<div class="page-graph">
    <div class="timeline-toolbar">
      ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="graph-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
    </div>
    <div id="graph-container" style="position:relative;"></div>
  </div>`);

  const showDoneEl = document.getElementById('graph-show-done');
  if (showDoneEl) showDoneEl.addEventListener('change', () => { _showDone = showDoneEl.checked; _drawGraph(); });

  const container = document.getElementById('graph-container');

  const containerRect = container.getBoundingClientRect();
  const svgW = Math.max(totalWidth, containerRect.width || 900);
  const svgH = Math.max(totalHeight, containerRect.height || 500);

  const svg = d3.select(container)
    .append('svg')
    .attr('width', svgW)
    .attr('height', svgH)
    .style('font-family', 'var(--font-mono)')
    .style('display', 'block')
    .on('click', () => _closeGraphDetail());

  // Column backgrounds — aligned to actual node positions
  layerNodes.forEach((nodesInLayer, l) => {
    if (nodesInLayer.length === 0) return;
    const x = PAD_X + l * (NODE_W + LAYER_GAP);
    const firstPos = pos.get(nodesInLayer[0].id);
    const lastPos = pos.get(nodesInLayer[nodesInLayer.length - 1].id);
    const minY = firstPos.y - 10;
    const maxY = lastPos.y + NODE_H + 10;
    svg.append('rect')
      .attr('x', x - 10)
      .attr('y', minY)
      .attr('width', NODE_W + 20)
      .attr('height', maxY - minY)
      .attr('rx', 6)
      .attr('fill', 'rgba(255,255,255,0.02)');
  });

  // Compute critical path (longest chain through the DAG)
  const criticalPath = new Set();
  (function computeCriticalPath() {
    // Find the node with highest layer value (deepest) — that's the end of the critical path
    let deepestId = null;
    let deepestLayer = -1;
    connectedTasks.forEach(t => {
      const l = layer.get(t.id);
      if (l > deepestLayer) { deepestLayer = l; deepestId = t.id; }
    });
    if (!deepestId) return;

    // Walk backward from deepest node, always choosing the predecessor with the highest layer
    criticalPath.add(deepestId);
    let current = deepestId;
    while (true) {
      const preds = validEdges.filter(e => e.to_task_id === current).map(e => e.from_task_id);
      if (preds.length === 0) break;
      let bestPred = preds[0];
      let bestLayer = layer.get(bestPred) || 0;
      preds.forEach(p => {
        const pl = layer.get(p) || 0;
        if (pl > bestLayer) { bestLayer = pl; bestPred = p; }
      });
      criticalPath.add(bestPred);
      current = bestPred;
    }
  })();

  // Draw edges (curved paths)
  const edgeG = svg.append('g');

  // Arrow markers
  const defs = svg.append('defs');
  defs.append('marker')
    .attr('id', 'dag-arrow')
    .attr('viewBox', '0 -4 8 8')
    .attr('refX', 8)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M0,-4L8,0L0,4')
    .attr('fill', '#30363d');

  defs.append('marker')
    .attr('id', 'dag-arrow-crit')
    .attr('viewBox', '0 -4 8 8')
    .attr('refX', 8)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M0,-4L8,0L0,4')
    .attr('fill', '#f85149');

  validEdges.forEach(e => {
    const from = pos.get(e.from_task_id);
    const to = pos.get(e.to_task_id);
    if (!from || !to) return;

    const x1 = from.x + NODE_W;
    const y1 = from.y + NODE_H / 2;
    const x2 = to.x;
    const y2 = to.y + NODE_H / 2;

    const midX = (x1 + x2) / 2;
    const path = `M${x1},${y1} C${midX},${y1} ${midX},${y2} ${x2},${y2}`;
    const isCrit = criticalPath.has(e.from_task_id) && criticalPath.has(e.to_task_id);

    edgeG.append('path')
      .attr('d', path)
      .attr('fill', 'none')
      .attr('stroke', isCrit ? '#f85149' : '#30363d')
      .attr('stroke-width', isCrit ? 2.5 : 1.5)
      .attr('marker-end', isCrit ? 'url(#dag-arrow-crit)' : 'url(#dag-arrow)')
      .attr('class', `edge-${e.from_task_id} edge-${e.to_task_id}`)
      .style('opacity', isCrit ? 0.9 : 1);
  });

  // Draw nodes
  const nodeG = svg.append('g');

  connectedTasks.forEach(t => {
    const p = pos.get(t.id);
    if (!p) return;
    const s = STATUS[t.status] || STATUS.todo;
    const g = nodeG.append('g')
      .attr('transform', `translate(${p.x},${p.y})`)
      .style('cursor', 'pointer')
      .attr('data-id', t.id);

    const isCritNode = criticalPath.has(t.id);

    // Node background
    g.append('rect')
      .attr('width', NODE_W)
      .attr('height', NODE_H)
      .attr('rx', 6)
      .attr('fill', s.bg)
      .attr('stroke', isCritNode ? '#f85149' : s.border)
      .attr('stroke-width', isCritNode ? 2 : 1.5);

    // Priority indicator (left edge)
    g.append('rect')
      .attr('x', 0)
      .attr('y', 8)
      .attr('width', 3)
      .attr('height', NODE_H - 16)
      .attr('rx', 1.5)
      .attr('fill', PRIORITY_COLORS[t.priority] || '#484f58');

    // Task ID
    g.append('text')
      .attr('x', 12)
      .attr('y', 18)
      .attr('fill', '#707070')
      .attr('font-size', '10px')
      .text(`${prefix}-${t.seq}`);

    // Status dot
    g.append('circle')
      .attr('cx', NODE_W - 14)
      .attr('cy', 14)
      .attr('r', 4)
      .attr('fill', s.text);

    // Title (truncated)
    const maxChars = 30;
    const title = t.title.length > maxChars ? t.title.slice(0, maxChars - 1) + '…' : t.title;
    g.append('text')
      .attr('x', 12)
      .attr('y', 34)
      .attr('fill', '#e0e0e0')
      .attr('font-size', '11px')
      .attr('font-weight', '500')
      .text(title);

    // Feature badge (parent name) — bottom of card
    if (t.parent_id && taskById.has(t.parent_id)) {
      const parent = taskById.get(t.parent_id);
      if (parent.type === 'feature') {
        g.append('text')
          .attr('x', 12)
          .attr('y', NODE_H - 8)
          .attr('fill', '#3d4450')
          .attr('font-size', '9px')
          .text(parent.title.length > 28 ? parent.title.slice(0, 27) + '…' : parent.title);
      }
    }

    // Hover highlight
    g.on('mouseenter', function() {
      _highlightConnected(t.id, validEdges, connectedTasks, nodeG, edgeG);
    })
    .on('mouseleave', function() {
      _resetHighlight(nodeG, edgeG);
    })
    .on('click', function(event) {
      event.stopPropagation();
      _openGraphDetail(t, prefix, validEdges, taskById);
    });
  });

  // Legend
  const legendY = Math.max(svgH - 36, totalHeight + 10);
  const legendG = svg.append('g').attr('transform', `translate(${PAD_X}, ${legendY})`);
  const legendItems = [
    { label: 'To Do', color: '#484f58' },
    { label: 'In Progress', color: '#58a6ff' },
    { label: 'Done', color: '#3fb950' },
    { label: 'Waiting', color: '#d29922' },
    { label: 'Critical Path', color: '#f85149' },
  ];
  legendItems.forEach((item, i) => {
    const lx = i * 120;
    legendG.append('circle').attr('cx', lx).attr('cy', 0).attr('r', 5).attr('fill', item.color);
    legendG.append('text')
      .attr('x', lx + 12).attr('y', 4)
      .attr('fill', '#8b949e').attr('font-size', '11px')
      .text(item.label);
  });
}

function _openGraphDetail(task, prefix, edges, taskById) {
  _closeGraphDetail();
  const displayId = `${prefix}-${task.seq}`;

  // Find deps
  const blockedBy = edges.filter(e => e.to_task_id === task.id).map(e => {
    const t = taskById.get(e.from_task_id);
    return t ? `${prefix}-${t.seq}: ${t.title}` : e.from_task_id.slice(-8);
  });
  const blocks = edges.filter(e => e.from_task_id === task.id).map(e => {
    const t = taskById.get(e.to_task_id);
    return t ? `${prefix}-${t.seq}: ${t.title}` : e.to_task_id.slice(-8);
  });

  // Find parent feature
  let parentLabel = '';
  if (task.parent_id && taskById.has(task.parent_id)) {
    const parent = taskById.get(task.parent_id);
    parentLabel = `${prefix}-${parent.seq}: ${parent.title}`;
  }

  const panel = document.createElement('div');
  panel.id = 'graph-detail-panel';
  panel.innerHTML = `
    <div class="graph-detail-header">
      <span class="graph-detail-id">${displayId}</span>
      <button class="graph-detail-close" aria-label="Close">&times;</button>
    </div>
    <h3 class="graph-detail-title">${escHtml(task.title)}</h3>
    <div class="graph-detail-badges">
      <span class="priority-badge ${task.priority}">${task.priority}</span>
      <span class="status-badge ${task.status}">${task.status.replace(/_/g, ' ')}</span>
      ${task.type && task.type !== 'task' ? `<span class="type-badge type-${task.type}">${task.type}</span>` : ''}
    </div>
    ${task.description ? `<p class="graph-detail-desc">${escHtml(task.description)}</p>` : ''}
    ${parentLabel ? `<div class="graph-detail-row"><span class="graph-detail-label">Feature</span><span class="graph-detail-value">${escHtml(parentLabel)}</span></div>` : ''}
    <div class="graph-detail-row"><span class="graph-detail-label">Source</span><span class="graph-detail-value">${escHtml(task.source_type || '—')}</span></div>
    <div class="graph-detail-row"><span class="graph-detail-label">Created</span><span class="graph-detail-value">${fmtDate(task.created_at)}</span></div>
    ${task.due_date ? `<div class="graph-detail-row"><span class="graph-detail-label">Due</span><span class="graph-detail-value">${fmtDate(task.due_date)}</span></div>` : ''}
    ${blockedBy.length > 0 ? `
      <div class="graph-detail-section">
        <span class="graph-detail-section-title">Blocked by (${blockedBy.length})</span>
        <ul class="graph-detail-dep-list">${blockedBy.map(d => `<li>${escHtml(d)}</li>`).join('')}</ul>
      </div>` : ''}
    ${blocks.length > 0 ? `
      <div class="graph-detail-section">
        <span class="graph-detail-section-title">Blocks (${blocks.length})</span>
        <ul class="graph-detail-dep-list">${blocks.map(d => `<li>${escHtml(d)}</li>`).join('')}</ul>
      </div>` : ''}
  `;

  document.getElementById('graph-container').appendChild(panel);
  panel.querySelector('.graph-detail-close').addEventListener('click', _closeGraphDetail);
}

function _closeGraphDetail() {
  const panel = document.getElementById('graph-detail-panel');
  if (panel) panel.remove();
}

function _highlightConnected(nodeId, edges, tasks, nodeG, edgeG) {
  const connected = new Set([nodeId]);
  // Walk upstream
  function walkUp(id) {
    edges.filter(e => e.to_task_id === id).forEach(e => {
      if (!connected.has(e.from_task_id)) {
        connected.add(e.from_task_id);
        walkUp(e.from_task_id);
      }
    });
  }
  // Walk downstream
  function walkDown(id) {
    edges.filter(e => e.from_task_id === id).forEach(e => {
      if (!connected.has(e.to_task_id)) {
        connected.add(e.to_task_id);
        walkDown(e.to_task_id);
      }
    });
  }
  walkUp(nodeId);
  walkDown(nodeId);

  nodeG.selectAll('g').style('opacity', function() {
    return connected.has(this.getAttribute('data-id')) ? 1 : 0.2;
  });
  edgeG.selectAll('path').style('opacity', function() {
    const classes = this.getAttribute('class') || '';
    return Array.from(connected).some(id => classes.includes(`edge-${id}`)) ? 1 : 0.08;
  });
}

function _resetHighlight(nodeG, edgeG) {
  nodeG.selectAll('g').style('opacity', 1);
  edgeG.selectAll('path').style('opacity', 1);
}

return renderGraph;
})();
