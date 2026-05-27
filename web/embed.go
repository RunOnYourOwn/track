package web

import "embed"

//go:embed static/index.html static/style.css static/app.js static/kanban.js static/timeline.js static/tree.js static/graph.js static/insights.js static/sessions.js static/knowledge.js
var StaticFiles embed.FS
