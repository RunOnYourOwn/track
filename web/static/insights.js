// insights.js — analytics dashboard. All metrics are computed server-side
// (GET /api/insights?days=N); this view only renders the returned data.
// Requires: api, render, escHtml globals.

let _insightsRange = 30; // days

async function renderInsights() {
  render(`
    <div class="page-insights">
      <div class="page-header">
        <h2>Insights</h2>
        <p class="page-subtitle" style="color:#8b949e;margin:4px 0 0">Cross-project analytics</p>
      </div>
      <div class="insights-controls">
        <label style="color:#8b949e;font-size:11px">Time range:</label>
        <select id="insights-range" class="form-select" style="min-width:100px">
          <option value="7">Last 7 days</option>
          <option value="14">Last 14 days</option>
          <option value="30">Last 30 days</option>
          <option value="0">All time</option>
        </select>
      </div>
      <div id="insights-loading" class="loading-state">Loading data…</div>
    </div>
  `);

  const sel = document.getElementById('insights-range');
  sel.value = String(_insightsRange);
  sel.addEventListener('change', e => {
    _insightsRange = parseInt(e.target.value, 10);
    drawInsights();
  });

  drawInsights();
}

async function drawInsights() {
  let data;
  try {
    data = await api.get(`/insights?days=${_insightsRange}`);
  } catch (e) {
    _replaceGrid(`<div class="empty-state">Failed to load insights: ${escHtml((e).message)}</div>`);
    return;
  }
  data = data || [];

  if (data.length === 0) {
    _replaceGrid(`<div class="empty-state">No projects found.</div>`);
    return;
  }

  _replaceGrid(
    buildThroughputCard(data) +
    buildCycleTimeCard(data) +
    buildAccuracyCard(data) +
    buildDistributionCard(data) +
    buildWIPCard(data)
  );
}

// Swap the grid contents in place; the loading row is removed on first draw.
function _replaceGrid(innerHtml) {
  const ld = document.getElementById('insights-loading');
  if (ld) ld.remove();
  let grid = document.getElementById('insights-grid');
  if (!grid) {
    const page = document.querySelector('.page-insights');
    if (!page) return;
    page.insertAdjacentHTML('beforeend', '<div class="insights-grid" id="insights-grid"></div>');
    grid = document.getElementById('insights-grid');
  }
  grid.innerHTML = innerHtml;
}

// ── Card 1: Throughput ───────────────────────────────────────────────────────

function buildThroughputCard(data) {
  const rows = data.map(p => ({ prefix: p.prefix, name: p.name, done: p.throughput.done, total: p.throughput.total }));
  const maxDone = Math.max(...rows.map(r => r.done), 1);

  const bars = rows.map(r => `
    <div class="chart-row">
      <div class="chart-label">
        <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
        <span style="color:#8b949e;font-size:12px;margin-left:6px">${escHtml(r.name)}</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill" style="width:${pct(r.done, maxDone)}%;background:#3fb950" title="${r.done} done"></div>
      </div>
      <div class="chart-value" style="color:#3fb950">${r.done}</div>
    </div>
  `).join('');

  return chartCard('Throughput', 'Tasks completed per project', bars || emptyRow());
}

// ── Card 2: Cycle Time ──────────────────────────────────────────────────────

function buildCycleTimeCard(data) {
  const rows = data.map(p => ({
    prefix: p.prefix,
    avg: p.cycle_time.source ? p.cycle_time.avg_hours : null,
    count: p.cycle_time.count,
    source: p.cycle_time.source,
  }));

  const maxAvg = Math.max(...rows.filter(r => r.avg !== null).map(r => r.avg), 1);

  const bars = rows.map(r => {
    if (r.avg === null) {
      return `
        <div class="chart-row">
          <div class="chart-label">
            <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
            <span class="chart-empty-msg">No cycle data</span>
          </div>
        </div>`;
    }
    const color = r.avg <= 4 ? '#3fb950' : r.avg <= 12 ? '#58a6ff' : r.avg <= 48 ? '#d29922' : '#f85149';
    const display = r.avg < 1 ? `${Math.round(r.avg * 60)}m` : r.avg < 24 ? `${r.avg.toFixed(1)}h` : `${(r.avg / 24).toFixed(1)}d`;
    const sourceLabel = r.source === 'lead' ? 'lead time' : 'active';
    return `
      <div class="chart-row">
        <div class="chart-label">
          <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
          <span style="color:#8b949e;font-size:11px;margin-left:6px">(${r.count} · ${sourceLabel})</span>
        </div>
        <div class="bar-track">
          <div class="bar-fill" style="width:${pct(r.avg, maxAvg)}%;background:${color}"></div>
        </div>
        <div class="chart-value" style="color:${color}">${display}</div>
      </div>`;
  }).join('');

  return chartCard('Cycle Time', 'Avg active time per task (excludes bulk imports)', bars || emptyRow());
}

// ── Card 3: Estimation Accuracy ──────────────────────────────────────────────

