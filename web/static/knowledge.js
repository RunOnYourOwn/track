// knowledge.js — decisions and learnings knowledge base
// Requires: api global, render global, fmtDate global

async function renderKnowledge() {
  render(`
    <div class="page-knowledge">
      <div class="page-header">
        <h2>Knowledge Base</h2>
        <p class="page-subtitle" style="color:#8b949e;margin:4px 0 0">
          Decisions and learnings across all projects
        </p>
      </div>

      <div class="tab-bar" style="display:flex;gap:0;border-bottom:1px solid #30363d;margin-bottom:20px">
        <button class="tab-btn active" data-tab="decisions">
          Decisions
        </button>
        <button class="tab-btn" data-tab="learnings">
          Learnings
        </button>
      </div>

      <div id="kb-decisions-panel">
        <div id="kb-decisions-loading" class="loading-state">Loading decisions…</div>
      </div>

      <div id="kb-learnings-panel" style="display:none">
        <div id="kb-learnings-loading" class="loading-state">Loading learnings…</div>
      </div>
    </div>
  `);

  // Wire tab buttons (no inline handlers — CSP blocks them).
  document.querySelectorAll('.tab-bar .tab-btn').forEach(btn => {
    btn.addEventListener('click', () => knowledgeSwitchTab(btn.dataset.tab));
  });

  // Load both in parallel
  let projects;
  try {
    projects = await api.get('/projects');
  } catch (e) {
    document.getElementById('kb-decisions-loading').outerHTML =
      `<div class="empty-state">Failed to load projects: ${escHtml((e).message)}</div>`;
    return;
  }

  if (!projects || projects.length === 0) {
    document.getElementById('kb-decisions-loading').outerHTML =
      `<div class="empty-state">No projects found.</div>`;
    document.getElementById('kb-learnings-loading').outerHTML = '';
    return;
  }

  // Fetch decisions + learnings in parallel
  const [decisionResults, learningResults] = await Promise.all([
    Promise.allSettled(projects.map(p => api.get(`/projects/${p.prefix}/decisions`))),
    Promise.allSettled(projects.map(p => api.get(`/projects/${p.prefix}/learnings`))),
  ]);

  // Merge with prefix tag
  const allDecisions = [];
  const allLearnings = [];

  decisionResults.forEach((r, i) => {
    if (r.status === 'fulfilled' && Array.isArray(r.value)) {
      r.value.forEach(d => allDecisions.push({ ...d, _prefix: projects[i].prefix }));
    }
  });

  learningResults.forEach((r, i) => {
    if (r.status === 'fulfilled' && Array.isArray(r.value)) {
      r.value.forEach(l => allLearnings.push({ ...l, _prefix: projects[i].prefix }));
    }
  });

  // Sort decisions: open first, then by created_at desc
  allDecisions.sort((a, b) => {
    const order = { open: 0, decided: 1, expired: 2, superseded: 3 };
    const oa = order[a.status] ?? 9;
    const ob = order[b.status] ?? 9;
    if (oa !== ob) return oa - ob;
    return new Date(b.created_at) - new Date(a.created_at);
  });

  // Sort learnings newest first
  allLearnings.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));

  // Render decisions panel
  renderDecisionsPanel(allDecisions, projects);

  // Render learnings panel
  renderLearningsPanel(allLearnings, projects);
}

// ── Tab switching ─────────────────────────────────────────────────────────────

function knowledgeSwitchTab(tab) {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.tab === tab);
  });
  document.getElementById('kb-decisions-panel').style.display =
    tab === 'decisions' ? '' : 'none';
  document.getElementById('kb-learnings-panel').style.display =
    tab === 'learnings' ? '' : 'none';
}

// ── Decisions panel ───────────────────────────────────────────────────────────

function renderDecisionsPanel(decisions, projects) {
  const panel = document.getElementById('kb-decisions-panel');

  const nowMs = Date.now();
  const sevenDaysMs = 7 * 24 * 60 * 60 * 1000;

  function isExpiringSoon(d) {
    if (!d.revisit_by) return false;
    const revisitMs = new Date(d.revisit_by).getTime();
    return revisitMs > nowMs && revisitMs - nowMs <= sevenDaysMs;
  }

  // Status filter state
  const activeStatuses = new Set(['open', 'decided', 'expired', 'superseded']);

  function renderDecisionList() {
    const filtered = decisions.filter(d => activeStatuses.has(d.status));
    const list = document.getElementById('kb-decision-list');
    if (!list) return;

    if (filtered.length === 0) {
      list.innerHTML = `<div class="empty-state">No decisions match the current filters.</div>`;
      return;
    }

    list.innerHTML = filtered.map(d => decisionCard(d, isExpiringSoon(d))).join('');
  }

  const statuses = ['open', 'decided', 'expired', 'superseded'];
  const checkboxes = statuses.map(s => `
    <label class="filter-check" style="display:inline-flex;align-items:center;gap:6px;cursor:pointer">
      <input type="checkbox" value="${s}" checked data-status-filter
        style="accent-color:#58a6ff">
      <span class="badge badge-decision-status badge-decision-${s}">${capitalize(s)}</span>
    </label>
  `).join('');

  panel.innerHTML = `
    <div class="decisions-filters" style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin-bottom:16px">
      <span style="color:#8b949e;font-size:13px">Show:</span>
      ${checkboxes}
    </div>
    <div id="kb-decision-list"></div>
  `;

  // Wire status filters via listeners (no inline handlers — CSP blocks them).
  panel.querySelectorAll('input[data-status-filter]').forEach(cb => {
    cb.addEventListener('change', () => {
      if (cb.checked) activeStatuses.add(cb.value);
      else activeStatuses.delete(cb.value);
      renderDecisionList();
    });
  });

  renderDecisionList();
}

