// Package browser - Form Auto-Fill profiles.
//
// What: Captures all form field values on a page, stores them as a named
//       profile, and can replay/restore them later.
// Why:  Teaching form workflows requires filling the same form repeatedly.
//       Profiles let the AI demonstrate "here's how to fill this form"
//       and the user can replay it instantly.
// How:  JS collects all input/select/textarea values keyed by name/id,
//       stores in window.__itak_autofill_profiles. Replay sets all fields.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// AutofillSave captures all form field values and saves them as a named profile.
func (e *Engine) AutofillSave(ctx context.Context, profileName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf(`
(function() {
	if (!window.__itak_autofill_profiles) window.__itak_autofill_profiles = {};
	const fields = {};

	document.querySelectorAll('input,select,textarea').forEach(el => {
		const key = el.name || el.id || el.getAttribute('aria-label');
		if (!key) return;
		if (el.type === 'hidden' || el.type === 'submit' || el.type === 'button') return;

		if (el.type === 'checkbox' || el.type === 'radio') {
			if (el.checked) fields[key] = {type: el.type, value: el.value, checked: true};
		} else if (el.tagName === 'SELECT') {
			fields[key] = {type: 'select', value: el.value, selectedIndex: el.selectedIndex};
		} else {
			fields[key] = {type: el.type || 'text', value: el.value};
		}
	});

	window.__itak_autofill_profiles[%q] = fields;
	return Object.keys(fields).length + " fields saved as profile: " + %q;
})()
`, profileName, profileName)

	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("autofill save: %w", err)
	}
	return nil
}

// AutofillLoad restores form fields from a named profile.
func (e *Engine) AutofillLoad(ctx context.Context, profileName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf(`
(function() {
	const profiles = window.__itak_autofill_profiles || {};
	const fields = profiles[%q];
	if (!fields) return "profile not found: " + %q;

	let filled = 0;
	Object.entries(fields).forEach(([key, data]) => {
		const el = document.querySelector('[name="' + key + '"],[id="' + key + '"]');
		if (!el) return;

		if (data.type === 'checkbox' || data.type === 'radio') {
			el.checked = data.checked;
		} else if (data.type === 'select') {
			el.value = data.value;
		} else {
			el.value = data.value;
			el.dispatchEvent(new Event('input', {bubbles: true}));
			el.dispatchEvent(new Event('change', {bubbles: true}));
		}
		filled++;

		// Visual flash effect.
		el.style.transition = 'box-shadow 0.3s ease';
		el.style.boxShadow = '0 0 0 3px rgba(59,130,246,0.4)';
		setTimeout(() => { el.style.boxShadow = ''; }, 1000);
	});

	return filled + " fields restored from: " + %q;
})()
`, profileName, profileName, profileName)

	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("autofill load: %w", err)
	}
	if result != "" && result[0] == 'p' { // "profile not found"
		return fmt.Errorf("autofill: %s", result)
	}
	return nil
}

// AutofillList returns the names of all saved autofill profiles.
func (e *Engine) AutofillList(ctx context.Context) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := `JSON.stringify(Object.keys(window.__itak_autofill_profiles || {}))`
	var result string
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &result)); err != nil {
		return nil, fmt.Errorf("autofill list: %w", err)
	}
	// Simple parse: ["name1","name2"] -> slice.
	var names []string
	if len(result) > 2 {
		inner := result[1 : len(result)-1]
		for _, part := range splitJSON(inner) {
			if len(part) > 2 {
				names = append(names, part[1:len(part)-1])
			}
		}
	}
	return names, nil
}

// splitJSON splits a comma-separated JSON string.
func splitJSON(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}
