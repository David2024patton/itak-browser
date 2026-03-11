// Package browser - Multi-Tab Dashboard with visual tab strip.
//
// What: An injected tab strip showing all open session tabs with favicons,
//       titles, and click-to-switch. Displayed below the toolbar.
// Why:  Multi-tab workflows (research, comparison) need visual tab management
//       without relying on Chrome's native tab bar (which is above our toolbar).
// How:  JS injection creates a horizontal tab strip. Tabs are populated from
//       window.__itak_tabs (set by Go side) with click handlers that call
//       back to the daemon to switch tabs.
package browser

import (
	"context"

	"github.com/chromedp/chromedp"
)

// TabDashboardOpen injects the visual tab strip.
func (e *Engine) TabDashboardOpen(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `
(function() {
	const old = document.getElementById('itak-tabstrip');
	if (old) old.remove();

	const strip = document.createElement('div');
	strip.id = 'itak-tabstrip';
	strip.style.cssText =
		'position:fixed;top:42px;left:0;right:0;height:32px;z-index:999998;' +
		'background:#1e293b;border-bottom:1px solid rgba(255,255,255,0.06);' +
		'display:flex;align-items:center;gap:2px;padding:0 8px;overflow-x:auto;' +
		'font:11px system-ui;';

	// Push body down more.
	document.body.style.marginTop = '74px';

	// Get tab data from Go side.
	const tabs = window.__itak_tabs || [{title: document.title || 'Tab', url: location.href, active: true}];

	tabs.forEach((tab, idx) => {
		const t = document.createElement('div');
		t.style.cssText =
			'display:flex;align-items:center;gap:5px;padding:4px 12px;border-radius:4px 4px 0 0;' +
			'cursor:pointer;white-space:nowrap;max-width:200px;overflow:hidden;' +
			'background:' + (tab.active ? 'rgba(255,255,255,0.08)' : 'transparent') + ';' +
			'color:' + (tab.active ? '#e2e8f0' : '#64748b') + ';' +
			'border-bottom:2px solid ' + (tab.active ? '#3b82f6' : 'transparent') + ';' +
			'transition:all 0.15s ease;font-weight:' + (tab.active ? '600' : '400') + ';';

		// Favicon.
		const fav = document.createElement('img');
		fav.src = 'https://www.google.com/s2/favicons?domain=' + new URL(tab.url).hostname + '&sz=16';
		fav.style.cssText = 'width:12px;height:12px;border-radius:2px;';
		fav.onerror = () => { fav.style.display = 'none'; };
		t.appendChild(fav);

		// Title.
		const title = document.createElement('span');
		title.textContent = (tab.title || 'Tab').substring(0, 25);
		title.style.cssText = 'overflow:hidden;text-overflow:ellipsis;';
		t.appendChild(title);

		// Close button for non-active tabs.
		const cls = document.createElement('span');
		cls.textContent = '\u00D7';
		cls.style.cssText =
			'margin-left:4px;color:#64748b;font-size:14px;cursor:pointer;line-height:1;' +
			'opacity:0;transition:opacity 0.15s;';
		t.onmouseenter = () => { cls.style.opacity = '1'; if (!tab.active) t.style.background = 'rgba(255,255,255,0.04)'; };
		t.onmouseleave = () => { cls.style.opacity = '0'; if (!tab.active) t.style.background = 'transparent'; };
		cls.onclick = (e) => { e.stopPropagation(); window.__itak_tab_close = idx; };
		t.appendChild(cls);

		t.onclick = () => { window.__itak_tab_switch = idx; };
		strip.appendChild(t);
	});

	// New tab button.
	const newTabBtn = document.createElement('div');
	newTabBtn.textContent = '+';
	newTabBtn.style.cssText =
		'padding:4px 8px;cursor:pointer;color:#64748b;font-size:14px;font-weight:700;' +
		'border-radius:4px;transition:all 0.15s;';
	newTabBtn.onmouseenter = () => { newTabBtn.style.color = '#e2e8f0'; newTabBtn.style.background = 'rgba(255,255,255,0.05)'; };
	newTabBtn.onmouseleave = () => { newTabBtn.style.color = '#64748b'; newTabBtn.style.background = ''; };
	newTabBtn.onclick = () => { window.__itak_tab_new = true; };
	strip.appendChild(newTabBtn);

	document.body.appendChild(strip);
	return true;
})()
`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// TabDashboardClose removes the tab strip.
func (e *Engine) TabDashboardClose(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){
		const s = document.getElementById('itak-tabstrip'); if(s) s.remove();
		document.body.style.marginTop = '42px';
		return true;
	})()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
