// sessions.js — session timeline view
// Requires: api global, render global, fmtDate global, escHtml global

const _sessionStatsCache = {};
const _expandedSessions = {};

async function renderSessions() {
  // Invalidate the stats cache on every view entry so re-rendering after edits
  // from other views always fetches fresh stats.
  Object.keys(_sessionStatsCache).forEach(k => delete _sessionStatsCache[k]);

  render(`
    <div class="page-sessions">
      <div class="page-header">
        <h2>Sessions</h2>
        <p class="page-subtitle" style="color:#8b949e;margin:4px 0 0">
          Work sessions across all projects
        </p>
      </div>

      <div class="sessions-controls" style="display:flex;align-items:center;gap:12px;margin:16px 0">
        <label style="color:#8b949e;font-size:13px">Filter by project:</label>
        <select id="sessions-project-filter" class="form-select" style="min-width:160px">
          <option value="">All projects</option>
        </select>
      </div>

      <div id="sessions-loading" class="loading-state">Loading sessions…</div>
      <div id="sessions-list"></div>
    </div>
  `);

  let projects;
  try {
    projects = await api.get('/projects');
  } catch (e) {
    document.getElementById('sessions-loading').outerHTML =
      `<div class="empty-state">Failed to load projects: ${escHtml((e).message)}</div>`;
    return;
  }

  if (!projects || projects.length === 0) {
    document.getElementById('sessions-loading').outerHTML =
      `<div class="empty-state">No projects found.</div>`;
    return;
  }

  const select = document.getElementById('sessions-project-filter');
  projects.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.prefix;
    opt.textContent = `${p.prefix} — ${p.name}`;
    select.appendChild(opt);
  });

  const prefixByProjectId = Object.fromEntries(projects.map(p => [p.id, p.prefix]));

  const sessionResults = await Promise.allSettled(
    projects.map(p => api.get(`/projects/${p.prefix}/sessions`))
  );

  let allSessions = [];
  sessionResults.forEach((r, i) => {
    if (r.status === 'fulfilled' && Array.isArray(r.value)) {
      r.value.forEach(s => {
        allSessions.push({ ...s, _prefix: projects[i].prefix });
      });
    }
  });

  allSessions.sort((a, b) => new Date(b.started_at) - new Date(a.started_at));

  document.getElementById('sessions-loading').remove();

  function renderList(filterPrefix) {
    const list = document.getElementById('sessions-list');
    const filtered = filterPrefix
      ? allSessions.filter(s => s._prefix === filterPrefix)
      : allSessions;

    if (filtered.length === 0) {
      list.innerHTML = `
        <div class="empty-state">
          No sessions recorded.<br>
          Start one with <code>track session start --project PREFIX</code>
        </div>`;
      return;
    }

    const groups = groupByDate(filtered);
    list.innerHTML = groups.map(([dateLabel, sessions]) => {
      const dayTotal = dayDurationTotal(sessions);
      return `<div class="session-date-group">
        <div class="session-date-header">
          <span>${dateLabel}</span>
          ${dayTotal ? `<span class="session-day-total">${dayTotal}</span>` : ''}
        </div>
        ${sessions.map(s => sessionCard(s)).join('')}
      </div>`;
    }).join('');

    list.querySelectorAll('.session-expand-btn').forEach(btn => {
      btn.addEventListener('click', () => toggleSessionDetail(btn));
    });
  }

  renderList('');
  select.addEventListener('change', e => renderList(e.target.value));
}

function groupByDate(sessions) {
  const map = new Map();
  sessions.forEach(s => {
    const d = new Date(s.started_at);
    const key = isoDate(d);
    if (!map.has(key)) map.set(key, []);
    map.get(key).push(s);
  });
  return [...map.entries()].map(([iso, ss]) => [friendlyDate(new Date(iso)), ss]);
}

function isoDate(d) {
  return d.toISOString().slice(0, 10);
}

function friendlyDate(d) {
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);

  if (isoDate(d) === isoDate(today)) return 'Today';
  if (isoDate(d) === isoDate(yesterday)) return 'Yesterday';

  return d.toLocaleDateString(undefined, {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
  });
}