function decisionCard(d, expiringSoon) {
  const statusColors = {
    open:       { bg: '#0d419d', text: '#58a6ff' },
    decided:    { bg: '#1a4a2e', text: '#3fb950' },
    expired:    { bg: '#4a1a1a', text: '#f85149' },
    superseded: { bg: '#30363d', text: '#8b949e' },
  };
  const sc = statusColors[d.status] || statusColors.superseded;

  const expiryWarning = expiringSoon
    ? `<span title="Revisit soon" style="color:#d29922;font-size:12px;margin-left:6px">⚠ revisit due ${escHtml(d.revisit_by)}</span>`
    : '';

  const decidedSection = d.decided_at ? `
    <div class="decision-body">
      ${d.decision ? `<div class="decision-text"><strong>Decision:</strong> ${escHtml(d.decision)}</div>` : ''}
      ${d.rationale ? `<div class="decision-rationale" style="color:#8b949e;margin-top:4px"><strong>Rationale:</strong> ${escHtml(d.rationale)}</div>` : ''}
      <div style="color:#8b949e;font-size:11px;margin-top:6px">
        Decided ${escHtml(fmtDateShort(d.decided_at))}
        ${d.decided_by ? ` by ${escHtml(d.decided_by)}` : ''}
      </div>
    </div>
  ` : '';

  const contextSection = d.context ? `
    <div class="decision-context" style="color:#8b949e;font-size:13px;margin-top:6px">
      ${escHtml(d.context)}
    </div>
  ` : '';

  return `
    <div class="knowledge-card">
      <div class="knowledge-card-header">
        <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
          <span class="badge badge-prefix">${escHtml(d._prefix)}</span>
          <span class="badge"
            style="background:${sc.bg};color:${sc.text}">${escHtml(capitalize(d.status))}</span>
          <span class="knowledge-card-title">${escHtml(d.title)}</span>
          ${expiryWarning}
        </div>
      </div>
      ${contextSection}
      ${decidedSection}
    </div>`;
}

// ── Learnings panel ───────────────────────────────────────────────────────────

