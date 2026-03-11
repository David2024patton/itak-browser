// Package browser - Accessibility audit overlay.
//
// What: Scans the page for common accessibility issues and overlays colored
//       badges on elements with problems.
// Why:  Teaching web accessibility is critical. Visual badges make issues
//       obvious without needing to read an audit report.
// How:  JS scans for missing alt text, low contrast, missing ARIA labels,
//       missing form labels, empty links/buttons. Color-coded badge per issue.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// A11yAudit runs an accessibility audit and overlays issue badges on the page.
func (e *Engine) A11yAudit(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `
(function() {
	// Remove old badges.
	document.querySelectorAll('.itak-a11y-badge').forEach(b => b.remove());
	const old = document.getElementById('itak-a11y-summary');
	if (old) old.remove();

	const issues = [];

	function badge(el, text, color) {
		const rect = el.getBoundingClientRect();
		if (rect.width === 0 || rect.height === 0) return;
		const b = document.createElement('div');
		b.className = 'itak-a11y-badge';
		b.textContent = text;
		b.title = text;
		b.style.cssText =
			'position:fixed;z-index:999960;pointer-events:auto;cursor:help;' +
			'background:' + color + ';color:#fff;font:bold 9px system-ui;' +
			'padding:2px 5px;border-radius:3px;white-space:nowrap;' +
			'box-shadow:0 1px 4px rgba(0,0,0,0.3);' +
			'left:' + rect.left + 'px;top:' + Math.max(0, rect.top - 16) + 'px;';
		document.body.appendChild(b);
		issues.push({element: el.tagName + (el.id ? '#'+el.id : ''), issue: text});
	}

	// 1. Images without alt text.
	document.querySelectorAll('img').forEach(img => {
		if (!img.hasAttribute('alt')) badge(img, 'Missing alt text', '#ef4444');
		else if (img.alt.trim() === '') badge(img, 'Empty alt text', '#f59e0b');
	});

	// 2. Links without accessible text.
	document.querySelectorAll('a').forEach(a => {
		const text = (a.textContent||'').trim();
		const aria = a.getAttribute('aria-label') || '';
		if (!text && !aria && !a.querySelector('img')) badge(a, 'Empty link', '#ef4444');
	});

	// 3. Buttons without accessible text.
	document.querySelectorAll('button').forEach(btn => {
		const text = (btn.textContent||'').trim();
		const aria = btn.getAttribute('aria-label') || '';
		if (!text && !aria) badge(btn, 'Empty button', '#ef4444');
	});

	// 4. Form inputs without labels.
	document.querySelectorAll('input,textarea,select').forEach(input => {
		if (input.type === 'hidden' || input.type === 'submit' || input.type === 'button') return;
		const id = input.id;
		const hasLabel = id && document.querySelector('label[for="' + id + '"]');
		const ariaLabel = input.getAttribute('aria-label') || input.getAttribute('aria-labelledby');
		const parentLabel = input.closest('label');
		if (!hasLabel && !ariaLabel && !parentLabel && !input.placeholder) {
			badge(input, 'No label', '#f59e0b');
		}
	});

	// 5. Missing lang attribute.
	if (!document.documentElement.lang) {
		issues.push({element: 'html', issue: 'Missing lang attribute'});
	}

	// 6. Missing page title.
	if (!document.title || document.title.trim() === '') {
		issues.push({element: 'head', issue: 'Missing page title'});
	}

	// 7. Low contrast text (simple heuristic).
	document.querySelectorAll('p,span,div,li,td,th,label,a').forEach(el => {
		const style = window.getComputedStyle(el);
		const color = style.color;
		const bg = style.backgroundColor;
		if (color === bg && el.textContent.trim()) {
			badge(el, 'Zero contrast', '#ef4444');
		}
	});

	// 8. Missing heading structure.
	const h1s = document.querySelectorAll('h1');
	if (h1s.length === 0 && document.querySelector('main,article,[role="main"]')) {
		issues.push({element: 'page', issue: 'No h1 found'});
	}

	// Summary panel.
	const summary = document.createElement('div');
	summary.id = 'itak-a11y-summary';
	summary.style.cssText =
		'position:fixed;bottom:16px;right:16px;z-index:999961;' +
		'background:rgba(15,23,42,0.95);border:1px solid rgba(255,255,255,0.1);' +
		'border-radius:10px;padding:12px 16px;width:260px;' +
		'font:12px system-ui;color:#e2e8f0;box-shadow:0 4px 24px rgba(0,0,0,0.5);' +
		'backdrop-filter:blur(12px);';

	const issueCount = issues.length;
	const color = issueCount === 0 ? '#22c55e' : issueCount < 5 ? '#f59e0b' : '#ef4444';
	summary.innerHTML =
		'<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">' +
		'<span style="font-weight:700;font-size:14px">\u267F A11y Audit</span>' +
		'<span style="background:' + color + ';color:#fff;padding:2px 8px;border-radius:10px;font:bold 11px system-ui">' +
		issueCount + ' issue' + (issueCount !== 1 ? 's' : '') + '</span></div>';

	if (issues.length > 0) {
		const list = document.createElement('div');
		list.style.cssText = 'max-height:200px;overflow-y:auto;';
		issues.slice(0, 20).forEach(iss => {
			const li = document.createElement('div');
			li.style.cssText = 'padding:3px 0;border-bottom:1px solid rgba(255,255,255,0.04);font-size:11px;';
			li.innerHTML = '<span style="color:#94a3b8">' + iss.element + '</span> ' +
				'<span style="color:#f59e0b">' + iss.issue + '</span>';
			list.appendChild(li);
		});
		summary.appendChild(list);
	} else {
		summary.innerHTML += '<div style="color:#22c55e;padding:4px 0">\u2705 No issues found!</div>';
	}

	const closeBtn = document.createElement('button');
	closeBtn.textContent = 'Close Audit';
	closeBtn.style.cssText =
		'margin-top:8px;width:100%;padding:6px;border:1px solid rgba(255,255,255,0.1);' +
		'border-radius:6px;background:rgba(255,255,255,0.05);color:#e2e8f0;' +
		'cursor:pointer;font:500 11px system-ui;';
	closeBtn.onclick = () => {
		document.querySelectorAll('.itak-a11y-badge').forEach(b => b.remove());
		summary.remove();
	};
	summary.appendChild(closeBtn);
	document.body.appendChild(summary);

	return issueCount + " issues found";
})()
`
	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("a11y audit: %w", err)
	}
	return nil
}

// A11yClear removes all accessibility audit badges.
func (e *Engine) A11yClear(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){
		document.querySelectorAll('.itak-a11y-badge').forEach(b=>b.remove());
		const s=document.getElementById('itak-a11y-summary'); if(s)s.remove();
		return true;
	})()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
