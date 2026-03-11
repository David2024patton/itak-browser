// Package browser - accessibility tree snapshot builder.
//
// What: Produces a compact, text-based accessibility tree from a live browser page.
// Why:  Raw DOM is 10,000+ tokens. An a11y tree with ref IDs is ~200 tokens.
//       Small LLMs can then reliably identify and interact with elements.
// How:  We inject JavaScript to walk the AX tree via chrome.accessibilityObject,
//       assign stable ref IDs (e1, e2, ...), and produce a clean text summary.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
)

// axNode is a minimal accessibility node returned by the injected JS.
type axNode struct {
	Role     string   `json:"role"`
	Name     string   `json:"name"`
	Value    string   `json:"value,omitempty"`
	Level    int      `json:"level,omitempty"`    // heading level
	Checked  *bool    `json:"checked,omitempty"` // checkbox state
	Children []axNode `json:"children,omitempty"`
	// NodeID is a numeric CDP backend node ID, used to build a CSS selector.
	NodeID int `json:"nodeId,omitempty"`
}

// interactiveRoles lists ARIA roles that agents typically need to interact with.
var interactiveRoles = map[string]bool{
	"button":          true,
	"link":            true,
	"textbox":         true,
	"searchbox":       true,
	"combobox":        true,
	"listbox":         true,
	"option":          true,
	"checkbox":        true,
	"radio":           true,
	"menuitem":        true,
	"menuitemcheckbox": true,
	"menuitemradio":   true,
	"tab":             true,
	"treeitem":        true,
	"switch":          true,
	"slider":          true,
	"spinbutton":      true,
}

// staticRoles are non-interactive but useful structural roles for the tree.
var staticRoles = map[string]bool{
	"heading":    true,
	"paragraph":  true,
	"img":        true,
	"list":       true,
	"listitem":   true,
	"table":      true,
	"row":        true,
	"cell":       true,
	"columnheader": true,
	"rowheader":  true,
	"form":       true,
	"region":     true,
	"navigation": true,
	"main":       true,
	"dialog":     true,
	"alert":      true,
	"status":     true,
}

// axTreeJS is injected into the page to extract a simplified accessibility tree.
// It returns a JSON-serialisable tree of only the nodes agents care about.
const axTreeJS = `
(function() {
  const interactive = new Set([
    'button','link','textbox','searchbox','combobox','listbox','option',
    'checkbox','radio','menuitem','menuitemcheckbox','menuitemradio','tab',
    'treeitem','switch','slider','spinbutton'
  ]);
  const structural = new Set([
    'heading','paragraph','img','list','listitem','table','row','cell',
    'columnheader','rowheader','form','region','navigation','main',
    'dialog','alert','status'
  ]);

  function extract(el, depth) {
    if (!el || depth > 12) return null;
    const role = el.getAttribute('role') ||
                 el.tagName.toLowerCase().replace('h1','heading')
                   .replace('h2','heading').replace('h3','heading')
                   .replace('h4','heading').replace('h5','heading')
                   .replace('h6','heading')
                   .replace('a','link').replace('button','button')
                   .replace('input','textbox').replace('textarea','textbox')
                   .replace('select','combobox').replace('img','img');

    const visible = interactive.has(role) || structural.has(role);
    if (!visible && el.children.length === 0) return null;

    const name = el.getAttribute('aria-label') ||
                 el.getAttribute('alt') ||
                 el.getAttribute('placeholder') ||
                 el.innerText?.trim().slice(0, 80) || '';

    const node = { role, name, children: [] };

    if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
      node.value = el.value || '';
    }

    const level = parseInt(el.tagName[1]);
    if (el.tagName.match(/^H[1-6]$/) && !isNaN(level)) {
      node.level = level;
      node.role = 'heading';
    }

    if (el.type === 'checkbox' || el.type === 'radio') {
      node.checked = el.checked;
    }

    for (const child of el.children) {
      const c = extract(child, depth + 1);
      if (c) node.children.push(c);
    }

    if (!visible && node.children.length === 0) return null;
    return node;
  }

  const root = extract(document.body, 0);
  return JSON.stringify(root);
})();
`

// selectorForRefJS generates a unique CSS selector for a given node by index.
// We build a secondary map from index -> querySelector path.
const selectorMapJS = `
(function() {
  const interactive = ['button','a','input','textarea','select',
                       '[role=button]','[role=link]','[role=textbox]',
                       '[role=combobox]','[role=listbox]','[role=checkbox]',
                       '[role=radio]','[role=menuitem]','[role=tab]',
                       '[role=searchbox]','[role=option]','[role=switch]',
                       '[role=slider]','[role=spinbutton]','[role=treeitem]'];

  const allQuery = interactive.join(',');
  const elements = Array.from(document.querySelectorAll(allQuery));

  return JSON.stringify(elements.map((el, i) => {
    // Build a unique nth-of-type path for reliable identification.
    function path(node) {
      if (!node || node === document.body) return 'body';
      const tag = node.tagName.toLowerCase();
      if (node.id) return '#' + CSS.escape(node.id);
      const parent = node.parentElement;
      if (!parent) return tag;
      const siblings = Array.from(parent.children).filter(c => c.tagName === node.tagName);
      const idx = siblings.indexOf(node) + 1;
      return path(parent) + ' > ' + tag + ':nth-of-type(' + idx + ')';
    }
    return { index: i, selector: path(el), role: el.getAttribute('role') || el.tagName.toLowerCase(), name: el.getAttribute('aria-label') || el.innerText?.trim().slice(0,80) || el.getAttribute('placeholder') || el.getAttribute('alt') || '' };
  }));
})();
`

