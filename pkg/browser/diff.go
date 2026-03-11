// Package browser - Visual Diff: snapshot before/after and overlay changes.
//
// What: Captures two screenshots (before/after) and generates a visual diff
//       that highlights pixel-level changes with a red overlay.
// Why:  When teaching someone about UI changes, seeing exactly what changed
//       is more impactful than a text description.
// How:  Uses CDP screenshots, converts to canvas, computes pixel differences,
//       overlays changed pixels in red. Stores result for retrieval.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// DiffSnapshot captures and stores a "before" screenshot for later comparison.
func (e *Engine) DiffSnapshot(ctx context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(e.browserCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return fmt.Errorf("diff snapshot: %w", err)
	}

	b64 := base64Encode(buf)
	js := fmt.Sprintf(`
		if (!window.__itak_diff_store) window.__itak_diff_store = {};
		window.__itak_diff_store[%q] = "data:image/png;base64,%s";
		true
	`, name, b64)
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// DiffCompare takes a new screenshot and compares it against the named
// "before" snapshot. The diff overlay is displayed on the page.
func (e *Engine) DiffCompare(ctx context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Capture "after" screenshot.
	var buf []byte
	if err := chromedp.Run(e.browserCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return fmt.Errorf("diff compare screenshot: %w", err)
	}

	b64 := base64Encode(buf)
	js := fmt.Sprintf(`
(function() {
	const beforeSrc = window.__itak_diff_store && window.__itak_diff_store[%q];
	if (!beforeSrc) return "no snapshot named " + %q;
	const afterSrc = "data:image/png;base64,%s";

	// Remove old diff overlay.
	const old = document.getElementById('itak-diff-overlay');
	if (old) old.remove();

	const overlay = document.createElement('div');
	overlay.id = 'itak-diff-overlay';
	overlay.style.cssText =
		'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:999970;' +
		'background:#0f172a;display:flex;flex-direction:column;';

	// Header bar.
	const hdr = document.createElement('div');
	hdr.style.cssText =
		'display:flex;align-items:center;justify-content:space-between;padding:8px 16px;' +
		'background:#1e293b;border-bottom:1px solid rgba(255,255,255,0.1);' +
		'font:600 13px system-ui;color:#e2e8f0;';
	hdr.innerHTML = '<span>Visual Diff: <span style="color:#3b82f6">' + %q +
		'</span></span><div style="display:flex;gap:8px"></div>';
	const btnGroup = hdr.querySelector('div');

	function mkBtn(label, active, onclick) {
		const b = document.createElement('button');
		b.textContent = label;
		b.style.cssText =
			'padding:4px 12px;border:1px solid rgba(255,255,255,0.15);border-radius:4px;' +
			'background:' + (active ? 'rgba(59,130,246,0.3)' : 'rgba(255,255,255,0.05)') + ';' +
			'color:#e2e8f0;cursor:pointer;font:500 11px system-ui;';
		b.onclick = onclick;
		return b;
	}

	const canvas = document.createElement('canvas');
	canvas.style.cssText = 'flex:1;object-fit:contain;';

	// Mode buttons.
	let mode = 'diff';
	function render() {
		const ctx = canvas.getContext('2d');
		ctx.clearRect(0, 0, canvas.width, canvas.height);
		if (mode === 'before') {
			const img = new Image(); img.onload = () => { ctx.drawImage(img, 0, 0); }; img.src = beforeSrc;
		} else if (mode === 'after') {
			const img = new Image(); img.onload = () => { ctx.drawImage(img, 0, 0); }; img.src = afterSrc;
		} else {
			// Pixel diff.
			const imgBefore = new Image();
			imgBefore.onload = () => {
				canvas.width = imgBefore.width; canvas.height = imgBefore.height;
				ctx.drawImage(imgBefore, 0, 0);
				const d1 = ctx.getImageData(0, 0, canvas.width, canvas.height);
				const imgAfter = new Image();
				imgAfter.onload = () => {
					ctx.drawImage(imgAfter, 0, 0);
					const d2 = ctx.getImageData(0, 0, canvas.width, canvas.height);
					const out = ctx.createImageData(canvas.width, canvas.height);
					let changes = 0;
					for (let i = 0; i < d1.data.length; i += 4) {
						const dr = Math.abs(d1.data[i] - d2.data[i]);
						const dg = Math.abs(d1.data[i+1] - d2.data[i+1]);
						const db = Math.abs(d1.data[i+2] - d2.data[i+2]);
						if (dr + dg + db > 30) {
							out.data[i] = 239; out.data[i+1] = 68; out.data[i+2] = 68; out.data[i+3] = 200;
							changes++;
						} else {
							out.data[i] = d2.data[i]; out.data[i+1] = d2.data[i+1];
							out.data[i+2] = d2.data[i+2]; out.data[i+3] = 60;
						}
					}
					ctx.putImageData(out, 0, 0);
					hdr.querySelector('span').innerHTML =
						'Visual Diff: <span style="color:#3b82f6">' + %q + '</span>' +
						' <span style="color:#f59e0b;margin-left:8px">' +
						changes + ' changed pixels</span>';
				};
				imgAfter.src = afterSrc;
			};
			imgBefore.src = beforeSrc;
		}
	}

	btnGroup.appendChild(mkBtn('Before', false, () => { mode='before'; render(); }));
	btnGroup.appendChild(mkBtn('Diff', true, () => { mode='diff'; render(); }));
	btnGroup.appendChild(mkBtn('After', false, () => { mode='after'; render(); }));

	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716 Close';
	closeBtn.style.cssText =
		'padding:4px 12px;border:1px solid rgba(239,68,68,0.3);border-radius:4px;' +
		'background:rgba(239,68,68,0.1);color:#ef4444;cursor:pointer;font:500 11px system-ui;';
	closeBtn.onclick = () => overlay.remove();
	btnGroup.appendChild(closeBtn);

	overlay.appendChild(hdr);
	overlay.appendChild(canvas);
	document.body.appendChild(overlay);

	render();
	return "ok";
})()
`, name, name, b64, name, name)

	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("diff compare: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("diff compare: %s", result)
	}
	return nil
}

// DiffClear removes the diff overlay.
func (e *Engine) DiffClear(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := `(function(){ const d = document.getElementById('itak-diff-overlay'); if(d) d.remove(); return true; })()`
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