function renderLearningsPanel(learnings, projects) {
  const panel = document.getElementById('kb-learnings-panel');

  const categories = [
    ...new Set(learnings.map(l => l.category).filter(Boolean))
  ].sort();

  const activeCategories = new Set(categories);
  let activeProject = '';
  let searchDebounce = null;
  let currentSearch = '';

  // Category badge colors
  const categoryColors = {
    pattern:       { bg: '#0d419d', text: '#58a6ff' },
    'anti-pattern':{ bg: '#4a1a1a', text: '#f85149' },
    tooling:       { bg: '#30363d', text: '#8b949e' },
    process:       { bg: '#3d2e00', text: '#d29922' },
    domain:        { bg: '#1a4a2e', text: '#3fb950' },
    performance:   { bg: '#0d2647', text: '#58a6ff' },
    security:      { bg: '#4a1a1a', text: '#f85149' },
  };
  const catColor = cat => categoryColors[cat] || { bg: '#30363d', text: '#8b949e' };

  // Project filter options
  const projectOpts = projects.map(p =>
    `<option value="${escHtml(p.prefix)}">${escHtml(p.prefix)} — ${escHtml(p.name)}</option>`
  ).join('');

  // Category pills
  const pills = categories.map(cat => {
    const cc = catColor(cat);
    return `
      <button class="cat-pill active"
        data-cat="${escHtml(cat)}"
        style="background:${cc.bg};color:${cc.text};border:1px solid transparent;
               border-radius:12px;padding:3px 10px;font-size:12px;cursor:pointer">
        ${escHtml(capitalize(cat))}
      </button>`;
  }).join('');

  panel.innerHTML = `
    <div class="learnings-controls" style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin-bottom:16px">
      <input id="kb-search" type="search" placeholder="Search learnings…"
        class="form-input" style="flex:1;min-width:200px;max-width:360px">
      <select id="kb-project-filter" class="form-select" style="min-width:160px">
        <option value="">All projects</option>
        ${projectOpts}
      </select>
    </div>
    ${categories.length > 0 ? `
      <div class="cat-pills" style="display:flex;flex-wrap:wrap;gap:6px;margin-bottom:16px">
        ${pills}
      </div>` : ''}
    <div id="kb-learnings-list"></div>
  `;

  const toggleCat = (cat, btn) => {
    if (activeCategories.has(cat)) {
      activeCategories.delete(cat);
      btn.style.opacity = '0.4';
      btn.classList.remove('active');
    } else {
      activeCategories.add(cat);
      btn.style.opacity = '1';
      btn.classList.add('active');
    }
    renderLearningsList(learnings, activeCategories, activeProject, currentSearch, catColor);
  };

  const filterProject = val => {
    activeProject = val;
    renderLearningsList(learnings, activeCategories, activeProject, currentSearch, catColor);
  };

  const onSearch = val => {
    clearTimeout(searchDebounce);
    searchDebounce = setTimeout(async () => {
      currentSearch = val.trim();
      if (currentSearch) {
        // Fetch search results from API for all projects (use first project as fallback
        // since API is per-project; collect from all and dedupe)
        let results = [];
        await Promise.allSettled(
          projects.map(async p => {
            try {
              const r = await api.get(
                `/projects/${p.prefix}/learnings?q=${encodeURIComponent(currentSearch)}`
              );
              if (Array.isArray(r)) {
                r.forEach(l => results.push({ ...l, _prefix: p.prefix }));
              }
            } catch (_) {}
          })
        );
        // Dedupe by id
        const seen = new Set();
        const deduped = results.filter(l => {
          if (seen.has(l.id)) return false;
          seen.add(l.id);
          return true;
        });
        deduped.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
        renderLearningsList(deduped, activeCategories, activeProject, currentSearch, catColor, true);
      } else {
        renderLearningsList(learnings, activeCategories, activeProject, '', catColor);
      }
    }, 300);
  };

  // Wire controls via listeners (no inline handlers — CSP blocks them).
  panel.querySelectorAll('.cat-pill').forEach(btn => {
    btn.addEventListener('click', () => toggleCat(btn.dataset.cat, btn));
  });
  const searchEl = document.getElementById('kb-search');
  if (searchEl) searchEl.addEventListener('input', () => onSearch(searchEl.value));
  const projEl = document.getElementById('kb-project-filter');
  if (projEl) projEl.addEventListener('change', () => filterProject(projEl.value));

  // Initial render
  renderLearningsList(learnings, activeCategories, activeProject, '', catColor);
}

function renderLearningsList(learnings, activeCategories, activeProject, search, catColor, isSearchMode = false) {
  const list = document.getElementById('kb-learnings-list');
  if (!list) return;

  let filtered = learnings;

  if (!isSearchMode) {
    filtered = filtered.filter(l => !l.category || activeCategories.has(l.category));
    if (activeProject) filtered = filtered.filter(l => l._prefix === activeProject);
  } else {
    if (activeProject) filtered = filtered.filter(l => l._prefix === activeProject);
  }

  if (filtered.length === 0) {
    list.innerHTML = `<div class="empty-state">No learnings match the current filters.</div>`;
    return;
  }

  list.innerHTML = filtered.map(l => learningCard(l, catColor)).join('');
}

function learningCard(l, catColor) {
  const cc = catColor(l.category);
  const bodyShort = l.body && l.body.length > 100
    ? escHtml(l.body.slice(0, 100)) + '…'
    : escHtml(l.body || '');

  const appliesToBadges = (l.applies_to || '')
    .split(',')
    .map(s => s.trim())
    .filter(Boolean)
    .map(s => `<span class="badge" style="background:#21262d;color:#8b949e">${escHtml(s)}</span>`)
    .join(' ');

  return `
    <div class="knowledge-card">
      <div class="knowledge-card-header">
        <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
          <span class="badge badge-prefix">${escHtml(l._prefix)}</span>
          ${l.category ? `<span class="badge" style="background:${cc.bg};color:${cc.text}">${escHtml(capitalize(l.category))}</span>` : ''}
          <span class="knowledge-card-title">${escHtml(l.title)}</span>
        </div>
      </div>
      ${bodyShort ? `<div class="knowledge-card-body">${bodyShort}</div>` : ''}
      ${appliesToBadges ? `<div style="margin-top:8px;display:flex;flex-wrap:wrap;gap:4px">${appliesToBadges}</div>` : ''}
      <div style="color:#8b949e;font-size:11px;margin-top:6px">
        ${fmtDateShort(l.created_at)}
      </div>
    </div>`;
}

// ── Shared helpers ────────────────────────────────────────────────────────────

function fmtDateShort(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric'
  });
}

function capitalize(str) {
  if (!str) return '';
  return str.charAt(0).toUpperCase() + str.slice(1);
}

