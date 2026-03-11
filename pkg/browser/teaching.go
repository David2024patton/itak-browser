// Package browser - Visual teaching annotations and element highlighting.
//
// What: Injects visual step badges and element highlights into the headed
//       browser window for teaching demonstrations.
// Why:  When the teacher model is showing a student how to do something,
//       the student needs to see WHICH element the AI is about to interact
//       with and WHAT step number they're on. Visual annotations make the
//       lesson immediately understandable.
// How:  JS injection that creates positioned DOM overlays for step badges
//       and pulsing CSS outlines for element highlighting.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// HighlightElement adds a pulsing blue glow around an element in headed mode.
// The highlight auto-removes after 3 seconds.
func HighlightElement(ctx context.Context, selector string) error {
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return false;

			// Store original outline.
			const orig = el.style.outline;
			const origTrans = el.style.transition;

			el.style.transition = 'outline 0.3s ease';
			el.style.outline = '3px solid #3b82f6';
			el.style.outlineOffset = '2px';

			// Add pulsing animation via a style tag.
			const styleId = 'itak-highlight-style';
			if (!document.getElementById(styleId)) {
				const style = document.createElement('style');
				style.id = styleId;
				style.textContent = '@keyframes itak-pulse { 0%% { outline-color: #3b82f6; } 50%% { outline-color: #60a5fa; } 100%% { outline-color: #3b82f6; } }';
				document.head.appendChild(style);
			}
			el.style.animation = 'itak-pulse 1s infinite';

			// Scroll into view.
			el.scrollIntoView({behavior: 'smooth', block: 'center'});

			// Remove after 3 seconds.
			setTimeout(function() {
				el.style.outline = orig;
				el.style.outlineOffset = '';
				el.style.transition = origTrans;
				el.style.animation = '';
			}, 3000);

			return true;
		})()
	`, selector)

	var found bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &found)); err != nil {
		return fmt.Errorf("highlight: %w", err)
	}
	if !found {
		return fmt.Errorf("highlight: element not found")
	}
	return nil
}

// ShowStepBadge injects a step number badge at the top-right of the page.
// Used by the teaching mode to show progress through a lesson.
func ShowStepBadge(ctx context.Context, step int, label string) error {
	js := fmt.Sprintf(`
		(function() {
			// Update or create the step badge.
			let badge = document.getElementById('itak-step-badge');
			if (!badge) {
				badge = document.createElement('div');
				badge.id = 'itak-step-badge';
				badge.style.cssText = 'position:fixed;top:12px;right:12px;z-index:999998;' +
					'background:linear-gradient(135deg,#2563eb,#1d4ed8);color:white;' +
					'padding:8px 16px;border-radius:24px;font-family:system-ui,sans-serif;' +
					'font-size:13px;font-weight:600;box-shadow:0 4px 12px rgba(37,99,235,0.4);' +
					'display:flex;align-items:center;gap:8px;' +
					'border:2px solid rgba(255,255,255,0.2);';
				document.body.appendChild(badge);
			}

			badge.innerHTML = '<span style="background:rgba(255,255,255,0.2);' +
				'width:28px;height:28px;border-radius:50%%;display:flex;' +
				'align-items:center;justify-content:center;font-size:14px;' +
				'font-weight:bold">%d</span>' +
				'<span>%s</span>';

			// Entrance animation.
			badge.style.transform = 'translateX(120%%)';
			badge.offsetHeight; // Force reflow.
			badge.style.transition = 'transform 0.4s cubic-bezier(0.34,1.56,0.64,1)';
			badge.style.transform = 'translateX(0)';

			return true;
		})()
	`, step, label)

	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// ClearStepBadge removes the step badge from the page.
func ClearStepBadge(ctx context.Context) error {
	js := `
		(function() {
			const badge = document.getElementById('itak-step-badge');
			if (badge) badge.remove();
			return true;
		})()
	`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// ShowTeachingOverlay displays a semi-transparent teaching overlay
// with a message at the bottom of the screen.
func ShowTeachingOverlay(ctx context.Context, message string) error {
	js := fmt.Sprintf(`
		(function() {
			let overlay = document.getElementById('itak-teaching-overlay');
			if (!overlay) {
				overlay = document.createElement('div');
				overlay.id = 'itak-teaching-overlay';
				overlay.style.cssText = 'position:fixed;bottom:0;left:0;right:0;z-index:999997;' +
					'background:linear-gradient(to top,rgba(0,0,0,0.85),rgba(0,0,0,0.0));' +
					'padding:60px 24px 20px;font-family:system-ui,sans-serif;' +
					'color:white;font-size:16px;line-height:1.5;' +
					'pointer-events:none;';
				document.body.appendChild(overlay);
			}
			overlay.textContent = %q;
			return true;
		})()
	`, message)

	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// ClearTeachingOverlay removes the teaching overlay.
func ClearTeachingOverlay(ctx context.Context) error {
	js := `
		(function() {
			const o = document.getElementById('itak-teaching-overlay');
			if (o) o.remove();
			return true;
		})()
	`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// GetElementStyles returns computed CSS properties for an element.
func GetElementStyles(ctx context.Context, selector string, properties []string) (map[string]string, error) {
	if len(properties) == 0 {
		// Return common properties by default.
		properties = []string{
			"display", "visibility", "opacity", "color", "background-color",
			"font-size", "font-weight", "width", "height", "position",
			"margin", "padding", "border", "overflow", "z-index",
		}
	}

	// Build the JS to get computed styles.
	propsJS := "["
	for i, p := range properties {
		if i > 0 {
			propsJS += ","
		}
		propsJS += fmt.Sprintf("%q", p)
	}
	propsJS += "]"

	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return null;
			const computed = window.getComputedStyle(el);
			const props = %s;
			const result = {};
			for (const p of props) {
				result[p] = computed.getPropertyValue(p);
			}
			return result;
		})()
	`, selector, propsJS)

	var result map[string]string
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return nil, fmt.Errorf("get styles: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("element not found")
	}
	return result, nil
}