function buildAccuracyCard(data) {
  const rows = data.map(p => ({
    prefix: p.prefix,
    avg: p.accuracy.count > 0 ? p.accuracy.avg_pct : null,
    count: p.accuracy.count,
  }));

  const bars = rows.map(r => {
    if (r.avg === null) {
      return `
        <div class="chart-row">
          <div class="chart-label">
            <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
            <span class="chart-empty-msg">No data (need estimate + actual)</span>
          </div>
        </div>`;
    }
    const color = r.avg >= 80 ? '#3fb950' : r.avg >= 60 ? '#d29922' : '#f85149';
    return `
      <div class="chart-row">
        <div class="chart-label">
          <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
          <span style="color:#8b949e;font-size:11px;margin-left:6px">(${r.count} tasks)</span>
        </div>
        <div class="bar-track">
          <div class="bar-fill" style="width:${r.avg.toFixed(1)}%;background:${color}"
               title="${r.avg.toFixed(1)}% accuracy"></div>
        </div>
        <div class="chart-value" style="color:${color}">${r.avg.toFixed(0)}%</div>
      </div>`;
  }).join('');

  return chartCard(
    'Estimation Accuracy',
    'How close estimates were to actuals (higher = better)',
    bars || emptyRow()
  );
}

// ── Card 4: Status Distribution ──────────────────────────────────────────────

function buildDistributionCard(data) {
  const bars = data.map(p => {
    const c = p.distribution;
    const total = c.done + c.in_progress + c.todo + c.blocked;
    if (total === 0) {
      return `
        <div class="chart-row">
          <div class="chart-label">
            <span class="badge badge-prefix">${escHtml(p.prefix)}</span>
            <span class="chart-empty-msg">No tasks</span>
          </div>
        </div>`;
    }
    const segments = [
      { val: c.done,        color: '#3fb950', label: 'Done' },
      { val: c.in_progress, color: '#58a6ff', label: 'In Progress' },
      { val: c.todo,        color: '#484f58', label: 'Todo' },
      { val: c.blocked,     color: '#f85149', label: 'Blocked' },
    ];
    const segs = segments
      .filter(s => s.val > 0)
      .map(s => `
        <div class="stacked-seg"
             style="width:${pct(s.val, total)}%;background:${s.color}"
             title="${s.label}: ${s.val}"></div>
      `).join('');

    const legend = segments
      .filter(s => s.val > 0)
      .map(s => `<span style="color:${s.color}">${s.val} ${s.label}</span>`)
      .join(' · ');

    return `
      <div class="chart-row" style="flex-direction:column;align-items:flex-start;gap:4px">
        <div class="chart-label" style="width:100%">
          <span class="badge badge-prefix">${escHtml(p.prefix)}</span>
          <span style="color:#8b949e;font-size:11px;margin-left:8px">${legend}</span>
        </div>
        <div class="stacked-bar">${segs}</div>
      </div>`;
  }).join('');

  return chartCard('Status Distribution', 'Task breakdown by status per project', bars || emptyRow());
}

// ── Card 5: WIP Check ────────────────────────────────────────────────────────

function buildWIPCard(data) {
  const rows = data.map(p => {
    const inProgress = p.wip.in_progress;
    const limit = p.wip.limit || 0;
    let color, label;
    if (limit === 0) {
      color = '#8b949e'; label = 'No limit set';
    } else if (inProgress < limit) {
      color = '#3fb950'; label = `${inProgress}/${limit} — under limit`;
    } else if (inProgress === limit) {
      color = '#d29922'; label = `${inProgress}/${limit} — at limit`;
    } else {
      color = '#f85149'; label = `${inProgress}/${limit} — over limit!`;
    }
    return { prefix: p.prefix, inProgress, limit, color, label };
  });

  const items = rows.map(r => `
    <div class="chart-row">
      <div class="chart-label" style="min-width:0;flex:0 0 auto">
        <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
      </div>
      <div class="wip-dots">
        ${buildWIPDots(r.inProgress, r.limit, r.color)}
      </div>
      <span style="font-size:11px;color:${r.color};white-space:nowrap">${r.label}</span>
    </div>
  `).join('');

  return chartCard('WIP Check', 'In-progress tasks vs WIP limit', items || emptyRow());
}

function buildWIPDots(current, limit, color) {
  if (limit === 0) {
    return `<span style="color:#8b949e;font-size:12px">${current} in progress</span>`;
  }
  const max = Math.max(current, limit);
  // Dots stay readable only for small limits; past ~10 the row would overflow the
  // card, so switch to a proportional bar (over-limit fills fully in red).
  if (max > 10) {
    const pct = Math.min(100, Math.round((current / limit) * 100));
    return `<div class="wip-bar"><div class="wip-bar-fill" style="width:${pct}%;background:${color}"></div></div>`;
  }
  const dots = [];
  for (let i = 0; i < max; i++) {
    const filled = i < current;
    const overLimit = i >= limit;
    const dotColor = filled ? (overLimit ? '#f85149' : color) : '#30363d';
    dots.push(`<span style="color:${dotColor};font-size:12px">●</span>`);
  }
  return dots.join('');
}

// ── Shared helpers ───────────────────────────────────────────────────────────

function chartCard(title, subtitle, bodyHtml) {
  return `
    <div class="chart-card">
      <div class="chart-card-header">
        <div class="chart-card-title">${escHtml(title)}</div>
        <div class="chart-card-subtitle">${escHtml(subtitle)}</div>
      </div>
      <div class="chart-card-body">${bodyHtml}</div>
    </div>`;
}

function emptyRow() {
  return `<div style="color:#8b949e;font-size:13px;padding:8px 0">No data available.</div>`;
}

function pct(value, max) {
  if (max === 0) return 0;
  return Math.min(100, (value / max) * 100);
}
