// Package browser - AI Spotlight Mode.
//
// What: Dims the entire page except a target element, creating a presentation
//       spotlight effect. The focused element gets a bright highlight ring.
// Why:  When the AI is teaching, this draws attention to exactly the right
//       element without distractions. Much stronger than a simple highlight.
// How:  Creates a full-page dark overlay with a CSS clip-path cutout around
//       the target element's bounding box. Animates in and auto-clears.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// Spotlight dims the entire page except the element matching the given
// selector/ref, creating a dramatic teaching focus effect.
func (e *Engine) Spotlight(ctx context.Context, selector string, label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf(`
(function() {
	const sel = %q;
	const label = %q;

	// Resolve ref (e0, e1, ...) or CSS selector.
	let el;
	if (/^e\d+$/.test(sel) && window.__itak_refs) {
		el = window.__itak_refs[sel];
	} else {
		el = document.querySelector(sel);
	}
	if (!el) return "element not found: " + sel;

	// Remove any existing spotlight.
	const old = document.getElementById('itak-spotlight');
	if (old) old.remove();

	const rect = el.getBoundingClientRect();
	const pad = 8;

	// Full-page dark overlay.
	const overlay = document.createElement('div');
	overlay.id = 'itak-spotlight';
	overlay.style.cssText =
		'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:999980;' +
		'background:rgba(0,0,0,0.7);transition:opacity 0.3s ease;opacity:0;' +
		'pointer-events:none;';

	// CSS clip-path to cut out the target element area.
	const x1 = rect.left - pad;
	const y1 = rect.top - pad;
	const x2 = rect.right + pad;
	const y2 = rect.bottom + pad;
	// Inset polygon: outer box minus inner cutout.
	overlay.style.clipPath =
		'polygon(0%% 0%%, 100%% 0%%, 100%% 100%%, 0%% 100%%, 0%% 0%%, ' +
		x1 + 'px ' + y1 + 'px, ' +
		x1 + 'px ' + y2 + 'px, ' +
		x2 + 'px ' + y2 + 'px, ' +
		x2 + 'px ' + y1 + 'px, ' +
		x1 + 'px ' + y1 + 'px)';

	// Bright highlight ring around the element.
	const ring = document.createElement('div');
	ring.style.cssText =
		'position:fixed;z-index:999981;pointer-events:none;' +
		'border:3px solid #3b82f6;border-radius:6px;' +
		'box-shadow:0 0 20px rgba(59,130,246,0.6),0 0 40px rgba(59,130,246,0.3);' +
		'transition:all 0.3s ease;' +
		'left:' + (rect.left - pad) + 'px;top:' + (rect.top - pad) + 'px;' +
		'width:' + (rect.width + pad*2) + 'px;height:' + (rect.height + pad*2) + 'px;';
	overlay.appendChild(ring);

	// Optional label.
	if (label) {
		const lbl = document.createElement('div');
		lbl.textContent = label;
		lbl.style.cssText =
			'position:fixed;z-index:999982;' +
			'left:' + rect.left + 'px;top:' + (rect.bottom + pad + 8) + 'px;' +
			'background:linear-gradient(135deg,#1e40af,#3b82f6);color:#fff;' +
			'padding:6px 14px;border-radius:6px;font:600 13px system-ui;' +
			'box-shadow:0 4px 12px rgba(0,0,0,0.3);pointer-events:none;';
		overlay.appendChild(lbl);
	}

	document.body.appendChild(overlay);
	requestAnimationFrame(() => { overlay.style.opacity = '1'; });

	return "ok";
})()
`, selector, label)

	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("spotlight: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("spotlight: %s", result)
	}
	return nil
}

// SpotlightClear removes the spotlight overlay.
func (e *Engine) SpotlightClear(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){
		const s = document.getElementById('itak-spotlight');
		if (s) { s.style.opacity='0'; setTimeout(()=>s.remove(), 300); }
		return true;
	})()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
