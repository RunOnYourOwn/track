// insights.js — analytics / insights dashboard
// Requires: api global, render global

async function renderInsights() {
  render(`
    <div class="page-insights">
      <div class="page-header">
        <h2>Insights</h2>
        <p class="page-subtitle" style="color:#8b949e;margin:4px 0 0">Cross-project analytics</p>
      </div>
      <div id="insights-loading" class="loading-state">Loading data…</div>
    </div>
  `);

  let projects;
  try {
    projects = await api.get('/projects');
  } catch (e) {
    document.getElementById('insights-loading').outerHTML =
      `<div class="empty-state">Failed to load projects: ${escHtml((e).message)}</div>`;
    return;
  }

  if (!projects || projects.length === 0) {
    document.getElementById('insights-loading').outerHTML =
      `<div class="empty-state">No projects found.</div>`;
    return;
  }

  // Fetch tasks for all projects in parallel
  const taskResults = await Promise.allSettled(
    projects.map(p => api.get(`/projects/${p.prefix}/tasks`))
  );

  const projectData = projects.map((p, i) => ({
    ...p,
    tasks: taskResults[i].status === 'fulfilled' ? (taskResults[i].value || []) : [],
  }));

  const container = document.getElementById('insights-loading').parentElement;
  document.getElementById('insights-loading').remove();

  container.insertAdjacentHTML('beforeend', `
    <div class="insights-grid">
      ${buildThroughputCard(projectData)}
      ${buildAccuracyCard(projectData)}
      ${buildDistributionCard(projectData)}
      ${buildWIPCard(projectData)}
    </div>
  `);
}

// ── Card 1: Throughput ───────────────────────────────────────────────────────

function buildThroughputCard(projectData) {
  const rows = projectData.map(p => {
    const done = p.tasks.filter(t => t.status === 'done').length;
    return { prefix: p.prefix, name: p.name, done, total: p.tasks.length };
  });

  const maxDone = Math.max(...rows.map(r => r.done), 1);

  const bars = rows.map(r => `
    <div class="chart-row">
      <div class="chart-label">
        <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
        <span style="color:#8b949e;font-size:12px;margin-left:6px">${escHtml(r.name)}</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill"
             style="width:${pct(r.done, maxDone)}%;background:#3fb950"
             title="${r.done} done"></div>
      </div>
      <div class="chart-value" style="color:#3fb950">${r.done}</div>
    </div>
  `).join('');

  return chartCard('Throughput', 'Tasks completed per project', bars || emptyRow());
}

// ── Card 2: Estimation Accuracy ──────────────────────────────────────────────

function buildAccuracyCard(projectData) {
  const rows = projectData.map(p => {
    const eligible = p.tasks.filter(
      t => t.estimate_hours > 0 && t.actual_hours > 0
    );
    const avg = eligible.length === 0
      ? null
      : eligible.reduce((sum, t) => {
          const acc = Math.min(t.estimate_hours, t.actual_hours) /
                      Math.max(t.estimate_hours, t.actual_hours) * 100;
          return sum + acc;
        }, 0) / eligible.length;

    return { prefix: p.prefix, name: p.name, avg, count: eligible.length };
  });

  const bars = rows.map(r => {
    if (r.avg === null) {
      return `
        <div class="chart-row">
          <div class="chart-label">
            <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
          </div>
          <div class="bar-track">
            <div style="color:#8b949e;font-size:12px;line-height:20px;padding-left:8px">
              No data (need estimate + actual)
            </div>
          </div>
          <div class="chart-value" style="color:#8b949e">—</div>
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

// ── Card 3: Status Distribution ──────────────────────────────────────────────

function buildDistributionCard(projectData) {
  const bars = projectData.map(p => {
    const total = p.tasks.length;
    if (total === 0) {
      return `
        <div class="chart-row">
          <div class="chart-label"><span class="badge badge-prefix">${escHtml(p.prefix)}</span></div>
          <div class="bar-track" style="color:#8b949e;font-size:12px;padding-left:8px">No tasks</div>
        </div>`;
    }
    const counts = { todo: 0, in_progress: 0, done: 0, blocked: 0 };
    p.tasks.forEach(t => {
      if (t.status === 'done') counts.done++;
      else if (t.status === 'in_progress') counts.in_progress++;
      else if (t.status === 'blocked' || t.status.startsWith('waiting')) counts.blocked++;
      else counts.todo++;
    });
    const segments = [
      { key: 'done',        color: '#3fb950', label: 'Done' },
      { key: 'in_progress', color: '#58a6ff', label: 'In Progress' },
      { key: 'todo',        color: '#484f58', label: 'Todo' },
      { key: 'blocked',     color: '#f85149', label: 'Blocked' },
    ];
    const segs = segments
      .filter(s => counts[s.key] > 0)
      .map(s => `
        <div class="stacked-seg"
             style="width:${pct(counts[s.key], total)}%;background:${s.color}"
             title="${s.label}: ${counts[s.key]}"></div>
      `).join('');

    const legend = segments
      .filter(s => counts[s.key] > 0)
      .map(s => `<span style="color:${s.color}">${counts[s.key]} ${s.label}</span>`)
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

// ── Card 4: WIP Check ────────────────────────────────────────────────────────

function buildWIPCard(projectData) {
  const rows = projectData.map(p => {
    const inProgress = p.tasks.filter(t => t.status === 'in_progress').length;
    const limit = p.wip_limit || 0;
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
    return { prefix: p.prefix, name: p.name, inProgress, limit, color, label };
  });

  const items = rows.map(r => `
    <div class="chart-row">
      <div class="chart-label">
        <span class="badge badge-prefix">${escHtml(r.prefix)}</span>
        <span style="color:#8b949e;font-size:12px;margin-left:6px">${escHtml(r.name)}</span>
      </div>
      <div style="display:flex;align-items:center;gap:8px;flex:1;min-width:0">
        <div class="wip-dots">
          ${buildWIPDots(r.inProgress, r.limit, r.color)}
        </div>
        <span style="font-size:12px;color:${r.color};white-space:nowrap">${r.label}</span>
      </div>
    </div>
  `).join('');

  return chartCard('WIP Check', 'In-progress tasks vs WIP limit', items || emptyRow());
}

function buildWIPDots(current, limit, color) {
  if (limit === 0) {
    return `<span style="color:#8b949e;font-size:12px">${current} in progress</span>`;
  }
  const dots = [];
  const max = Math.max(current, limit);
  for (let i = 0; i < max; i++) {
    const filled = i < current;
    const overLimit = i >= limit;
    const dotColor = filled ? (overLimit ? '#f85149' : color) : '#30363d';
    dots.push(`<span style="color:${dotColor};font-size:16px">●</span>`);
  }
  return dots.join('');
}

// ── Shared helpers ───────────────────────────────────────────────────────────

function chartCard(title, subtitle, bodyHtml) {
  return `
    <div class="chart-card">
      <div class="chart-card-header">
        <div class="chart-card-title">${title}</div>
        <div class="chart-card-subtitle">${subtitle}</div>
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

