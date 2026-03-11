// Package browser - Performance waterfall panel.
//
// What: Bottom panel displaying network request timeline as a waterfall chart
//       showing resource URLs, types, sizes, durations, and load times.
// Why:  Teaching performance optimization requires seeing what the browser
//       loads and how long each resource takes.
// How:  Uses the Performance API (performance.getEntriesByType('resource'))
//       to gather all network requests and renders them as horizontal bars
//       scaled to the page load timeline.
package browser

import (
	"context"

	"github.com/chromedp/chromedp"
)

// WaterfallOpen opens the performance waterfall panel at the bottom.
func (e *Engine) WaterfallOpen(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `
(function() {
	const old = document.getElementById('itak-waterfall');
	if (old) { old.remove(); return true; }

	const entries = performance.getEntriesByType('resource');
	const navEntries = performance.getEntriesByType('navigation');
	const totalTime = navEntries.length > 0 ? navEntries[0].loadEventEnd : (entries.length > 0 ? Math.max(...entries.map(e=>e.responseEnd)) : 1000);

	const panel = document.createElement('div');
	panel.id = 'itak-waterfall';
	panel.style.cssText =
		'position:fixed;bottom:0;left:0;right:0;height:280px;z-index:999972;' +
		'background:#0f172a;border-top:1px solid rgba(255,255,255,0.08);' +
		'font:11px system-ui;color:#e2e8f0;display:flex;flex-direction:column;' +
		'box-shadow:0 -4px 24px rgba(0,0,0,0.4);';

	// Header.
	const hdr = document.createElement('div');
	hdr.style.cssText =
		'display:flex;justify-content:space-between;align-items:center;padding:8px 16px;' +
		'background:#1e293b;border-bottom:1px solid rgba(255,255,255,0.06);flex-shrink:0;';

	const totalSize = entries.reduce((sum, e) => sum + (e.transferSize || 0), 0);
	hdr.innerHTML =
		'<span style="font:700 13px system-ui;color:#e2e8f0">\u26A1 Performance Waterfall</span>' +
		'<div style="display:flex;gap:16px;align-items:center;font-size:11px">' +
		'<span style="color:#3b82f6">' + entries.length + ' resources</span>' +
		'<span style="color:#22c55e">' + formatBytes(totalSize) + '</span>' +
		'<span style="color:#f59e0b">' + Math.round(totalTime) + 'ms total</span>' +
		'</div>';

	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716';
	closeBtn.style.cssText = 'border:none;background:none;color:#94a3b8;cursor:pointer;font-size:14px;';
	closeBtn.onclick = () => panel.remove();
	hdr.appendChild(closeBtn);
	panel.appendChild(hdr);

	// Column headers.
	const colHdr = document.createElement('div');
	colHdr.style.cssText =
		'display:flex;padding:4px 16px;background:rgba(255,255,255,0.02);' +
		'border-bottom:1px solid rgba(255,255,255,0.04);font-size:10px;color:#64748b;font-weight:600;';
	colHdr.innerHTML =
		'<span style="width:280px;flex-shrink:0">Resource</span>' +
		'<span style="width:60px;flex-shrink:0">Type</span>' +
		'<span style="width:70px;flex-shrink:0">Size</span>' +
		'<span style="width:50px;flex-shrink:0">Time</span>' +
		'<span style="flex:1">Timeline</span>';
	panel.appendChild(colHdr);

	// Scrollable body.
	const body = document.createElement('div');
	body.style.cssText = 'flex:1;overflow-y:auto;';

	const typeColors = {
		script: '#f59e0b', css: '#8b5cf6', img: '#22c55e', font: '#ec4899',
		fetch: '#3b82f6', xmlhttprequest: '#3b82f6', other: '#64748b',
		link: '#8b5cf6', beacon: '#06b6d4', video: '#ef4444', audio: '#ef4444'
	};

	entries.sort((a,b) => a.startTime - b.startTime).forEach(entry => {
		const row = document.createElement('div');
		row.style.cssText =
			'display:flex;align-items:center;padding:3px 16px;' +
			'border-bottom:1px solid rgba(255,255,255,0.02);' +
			'transition:background 0.1s;cursor:default;';
		row.onmouseenter = () => { row.style.background = 'rgba(255,255,255,0.03)'; };
		row.onmouseleave = () => { row.style.background = ''; };

		const url = entry.name.split('/').pop().split('?')[0] || entry.name;
		const type = entry.initiatorType || 'other';
		const color = typeColors[type] || typeColors.other;
		const size = entry.transferSize || 0;
		const duration = Math.round(entry.responseEnd - entry.startTime);
		const startPct = (entry.startTime / totalTime) * 100;
		const widthPct = Math.max(0.5, ((entry.responseEnd - entry.startTime) / totalTime) * 100);

		row.innerHTML =
			'<span style="width:280px;flex-shrink:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#94a3b8" title="' +
			entry.name.replace(/"/g,'&quot;') + '">' + url + '</span>' +
			'<span style="width:60px;flex-shrink:0;color:' + color + ';font-weight:600">' + type + '</span>' +
			'<span style="width:70px;flex-shrink:0;color:#e2e8f0">' + formatBytes(size) + '</span>' +
			'<span style="width:50px;flex-shrink:0;color:' + (duration > 500 ? '#ef4444' : '#22c55e') + '">' + duration + 'ms</span>' +
			'<div style="flex:1;height:12px;background:rgba(255,255,255,0.03);border-radius:2px;position:relative;overflow:hidden">' +
			'<div style="position:absolute;left:' + startPct + '%;width:' + widthPct + '%;height:100%;' +
			'background:' + color + ';border-radius:2px;opacity:0.7"></div></div>';
		body.appendChild(row);
	});

	panel.appendChild(body);
	document.body.appendChild(panel);

	function formatBytes(bytes) {
		if (bytes === 0) return '0 B';
		if (bytes < 1024) return bytes + ' B';
		if (bytes < 1024*1024) return (bytes/1024).toFixed(1) + ' KB';
		return (bytes/1024/1024).toFixed(1) + ' MB';
	}

	return true;
})()
`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// WaterfallClose closes the performance waterfall panel.
func (e *Engine) WaterfallClose(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){ const w = document.getElementById('itak-waterfall'); if(w) w.remove(); return true; })()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
