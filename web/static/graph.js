// graph.js — DAG dependency graph with layered layout.
// Layering + critical path come from GET /api/projects/{prefix}/graph; this view
// only does layout (within-layer ordering, coordinate math) and rendering.
// Requires: d3 (v7) global, api global, render global, escHtml global

var renderGraph = (function() {
'use strict';

let _prefix = '';
let _allTasks = [];
let _graph = { nodes: [], edges: [], max_layer: 0, has_cycle: false };
let _showDone = false;
let _collapsed = new Set(); // container node ids whose subtree is folded away
let _focusRoot = '';        // when set, show only this node + its subtree
let _critOnly = false;      // when true, show only critical-path nodes

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

  await _loadGraph();
  _drawGraph();
}

// Fetch the server-computed layers + critical path for the current done filter.
async function _loadGraph() {
  try {
    _graph = await api.get(`/projects/${_prefix}/graph?include_done=${_showDone}`);
  } catch (e) {
    _graph = { nodes: [], edges: [], max_layer: 0, has_cycle: false };
  }
}

// Re-fetch (the connected set/layering depend on the done filter) then redraw.
async function _reloadGraph() {
  await _loadGraph();
  _drawGraph();
}

// Collapse/expand is pure view state over the already-loaded graph — no refetch.
function _toggleCollapse(id) {
  if (_collapsed.has(id)) _collapsed.delete(id);
  else _collapsed.add(id);
  _drawGraph();
}

// All ids in a node's subtree (the node itself + every descendant).
function _subtreeIds(rootId, childrenOf) {
  const ids = new Set([rootId]);
  (function walk(id) { (childrenOf[id] || []).forEach(c => { ids.add(c.id); walk(c.id); }); })(rootId);
  return ids;
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
    if (showDoneEl) showDoneEl.addEventListener('change', () => { _showDone = showDoneEl.checked; _reloadGraph(); });
    return;
  }

  // Nodes, edges (hierarchy + dependency), layers and critical path are computed
  // server-side; the UI only lays them out and renders.
  const validEdges = _graph.edges.map(e => ({ from_task_id: e.from, to_task_id: e.to, kind: e.kind }));
  const taskById = new Map(tasks.map(t => [t.id, t]));
  const doneCount = _allTasks.filter(t => t.status === 'done').length;

  const layer = new Map(_graph.nodes.map(n => [n.id, n.layer]));
  const criticalPath = new Set(_graph.nodes.filter(n => n.critical).map(n => n.id));
  const connectedIds = new Set(_graph.nodes.map(n => n.id));
  const connectedTasks = tasks.filter(t => connectedIds.has(t.id));

  // Order nodes within each layer by a DFS of the hierarchy, so a subtree stays
  // contiguous and children sit near their parent.
  const childrenOf = {};
  connectedTasks.forEach(t => {
    if (t.parent_id && connectedIds.has(t.parent_id)) {
      (childrenOf[t.parent_id] = childrenOf[t.parent_id] || []).push(t);
    }
  });
  Object.values(childrenOf).forEach(arr => arr.sort((a, b) => (a.seq || 0) - (b.seq || 0)));
  const roots = connectedTasks
    .filter(t => !(t.parent_id && connectedIds.has(t.parent_id)))
    .sort((a, b) => (a.seq || 0) - (b.seq || 0));
  const dfsOrder = new Map();
  (function walk(nodes) {
    nodes.forEach(n => { dfsOrder.set(n.id, dfsOrder.size); walk(childrenOf[n.id] || []); });
  })(roots);

  // Collapse/expand: a collapsed epic/feature folds its whole subtree away. Edges
  // that touched a hidden node are rolled up to the nearest visible ancestor (the
  // collapsed container) and de-duplicated, so a cross-subtree dependency still
  // shows as a single edge between the collapsed boxes. This is a view-state
  // filter over the already-fetched graph; the server-side structure is untouched.
  const parentById = new Map();
  connectedTasks.forEach(t => {
    if (t.parent_id && connectedIds.has(t.parent_id)) parentById.set(t.id, t.parent_id);
  });
  const isCollapsible = (id) => (childrenOf[id] || []).length > 0;
  const descCount = new Map();
  function countDesc(id) {
    if (descCount.has(id)) return descCount.get(id);
    let n = 0;
    (childrenOf[id] || []).forEach(c => { n += 1 + countDesc(c.id); });
    descCount.set(id, n);
    return n;
  }
  connectedTasks.forEach(t => countDesc(t.id));

  // Filters that compose with collapse: focus narrows to one subtree; crit-only
  // keeps just the critical-path nodes (so blocks edges among them survive while
  // the non-critical hierarchy skeleton drops out). Both are view filters over the
  // fetched graph. A focus root that no longer exists (e.g. after a done toggle)
  // is treated as "no focus".
  const focusValid = _focusRoot && connectedIds.has(_focusRoot) && isCollapsible(_focusRoot);
  const focusSet = focusValid ? _subtreeIds(_focusRoot, childrenOf) : null;
  const inScope = (id) => (!focusSet || focusSet.has(id)) && (!_critOnly || criticalPath.has(id));

  const hidden = new Set();
  connectedTasks.forEach(t => {
    if (!inScope(t.id)) return;
    let p = parentById.get(t.id);
    while (p) { if (_collapsed.has(p) && isCollapsible(p) && inScope(p)) { hidden.add(t.id); break; } p = parentById.get(p); }
  });
  const visibleTasks = connectedTasks.filter(t => inScope(t.id) && !hidden.has(t.id));
  const visibleIds = new Set(visibleTasks.map(t => t.id));

  // Map any endpoint to its nearest visible ancestor (skipping collapsed-away,
  // out-of-focus, or non-critical nodes); undefined if none is visible.
  const nearestVisible = (id) => { let c = id; while (c && !visibleIds.has(c)) c = parentById.get(c); return c; };
  const seenEdge = new Set();
  const drawEdges = [];
  validEdges.forEach(e => {
    const f = nearestVisible(e.from_task_id);
    const tt = nearestVisible(e.to_task_id);
    if (!f || !tt || f === tt || !visibleIds.has(f) || !visibleIds.has(tt)) return;
    const key = `${e.kind}|${f}|${tt}`;
    if (seenEdge.has(key)) return;
    seenEdge.add(key);
    drawEdges.push({ from_task_id: f, to_task_id: tt, kind: e.kind });
  });

  const layerVals = visibleTasks.map(t => layer.get(t.id) ?? 0);
  const maxLayer = layerVals.length ? Math.max(...layerVals) : 0;
  const layerNodes = [];
  for (let l = 0; l <= maxLayer; l++) {
    const nodesInLayer = visibleTasks
      .filter(t => layer.get(t.id) === l)
      .sort((a, b) => (dfsOrder.get(a.id) ?? 0) - (dfsOrder.get(b.id) ?? 0));
    layerNodes.push(nodesInLayer);
  }

  // Layout parameters
  const NODE_W = 220;
  const NODE_H = 58;
  const LAYER_GAP = 110;
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
    cancelled:         { bg: '#161616', border: '#484f58', text: '#6e7681', label: 'Cancelled' },
    waiting_external:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
    waiting_decision:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
    waiting_feedback:  { bg: '#2d1f00', border: '#d29922', text: '#d29922', label: 'Waiting' },
  };

  const PRIORITY_COLORS = { urgent: '#f85149', high: '#d29922', medium: '#58a6ff', low: '#484f58' };

  // Focus dropdown lists the container nodes (epics/features), ordered by column.
  const containers = connectedTasks
    .filter(t => isCollapsible(t.id))
    .sort((a, b) => ((layer.get(a.id) ?? 0) - (layer.get(b.id) ?? 0)) || ((a.seq || 0) - (b.seq || 0)));
  const focusOptions = containers
    .map(t => `<option value="${t.id}" ${_focusRoot === t.id ? 'selected' : ''}>${escHtml(t.title)} · ${t.type}</option>`)
    .join('');

  // Render with toolbar (filters on the left + pan/zoom controls on the right)
  render(`<div class="page-graph">
    <div class="timeline-toolbar">
      ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="graph-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
      ${containers.length ? `<label class="filter-checkbox" style="margin-left:12px;"><span class="text-muted">Focus</span><select id="graph-focus" class="filter-select" style="margin-left:6px;"><option value="">All</option>${focusOptions}</select></label>` : ''}
      <label class="filter-checkbox" style="margin-left:12px;"><input type="checkbox" id="graph-crit-only" ${_critOnly ? 'checked' : ''}><span class="text-muted">Critical path only</span></label>
      ${_graph.has_cycle ? `<span class="text-warning" title="A dependency cycle was detected; cyclic edges are ignored for layering" style="margin-left:12px;font-size:12px;">⚠ dependency cycle detected</span>` : ''}
      <span style="margin-left:auto;display:inline-flex;gap:6px;">
        ${_collapsed.size ? `<button class="btn-ghost btn-sm" id="graph-expand-all" title="Expand every collapsed node">Expand all</button>` : ''}
        <button class="btn-ghost btn-sm" id="graph-zoom-out" title="Zoom out">−</button>
        <button class="btn-ghost btn-sm" id="graph-zoom-in" title="Zoom in">+</button>
        <button class="btn-ghost btn-sm" id="graph-fit" title="Fit graph to screen">Fit</button>
      </span>
    </div>
    <div id="graph-container" style="position:relative;"></div>
  </div>`);

  const showDoneEl = document.getElementById('graph-show-done');
  if (showDoneEl) showDoneEl.addEventListener('change', () => { _showDone = showDoneEl.checked; _reloadGraph(); });
  document.getElementById('graph-expand-all')?.addEventListener('click', e => { e.stopPropagation(); _collapsed.clear(); _drawGraph(); });
  document.getElementById('graph-focus')?.addEventListener('change', function() { _focusRoot = this.value; _drawGraph(); });
  document.getElementById('graph-crit-only')?.addEventListener('change', function() { _critOnly = this.checked; _drawGraph(); });

  const container = document.getElementById('graph-container');
  if (visibleTasks.length === 0) {
    container.innerHTML = `<div class="empty-state">${_critOnly ? 'No tasks are on the critical path.' : 'No nodes match the current filter.'}</div>`;
    return;
  }
  const containerRect = container.getBoundingClientRect();

  // The SVG fills the viewport; all content lives in a single pannable/zoomable
  // layer so arbitrarily large graphs stay navigable (drag to pan, scroll/buttons
  // to zoom, Fit to reset). Real projects produce graphs far larger than one screen.
  const viewW = Math.max(containerRect.width || 900, 600);
  const viewH = Math.max((window.innerHeight || 800) - 160, 480);

  const svg = d3.select(container)
    .append('svg')
    .attr('width', viewW)
    .attr('height', viewH)
    .style('font-family', 'var(--font-mono)')
    .style('display', 'block')
    .style('cursor', 'grab')
    .on('click', () => _closeGraphDetail());

  const zoomLayer = svg.append('g'); // pan/zoom transforms apply here

  // Column backgrounds — aligned to actual node positions
  layerNodes.forEach((nodesInLayer, l) => {
    if (nodesInLayer.length === 0) return;
    const x = PAD_X + l * (NODE_W + LAYER_GAP);
    const firstPos = pos.get(nodesInLayer[0].id);
    const lastPos = pos.get(nodesInLayer[nodesInLayer.length - 1].id);
    const minY = firstPos.y - 10;
    const maxY = lastPos.y + NODE_H + 10;
    zoomLayer.append('rect')
      .attr('x', x - 10)
      .attr('y', minY)
      .attr('width', NODE_W + 20)
      .attr('height', maxY - minY)
      .attr('rx', 6)
      .attr('fill', 'rgba(255,255,255,0.02)');
  });

  // Draw edges (curved paths)
  const edgeG = zoomLayer.append('g');

  // Arrow markers
  const defs = svg.append('defs');
  defs.append('marker')
    .attr('id', 'dag-arrow-blocks')
    .attr('viewBox', '0 -4 8 8')
    .attr('refX', 8)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M0,-4L8,0L0,4')
    .attr('fill', '#58a6ff');

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

  drawEdges.forEach(e => {
    const from = pos.get(e.from_task_id);
    const to = pos.get(e.to_task_id);
    if (!from || !to) return;

    const x1 = from.x + NODE_W;
    const y1 = from.y + NODE_H / 2;
    const x2 = to.x;
    const y2 = to.y + NODE_H / 2;

    const midX = (x1 + x2) / 2;
    const path = `M${x1},${y1} C${midX},${y1} ${midX},${y2} ${x2},${y2}`;

    if (e.kind === 'contains') {
      // Hierarchy skeleton: quiet grey line, no arrowhead.
      edgeG.append('path')
        .attr('d', path)
        .attr('fill', 'none')
        .attr('stroke', '#6e7681')
        .attr('stroke-width', 1.5)
        .attr('class', `edge-${e.from_task_id} edge-${e.to_task_id}`)
        .style('opacity', 0.7);
      return;
    }

    // Dependency (blocks): arrowed; red on the critical chain, else blue.
    const isCrit = criticalPath.has(e.from_task_id) && criticalPath.has(e.to_task_id);
    edgeG.append('path')
      .attr('d', path)
      .attr('fill', 'none')
      .attr('stroke', isCrit ? '#f85149' : '#58a6ff')
      .attr('stroke-width', isCrit ? 2.5 : 1.5)
      .attr('marker-end', isCrit ? 'url(#dag-arrow-crit)' : 'url(#dag-arrow-blocks)')
      .attr('class', `edge-${e.from_task_id} edge-${e.to_task_id}`)
      .style('opacity', isCrit ? 0.95 : 0.85);
  });

  // Draw nodes
  const nodeG = zoomLayer.append('g');
  // Overlay layer for hover: connected edges are re-drawn here, ABOVE the cards,
  // so they're never occluded (cards are opaque and paint over the base edges).
  const overlayG = zoomLayer.append('g');

  visibleTasks.forEach(t => {
    const p = pos.get(t.id);
    if (!p) return;
    const s = STATUS[t.status] || STATUS.todo;
    const g = nodeG.append('g')
      .attr('transform', `translate(${p.x},${p.y})`)
      .style('cursor', 'pointer')
      .attr('data-id', t.id);

    const isCritNode = criticalPath.has(t.id);
    const collapsible = isCollapsible(t.id);
    const collapsedNow = collapsible && _collapsed.has(t.id);

    // Collapsed nodes get a stacked shadow behind them, hinting at folded content.
    if (collapsedNow) {
      g.append('rect')
        .attr('x', 5).attr('y', 5)
        .attr('width', NODE_W).attr('height', NODE_H)
        .attr('rx', 6)
        .attr('fill', s.bg)
        .attr('stroke', isCritNode ? '#f85149' : s.border)
        .attr('stroke-width', 1)
        .style('opacity', 0.5);
    }

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

    // Collapse/expand control for container nodes: "−" when expanded, "+N" (hidden
    // descendant count) when collapsed. Sits left of the status dot; its own click
    // toggles fold state without opening the detail panel.
    if (collapsible) {
      const label = collapsedNow ? `+${descCount.get(t.id)}` : '−';
      const pillW = Math.max(18, 10 + label.length * 7);
      const px = NODE_W - 24 - pillW;
      const tg = g.append('g').style('cursor', 'pointer').attr('data-collapse', t.id);
      tg.append('rect')
        .attr('x', px).attr('y', 6)
        .attr('width', pillW).attr('height', 16)
        .attr('rx', 8)
        .attr('fill', '#21262d')
        .attr('stroke', '#484f58')
        .attr('stroke-width', 1);
      tg.append('text')
        .attr('x', px + pillW / 2).attr('y', 17)
        .attr('text-anchor', 'middle')
        .attr('fill', '#8b949e')
        .attr('font-size', '10px')
        .text(label);
      tg.on('click', function(event) { event.stopPropagation(); _toggleCollapse(t.id); });
    }

    // Hover highlight
    g.on('mouseenter', function() {
      _highlightConnected(t.id, drawEdges, visibleTasks, nodeG, edgeG, overlayG);
    })
    .on('mouseleave', function() {
      _resetHighlight(nodeG, edgeG, overlayG);
    })
    .on('click', function(event) {
      event.stopPropagation();
      _openGraphDetail(t, prefix, validEdges, taskById);
    });
  });

  // Legend — pinned to the viewport bottom (on svg, outside the zoom layer).
  const legendY = viewH - 24;
  const legendG = svg.append('g').attr('transform', `translate(${PAD_X}, ${legendY})`);
  const legendItems = [
    { label: 'To Do', color: '#484f58' },
    { label: 'In Progress', color: '#58a6ff' },
    { label: 'Done', color: '#3fb950' },
    { label: 'Waiting', color: '#d29922' },
    { label: 'Contains', color: '#6e7681', line: true },
    { label: 'Blocks', color: '#58a6ff', line: true },
    { label: 'Critical Path', color: '#f85149', line: true },
  ];
  let lx = 0;
  legendItems.forEach(item => {
    if (item.line) {
      legendG.append('line')
        .attr('x1', lx - 6).attr('y1', 0).attr('x2', lx + 6).attr('y2', 0)
        .attr('stroke', item.color).attr('stroke-width', 2.5);
    } else {
      legendG.append('circle').attr('cx', lx).attr('cy', 0).attr('r', 5).attr('fill', item.color);
    }
    legendG.append('text')
      .attr('x', lx + 12).attr('y', 4)
      .attr('fill', '#8b949e').attr('font-size', '11px')
      .text(item.label);
    lx += 22 + item.label.length * 7;
  });

  // Pan/zoom: drag to pan, scroll or the +/−/Fit buttons to zoom. Content scales
  // in zoomLayer; the legend (on svg) stays pinned.
  const zoom = d3.zoom().scaleExtent([0.1, 2.5]).on('zoom', ev => zoomLayer.attr('transform', ev.transform));
  svg.call(zoom).on('dblclick.zoom', null);

  function _fitToView() {
    const pad = 30;
    const k = Math.max(0.1, Math.min((viewW - pad * 2) / totalWidth, (viewH - pad * 2) / totalHeight, 1.5));
    const tx = (viewW - totalWidth * k) / 2;
    const ty = Math.max(pad, (viewH - totalHeight * k) / 2);
    svg.call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(k));
  }
  _fitToView();

  document.getElementById('graph-fit')?.addEventListener('click', e => { e.stopPropagation(); _fitToView(); });
  document.getElementById('graph-zoom-in')?.addEventListener('click', e => { e.stopPropagation(); svg.transition().duration(150).call(zoom.scaleBy, 1.3); });
  document.getElementById('graph-zoom-out')?.addEventListener('click', e => { e.stopPropagation(); svg.transition().duration(150).call(zoom.scaleBy, 1 / 1.3); });
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

