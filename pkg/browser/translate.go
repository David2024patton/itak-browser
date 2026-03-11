// Package browser - Page Translation Overlay.
//
// What: Translates visible text on the page in-place with hover to show
//       the original text. Uses a simple dictionary/API approach.
// Why:  Teaching users to navigate foreign-language sites or understanding
//       international content without leaving the browser.
// How:  JS walks the DOM text nodes, replaces visible text, stores originals
//       in data attributes. Hover tooltip shows original text.
//       NOTE: This implementation uses a client-side placeholder that
//       can be wired to any translation API (Google, DeepL, etc).
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// TranslateOverlay applies a visual translation overlay to the page.
// targetLang is the ISO language code (e.g., "es", "fr", "ja").
// This creates the infrastructure; actual translation calls the browser's
// built-in capabilities or can be wired to an external API.
func (e *Engine) TranslateOverlay(ctx context.Context, targetLang string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf(`
(function() {
	const targetLang = %q;

	// Remove old overlay.
	const oldBanner = document.getElementById('itak-translate-banner');
	if (oldBanner) oldBanner.remove();

	// Banner at top.
	const banner = document.createElement('div');
	banner.id = 'itak-translate-banner';
	banner.style.cssText =
		'position:fixed;bottom:16px;left:50%%;transform:translateX(-50%%);z-index:999965;' +
		'background:rgba(15,23,42,0.95);border:1px solid rgba(255,255,255,0.1);' +
		'border-radius:10px;padding:8px 16px;display:flex;align-items:center;gap:12px;' +
		'font:12px system-ui;color:#e2e8f0;box-shadow:0 4px 24px rgba(0,0,0,0.5);' +
		'backdrop-filter:blur(12px);';

	banner.innerHTML =
		'<span style="font-size:16px">\uD83C\uDF10</span>' +
		'<span>Translation mode: <strong style="color:#3b82f6">' + targetLang.toUpperCase() + '</strong></span>' +
		'<span style="color:#64748b">|</span>' +
		'<span style="color:#94a3b8">Hover text to see original</span>';

	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716';
	closeBtn.style.cssText = 'border:none;background:none;color:#94a3b8;cursor:pointer;font-size:12px;margin-left:8px;';
	closeBtn.onclick = () => { restoreAll(); banner.remove(); };
	banner.appendChild(closeBtn);
	document.body.appendChild(banner);

	// Walk visible text nodes and mark them for translation.
	const textNodes = [];
	const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
		acceptNode: (node) => {
			if (!node.textContent.trim()) return NodeFilter.FILTER_REJECT;
			const parent = node.parentElement;
			if (!parent) return NodeFilter.FILTER_REJECT;
			if (parent.closest('#itak-toolbar,#itak-translate-banner,[id^="itak-"]')) return NodeFilter.FILTER_REJECT;
			if (['SCRIPT','STYLE','NOSCRIPT'].includes(parent.tagName)) return NodeFilter.FILTER_REJECT;
			return NodeFilter.FILTER_ACCEPT;
		}
	});

	while (walker.nextNode()) {
		textNodes.push(walker.currentNode);
	}

	// Wrap each text node in a span for hover behavior.
	let translated = 0;
	textNodes.forEach(node => {
		const text = node.textContent.trim();
		if (!text || text.length < 2) return;

		const span = document.createElement('span');
		span.className = 'itak-translated';
		span.dataset.original = text;
		span.style.cssText = 'position:relative;border-bottom:1px dashed rgba(59,130,246,0.3);cursor:help;';
		span.textContent = text; // Placeholder: real API would translate here.

		// Hover tooltip showing original.
		const tooltip = document.createElement('div');
		tooltip.style.cssText =
			'position:absolute;bottom:100%%;left:0;z-index:999966;display:none;' +
			'background:rgba(15,23,42,0.95);color:#e2e8f0;padding:6px 10px;' +
			'border-radius:6px;font:11px system-ui;white-space:nowrap;max-width:300px;' +
			'overflow:hidden;text-overflow:ellipsis;box-shadow:0 2px 12px rgba(0,0,0,0.4);' +
			'pointer-events:none;';
		tooltip.innerHTML = '<span style="color:#64748b">Original:</span> ' + text.substring(0,100).replace(/</g,'&lt;');
		span.appendChild(tooltip);

		span.onmouseenter = () => { tooltip.style.display = 'block'; };
		span.onmouseleave = () => { tooltip.style.display = 'none'; };

		node.parentNode.replaceChild(span, node);
		translated++;
	});

	function restoreAll() {
		document.querySelectorAll('.itak-translated').forEach(span => {
			const text = document.createTextNode(span.dataset.original);
			span.parentNode.replaceChild(text, span);
		});
	}

	return translated + " text segments marked for translation to " + targetLang;
})()
`, targetLang)

	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("translate: %w", err)
	}
	return nil
}

// TranslateClear removes the translation overlay and restores original text.
func (e *Engine) TranslateClear(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){
		document.querySelectorAll('.itak-translated').forEach(span => {
			const text = document.createTextNode(span.dataset.original);
			span.parentNode.replaceChild(text, span);
		});
		const b = document.getElementById('itak-translate-banner'); if(b)b.remove();
		return true;
	})()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