function dayDurationTotal(sessions) {
  let totalMs = 0;
  let hasAny = false;
  sessions.forEach(s => {
    if (s.ended_at) {
      totalMs += new Date(s.ended_at) - new Date(s.started_at);
      hasAny = true;
    }
  });
  if (!hasAny) return null;
  const totalMin = Math.round(totalMs / 60000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  if (h === 0) return `${m}m total`;
  if (m === 0) return `${h}h total`;
  return `${h}h ${m}m total`;
}

function sessionDuration(s) {
  if (!s.ended_at) return null;
  const ms = new Date(s.ended_at) - new Date(s.started_at);
  const totalMin = Math.round(ms / 60000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  if (h === 0) return `${m}m`;
  if (m === 0) return `${h}h`;
  return `${h}h ${m}m`;
}

function sessionCard(s) {
  const duration = sessionDuration(s);
  const startTime = new Date(s.started_at).toLocaleTimeString(undefined, {
    hour: '2-digit', minute: '2-digit'
  });

  const stats = s.stats;
  let statsRow = '';
  if (stats && (stats.tasks_completed || stats.tasks_touched || stats.total_hours || stats.commit_count)) {
    const chips = [];
    if (stats.tasks_completed > 0)
      chips.push(`<span class="session-stat-chip completed">${icon('check', {size: 12})} ${stats.tasks_completed} done</span>`);
    if (stats.tasks_touched > 0)
      chips.push(`<span class="session-stat-chip has-value">${icon('circle-dot', {size: 12})} ${stats.tasks_touched} touched</span>`);
    if (stats.total_hours > 0)
      chips.push(`<span class="session-stat-chip hours">${icon('clock', {size: 12})} ${stats.total_hours.toFixed(1)}h</span>`);
    if (stats.commit_count > 0)
      chips.push(`<span class="session-stat-chip has-value">${icon('git-commit', {size: 12})} ${stats.commit_count} commit${stats.commit_count > 1 ? 's' : ''}</span>`);
    statsRow = `<div class="session-stats-row">${chips.join('')}</div>`;
  }

  const hasDetail = stats && (stats.tasks_touched > 0 || stats.commit_count > 0);

  return `
    <div class="session-card" data-session-id="${s.id}">
      <div class="session-card-meta">
        <span class="badge-prefix">${escHtml(s._prefix)}</span>
        ${s.branch ? `<span class="badge-branch">${icon('git-branch', {size: 12})} ${escHtml(s.branch)}</span>` : ''}
        <span style="color:#8b949e;font-size:12px">${startTime}</span>
        ${duration ? `<span class="session-duration">${duration}</span>` : '<span style="color:#d29922;font-size:12px">in progress</span>'}
      </div>
      ${s.summary ? `<div class="session-summary">${escHtml(s.summary)}</div>` : ''}
      ${statsRow}
      ${hasDetail ? `<button class="session-expand-btn" data-sid="${s.id}" data-prefix="${escHtml(s._prefix)}">${icon('chevron-down', {size: 12})} Details</button>` : ''}
      <div class="session-detail-slot" id="detail-${s.id}"></div>
    </div>`;
}

async function toggleSessionDetail(btn) {
  const sid = btn.dataset.sid;
  const prefix = btn.dataset.prefix || '';
  const slot = document.getElementById('detail-' + sid);

  if (_expandedSessions[sid]) {
    delete _expandedSessions[sid];
    slot.innerHTML = '';
    btn.innerHTML = `${icon('chevron-down', {size: 12})} Details`;
    return;
  }

  _expandedSessions[sid] = true;
  btn.innerHTML = `${icon('chevron-up', {size: 12})} Hide`;

  if (_sessionStatsCache[sid]) {
    mountDetailPanel(slot, _sessionStatsCache[sid], prefix);
    return;
  }

  slot.innerHTML = '<div style="color:#8b949e;font-size:11px;padding:6px 0">Loading…</div>';

  try {
    const stats = await api.get(`/sessions/${sid}/stats`);
    _sessionStatsCache[sid] = stats;
    mountDetailPanel(slot, stats, prefix);
  } catch (e) {
    slot.innerHTML = `<div style="color:var(--danger);font-size:11px;padding:6px 0">Failed to load details</div>`;
  }
}

// Render the detail panel and wire each listed task to the shared detail modal.
function mountDetailPanel(slot, stats, prefix) {
  slot.innerHTML = renderDetailPanel(stats);
  slot.querySelectorAll('.session-task-item[data-task-id]').forEach(item => {
    const open = () => openSessionTaskDetail(item.dataset.taskId, prefix);
    item.addEventListener('click', open);
    item.addEventListener('keydown', e => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); open(); }
    });
  });
}