// ---- Screen Annotations for Teaching ----

// CircleElement draws a red circle around an element for teaching emphasis.
// The circle stays until ClearAnnotations is called.
func CircleElement(ctx context.Context, selector, label string) error {
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return false;

			const rect = el.getBoundingClientRect();
			const pad = 12;
			const cx = rect.left + rect.width/2;
			const cy = rect.top + rect.height/2;
			const rx = (rect.width/2) + pad;
			const ry = (rect.height/2) + pad;

			// Create SVG overlay if not present.
			let svg = document.getElementById('itak-annotations');
			if (!svg) {
				svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
				svg.id = 'itak-annotations';
				svg.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;' +
					'z-index:999996;pointer-events:none;';
				document.body.appendChild(svg);
			}

			// Draw ellipse.
			const ellipse = document.createElementNS('http://www.w3.org/2000/svg', 'ellipse');
			ellipse.setAttribute('cx', cx);
			ellipse.setAttribute('cy', cy);
			ellipse.setAttribute('rx', rx);
			ellipse.setAttribute('ry', ry);
			ellipse.setAttribute('fill', 'none');
			ellipse.setAttribute('stroke', '#ef4444');
			ellipse.setAttribute('stroke-width', '3');
			ellipse.setAttribute('stroke-dasharray', '1000');
			ellipse.setAttribute('stroke-dashoffset', '1000');
			ellipse.classList.add('itak-anno');

			// Animate the circle drawing.
			const anim = document.createElementNS('http://www.w3.org/2000/svg', 'animate');
			anim.setAttribute('attributeName', 'stroke-dashoffset');
			anim.setAttribute('from', '1000');
			anim.setAttribute('to', '0');
			anim.setAttribute('dur', '0.6s');
			anim.setAttribute('fill', 'freeze');
			ellipse.appendChild(anim);
			svg.appendChild(ellipse);

			// Label text below the circle.
			if (%q) {
				const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
				text.setAttribute('x', cx);
				text.setAttribute('y', cy + ry + 20);
				text.setAttribute('text-anchor', 'middle');
				text.setAttribute('fill', '#ef4444');
				text.setAttribute('font-family', 'system-ui, sans-serif');
				text.setAttribute('font-size', '14');
				text.setAttribute('font-weight', '600');
				text.classList.add('itak-anno');
				text.textContent = %q;
				svg.appendChild(text);
			}

			el.scrollIntoView({behavior: 'smooth', block: 'center'});
			return true;
		})()
	`, selector, label, label)

	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
		return fmt.Errorf("circle: %w", err)
	}
	if !ok {
		return fmt.Errorf("circle: element not found")
	}
	return nil
}

// ArrowToElement draws an SVG arrow pointing at an element with a text callout.
func ArrowToElement(ctx context.Context, selector, label string) error {
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return false;

			const rect = el.getBoundingClientRect();
			const targetX = rect.left + rect.width/2;
			const targetY = rect.top - 8;
			// Arrow starts from 80px above and 60px to the right.
			const startX = targetX + 60;
			const startY = targetY - 80;

			let svg = document.getElementById('itak-annotations');
			if (!svg) {
				svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
				svg.id = 'itak-annotations';
				svg.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;' +
					'z-index:999996;pointer-events:none;';
				document.body.appendChild(svg);
			}

			// Arrowhead marker.
			let defs = svg.querySelector('defs');
			if (!defs) {
				defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
				const marker = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
				marker.setAttribute('id', 'itak-arrowhead');
				marker.setAttribute('markerWidth', '10');
				marker.setAttribute('markerHeight', '7');
				marker.setAttribute('refX', '10');
				marker.setAttribute('refY', '3.5');
				marker.setAttribute('orient', 'auto');
				const polygon = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
				polygon.setAttribute('points', '0 0, 10 3.5, 0 7');
				polygon.setAttribute('fill', '#ef4444');
				marker.appendChild(polygon);
				defs.appendChild(marker);
				svg.appendChild(defs);
			}

			// Arrow line.
			const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
			line.setAttribute('x1', startX);
			line.setAttribute('y1', startY);
			line.setAttribute('x2', targetX);
			line.setAttribute('y2', targetY);
			line.setAttribute('stroke', '#ef4444');
			line.setAttribute('stroke-width', '2.5');
			line.setAttribute('marker-end', 'url(#itak-arrowhead)');
			line.classList.add('itak-anno');
			svg.appendChild(line);

			// Label at the arrow start.
			if (%q) {
				const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
				text.setAttribute('x', startX + 8);
				text.setAttribute('y', startY);
				text.setAttribute('fill', '#ef4444');
				text.setAttribute('font-family', 'system-ui, sans-serif');
				text.setAttribute('font-size', '13');
				text.setAttribute('font-weight', '600');
				text.classList.add('itak-anno');
				text.textContent = %q;
				svg.appendChild(text);
			}

			el.scrollIntoView({behavior: 'smooth', block: 'center'});
			return true;
		})()
	`, selector, label, label)

	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
		return fmt.Errorf("arrow: %w", err)
	}
	if !ok {
		return fmt.Errorf("arrow: element not found")
	}
	return nil
}

