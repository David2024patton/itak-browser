// Package browser - Bulk page data extraction for agents.
//
// What: Extract all links and all forms from a page in one call.
// Why:  Agents commonly need to enumerate navigation options or find forms
//       to fill. Doing this via snapshot + manual parsing wastes tokens.
// How:  JS eval that collects structured data from the live DOM.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// PageLink represents an extracted hyperlink.
type PageLink struct {
	Text   string `json:"text"`
	Href   string `json:"href"`
	Rel    string `json:"rel,omitempty"`
	Target string `json:"target,omitempty"`
}

// FormField represents a single form input.
type FormField struct {
	Tag      string `json:"tag"`             // input, select, textarea
	Type     string `json:"type,omitempty"`  // text, email, password, etc.
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Value    string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// PageForm represents an extracted HTML form.
type PageForm struct {
	Action string      `json:"action"`
	Method string      `json:"method"`
	ID     string      `json:"id,omitempty"`
	Fields []FormField `json:"fields"`
}

// ExtractLinks returns all hyperlinks from the current page.
func ExtractLinks(ctx context.Context) ([]PageLink, error) {
	js := `
		(function() {
			const links = [];
			document.querySelectorAll('a[href]').forEach(a => {
				const text = (a.textContent || '').trim().substring(0, 100);
				if (text || a.href) {
					links.push({
						text: text,
						href: a.href,
						rel: a.rel || '',
						target: a.target || ''
					});
				}
			});
			return links;
		})()
	`
	var links []PageLink
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &links)); err != nil {
		return nil, fmt.Errorf("extract links: %w", err)
	}
	return links, nil
}

// ExtractForms returns all forms and their fields from the current page.
func ExtractForms(ctx context.Context) ([]PageForm, error) {
	js := `
		(function() {
			const forms = [];
			document.querySelectorAll('form').forEach(form => {
				const fields = [];
				form.querySelectorAll('input, select, textarea').forEach(el => {
					fields.push({
						tag: el.tagName.toLowerCase(),
						type: el.type || '',
						name: el.name || '',
						id: el.id || '',
						value: el.value || '',
						placeholder: el.placeholder || '',
						required: el.required || false
					});
				});
				forms.push({
					action: form.action || '',
					method: (form.method || 'GET').toUpperCase(),
					id: form.id || '',
					fields: fields
				});
			});
			return forms;
		})()
	`
	var forms []PageForm
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &forms)); err != nil {
		return nil, fmt.Errorf("extract forms: %w", err)
	}
	return forms, nil
}
