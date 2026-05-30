/* =========================================================
   Track — Tabler icon set (subset)
   Exposes: window.icon(name, opts) -> SVG markup string.
   Source: https://github.com/tabler/tabler-icons (MIT)
   ========================================================= */

'use strict';

// name -> {p: inner-svg, f: filled?} (filled icons render with fill=currentColor
// and no stroke; outline icons render with stroke=currentColor + fill=none.)
const TABLER = {
  'affiliate': { p: '<path d="M5.931 6.936l1.275 4.249m5.607 5.609l4.251 1.275" /> <path d="M11.683 12.317l5.759 -5.759" /> <path d="M4 5.5a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0" /> <path d="M17 5.5a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0" /> <path d="M17 18.5a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0 -3 0" /> <path d="M4 15.5a4.5 4.5 0 1 0 9 0a4.5 4.5 0 1 0 -9 0" />' },
  'alert-triangle': { p: '<path d="M12 9v4" /> <path d="M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0" /> <path d="M12 16h.01" />' },
  'arrow-left': { p: '<path d="M5 12l14 0" /> <path d="M5 12l6 6" /> <path d="M5 12l6 -6" />' },
  'binary-tree': { p: '<path d="M6 20a2 2 0 1 0 -4 0a2 2 0 0 0 4 0" /> <path d="M16 4a2 2 0 1 0 -4 0a2 2 0 0 0 4 0" /> <path d="M16 20a2 2 0 1 0 -4 0a2 2 0 0 0 4 0" /> <path d="M11 12a2 2 0 1 0 -4 0a2 2 0 0 0 4 0" /> <path d="M21 12a2 2 0 1 0 -4 0a2 2 0 0 0 4 0" /> <path d="M5.058 18.306l2.88 -4.606" /> <path d="M10.061 10.303l2.877 -4.604" /> <path d="M10.065 13.705l2.876 4.6" /> <path d="M15.063 5.7l2.881 4.61" />' },
  'book': { p: '<path d="M3 19a9 9 0 0 1 9 0a9 9 0 0 1 9 0" /> <path d="M3 6a9 9 0 0 1 9 0a9 9 0 0 1 9 0" /> <path d="M3 6l0 13" /> <path d="M12 6l0 13" /> <path d="M21 6l0 13" />' },
  'chart-bar': { p: '<path d="M3 13a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v6a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1l0 -6" /> <path d="M15 9a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v10a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1l0 -10" /> <path d="M9 5a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v14a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1l0 -14" /> <path d="M4 20h14" />' },
  'check': { p: '<path d="M5 12l5 5l10 -10" />' },
  'chevron-down': { p: '<path d="M6 9l6 6l6 -6" />' },
  'chevron-left': { p: '<path d="M15 6l-6 6l6 6" />' },
  'chevron-right': { p: '<path d="M9 6l6 6l-6 6" />' },
  'chevron-up': { p: '<path d="M6 15l6 -6l6 6" />' },
  'circle-dot': { p: '<path d="M11 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0" />' },
  'clock': { p: '<path d="M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0" /> <path d="M12 7v5l3 3" />' },
  'dots': { p: '<path d="M4 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M11 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M18 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" />' },
  'eye': { p: '<path d="M10 12a2 2 0 1 0 4 0a2 2 0 0 0 -4 0" /> <path d="M21 12c-2.4 4 -5.4 6 -9 6c-3.6 0 -6.6 -2 -9 -6c2.4 -4 5.4 -6 9 -6c3.6 0 6.6 2 9 6" />' },
  'git-branch': { p: '<path d="M5 18a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" /> <path d="M5 6a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" /> <path d="M15 6a2 2 0 1 0 4 0a2 2 0 1 0 -4 0" /> <path d="M7 8l0 8" /> <path d="M9 18h6a2 2 0 0 0 2 -2v-5" /> <path d="M14 14l3 -3l3 3" />' },
  'git-commit': { p: '<path d="M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0" /> <path d="M12 3l0 6" /> <path d="M12 15l0 6" />' },
  'grip-vertical': { p: '<path d="M8 5a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M8 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M8 19a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M14 5a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M14 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M14 19a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" />' },
  'history': { p: '<path d="M12 8l0 4l2 2" /> <path d="M3.05 11a9 9 0 1 1 .5 4m-.5 5v-5h5" />' },
  'home': { p: '<path d="M5 12l-2 0l9 -9l9 9l-2 0" /> <path d="M5 12v7a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-7" /> <path d="M9 21v-6a2 2 0 0 1 2 -2h2a2 2 0 0 1 2 2v6" />' },
  'inbox': { p: '<path d="M4 6a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2l0 -12" /> <path d="M4 13h3l3 3h4l3 -3h3" />' },
  'layout-kanban': { p: '<path d="M4 4l6 0" /> <path d="M14 4l6 0" /> <path d="M4 10a2 2 0 0 1 2 -2h2a2 2 0 0 1 2 2v8a2 2 0 0 1 -2 2h-2a2 2 0 0 1 -2 -2l0 -8" /> <path d="M14 10a2 2 0 0 1 2 -2h2a2 2 0 0 1 2 2v2a2 2 0 0 1 -2 2h-2a2 2 0 0 1 -2 -2l0 -2" />' },
  'package': { p: '<path d="M12 3l8 4.5l0 9l-8 4.5l-8 -4.5l0 -9l8 -4.5" /> <path d="M12 12l8 -4.5" /> <path d="M12 12l0 9" /> <path d="M12 12l-8 -4.5" /> <path d="M16 5.25l-8 4.5" />' },
  'player-play-filled': { p: '<path d="M6 4v16a1 1 0 0 0 1.524 .852l13 -8a1 1 0 0 0 0 -1.704l-13 -8a1 1 0 0 0 -1.524 .852z" />', f: 1 },
  'point': { p: '<path d="M8 12a4 4 0 1 0 8 0a4 4 0 1 0 -8 0" />' },
  'target': { p: '<path d="M11 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M7 12a5 5 0 1 0 10 0a5 5 0 1 0 -10 0" /> <path d="M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0" />' },
  'timeline': { p: '<path d="M4 16l6 -7l5 5l5 -6" /> <path d="M14 14a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M9 9a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M3 16a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" /> <path d="M19 8a1 1 0 1 0 2 0a1 1 0 1 0 -2 0" />' },
};

// icon(name, {size, cls, title}) -> svg string. Drop-in for template literals.
// Defaults: 16px, no extra class. Use cls to tint via CSS (currentColor).
function icon(name, opts) {
  const ic = TABLER[name];
  if (!ic) return '';
  const o = opts || {};
  const size = o.size || 16;
  const cls = o.cls ? ('icon ' + o.cls) : 'icon';
  const title = o.title ? `<title>${o.title}</title>` : '';
  const style = ic.f
    ? `fill="currentColor" stroke="none"`
    : `fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`;
  return `<svg class="${cls}" width="${size}" height="${size}" viewBox="0 0 24 24" ${style} aria-hidden="true" focusable="false">${title}${ic.p}</svg>`;
}

window.icon = icon;
