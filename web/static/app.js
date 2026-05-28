/* =========================================================
   Track — Core: API client, router, helpers, dashboard
   ========================================================= */

'use strict';

// =========================================================
// API client
// =========================================================

const api = {
  async get(path) {
    const resp = await fetch(`/api${path}`);
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.json();
  },

  async post(path, body) {
    const resp = await fetch(`/api${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.json();
  },

  async patch(path, body) {
    const resp = await fetch(`/api${path}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.json();
  },

  async del(path) {
    const resp = await fetch(`/api${path}`, { method: 'DELETE' });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.status === 204 ? null : resp.json();
  },
};

// =========================================================
// Router
// =========================================================

const router = {
  _routes: [],

  register(pattern, handler) {
    // Convert :param segments to named capture groups
    const keys = [];
    const regexStr = pattern
      .replace(/:([a-zA-Z_]+)/g, (_, k) => { keys.push(k); return '([^/]+)'; })
      .replace(/\//g, '\\/');
    const regex = new RegExp(`^${regexStr}$`);
    this._routes.push({ regex, keys, handler });
  },

  navigate(hash) {
    location.hash = hash;
  },

  _match(hash) {
    const path = hash.replace(/^#/, '') || '/';
    for (const route of this._routes) {
      const m = path.match(route.regex);
      if (m) {
        const params = {};
        route.keys.forEach((k, i) => { params[k] = decodeURIComponent(m[i + 1]); });
        return { handler: route.handler, params };
      }
    }
    return null;
  },

  _dispatch() {
    const hash = location.hash || '#/';
    const match = this._match(hash);
    if (match) {
      match.handler(match.params);
    } else {
      render('<div class="empty-state"><div class="empty-state-title">404 — page not found</div></div>');
    }
    updateNav(hash);
  },

  start() {
    window.addEventListener('hashchange', () => this._dispatch());
    this._dispatch();
  },
};

// =========================================================
// Render helper
// =========================================================

function render(html) {
  document.getElementById('app').innerHTML = html;
}

// =========================================================
// Format helpers
// =========================================================

function fmtDate(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function fmtDatetime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  return fmtDate(iso) + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function fmtHours(h) {
  if (!h) return '0h';
  return `${(+h).toFixed(1)}h`;
}

function priorityColor(p) {
  const map = { urgent: 'danger', high: 'warning', medium: 'accent', low: 'muted' };
  return map[p] || 'muted';
}

const _validPriorities = new Set(['urgent', 'high', 'medium', 'low']);
const _validStatuses = new Set(['todo', 'in_progress', 'done', 'blocked', 'waiting', 'waiting_review', 'waiting_external', 'waiting_dependency']);

function priorityBadge(p) {
  if (!p) return '';
  const cls = _validPriorities.has(p) ? p : 'medium';
  return `<span class="priority-badge ${cls}">${escHtml(p)}</span>`;
}

function statusBadge(s) {
  if (!s) return '';
  const label = s.replace(/_/g, ' ');
  const cls = s.startsWith('waiting') ? 'waiting' : (_validStatuses.has(s) ? s : 'todo');
  return `<span class="status-badge ${cls}">${escHtml(label)}</span>`;
}

function phaseBadge(phase) {
  if (!phase) return '';
  const cls = /^[a-z0-9_-]+$/i.test(phase) ? phase.toLowerCase() : 'default';
  return `<span class="phase-badge ${cls}">${escHtml(phase)}</span>`;
}

function healthDots(score) {
  // score is 0–100, map to 5 dots
  const filled = Math.round((score || 0) / 20);
  let cls = 'filled';
  if (filled <= 1) cls = 'filled danger';
  else if (filled <= 2) cls = 'filled warn';
  let html = '<div class="health-dots">';
  for (let i = 0; i < 5; i++) {
    html += `<div class="dot ${i < filled ? cls : ''}"></div>`;
  }
  html += '</div>';
  return html;
}

function escHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function daysUntil(dateStr) {
  if (!dateStr) return null;
  const now = new Date();
  now.setHours(0, 0, 0, 0);
  const target = new Date(dateStr);
  target.setHours(0, 0, 0, 0);
  return Math.round((target - now) / 86400000);
}

// =========================================================
// Nav update — highlights active link, adds project tabs
// =========================================================

function updateNav(hash) {
  const navLinks = document.getElementById('nav-links');
  if (!navLinks) return;

  // Check if we're inside a project view
  const projMatch = hash.match(/^#\/project\/([^/]+)\/(focus|board|timeline|tree|graph)$/);

  if (projMatch) {
    const prefix = projMatch[1];
    const view = projMatch[2];
    const ep = encodeURIComponent(prefix);
    navLinks.innerHTML = `
      <a href="#/" class="nav-links-back" data-icon="←" title="Dashboard"><span class="nav-label">←</span></a>
      <span class="nav-prefix">${escHtml(prefix)}</span>
      <a href="#/project/${ep}/focus"    class="nav-view ${view === 'focus'    ? 'active' : ''}" data-icon="◉" title="Focus"><span class="nav-label">Focus</span></a>
      <a href="#/project/${ep}/board"    class="nav-view ${view === 'board'    ? 'active' : ''}" data-icon="▦" title="Board"><span class="nav-label">Board</span></a>
      <a href="#/project/${ep}/timeline" class="nav-view ${view === 'timeline' ? 'active' : ''}" data-icon="▬" title="Timeline"><span class="nav-label">Timeline</span></a>
      <a href="#/project/${ep}/tree"     class="nav-view ${view === 'tree'     ? 'active' : ''}" data-icon="⊞" title="Tree"><span class="nav-label">Tree</span></a>
      <a href="#/project/${ep}/graph"    class="nav-view ${view === 'graph'    ? 'active' : ''}" data-icon="◈" title="Graph"><span class="nav-label">Graph</span></a>
    `;
  } else {
    navLinks.innerHTML = `
      <a href="#/"          class="nav-view ${hash === '#/' || hash === '' ? 'active' : ''}" data-icon="⌂" title="Dashboard"><span class="nav-label">Dashboard</span></a>
      <a href="#/sessions"  class="nav-view ${hash === '#/sessions'  ? 'active' : ''}" data-icon="◷" title="Sessions"><span class="nav-label">Sessions</span></a>
      <a href="#/knowledge" class="nav-view ${hash === '#/knowledge' ? 'active' : ''}" data-icon="◆" title="Knowledge"><span class="nav-label">Knowledge</span></a>
      <a href="#/insights"  class="nav-view ${hash === '#/insights'  ? 'active' : ''}" data-icon="▤" title="Insights"><span class="nav-label">Insights</span></a>
    `;
  }
}

// =========================================================
// Dashboard route
// =========================================================

async function renderDashboard() {
  render('<div class="loading"><div class="spinner"></div> Loading…</div>');

  let data;
  try {
    data = await api.get('/dashboard');
  } catch (e) {
    render(`<div class="alert alert-danger">Failed to load dashboard: ${escHtml(e.message)}</div>`);
    return;
  }

  const projects = data.projects || [];

  // Collect blockers across all projects in parallel
  let allBlockers = [];
  try {
    const blockerArrays = await Promise.all(
      projects.map(p => api.get(`/projects/${encodeURIComponent(p.prefix)}/blockers?open=true`).catch(() => []))
    );
    blockerArrays.forEach((arr, i) => {
      (arr || []).forEach(b => { allBlockers.push({ ...b, _prefix: projects[i].prefix }); });
    });
  } catch (_) { /* non-fatal */ }

  // Check for expiring decisions (revisit_by within 7 days)
  let expiringDecisions = [];
  try {
    const decisionArrays = await Promise.all(
      projects.map(p => api.get(`/projects/${encodeURIComponent(p.prefix)}/decisions?expiring=true`).catch(() => []))
    );
    decisionArrays.forEach((arr, i) => {
      (arr || []).forEach(d => { expiringDecisions.push({ ...d, _prefix: projects[i].prefix }); });
    });
  } catch (_) { /* non-fatal */ }

  // ---- Alerts ----
  let alertsHtml = '';

  if (expiringDecisions.length > 0) {
    const list = expiringDecisions.slice(0, 3).map(d => {
      const days = daysUntil(d.revisit_by);
      const when = days === 0 ? 'today' : days < 0 ? `${Math.abs(days)}d overdue` : `in ${days}d`;
      return `<b>${escHtml(d._prefix)}</b> — ${escHtml(d.title)} (${when})`;
    }).join('; ');
    const more = expiringDecisions.length > 3 ? ` +${expiringDecisions.length - 3} more` : '';
    alertsHtml += `
      <div class="alert alert-warning">
        <span>⚠</span>
        <span>Decisions need review: ${list}${more}</span>
      </div>`;
  }

  // ---- Project cards ----
  let cardsHtml = '';
  if (projects.length === 0) {
    cardsHtml = `
      <div class="empty-state" style="grid-column:1/-1">
        <div class="empty-state-icon">📋</div>
        <div class="empty-state-title">No projects yet</div>
        <div class="empty-state-body">Create your first project with <code>track project add</code></div>
      </div>`;
  } else {
    cardsHtml = projects.map(p => {
      const c = p.counts || {};
      const total = c.total || 0;
      const done = c.done || 0;
      const pct = total > 0 ? Math.round((done / total) * 100) : 0;
      // health score approximation from counts (server doesn't send it in dashboard)
      const healthScore = total > 0 ? Math.max(0, Math.min(100, pct - (c.blocked || 0) * 10)) : 50;

      return `
        <a class="project-card" href="#/project/${encodeURIComponent(p.prefix)}/board">
          <div class="project-card-header">
            <span class="project-prefix">${escHtml(p.prefix)}</span>
            ${healthDots(healthScore)}
          </div>
          <div class="project-name">${escHtml(p.name)}</div>
          <div class="project-meta">
            ${phaseBadge(p.phase)}
            <span class="text-muted" style="font-size:12px">${pct}% done</span>
          </div>
          <div class="counts-row">
            <span class="count-chip done"><span class="count-num">${c.done || 0}</span> done</span>
            <span class="count-chip wip"><span class="count-num">${c.in_progress || 0}</span> wip</span>
            <span class="count-chip todo"><span class="count-num">${c.todo || 0}</span> todo</span>
            ${(c.waiting || 0) > 0 ? `<span class="count-chip waiting"><span class="count-num">${c.waiting}</span> waiting</span>` : ''}
            ${(c.blocked || 0) > 0 ? `<span class="count-chip blocked"><span class="count-num">${c.blocked}</span> blocked</span>` : ''}
          </div>
        </a>`;
    }).join('');
  }

  // ---- Blockers ----
  let blockersHtml = '';
  if (allBlockers.length > 0) {
    const items = allBlockers.map(b => `
      <div class="blocker-item">
        <div class="blocker-info">
          <div class="blocker-title">${escHtml(b.title)}</div>
          <div class="blocker-meta">
            <span class="mono">${escHtml(b._prefix)}</span>
            <span>${escHtml(b.blocker_type || '')}</span>
            ${b.owner ? `<span>Owner: ${escHtml(b.owner)}</span>` : ''}
            ${b.escalation_date ? `<span class="text-warning">Escalate: ${fmtDate(b.escalation_date)}</span>` : ''}
          </div>
          ${b.notes ? `<div class="decision-body mt-8">${escHtml(b.notes)}</div>` : ''}
        </div>
      </div>`).join('');

    blockersHtml = `
      <div class="blockers-section">
        <div class="section-title text-danger">Open Blockers (${allBlockers.length})</div>
        <div class="blocker-list">${items}</div>
      </div>`;
  }

  render(`
    <div>
      <div class="page-header">
        <div>
          <div class="page-title">Dashboard</div>
          <div class="page-subtitle">${projects.length} project${projects.length !== 1 ? 's' : ''}</div>
        </div>
      </div>
      ${alertsHtml}
      <div class="dashboard-grid">${cardsHtml}</div>
      ${blockersHtml}
    </div>
  `);
}

// =========================================================
// Route registrations
// =========================================================

router.register('/', () => renderDashboard());
router.register('/project/:prefix/focus',    (p) => renderFocus(p.prefix));
router.register('/project/:prefix/board',    (p) => renderKanban(p.prefix));
router.register('/project/:prefix/timeline', (p) => renderTimeline(p.prefix));
router.register('/project/:prefix/tree',     (p) => renderTree(p.prefix));
router.register('/project/:prefix/graph',    (p) => renderGraph(p.prefix));
router.register('/sessions',  () => renderSessions());
router.register('/knowledge', () => renderKnowledge());
router.register('/insights',  () => renderInsights());

// Boot once the document and all view-module scripts have loaded. Done here
// (not via an inline <script>) so the Content-Security-Policy can forbid inline
// scripts. DOMContentLoaded fires after all synchronous <script> tags execute,
// so every render* global is defined by the time we dispatch the first route.
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => router.start());
} else {
  router.start();
}
