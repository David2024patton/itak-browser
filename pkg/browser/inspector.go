// Package browser - Element Inspector panel.
//
// What: Interactive side panel that shows the DOM tree, computed CSS properties,
//       and event listeners for any element the user clicks on.
// Why:  Core teaching tool for web development. Shows structure and styling
//       in context without opening Chrome DevTools.
// How:  JS injects a panel + click-to-inspect mode. Clicking an element
//       populates the panel with tag info, attributes, CSS, and box model.
package browser

import (
	"context"

	"github.com/chromedp/chromedp"
)

// InspectorOpen opens the element inspector panel and enables click-to-inspect.
func (e *Engine) InspectorOpen(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `
(function() {
	if (document.getElementById('itak-inspector')) return true;

	const panel = document.createElement('div');
	panel.id = 'itak-inspector';
	panel.style.cssText =
		'position:fixed;top:0;right:0;width:360px;height:100vh;z-index:999974;' +
		'background:#0f172a;border-left:1px solid rgba(255,255,255,0.08);' +
		'overflow-y:auto;font:12px system-ui;color:#e2e8f0;' +
		'box-shadow:-4px 0 24px rgba(0,0,0,0.4);display:flex;flex-direction:column;';

	// Top bar.
	const topBar = document.createElement('div');
	topBar.style.cssText =
		'display:flex;justify-content:space-between;align-items:center;padding:10px 16px;' +
		'background:linear-gradient(180deg,#0f172a,#1e293b);border-bottom:1px solid rgba(255,255,255,0.08);flex-shrink:0;';
	topBar.innerHTML =
		'<span style="font:700 14px system-ui;background:linear-gradient(135deg,#3b82f6,#8b5cf6);' +
		'-webkit-background-clip:text;-webkit-text-fill-color:transparent">\uD83D\uDD0D Element Inspector</span>';

	const btnRow = document.createElement('div');
	btnRow.style.cssText = 'display:flex;gap:6px;';

	const pickBtn = document.createElement('button');
	pickBtn.textContent = '\uD83C\uDFAF Pick';
	pickBtn.style.cssText =
		'padding:4px 10px;border:1px solid rgba(59,130,246,0.3);border-radius:4px;' +
		'background:rgba(59,130,246,0.15);color:#60a5fa;cursor:pointer;font:600 11px system-ui;';
	pickBtn.onclick = enablePick;
	btnRow.appendChild(pickBtn);

	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716';
	closeBtn.style.cssText = 'border:none;background:none;color:#94a3b8;cursor:pointer;font-size:14px;';
	closeBtn.onclick = () => { disablePick(); panel.remove(); };
	btnRow.appendChild(closeBtn);
	topBar.appendChild(btnRow);
	panel.appendChild(topBar);

	// Content area.
	const content = document.createElement('div');
	content.id = 'itak-inspector-content';
	content.style.cssText = 'flex:1;overflow-y:auto;padding:12px 16px;';
	content.innerHTML = '<div style="color:#475569;font-style:italic;padding:20px 0;">Click \uD83C\uDFAF Pick, then click an element on the page to inspect it.</div>';
	panel.appendChild(content);

	document.body.appendChild(panel);

	// Hover highlight.
	const hoverBox = document.createElement('div');
	hoverBox.id = 'itak-inspector-hover';
	hoverBox.style.cssText =
		'position:fixed;z-index:999973;pointer-events:none;display:none;' +
		'border:2px solid #3b82f6;background:rgba(59,130,246,0.08);border-radius:2px;' +
		'transition:all 0.1s ease;';
	document.body.appendChild(hoverBox);

	let picking = false;

	function enablePick() {
		picking = true;
		pickBtn.style.background = 'rgba(59,130,246,0.4)';
		pickBtn.textContent = '\uD83C\uDFAF Picking...';
		document.body.style.cursor = 'crosshair';
	}
	function disablePick() {
		picking = false;
		pickBtn.style.background = 'rgba(59,130,246,0.15)';
		pickBtn.textContent = '\uD83C\uDFAF Pick';
		document.body.style.cursor = '';
		hoverBox.style.display = 'none';
	}

	document.addEventListener('mousemove', (e) => {
		if (!picking) return;
		const el = document.elementFromPoint(e.clientX, e.clientY);
		if (!el || el.closest('#itak-inspector') || el.id === 'itak-inspector-hover') return;
		const r = el.getBoundingClientRect();
		hoverBox.style.display = 'block';
		hoverBox.style.left = r.left + 'px';
		hoverBox.style.top = r.top + 'px';
		hoverBox.style.width = r.width + 'px';
		hoverBox.style.height = r.height + 'px';
	}, true);

	document.addEventListener('click', (e) => {
		if (!picking) return;
		const el = document.elementFromPoint(e.clientX, e.clientY);
		if (!el || el.closest('#itak-inspector')) return;
		e.preventDefault(); e.stopPropagation();
		disablePick();
		inspectElement(el);
	}, true);

	function inspectElement(el) {
		const content = document.getElementById('itak-inspector-content');
		content.innerHTML = '';

		function section(title) {
			const s = document.createElement('div');
			s.style.cssText = 'margin-bottom:12px;';
			const h = document.createElement('div');
			h.style.cssText =
				'font-weight:700;font-size:12px;color:#94a3b8;text-transform:uppercase;' +
				'letter-spacing:0.5px;margin-bottom:6px;padding-bottom:4px;' +
				'border-bottom:1px solid rgba(255,255,255,0.06);';
			h.textContent = title;
			s.appendChild(h);
			content.appendChild(s);
			return s;
		}

		function prop(parent, key, val) {
			const row = document.createElement('div');
			row.style.cssText = 'display:flex;gap:6px;padding:2px 0;font-size:11px;';
			row.innerHTML =
				'<span style="color:#3b82f6;min-width:110px;font-weight:500">' + key + '</span>' +
				'<span style="color:#e2e8f0;word-break:break-all">' + (val||'').toString().replace(/</g,'&lt;').substring(0,200) + '</span>';
			parent.appendChild(row);
		}

		// Element info.
		const elSec = section('\uD83C\uDFF7 Element');
		const tag = el.tagName.toLowerCase();
		const cls = el.className ? '.' + el.className.toString().split(' ').join('.') : '';
		const id = el.id ? '#' + el.id : '';
		prop(elSec, 'Selector', tag + id + cls);
		prop(elSec, 'Tag', tag);
		if (el.id) prop(elSec, 'ID', el.id);
		if (el.className) prop(elSec, 'Classes', el.className);
		const text = (el.textContent||'').trim();
		if (text) prop(elSec, 'Text', text.substring(0,100));

		// Attributes.
		if (el.attributes.length > 0) {
			const attrSec = section('\uD83D\uDCCB Attributes (' + el.attributes.length + ')');
			for (const attr of el.attributes) {
				prop(attrSec, attr.name, attr.value);
			}
		}

		// Computed CSS.
		const cssSec = section('\uD83C\uDFA8 Computed CSS');
		const style = window.getComputedStyle(el);
		const cssProps = [
			'display','position','width','height','margin','padding',
			'font-family','font-size','font-weight','color','background-color',
			'border','border-radius','box-shadow','opacity','z-index','overflow',
			'flex-direction','align-items','justify-content','grid-template-columns'
		];
		cssProps.forEach(p => {
			const v = style.getPropertyValue(p);
			if (v && v !== 'none' && v !== 'normal' && v !== 'auto' && v !== '0px' && v !== 'visible') {
				prop(cssSec, p, v);
			}
		});

		// Box model.
		const boxSec = section('\uD83D\uDCCF Box Model');
		const rect = el.getBoundingClientRect();
		prop(boxSec, 'Position', Math.round(rect.left) + ', ' + Math.round(rect.top));
		prop(boxSec, 'Size', Math.round(rect.width) + ' \u00D7 ' + Math.round(rect.height) + 'px');
		prop(boxSec, 'Margin', style.margin);
		prop(boxSec, 'Padding', style.padding);
		prop(boxSec, 'Border', style.border);

		// Children summary.
		if (el.children.length > 0) {
			const childSec = section('\uD83C\uDF33 Children (' + el.children.length + ')');
			Array.from(el.children).slice(0, 10).forEach(child => {
				const ct = child.tagName.toLowerCase();
				const ci = child.id ? '#' + child.id : '';
				const cc = child.className ? '.' + child.className.toString().split(' ')[0] : '';
				prop(childSec, ct + ci + cc, (child.textContent||'').trim().substring(0,50));
			});
		}
	}

	enablePick();
	return true;
})()
`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// InspectorClose closes the element inspector panel.
func (e *Engine) InspectorClose(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){
		const p = document.getElementById('itak-inspector'); if(p)p.remove();
		const h = document.getElementById('itak-inspector-hover'); if(h)h.remove();
		document.body.style.cursor = '';
		return true;
	})()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