// BuildAccessibilityTree extracts the page accessibility tree.
// Returns:
//   - refs:  map of ref ID -> Ref (with CSS selectors for internal resolution)
//   - tree:  human-readable text tree for the agent prompt
//   - error: any CDP error
func BuildAccessibilityTree(ctx context.Context) (map[string]Ref, string, error) {
	// Step 1: Extract interactive elements with selectors.
	var selectorJSON string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(selectorMapJS, &selectorJSON),
	); err != nil {
		return nil, "", fmt.Errorf("ax: selector map: %w", err)
	}

	type rawEl struct {
		Index    int    `json:"index"`
		Selector string `json:"selector"`
		Role     string `json:"role"`
		Name     string `json:"name"`
	}
	var rawEls []rawEl
	if err := json.Unmarshal([]byte(selectorJSON), &rawEls); err != nil {
		return nil, "", fmt.Errorf("ax: parse selector map: %w", err)
	}

	// Step 2: Build refs map.
	refs := make(map[string]Ref, len(rawEls))
	for i, el := range rawEls {
		id := fmt.Sprintf("e%d", i+1)
		role := normaliseRole(el.Role)
		refs[id] = Ref{
			ID:       id,
			Role:     role,
			Name:     el.Name,
			Selector: el.Selector,
		}
	}

	// Step 3: Extract full structural tree for the text representation.
	var treeJSON string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(axTreeJS, &treeJSON),
	); err != nil {
		return refs, buildSimpleTree(refs), nil // degrade gracefully
	}

	var root axNode
	if err := json.Unmarshal([]byte(treeJSON), &root); err != nil {
		return refs, buildSimpleTree(refs), nil
	}

	tree := renderTree(root, refs, 0)
	return refs, tree, nil
}

// renderTree converts the axNode tree to the compact agent-readable format.
func renderTree(node axNode, refs map[string]Ref, depth int) string {
	if node.Role == "" {
		return ""
	}

	indent := strings.Repeat("  ", depth)
	var sb strings.Builder

	// Find the ref ID for this node.
	refID := findRefByName(refs, node.Role, node.Name)
	refStr := ""
	if refID != "" {
		refStr = fmt.Sprintf(" [ref=%s]", refID)
	}

	switch node.Role {
	case "heading":
		level := node.Level
		if level == 0 {
			level = 1
		}
		sb.WriteString(fmt.Sprintf("%s- heading %q%s [level=%d]\n", indent, node.Name, refStr, level))
	case "img":
		sb.WriteString(fmt.Sprintf("%s- image %q%s\n", indent, node.Name, refStr))
	default:
		namePart := ""
		if node.Name != "" {
			namePart = fmt.Sprintf(" %q", node.Name)
		}
		valuePart := ""
		if node.Value != "" {
			valuePart = fmt.Sprintf(" value=%q", node.Value)
		}
		checkedPart := ""
		if node.Checked != nil {
			if *node.Checked {
				checkedPart = " [checked]"
			} else {
				checkedPart = " [unchecked]"
			}
		}
		sb.WriteString(fmt.Sprintf("%s- %s%s%s%s%s\n", indent, node.Role, namePart, valuePart, checkedPart, refStr))
	}

	for _, child := range node.Children {
		sb.WriteString(renderTree(child, refs, depth+1))
	}

	return sb.String()
}

// buildSimpleTree constructs a flat tree from the refs map when the full AX walk fails.
func buildSimpleTree(refs map[string]Ref) string {
	var sb strings.Builder
	for i := 1; i <= len(refs); i++ {
		id := fmt.Sprintf("e%d", i)
		ref, ok := refs[id]
		if !ok {
			continue
		}
		name := ""
		if ref.Name != "" {
			name = fmt.Sprintf(" %q", ref.Name)
		}
		sb.WriteString(fmt.Sprintf("- %s%s [ref=%s]\n", ref.Role, name, id))
	}
	return sb.String()
}

// findRefByName searches the refs map for an element matching role+name.
// This bridges the structural tree walk with the selector-indexed ref map.
func findRefByName(refs map[string]Ref, role, name string) string {
	normRole := normaliseRole(role)
	for id, ref := range refs {
		if ref.Role == normRole && (name == "" || ref.Name == name || strings.HasPrefix(ref.Name, name)) {
			return id
		}
	}
	return ""
}

// normaliseRole maps raw tag names to canonical roles.
func normaliseRole(tag string) string {
	switch strings.ToLower(tag) {
	case "a":
		return "link"
	case "input", "textarea":
		return "textbox"
	case "select":
		return "combobox"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	default:
		return strings.ToLower(tag)
	}
}
