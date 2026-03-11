// Package browser - types re-exported from the Core contract for internal use.
//
// What: Local type aliases mirroring pkg/contract so this module compiles
//
//	standalone without a hard dependency on iTaKCore.
//
// Why:  iTaK Browser ships as a self-contained binary. The full Core module
//
//	is only required at runtime when integrating into the wider ecosystem.
//
// How:  The daemon's registration call uses the Core registry at runtime;
//
//	the types declared here are structurally identical to contract types.
package browser

// Snapshot is the compact accessibility tree representation of a page.
// Uses Ref IDs instead of CSS selectors -- 93% fewer tokens than raw DOM.
type Snapshot struct {
	URL   string         `json:"url"`
	Title string         `json:"title"`
	Tree  string         `json:"tree"` // Text-based accessibility tree
	Refs  map[string]Ref `json:"refs"` // ref ID -> element metadata
}

// Ref is a single interactive element in the snapshot tree.
type Ref struct {
	ID       string `json:"id"`       // e.g., "e1", "e2"
	Role     string `json:"role"`     // e.g., "button", "textbox", "link"
	Name     string `json:"name"`     // visible text or ARIA label
	Selector string `json:"selector"` // resolved CSS selector (internal, not shown to agent)
}

// ScreenshotResult contains a captured screenshot and optional annotation data.
type ScreenshotResult struct {
	Path      string `json:"path"`      // Absolute path to the saved PNG
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Annotated bool   `json:"annotated"` // True if numbered element labels are overlaid
}