// DrawText places a floating text label at a fixed position on screen.
// Looks like a sticky note with slightly rotated placement.
func DrawText(ctx context.Context, x, y int, text string) error {
	js := fmt.Sprintf(`
		(function() {
			const note = document.createElement('div');
			note.className = 'itak-anno-note';
			note.style.cssText = 'position:fixed;left:%dpx;top:%dpx;z-index:999996;' +
				'background:#fef3c7;color:#92400e;padding:8px 12px;' +
				'font-family:system-ui,sans-serif;font-size:13px;font-weight:500;' +
				'border-radius:4px;box-shadow:0 2px 8px rgba(0,0,0,0.15);' +
				'border-left:3px solid #f59e0b;max-width:240px;line-height:1.4;' +
				'transform:rotate(-1deg);pointer-events:none;';
			note.textContent = %q;
			document.body.appendChild(note);

			// Fade in.
			note.style.opacity = '0';
			note.offsetHeight;
			note.style.transition = 'opacity 0.3s ease';
			note.style.opacity = '1';

			return true;
		})()
	`, x, y, text)

	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// ClearAnnotations removes all teaching annotations (circles, arrows, text, notes).
func ClearAnnotations(ctx context.Context) error {
	js := `
		(function() {
			const svg = document.getElementById('itak-annotations');
			if (svg) {
				svg.querySelectorAll('.itak-anno').forEach(el => el.remove());
				if (svg.childElementCount <= 1) svg.remove(); // Only defs left
			}
			document.querySelectorAll('.itak-anno-note').forEach(el => el.remove());
			return true;
		})()
	`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}