function _highlightConnected(nodeId, edges, tasks, nodeG, edgeG, overlayG) {
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

  nodeG.selectAll('g[data-id]').style('opacity', function() {
    return connected.has(this.getAttribute('data-id')) ? 1 : 0.2;
  });
  // Dim base edges; re-draw the connected ones bright + thicker in the overlay
  // layer so they render ABOVE the (opaque) cards instead of hidden behind them.
  if (overlayG) overlayG.selectAll('*').remove();
  edgeG.selectAll('path').style('opacity', function() {
    const classes = this.getAttribute('class') || '';
    const isConnected = Array.from(connected).some(id => classes.includes(`edge-${id}`));
    if (isConnected && overlayG) {
      overlayG.append('path')
        .attr('d', this.getAttribute('d'))
        .attr('fill', 'none')
        .attr('stroke', this.getAttribute('stroke'))
        .attr('stroke-width', (parseFloat(this.getAttribute('stroke-width')) || 1.5) + 1.25)
        .attr('marker-end', this.getAttribute('marker-end') || null)
        .style('opacity', 1);
    }
    return isConnected ? 0.25 : 0.05;
  });
}

function _resetHighlight(nodeG, edgeG, overlayG) {
  nodeG.selectAll('g[data-id]').style('opacity', 1);
  edgeG.selectAll('path').style('opacity', 1);
  if (overlayG) overlayG.selectAll('*').remove();
}

return renderGraph;
})();
