package web

import "embed"

//go:embed static/index.html static/style.css static/d3.min.js static/app.js static/task-modal.js static/focus.js static/kanban.js static/timeline.js static/tree.js static/graph.js static/insights.js static/sessions.js static/knowledge.js
var StaticFiles embed.FS