// Fetch the project's tasks (full objects + the list the modal needs for the
// parent dropdown) and open the shared task modal for the clicked task.
async function openSessionTaskDetail(taskId, prefix) {
  if (!taskId || typeof openTaskModal !== 'function') return;
  try {
    const tasks = await api.get(`/projects/${encodeURIComponent(prefix)}/tasks`);
    const task = (tasks || []).find(t => t.id === taskId);
    if (!task) return;
    const refresh = () => {
      Object.keys(_sessionStatsCache).forEach(k => delete _sessionStatsCache[k]);
      renderSessions();
    };
    openTaskModal(task, { prefix, allTasks: tasks, onSaved: refresh, onDeleted: refresh });
  } catch (_) { /* leave the panel as-is on failure */ }
}

function renderDetailPanel(stats) {
  let html = '<div class="session-detail-panel">';

  if (stats.tasks && stats.tasks.length > 0) {
    html += '<div class="session-detail-section">';
    html += '<div class="session-detail-title">Tasks</div>';
    stats.tasks.forEach(t => {
      const taskIcon = t.completed
        ? `<span class="session-task-icon done">${icon('check', {size: 12})}</span>`
        : `<span class="session-task-icon touched">${icon('circle-dot', {size: 12})}</span>`;
      const cycleTime = t.cycle_time_seconds
        ? `<span class="session-cycle-time">${formatCycleTime(t.cycle_time_seconds)}</span>`
        : '';
      const estimates = formatEstimates(t);
      html += `<div class="session-task-item" data-task-id="${t.task_id}" role="button" tabindex="0" title="View task detail">
        ${taskIcon}
        <span class="session-task-id">#${t.seq}</span>
        <span>${escHtml(t.title)}</span>
        ${estimates}
        ${cycleTime}
      </div>`;
    });
    html += '</div>';
  }

  if (stats.commits && stats.commits.length > 0) {
    html += '<div class="session-detail-section">';
    html += '<div class="session-detail-title">Commits</div>';
    stats.commits.forEach(c => {
      const time = new Date(c.committed_at).toLocaleTimeString(undefined, {
        hour: '2-digit', minute: '2-digit'
      });
      html += `<div class="session-commit-item">
        <span class="session-commit-hash">${escHtml(c.commit_hash.slice(0, 7))}</span>
        <span>${escHtml(c.message || '(no message)')}</span>
        <span class="session-commit-time">${time}</span>
      </div>`;
    });
    html += '</div>';
  }

  html += '</div>';
  return html;
}

function formatEstimates(task) {
  const parts = [];
  if (task.estimate_hours > 0) parts.push(`est ${task.estimate_hours}h`);
  if (task.estimate_agent_minutes > 0) parts.push(`agent ${Math.round(task.estimate_agent_minutes / 60 * 10) / 10}h`);
  if (task.actual_hours > 0) parts.push(`actual ${task.actual_hours.toFixed(1)}h`);
  if (parts.length === 0) return '';
  return `<span class="session-estimate-info">${parts.join(' / ')}</span>`;
}

function formatCycleTime(seconds) {
  if (seconds < 60) return `${seconds}s`;
  const min = Math.round(seconds / 60);
  if (min < 60) return `${min}m`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  if (m === 0) return `${h}h`;
  return `${h}h ${m}m`;
}
