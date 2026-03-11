// Package browser - Cookie/Storage Inspector panel.
//
// What: Visual side panel showing cookies, localStorage, and sessionStorage
//       with the ability to view, edit, and delete entries.
// Why:  Teaching web dev requires understanding storage. This makes it
//       visible and interactive without opening DevTools.
// How:  JS injection reads document.cookie, localStorage, sessionStorage,
//       renders a dark side panel with tables and inline edit/delete buttons.
package browser

import (
	"context"

	"github.com/chromedp/chromedp"
)

// StorageInspect opens the cookie/storage inspector panel.
func (e *Engine) StorageInspect(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `
(function() {
	const old = document.getElementById('itak-storage-panel');
	if (old) { old.remove(); return true; }

	const panel = document.createElement('div');
	panel.id = 'itak-storage-panel';
	panel.style.cssText =
		'position:fixed;top:0;right:0;width:380px;height:100vh;z-index:999975;' +
		'background:#0f172a;border-left:1px solid rgba(255,255,255,0.08);' +
		'overflow-y:auto;font:12px system-ui;color:#e2e8f0;' +
		'box-shadow:-4px 0 24px rgba(0,0,0,0.4);';

	function header(title) {
		const h = document.createElement('div');
		h.style.cssText =
			'padding:10px 16px;background:#1e293b;border-bottom:1px solid rgba(255,255,255,0.06);' +
			'font-weight:700;font-size:13px;display:flex;justify-content:space-between;align-items:center;';
		h.textContent = title;
		return h;
	}

	function row(key, val, onDelete, onEdit) {
		const r = document.createElement('div');
		r.style.cssText =
			'display:flex;align-items:center;padding:6px 16px;gap:8px;' +
			'border-bottom:1px solid rgba(255,255,255,0.04);';
		r.innerHTML =
			'<span style="color:#3b82f6;font-weight:600;min-width:100px;word-break:break-all">' +
			key.replace(/</g,'&lt;') + '</span>' +
			'<span style="flex:1;color:#94a3b8;word-break:break-all;font-size:11px">' +
			(val||'').substring(0,120).replace(/</g,'&lt;') + '</span>';

		const editBtn = document.createElement('button');
		editBtn.textContent = '\u270F';
		editBtn.title = 'Edit';
		editBtn.style.cssText = 'border:none;background:none;color:#f59e0b;cursor:pointer;font-size:12px;padding:2px 4px;';
		editBtn.onclick = onEdit;
		r.appendChild(editBtn);

		const delBtn = document.createElement('button');
		delBtn.textContent = '\u2716';
		delBtn.title = 'Delete';
		delBtn.style.cssText = 'border:none;background:none;color:#ef4444;cursor:pointer;font-size:11px;padding:2px 4px;';
		delBtn.onclick = onDelete;
		r.appendChild(delBtn);
		return r;
	}

	// Top bar with close.
	const topBar = document.createElement('div');
	topBar.style.cssText =
		'display:flex;justify-content:space-between;align-items:center;padding:10px 16px;' +
		'background:linear-gradient(180deg,#0f172a,#1e293b);border-bottom:1px solid rgba(255,255,255,0.08);';
	topBar.innerHTML =
		'<span style="font:700 15px system-ui;background:linear-gradient(135deg,#3b82f6,#8b5cf6);' +
		'-webkit-background-clip:text;-webkit-text-fill-color:transparent">Storage Inspector</span>';
	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716';
	closeBtn.style.cssText = 'border:none;background:none;color:#94a3b8;cursor:pointer;font-size:14px;';
	closeBtn.onclick = () => panel.remove();
	topBar.appendChild(closeBtn);
	panel.appendChild(topBar);

	// --- Cookies ---
	panel.appendChild(header('\uD83C\uDF6A Cookies (' + (document.cookie ? document.cookie.split(';').length : 0) + ')'));
	if (document.cookie) {
		document.cookie.split(';').forEach(c => {
			const parts = c.trim().split('=');
			const k = parts[0];
			const v = parts.slice(1).join('=');
			panel.appendChild(row(k, v,
				() => { document.cookie = k + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/'; refresh(); },
				() => { const nv = prompt('Edit value for ' + k + ':', v); if (nv !== null) { document.cookie = k + '=' + nv + ';path=/'; refresh(); } }
			));
		});
	} else {
		const empty = document.createElement('div');
		empty.style.cssText = 'padding:12px 16px;color:#475569;font-style:italic;';
		empty.textContent = 'No cookies set';
		panel.appendChild(empty);
	}

	// --- localStorage ---
	panel.appendChild(header('\uD83D\uDCE6 localStorage (' + localStorage.length + ')'));
	for (let i = 0; i < localStorage.length; i++) {
		const k = localStorage.key(i);
		const v = localStorage.getItem(k);
		panel.appendChild(row(k, v,
			() => { localStorage.removeItem(k); refresh(); },
			() => { const nv = prompt('Edit value for ' + k + ':', v); if (nv !== null) { localStorage.setItem(k, nv); refresh(); } }
		));
	}
	if (localStorage.length === 0) {
		const empty = document.createElement('div');
		empty.style.cssText = 'padding:12px 16px;color:#475569;font-style:italic;';
		empty.textContent = 'No localStorage entries';
		panel.appendChild(empty);
	}

	// --- sessionStorage ---
	panel.appendChild(header('\uD83D\uDCC4 sessionStorage (' + sessionStorage.length + ')'));
	for (let i = 0; i < sessionStorage.length; i++) {
		const k = sessionStorage.key(i);
		const v = sessionStorage.getItem(k);
		panel.appendChild(row(k, v,
			() => { sessionStorage.removeItem(k); refresh(); },
			() => { const nv = prompt('Edit value for ' + k + ':', v); if (nv !== null) { sessionStorage.setItem(k, nv); refresh(); } }
		));
	}
	if (sessionStorage.length === 0) {
		const empty = document.createElement('div');
		empty.style.cssText = 'padding:12px 16px;color:#475569;font-style:italic;';
		empty.textContent = 'No sessionStorage entries';
		panel.appendChild(empty);
	}

	function refresh() { panel.remove(); setTimeout(arguments.callee, 0); }

	document.body.appendChild(panel);
	return true;
})()
`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// StorageClose closes the storage inspector panel.
func (e *Engine) StorageClose(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){ const p = document.getElementById('itak-storage-panel'); if(p)p.remove(); return true; })()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
