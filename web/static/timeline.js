// timeline.js — Feature-focused timeline with sprint columns
// Depends on globals: api, render, fmtDate, fmtHours, escHtml, d3
// Exposes: renderTimeline(prefix)

var renderTimeline = (function() {
'use strict';

const BAR_HEIGHT = 34;
const BAR_GAP = 8;
const ROW_HEIGHT = BAR_HEIGHT + BAR_GAP;
const HEADER_HEIGHT = 56;
const PRIORITY_ORDER = { urgent: 0, high: 1, medium: 2, low: 3 };

function _getLeftPanelWidth() {
  return window.innerWidth < 600 ? 140 : 300;
}

const STATUS_COLORS = {
  todo:              { bar: '#222', progress: '#484f58' },
  in_progress:       { bar: '#1f3a5f', progress: '#58a6ff' },
  done:              { bar: '#1a3a22', progress: '#3fb950' },
  waiting_external:  { bar: '#3d2c00', progress: '#d29922' },
  waiting_decision:  { bar: '#3d2c00', progress: '#d29922' },
  waiting_feedback:  { bar: '#3d2c00', progress: '#d29922' },
};

let _prefix = '';
let _tasks = [];
let _sprints = [];
let _expandedFeatures = {};
let _showDone = false;

async function renderTimeline(prefix) {
  _prefix = prefix;
  render('<div class="loading"><div class="spinner"></div> Loading timeline…</div>');

  try {
    const [tasks, sprints] = await Promise.all([
      api.get(`/projects/${prefix}/tasks`),
      api.get(`/projects/${prefix}/sprints`),
    ]);
    _tasks = tasks;
    _sprints = sprints.sort((a, b) => {
      const da = a.start_date || a.created_at;
      const db = b.start_date || b.created_at;
      return da < db ? -1 : da > db ? 1 : 0;
    });
  } catch (err) {
    render(`<div class="alert alert-danger">Failed to load: ${escHtml(err.message)}</div>`);
    return;
  }

  if (_tasks.length === 0) {
    render('<div class="empty-state"><div class="empty-state-title">No tasks yet</div></div>');
    return;
  }

  _renderTimeline();
}

function _renderTimeline() {
  const activeTasks = _showDone ? _tasks : _tasks.filter(t => t.status !== 'done');

  // Build feature list (primary rows) sorted by priority then seq
  const features = activeTasks
    .filter(t => (t.type || 'task') === 'feature')
    .sort((a, b) => {
      const pa = PRIORITY_ORDER[a.priority] ?? 99;
      const pb = PRIORITY_ORDER[b.priority] ?? 99;
      if (pa !== pb) return pa - pb;
      return (a.seq ?? 0) - (b.seq ?? 0);
    });

  // Map tasks to their parent feature (not epic — epics are milestones, not board groups)
  const epicIds = new Set(activeTasks.filter(t => t.type === 'epic').map(t => t.id));
  const tasksByParent = {};
  activeTasks.filter(t => (t.type || 'task') === 'task' && t.parent_id && !epicIds.has(t.parent_id)).forEach(t => {
    if (!tasksByParent[t.parent_id]) tasksByParent[t.parent_id] = [];
    tasksByParent[t.parent_id].push(t);
  });

  // Build visible rows
  const rows = [];
  features.forEach(feat => {
    const children = (tasksByParent[feat.id] || []).sort((a, b) => {
      const pa = PRIORITY_ORDER[a.priority] ?? 99;
      const pb = PRIORITY_ORDER[b.priority] ?? 99;
      if (pa !== pb) return pa - pb;
      return (a.seq ?? 0) - (b.seq ?? 0);
    });
    const doneCount = children.filter(t => t.status === 'done').length;
    rows.push({ type: 'feature', task: feat, childCount: children.length, doneCount });
    if (_expandedFeatures[feat.id]) {
      children.forEach(child => {
        rows.push({ type: 'task', task: child, indent: true });
      });
    }
  });

  // Also include orphan tasks and epic-parented tasks at the bottom
  const orphans = activeTasks.filter(t => (t.type || 'task') === 'task' && (!t.parent_id || epicIds.has(t.parent_id)));
  if (orphans.length > 0) {
    orphans.sort((a, b) => {
      const pa = PRIORITY_ORDER[a.priority] ?? 99;
      const pb = PRIORITY_ORDER[b.priority] ?? 99;
      if (pa !== pb) return pa - pb;
      return (a.seq ?? 0) - (b.seq ?? 0);
    }).forEach(t => {
      rows.push({ type: 'task', task: t, indent: false });
    });
  }

  // Determine time range — keep it tight around actual data
  const allDates = _tasks.map(t => new Date(t.created_at));
  _tasks.filter(t => t.due_date).forEach(t => allDates.push(new Date(t.due_date)));
  _sprints.forEach(s => {
    if (s.start_date) allDates.push(new Date(s.start_date));
    if (s.end_date) allDates.push(new Date(s.end_date));
  });

  const today = new Date();
  allDates.push(today);
  const rawMin = d3.min(allDates);
  const rawMax = d3.max(allDates);
  // Ensure at least 4 weeks visible for readability
  const rangeDays = (rawMax - rawMin) / 86400000;
  const minDate = _addDays(rawMin, -3);
  const maxDate = _addDays(rawMax, Math.max(7, 28 - rangeDays));

  const contentHeight = rows.length * ROW_HEIGHT;
  const totalHeight = HEADER_HEIGHT + contentHeight + 20;

  const doneCount = _tasks.filter(t => t.status === 'done').length;
  render(`<div id="timeline-wrapper" class="timeline-wrapper">
    <div class="timeline-toolbar">
      ${doneCount > 0 ? `<label class="filter-checkbox"><input type="checkbox" id="timeline-show-done" ${_showDone ? 'checked' : ''}><span class="text-muted">Show done (${doneCount})</span></label>` : ''}
    </div>
    <div id="timeline-container" class="timeline-container"></div>
  </div>`);

  const showDoneEl = document.getElementById('timeline-show-done');
  if (showDoneEl) {
    showDoneEl.addEventListener('change', () => {
      _showDone = showDoneEl.checked;
      _renderTimeline();
    });
  }

  const container = document.getElementById('timeline-container');
  if (!container) return;

  const LEFT_PANEL_WIDTH = _getLeftPanelWidth();
  const totalWidth = Math.max(window.innerWidth - 24, LEFT_PANEL_WIDTH + 200);
  const chartWidth = totalWidth - LEFT_PANEL_WIDTH;

  const x = d3.scaleTime().domain([minDate, maxDate]).range([0, chartWidth]);

  const svg = d3.select(container)
    .append('svg')
    .attr('width', totalWidth)
    .attr('height', totalHeight)
    .style('font-family', 'var(--font-mono)')
    .style('font-size', '11px');

  // Left panel background
  svg.append('rect')
    .attr('width', LEFT_PANEL_WIDTH)
    .attr('height', totalHeight)
    .attr('fill', '#0a0a0a');

  svg.append('line')
    .attr('x1', LEFT_PANEL_WIDTH).attr('x2', LEFT_PANEL_WIDTH)
    .attr('y1', 0).attr('y2', totalHeight)
    .attr('stroke', '#222');

  const chartG = svg.append('g').attr('transform', `translate(${LEFT_PANEL_WIDTH}, ${HEADER_HEIGHT})`);

  // Sprint header bands
  _sprints.forEach(sprint => {
    const start = sprint.start_date ? new Date(sprint.start_date) : null;
    const end = sprint.end_date ? new Date(sprint.end_date) : null;
    if (!start || !end) return;

    const sx = x(start);
    const ex = x(end);
    if (ex < 0 || sx > chartWidth) return;

    const clampedSx = Math.max(0, sx);
    const clampedEx = Math.min(chartWidth, ex);

    // Header label
    svg.append('rect')
      .attr('x', LEFT_PANEL_WIDTH + clampedSx)
      .attr('y', 0)
      .attr('width', clampedEx - clampedSx)
      .attr('height', HEADER_HEIGHT)
      .attr('fill', sprint.status === 'active' ? 'rgba(88,166,255,0.06)' : 'rgba(255,255,255,0.02)')
      .attr('stroke', '#222')
      .attr('stroke-width', 1);

    svg.append('text')
      .attr('x', LEFT_PANEL_WIDTH + clampedSx + (clampedEx - clampedSx) / 2)
      .attr('y', 20)
      .attr('text-anchor', 'middle')
      .attr('fill', sprint.status === 'active' ? '#58a6ff' : '#e0e0e0')
      .attr('font-size', '13px')
      .attr('font-weight', '600')
      .text(sprint.name);

    const dateLabel = _shortDate(start) + ' – ' + _shortDate(end);
    svg.append('text')
      .attr('x', LEFT_PANEL_WIDTH + clampedSx + (clampedEx - clampedSx) / 2)
      .attr('y', 38)
      .attr('text-anchor', 'middle')
      .attr('fill', '#707070')
      .attr('font-size', '11px')
      .text(dateLabel);

    // Column band in chart area
    chartG.append('rect')
      .attr('x', clampedSx)
      .attr('y', 0)
      .attr('width', clampedEx - clampedSx)
      .attr('height', contentHeight)
      .attr('fill', sprint.status === 'active' ? 'rgba(88,166,255,0.03)' : 'transparent')
      .attr('stroke', '#1a1a1a')
      .attr('stroke-width', 1);
  });

  // Weekly grid lines (Monday)
  const mondayTicks = d3.timeMonday.range(minDate, maxDate);
  chartG.selectAll('.grid-line')
    .data(mondayTicks)
    .join('line')
    .attr('x1', d => x(d)).attr('x2', d => x(d))
    .attr('y1', 0).attr('y2', contentHeight)
    .attr('stroke', '#333').attr('stroke-width', 1)
    .attr('stroke-dasharray', '2,3');

  // Week labels at top of grid
  chartG.selectAll('.week-label')
    .data(mondayTicks)
    .join('text')
    .attr('x', d => x(d) + 4)
    .attr('y', -8)
    .attr('fill', '#666')
    .attr('font-size', '11px')
    .text(d => d3.timeFormat('%b %d')(d));

  // Today line
  if (today >= minDate && today <= maxDate) {
    const tx = x(today);
    chartG.append('line')
      .attr('x1', tx).attr('x2', tx)
      .attr('y1', -HEADER_HEIGHT + 10).attr('y2', contentHeight)
      .attr('stroke', '#58a6ff').attr('stroke-width', 1.5)
      .attr('stroke-dasharray', '4,3').attr('opacity', 0.7);
    svg.append('text')
      .attr('x', LEFT_PANEL_WIDTH + tx).attr('y', HEADER_HEIGHT - 4)
      .attr('text-anchor', 'middle').attr('fill', '#58a6ff').attr('font-size', '9px')
      .text('Today');
  }

  // Row backgrounds + left panel labels
  rows.forEach((row, i) => {
    const y = i * ROW_HEIGHT;

    // Alternating row bg
    if (i % 2 === 0) {
      chartG.append('rect')
        .attr('x', 0).attr('y', y)
        .attr('width', chartWidth).attr('height', ROW_HEIGHT)
        .attr('fill', 'rgba(255,255,255,0.01)');
    }

    // Left label
    const labelX = row.indent ? 28 : 8;
    const labelG = svg.append('g')
      .attr('transform', `translate(${labelX}, ${HEADER_HEIGHT + y})`)
      .style('cursor', row.type === 'feature' ? 'pointer' : 'default');

    const isNarrow = LEFT_PANEL_WIDTH < 200;

    if (row.type === 'feature') {
      const hasChildren = row.childCount > 0;
      const expanded = !!_expandedFeatures[row.task.id];

      if (hasChildren) {
        labelG.append('text')
          .attr('x', 0).attr('y', ROW_HEIGHT / 2)
          .attr('dominant-baseline', 'middle')
          .attr('fill', '#58a6ff')
          .attr('font-size', '10px')
          .text(expanded ? '▾' : '▸');
      }

      const maxChars = isNarrow ? (hasChildren ? 14 : 16) : (hasChildren ? 28 : 34);
      labelG.append('text')
        .attr('x', hasChildren ? 16 : 0).attr('y', ROW_HEIGHT / 2)
        .attr('dominant-baseline', 'middle')
        .attr('fill', '#e0e0e0')
        .attr('font-size', isNarrow ? '11px' : '13px')
        .attr('font-weight', '500')
        .text(_truncLabel(row.task.title, maxChars));

      // Progress badge
      if (hasChildren) {
        const badge = `${row.doneCount}/${row.childCount}`;
        labelG.append('text')
          .attr('x', LEFT_PANEL_WIDTH - labelX - 8).attr('y', ROW_HEIGHT / 2)
          .attr('dominant-baseline', 'middle')
          .attr('text-anchor', 'end')
          .attr('fill', row.doneCount === row.childCount ? '#3fb950' : '#707070')
          .attr('font-size', '11px')
          .text(badge);
      }

      labelG.on('click', () => {
        _expandedFeatures[row.task.id] = !_expandedFeatures[row.task.id];
        _renderTimeline();
      });
    } else {
      // Task row
      const taskMaxChars = isNarrow ? (row.indent ? 12 : 16) : (row.indent ? 28 : 34);
      labelG.append('text')
        .attr('x', 0).attr('y', ROW_HEIGHT / 2)
        .attr('dominant-baseline', 'middle')
        .attr('fill', row.indent ? '#999' : '#ccc')
        .attr('font-size', row.indent ? (isNarrow ? '9px' : '11px') : (isNarrow ? '10px' : '12px'))
        .text(_truncLabel(row.task.title, taskMaxChars));
    }
  });

  // Task bars
  const priorityColors = { urgent: '#f85149', high: '#d29922', medium: '#58a6ff', low: '#484f58' };

  rows.forEach((row, i) => {
    const t = row.task;
    const y = i * ROW_HEIGHT + 2;
    const h = BAR_HEIGHT - 4;

    let start, end;

    if (row.type === 'feature' && row.childCount > 0) {
      // Feature bar spans its child tasks' range
      const children = _tasks.filter(c => c.parent_id === t.id);
      const childStarts = children.map(c => new Date(c.created_at));
      const childEnds = children.map(c => c.due_date ? new Date(c.due_date) : _addDays(new Date(c.created_at), 5));
      start = d3.min(childStarts) || new Date(t.created_at);
      end = d3.max(childEnds) || _addDays(start, 7);
    } else {
      start = new Date(t.created_at);
      if (t.due_date) {
        end = new Date(t.due_date);
        if (end <= start) end = _addDays(start, 1);
      } else {
        const days = row.type === 'feature' ? 7 : 5;
        end = _addDays(start, days);
      }
    }

    const barX = x(start);
    const barW = Math.max(6, x(end) - x(start));
    const progress = t.status === 'done' ? 1 : t.status === 'in_progress' ? 0.5 : 0;
    const colors = STATUS_COLORS[t.status] || STATUS_COLORS.todo;

    // Background bar
    chartG.append('rect')
      .attr('x', barX).attr('y', y)
      .attr('width', barW).attr('height', h)
      .attr('rx', 3)
      .attr('fill', colors.bar);

    // Progress fill
    if (progress > 0) {
      chartG.append('rect')
        .attr('x', barX).attr('y', y)
        .attr('width', barW * progress).attr('height', h)
        .attr('rx', 3)
        .attr('fill', colors.progress)
        .attr('opacity', 0.85);
    }

    // Priority dot
    chartG.append('circle')
      .attr('cx', barX - 5).attr('cy', y + h / 2)
      .attr('r', 2.5)
      .attr('fill', priorityColors[t.priority] || '#484f58');

    // Hover target
    chartG.append('rect')
      .attr('x', barX).attr('y', y)
      .attr('width', Math.max(20, barW)).attr('height', h)
      .attr('fill', 'transparent')
      .attr('cursor', 'pointer')
      .on('mouseenter', function(event) { _showTooltip(event, t, start, end); })
      .on('mouseleave', _hideTooltip);
  });
}

function _showTooltip(event, d, start, end) {
  _hideTooltip();
  const tip = document.createElement('div');
  tip.id = 'timeline-tooltip';
  tip.style.cssText = 'position:fixed;background:#0a0a0a;border:1px solid #333;border-radius:4px;padding:8px 10px;font-size:11px;color:#e0e0e0;pointer-events:none;z-index:999;max-width:260px';
  tip.innerHTML = `
    <div style="font-family:var(--font-mono);color:#707070;font-size:10px">${_prefix}-${d.seq} · ${d.type || 'task'}</div>
    <div style="font-weight:600;margin:2px 0">${escHtml(d.title)}</div>
    <div style="color:#707070">${d.status.replace(/_/g,' ')} · ${d.priority}</div>
    <div style="color:#707070;font-size:10px;margin-top:3px">${fmtDate(start.toISOString())} → ${fmtDate(end.toISOString())}</div>
  `;
  document.body.appendChild(tip);
  const rect = event.target.getBoundingClientRect();
  tip.style.left = Math.min(rect.left, window.innerWidth - 270) + 'px';
  tip.style.top = (rect.bottom + 6) + 'px';
}

function _hideTooltip() {
  const tip = document.getElementById('timeline-tooltip');
  if (tip) tip.remove();
}

function _shortDate(d) {
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[d.getMonth()] + ' ' + d.getDate();
}

function _truncLabel(text, maxChars) {
  if (!text) return '';
  return text.length > maxChars ? text.slice(0, maxChars - 1) + '…' : text;
}

function _addDays(date, days) { const d = new Date(date); d.setDate(d.getDate() + days); return d; }

return renderTimeline;
})();
