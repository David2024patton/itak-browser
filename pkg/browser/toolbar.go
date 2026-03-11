// Package browser - Headed-mode annotation toolbar.
//
// What: Full-width toolbar at top of headed browser for screenshot markup.
// Why:  Teacher model and student need bidirectional visual communication.
//       Human circles bugs, draws arrows to confusing UI, types questions.
//       AI reads the annotated PNG back via GetAnnotatedScreenshot().
// How:  JS injection creates a toolbar + canvas overlay. Capture uses
//       CDP screenshot (not SVG foreignObject) so text renders clean.
//       Page body gets pushed down so content sits below the toolbar.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// toolbarJS is the complete JS for the annotation toolbar.
// All styles are inline to avoid any CSS injection issues.
const toolbarJS = `
(function() {
	if (document.getElementById('itak-toolbar')) return true;

	var BAR = 38;
	var S = {
		mode: null, color: '#ef4444', lw: 3,
		drawing: false, sx: 0, sy: 0,
		captured: false, anns: [], base: null,
	};

	document.body.style.marginTop = BAR + 'px';

	// ---- Toolbar bar ----
	var bar = document.createElement('div');
	bar.id = 'itak-toolbar';
	bar.style.cssText =
		'position:fixed;top:0;left:0;right:0;height:' + BAR + 'px;z-index:999999;' +
		'display:flex;align-items:center;gap:4px;padding:0 10px;' +
		'background:#1a1a2e;border-bottom:1px solid #2a2a3e;' +
		'font-family:system-ui,-apple-system,sans-serif;font-size:12px;' +
		'user-select:none;color:#c9d1d9;';

	// ---- Logo ----
	var logo = document.createElement('span');
	logo.textContent = 'iTaK';
	logo.style.cssText =
		'font-weight:700;font-size:13px;letter-spacing:-0.02em;margin-right:6px;' +
		'color:#7c8aff;';
	bar.appendChild(logo);

	// ---- Button helper ----
	function mkBtn(text, cb, id) {
		var b = document.createElement('button');
		if (id) b.id = id;
		b.textContent = text;
		b.style.cssText =
			'display:inline-flex;align-items:center;height:26px;padding:0 10px;' +
			'border:1px solid #333;border-radius:5px;background:#252540;color:#c9d1d9;' +
			'cursor:pointer;font-size:11px;font-family:inherit;font-weight:500;' +
			'transition:background 0.12s;white-space:nowrap;line-height:1;';
		b.onmouseenter = function() { b.style.background = '#333357'; };
		b.onmouseleave = function() {
			if (!b.getAttribute('data-active')) b.style.background = '#252540';
		};
		b.onclick = cb;
		return b;
	}

	function mkSep() {
		var s = document.createElement('div');
		s.style.cssText = 'width:1px;height:20px;background:#333;margin:0 3px;flex-shrink:0;';
		return s;
	}

	// ---- Capture ----
	var capBtn = mkBtn('Capture', doCapture, 'itak-tb-capture');
	capBtn.style.background = '#2d3abf';
	capBtn.style.borderColor = '#4451e6';
	capBtn.style.color = '#e0e4ff';
	capBtn.onmouseenter = function() { capBtn.style.background = '#3d4ad0'; };
	capBtn.onmouseleave = function() { capBtn.style.background = '#2d3abf'; };
	bar.appendChild(capBtn);

	bar.appendChild(mkSep());

	// ---- Drawing tools ----
	var penBtn = mkBtn('Pen', function() { setMode('pen'); }, 'itak-tb-pen');
	var txtBtn = mkBtn('Text', function() { setMode('text'); }, 'itak-tb-text');
	var cirBtn = mkBtn('Circle', function() { setMode('circle'); }, 'itak-tb-circle');
	var arrBtn = mkBtn('Arrow', function() { setMode('arrow'); }, 'itak-tb-arrow');
	var eraBtn = mkBtn('Eraser', function() { setMode('eraser'); }, 'itak-tb-eraser');

	var toolBtns = [penBtn, txtBtn, cirBtn, arrBtn, eraBtn];
	toolBtns.forEach(function(b) { b.style.opacity = '0.35'; b.style.pointerEvents = 'none'; bar.appendChild(b); });

	bar.appendChild(mkSep());

	// ---- Colors ----
	var colorsDiv = document.createElement('div');
	colorsDiv.style.cssText = 'display:flex;gap:3px;align-items:center;opacity:0.35;pointer-events:none;';
	['#ef4444','#f59e0b','#22c55e','#6366f1','#a855f7','#ffffff'].forEach(function(c) {
		var dot = document.createElement('div');
		dot.style.cssText =
			'width:14px;height:14px;border-radius:50%;cursor:pointer;flex-shrink:0;' +
			'border:2px solid ' + (c === S.color ? '#aaa' : '#444') + ';' +
			'background:' + c + ';transition:border-color 0.12s;';
		dot.onclick = function() {
			S.color = c;
			colorsDiv.querySelectorAll('div').forEach(function(d) { d.style.borderColor = '#444'; });
			dot.style.borderColor = '#aaa';
		};
		colorsDiv.appendChild(dot);
	});
	bar.appendChild(colorsDiv);

	bar.appendChild(mkSep());

	// ---- Size ----
	var szDiv = document.createElement('div');
	szDiv.style.cssText = 'display:flex;align-items:center;gap:4px;opacity:0.35;pointer-events:none;';
	var slider = document.createElement('input');
	slider.type = 'range'; slider.min = '1'; slider.max = '12'; slider.value = '3';
	slider.style.cssText = 'width:48px;height:3px;accent-color:#6366f1;';
	var szLbl = document.createElement('span');
	szLbl.textContent = '3';
	szLbl.style.cssText = 'font-size:10px;color:#666;min-width:12px;';
	slider.oninput = function() { S.lw = parseInt(slider.value); szLbl.textContent = slider.value; };
	szDiv.appendChild(slider); szDiv.appendChild(szLbl);
	bar.appendChild(szDiv);

	// ---- Spacer ----
	var spacer = document.createElement('div');
	spacer.style.flex = '1';
	bar.appendChild(spacer);

	// ---- Right side ----
	var undoBtn = mkBtn('Undo', doUndo, 'itak-tb-undo');
	undoBtn.style.opacity = '0.35'; undoBtn.style.pointerEvents = 'none';
	bar.appendChild(undoBtn);

	var clrBtn = mkBtn('Clear', doClear, 'itak-tb-clear');
	clrBtn.style.opacity = '0.35'; clrBtn.style.pointerEvents = 'none';
	bar.appendChild(clrBtn);

	bar.appendChild(mkSep());

	var saveBtn = mkBtn('Save', doSave, 'itak-tb-save');
	saveBtn.style.opacity = '0.35'; saveBtn.style.pointerEvents = 'none';
	bar.appendChild(saveBtn);

	var sendBtn = mkBtn('Send to AI', doSend, 'itak-tb-send');
	sendBtn.style.background = '#1a7a3a';
	sendBtn.style.borderColor = '#2a9a4a';
	sendBtn.style.color = '#c0ffd0';
	sendBtn.style.opacity = '0.35'; sendBtn.style.pointerEvents = 'none';
	sendBtn.onmouseenter = function() { sendBtn.style.background = '#2a8a4a'; };
	sendBtn.onmouseleave = function() { sendBtn.style.background = '#1a7a3a'; };
	bar.appendChild(sendBtn);

	bar.appendChild(mkSep());

	// ---- Collapse ----
	var colBtn = mkBtn('\u25B2', null, 'itak-tb-collapse');
	colBtn.style.padding = '0 6px';
	colBtn.style.fontSize = '9px';
	var collapsed = false;
	colBtn.onclick = function() {
		collapsed = !collapsed;
		if (collapsed) {
			bar.style.height = '6px'; bar.style.overflow = 'hidden'; bar.style.padding = '0';
			bar.style.cursor = 'pointer'; document.body.style.marginTop = '6px';
			colBtn.textContent = '\u25BC';
			bar.onclick = function(e) { if (e.target === bar) colBtn.click(); };
		} else {
			bar.style.height = BAR + 'px'; bar.style.overflow = ''; bar.style.padding = '0 10px';
			bar.style.cursor = ''; document.body.style.marginTop = BAR + 'px';
			colBtn.textContent = '\u25B2';
			bar.onclick = null;
		}
	};
	bar.appendChild(colBtn);

	document.body.appendChild(bar);

	// ---- Canvas overlay ----
	var ov = document.createElement('div');
	ov.id = 'itak-canvas-overlay';
	ov.style.cssText =
		'position:fixed;top:' + BAR + 'px;left:0;width:100vw;' +
		'height:calc(100vh - ' + BAR + 'px);z-index:999990;' +
		'display:none;cursor:crosshair;background:#11111b;';
	var cvs = document.createElement('canvas');
	cvs.id = 'itak-draw-canvas';
	cvs.style.cssText = 'display:block;';
	ov.appendChild(cvs);
	document.body.appendChild(ov);

	// ---- Enable tools ----
	function enableTools() {
		toolBtns.forEach(function(b) { b.style.opacity = '1'; b.style.pointerEvents = 'auto'; });
		[colorsDiv, szDiv].forEach(function(el) { el.style.opacity = '1'; el.style.pointerEvents = 'auto'; });
		[undoBtn, clrBtn, saveBtn, sendBtn].forEach(function(b) { b.style.opacity = '1'; b.style.pointerEvents = 'auto'; });
	}

	function disableTools() {
		toolBtns.forEach(function(b) { b.style.opacity = '0.35'; b.style.pointerEvents = 'none'; b.style.background = '#252540'; b.removeAttribute('data-active'); });
		[colorsDiv, szDiv].forEach(function(el) { el.style.opacity = '0.35'; el.style.pointerEvents = 'none'; });
		[undoBtn, clrBtn, saveBtn, sendBtn].forEach(function(b) { b.style.opacity = '0.35'; b.style.pointerEvents = 'none'; });
		S.mode = null;
	}

	function setMode(m) {
		S.mode = m;
		var map = {pen:penBtn, text:txtBtn, circle:cirBtn, arrow:arrBtn, eraser:eraBtn};
		toolBtns.forEach(function(b) { b.style.background = '#252540'; b.removeAttribute('data-active'); });
		if (map[m]) { map[m].style.background = '#3d3d6e'; map[m].setAttribute('data-active', '1'); }
		ov.style.cursor = m === 'text' ? 'text' : m === 'eraser' ? 'cell' : 'crosshair';
	}

	// ---- Capture ----
	function doCapture() {
		bar.style.display = 'none';
		ov.style.display = 'none';
		document.body.style.marginTop = '0';

		function finish(dataUrl) {
			document.body.style.marginTop = BAR + 'px';
			bar.style.display = 'flex';
			var w = window.innerWidth, h = window.innerHeight - BAR;
			var dpr = window.devicePixelRatio || 1;
			cvs.width = w * dpr; cvs.height = h * dpr;
			cvs.style.width = w + 'px'; cvs.style.height = h + 'px';
			var ctx = cvs.getContext('2d');
			ctx.scale(dpr, dpr);
			var img = new Image();
			img.onload = function() {
				ctx.drawImage(img, 0, 0, w, h);
				S.base = img; S.captured = true; S.anns = [];
				ov.style.display = 'block'; enableTools(); setMode('pen');
				// Visual feedback: green border flash so user knows capture worked.
				ov.style.outline = '3px solid #22c55e';
				setTimeout(function() { ov.style.outline = 'none'; }, 600);
				toast('Captured - draw your annotations');
			};
			img.src = dataUrl;
		}

		// Signal the Go daemon to take a CDP screenshot.
		window.__itak_capture_requested = true;
		window.__itak_cdp_screenshot = null;

		// Poll for the Go daemon to inject the screenshot.
		var attempts = 0;
		var poll = setInterval(function() {
			attempts++;
			if (window.__itak_cdp_screenshot) {
				clearInterval(poll);
				var src = window.__itak_cdp_screenshot;
				window.__itak_cdp_screenshot = null;
				window.__itak_capture_requested = false;
				finish(src);
			} else if (attempts > 25) {
				// 5 seconds max wait - give up.
				clearInterval(poll);
				window.__itak_capture_requested = false;
				document.body.style.marginTop = BAR + 'px';
				bar.style.display = 'flex';
				toast('Capture timed out');
			}
		}, 200);
	}

	function redraw() {
		var ctx = cvs.getContext('2d');
		var dpr = window.devicePixelRatio || 1;
		var w = cvs.width / dpr, h = cvs.height / dpr;
		ctx.clearRect(0, 0, w, h);

		// Layer 1: base image.
		if (S.base) { ctx.drawImage(S.base, 0, 0, w, h); }
		else { ctx.fillStyle = '#11111b'; ctx.fillRect(0, 0, w, h); }

		// Layer 2: annotations on a temp offscreen canvas.
		// Eraser uses destination-out on only this layer so it doesn't
		// erase the base image (which would show black).
		var tmp = document.createElement('canvas');
		tmp.width = cvs.width; tmp.height = cvs.height;
		var tc = tmp.getContext('2d');
		tc.scale(dpr, dpr);
		S.anns.forEach(function(op) { drawOp(tc, op); });

		// Composite annotations onto main canvas.
		ctx.save();
		ctx.setTransform(1, 0, 0, 1, 0, 0); // reset transform for raw pixel copy
		ctx.drawImage(tmp, 0, 0);
		ctx.restore();
	}

	function drawOp(ctx, op) {
		ctx.strokeStyle = op.color; ctx.fillStyle = op.color;
		ctx.lineWidth = op.lw; ctx.lineCap = 'round'; ctx.lineJoin = 'round';
		switch(op.type) {
			case 'pen':
				if (op.pts.length < 2) break;
				ctx.beginPath(); ctx.moveTo(op.pts[0].x, op.pts[0].y);
				for (var i = 1; i < op.pts.length; i++) ctx.lineTo(op.pts[i].x, op.pts[i].y);
				ctx.stroke(); break;
			case 'circle':
				var cx = (op.x1 + op.x2) / 2, cy = (op.y1 + op.y2) / 2;
				var rx = Math.abs(op.x2 - op.x1) / 2, ry = Math.abs(op.y2 - op.y1) / 2;
				ctx.beginPath(); ctx.ellipse(cx, cy, Math.max(rx,1), Math.max(ry,1), 0, 0, 2*Math.PI);
				ctx.stroke(); break;
			case 'arrow':
				var dx = op.x2 - op.x1, dy = op.y2 - op.y1, a = Math.atan2(dy, dx), hl = 14;
				ctx.beginPath(); ctx.moveTo(op.x1, op.y1); ctx.lineTo(op.x2, op.y2); ctx.stroke();
				ctx.beginPath(); ctx.moveTo(op.x2, op.y2);
				ctx.lineTo(op.x2 - hl*Math.cos(a-Math.PI/6), op.y2 - hl*Math.sin(a-Math.PI/6));
				ctx.moveTo(op.x2, op.y2);
				ctx.lineTo(op.x2 - hl*Math.cos(a+Math.PI/6), op.y2 - hl*Math.sin(a+Math.PI/6));
				ctx.stroke(); break;
			case 'text':
				ctx.font = 'bold ' + (op.lw*5+12) + 'px system-ui,sans-serif';
				ctx.fillText(op.text, op.x1, op.y1); break;
			case 'eraser':
				if (op.pts.length < 2) break;
				ctx.save(); ctx.globalCompositeOperation = 'destination-out';
				ctx.lineWidth = op.lw * 4; ctx.beginPath();
				ctx.moveTo(op.pts[0].x, op.pts[0].y);
				for (var j = 1; j < op.pts.length; j++) ctx.lineTo(op.pts[j].x, op.pts[j].y);
				ctx.stroke(); ctx.restore(); break;
		}
	}

	function doUndo() { if (S.anns.length > 0) { S.anns.pop(); redraw(); } }
	function doClear() {
		if (!S.captured) return;
		S.anns = []; redraw(); toast('Cleared');
	}
	function doSave() {
		if (!S.captured) return;
		var dataUrl = cvs.toDataURL('image/png');
		var parts = dataUrl.split(',');
		var raw = atob(parts[1]);
		var arr = new Uint8Array(raw.length);
		for (var i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
		var blob = new Blob([arr], {type: 'image/png'});
		var fname = 'itak-screenshot-' + Date.now() + '.png';

		// Store for Go daemon to save + get full path for clipboard.
		window.__itak_save_data = dataUrl;
		window.__itak_save_requested = true;
		window.__itak_saved_path = null;

		// Show native Save As dialog.
		if (window.showSaveFilePicker) {
			window.showSaveFilePicker({
				suggestedName: fname,
				types: [{description:'PNG Image', accept:{'image/png':['.png']}}]
			}).then(function(handle) {
				return handle.createWritable().then(function(wr) {
					return wr.write(blob).then(function() { return wr.close(); });
				});
			}).then(function() {
				// Also wait for Go daemon to inject the known path for clipboard.
				waitForPath();
			}).catch(function(err) {
				if (err.name !== 'AbortError') toast('Save failed');
			});
		} else {
			var url = URL.createObjectURL(blob);
			var a = document.createElement('a');
			a.download = fname; a.href = url; a.click();
			setTimeout(function() { URL.revokeObjectURL(url); }, 2000);
			waitForPath();
		}

		function waitForPath() {
			var tries = 0;
			var chk = setInterval(function() {
				tries++;
				if (window.__itak_saved_path) {
					clearInterval(chk);
					var p = window.__itak_saved_path;
					window.__itak_saved_path = null;
					navigator.clipboard.writeText(p).then(function() {
						toast('Saved + path copied');
					}).catch(function() {
						toast('Saved (path: ' + p + ')');
					});
					ov.style.display = 'none'; S.captured = false;
					disableTools();
				} else if (tries > 15) {
					clearInterval(chk);
					toast('Saved');
					ov.style.display = 'none'; S.captured = false;
					disableTools();
				}
			}, 200);
		}
	}
	function doSend() {
		if (!S.captured) return;
		window.__itak_annotated_screenshot = cvs.toDataURL('image/png');
		toast('Stored for AI');
		ov.style.display = 'none'; S.captured = false;
		disableTools();
	}

	function toast(msg) {
		var t = document.createElement('div');
		t.style.cssText =
			'position:fixed;bottom:20px;left:50%;transform:translateX(-50%);z-index:999999;' +
			'background:#1a1a2e;color:#c9d1d9;padding:8px 16px;border-radius:6px;' +
			'font-family:system-ui;font-size:12px;font-weight:500;' +
			'border:1px solid #2a2a3e;box-shadow:0 4px 12px rgba(0,0,0,0.5);';
		t.textContent = msg;
		document.body.appendChild(t);
		setTimeout(function() { t.style.transition = 'opacity 0.3s'; t.style.opacity = '0'; setTimeout(function() { t.remove(); }, 300); }, 2000);
	}

	// ---- Drawing events ----
	var curOp = null;
	ov.addEventListener('mousedown', function(e) {
		if (!S.mode || !S.captured) return;
		var r = cvs.getBoundingClientRect(), x = e.clientX - r.left, y = e.clientY - r.top;
		if (S.mode === 'text') {
			var txt = prompt('Annotation text:');
			if (txt) { S.anns.push({type:'text',x1:x,y1:y,color:S.color,lw:S.lw,text:txt}); redraw(); }
			return;
		}
		S.drawing = true; S.sx = x; S.sy = y;
		if (S.mode === 'pen' || S.mode === 'eraser') {
			curOp = {type:S.mode,pts:[{x:x,y:y}],color:S.color,lw:S.lw};
		} else {
			curOp = {type:S.mode,x1:x,y1:y,x2:x,y2:y,color:S.color,lw:S.lw};
		}
	});
	ov.addEventListener('mousemove', function(e) {
		if (!S.drawing || !curOp) return;
		var r = cvs.getBoundingClientRect(), x = e.clientX - r.left, y = e.clientY - r.top;
		if (curOp.type === 'pen' || curOp.type === 'eraser') { curOp.pts.push({x:x,y:y}); }
		else { curOp.x2 = x; curOp.y2 = y; }
		redraw(); drawOp(cvs.getContext('2d'), curOp);
	});
	ov.addEventListener('mouseup', function() {
		if (!S.drawing || !curOp) return;
		S.drawing = false; S.anns.push(curOp); curOp = null; redraw();
	});
	document.addEventListener('keydown', function(e) {
		if (!S.captured) return;
		if (e.ctrlKey && e.key === 'z') { doUndo(); e.preventDefault(); }
		if (e.key === 'Escape') { ov.style.display = 'none'; S.captured = false; }
	});

	return true;
})()
`

// InjectToolbar injects the annotation toolbar into the headed browser.
func InjectToolbar(ctx context.Context) error {
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(toolbarJS, &ok)); err != nil {
		return fmt.Errorf("inject toolbar: %w", err)
	}
	return nil
}

// InjectToolbarWithScreenshot injects the toolbar AND pre-captures a CDP
// screenshot so the Capture button shows a pixel-perfect image instead of
// a blank canvas.
func InjectToolbarWithScreenshot(ctx context.Context) error {
	// Take a CDP screenshot first.
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return fmt.Errorf("cdp screenshot: %w", err)
	}

	// Inject the toolbar.
	if err := InjectToolbar(ctx); err != nil {
		return err
	}

	// Store the screenshot as a base64 data URL for the JS side to pick up.
	js := fmt.Sprintf(`window.__itak_cdp_screenshot = "data:image/png;base64,%s"; true`,
		base64Encode(buf))
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// RemoveToolbar removes the annotation toolbar.
func RemoveToolbar(ctx context.Context) error {
	js := `
		(function() {
			var tb = document.getElementById('itak-toolbar');
			if (tb) tb.remove();
			var ov = document.getElementById('itak-canvas-overlay');
			if (ov) ov.remove();
			document.body.style.marginTop = '';
			return true;
		})()
	`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// GetAnnotatedScreenshot reads the annotated screenshot the user drew.
func GetAnnotatedScreenshot(ctx context.Context) (string, error) {
	js := `window.__itak_annotated_screenshot || ""`
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return "", fmt.Errorf("get annotated screenshot: %w", err)
	}
	return result, nil
}

// ClearAnnotatedScreenshot clears the stored annotated screenshot.
func ClearAnnotatedScreenshot(ctx context.Context) error {
	js := `window.__itak_annotated_screenshot = ""; true`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// base64Encode encodes bytes to base64 string.
func base64Encode(data []byte) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	n := len(data)
	for i := 0; i < n; i += 3 {
		b0 := data[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < n {
			b1 = data[i+1]
		}
		if i+2 < n {
			b2 = data[i+2]
		}
		out = append(out, table[b0>>2])
		out = append(out, table[((b0&3)<<4)|(b1>>4)])
		if i+1 < n {
			out = append(out, table[((b1&0xf)<<2)|(b2>>6)])
		} else {
			out = append(out, '=')
		}
		if i+2 < n {
			out = append(out, table[b2&0x3f])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}
