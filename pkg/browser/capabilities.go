// Package browser - Self-describing API capabilities for AI agents.
//
// What: Returns a structured description of every available command,
//       its endpoint, parameters, and natural-language description.
// Why:  An AI agent connecting to the browser for the first time needs to
//       know what tools it has. This enables zero-setup agent integration -
//       the agent reads /capabilities and immediately knows its full toolset.
//       Also works as an injectable system prompt for the teacher model.
// How:  Static definition of all commands, returned as structured JSON.
package browser

// Capability describes a single browser command available to the agent.
type Capability struct {
	Name        string   `json:"name"`
	Endpoint    string   `json:"endpoint"`
	Method      string   `json:"method"`
	Args        []string `json:"args,omitempty"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
}

// CapabilitiesResponse is the full self-describing API response.
type CapabilitiesResponse struct {
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Categories   []string     `json:"categories"`
	Commands     []Capability `json:"commands"`
	TotalCommands int         `json:"total_commands"`
}

// GetCapabilities returns the complete list of available browser commands.
func GetCapabilities() CapabilitiesResponse {
	commands := []Capability{
		// Navigation
		{Name: "open", Endpoint: "/open", Method: "POST", Args: []string{"url"}, Description: "Navigate to a URL. Auto-scans for AI poisoning after navigation.", Category: "navigation"},
		{Name: "snapshot", Endpoint: "/snapshot", Method: "POST", Args: nil, Description: "Get compact accessibility tree of the page (Snapshot+Refs). Returns element refs like e1, e2 for interaction.", Category: "navigation"},
		{Name: "screenshot", Endpoint: "/screenshot", Method: "POST", Args: []string{"annotate (optional)"}, Description: "Capture a PNG screenshot. Use annotate=true to overlay element refs for vision models.", Category: "navigation"},
		{Name: "pdf", Endpoint: "/pdf", Method: "POST", Args: nil, Description: "Save current page as PDF.", Category: "navigation"},
		{Name: "wait-nav", Endpoint: "/wait-nav", Method: "POST", Args: nil, Description: "Wait for page navigation to complete (30s timeout). Use after clicking links.", Category: "navigation"},
		{Name: "wait-idle", Endpoint: "/wait-idle", Method: "POST", Args: nil, Description: "Wait until no network requests for 2 seconds (30s timeout). Use for AJAX pages.", Category: "navigation"},
		{Name: "history", Endpoint: "/history", Method: "POST", Args: nil, Description: "Get full navigation history (URLs visited in this session).", Category: "navigation"},

		// Interaction
		{Name: "click", Endpoint: "/click", Method: "POST", Args: []string{"ref"}, Description: "Click an element by its ref (e.g., e1, e2 from snapshot).", Category: "interaction"},
		{Name: "dblclick", Endpoint: "/dblclick", Method: "POST", Args: []string{"ref"}, Description: "Double-click an element by ref.", Category: "interaction"},
		{Name: "fill", Endpoint: "/fill", Method: "POST", Args: []string{"ref", "text"}, Description: "Type text into an input field by ref. Clears existing content first.", Category: "interaction"},
		{Name: "press", Endpoint: "/press", Method: "POST", Args: []string{"text"}, Description: "Send keyboard keys (e.g., 'Enter', 'Tab', 'Escape', 'ArrowDown').", Category: "interaction"},
		{Name: "scroll", Endpoint: "/scroll", Method: "POST", Args: []string{"delta_x", "delta_y"}, Description: "Scroll the page. Positive delta_y scrolls down, negative scrolls up.", Category: "interaction"},
		{Name: "hover", Endpoint: "/hover", Method: "POST", Args: []string{"ref"}, Description: "Hover over an element by ref. Useful for dropdown menus and tooltips.", Category: "interaction"},
		{Name: "select", Endpoint: "/select", Method: "POST", Args: []string{"ref", "values"}, Description: "Select options in a <select> element by value.", Category: "interaction"},
		{Name: "check", Endpoint: "/check", Method: "POST", Args: []string{"ref"}, Description: "Check a checkbox by ref.", Category: "interaction"},
		{Name: "uncheck", Endpoint: "/uncheck", Method: "POST", Args: []string{"ref"}, Description: "Uncheck a checkbox by ref.", Category: "interaction"},
		{Name: "upload", Endpoint: "/upload", Method: "POST", Args: []string{"ref", "path"}, Description: "Upload a file to a file input element.", Category: "interaction"},
		{Name: "drag", Endpoint: "/drag", Method: "POST", Args: []string{"ref", "delta_x", "delta_y"}, Description: "Drag an element by ref with pixel offset.", Category: "interaction"},

		// Data Extraction
		{Name: "get-text", Endpoint: "/get-text", Method: "POST", Args: []string{"ref"}, Description: "Get the text content of an element by ref.", Category: "extraction"},
		{Name: "get-attr", Endpoint: "/get-attr", Method: "POST", Args: []string{"ref", "attr"}, Description: "Get an attribute value of an element (e.g., href, src, class).", Category: "extraction"},
		{Name: "eval", Endpoint: "/eval", Method: "POST", Args: []string{"js"}, Description: "Execute arbitrary JavaScript and return the result.", Category: "extraction"},
		{Name: "links", Endpoint: "/get-links", Method: "POST", Args: nil, Description: "Extract all hyperlinks from the page (text, href, rel, target).", Category: "extraction"},
		{Name: "forms", Endpoint: "/get-forms", Method: "POST", Args: nil, Description: "Extract all forms and their fields (action, method, type, name, value).", Category: "extraction"},
		{Name: "metrics", Endpoint: "/metrics", Method: "POST", Args: nil, Description: "Get page performance data: DOM nodes, depth, load time, transfer size, JS heap.", Category: "extraction"},
		{Name: "style", Endpoint: "/style", Method: "POST", Args: []string{"ref", "properties (optional)"}, Description: "Get CSS computed styles for an element (color, display, visibility, etc.).", Category: "extraction"},

		// Tabs
		{Name: "tab new", Endpoint: "/tab/new", Method: "POST", Args: []string{"url (optional)"}, Description: "Open a new browser tab, optionally navigating to a URL.", Category: "tabs"},
		{Name: "tab switch", Endpoint: "/tab/switch", Method: "POST", Args: []string{"ref"}, Description: "Switch to a different tab by its ID.", Category: "tabs"},
		{Name: "tab close", Endpoint: "/tab/close", Method: "POST", Args: []string{"ref"}, Description: "Close a tab by its ID.", Category: "tabs"},
		{Name: "tab list", Endpoint: "/tab/list", Method: "POST", Args: nil, Description: "List all open tabs with their URLs and titles.", Category: "tabs"},

		// Cookies & Storage
		{Name: "cookies get", Endpoint: "/cookies", Method: "POST", Args: nil, Description: "List all cookies (name, value, domain, path, httpOnly, secure).", Category: "state"},
		{Name: "cookies set", Endpoint: "/cookies/set", Method: "POST", Args: []string{"key", "value", "domain"}, Description: "Set a cookie.", Category: "state"},
		{Name: "cookies delete", Endpoint: "/cookies/delete", Method: "POST", Args: []string{"key"}, Description: "Delete a cookie by name.", Category: "state"},
		{Name: "cookies clear", Endpoint: "/cookies/clear", Method: "POST", Args: nil, Description: "Delete all cookies.", Category: "state"},
		{Name: "storage get", Endpoint: "/storage", Method: "POST", Args: []string{"storage_type (local|session)"}, Description: "List all localStorage or sessionStorage entries.", Category: "state"},
		{Name: "storage set", Endpoint: "/storage/set", Method: "POST", Args: []string{"key", "value", "storage_type"}, Description: "Set a localStorage/sessionStorage entry.", Category: "state"},
		{Name: "storage clear", Endpoint: "/storage/clear", Method: "POST", Args: []string{"storage_type"}, Description: "Clear all localStorage or sessionStorage entries.", Category: "state"},

		// Frames
		{Name: "frames list", Endpoint: "/frames", Method: "POST", Args: nil, Description: "List all iframes on the page (index, id, name, src).", Category: "frames"},
		{Name: "frames content", Endpoint: "/frames/content", Method: "POST", Args: []string{"index"}, Description: "Get text content from an iframe by index.", Category: "frames"},

		// Dialogs
		{Name: "dialogs list", Endpoint: "/dialogs", Method: "POST", Args: nil, Description: "Show captured JS dialog events (alert/confirm/prompt). Dialogs are auto-handled.", Category: "dialogs"},
		{Name: "dialogs mode", Endpoint: "/dialogs/mode", Method: "POST", Args: []string{"mode (accept|dismiss)"}, Description: "Set dialog auto-handling behavior.", Category: "dialogs"},

		// DOM Mutations
		{Name: "mutations start", Endpoint: "/mutations/start", Method: "POST", Args: []string{"selector (optional)"}, Description: "Begin watching for DOM changes (added/removed elements, attribute changes).", Category: "mutations"},
		{Name: "mutations get", Endpoint: "/mutations", Method: "POST", Args: []string{"clear (optional)"}, Description: "Retrieve captured DOM mutations since last read.", Category: "mutations"},
		{Name: "mutations stop", Endpoint: "/mutations/stop", Method: "POST", Args: nil, Description: "Stop watching DOM changes.", Category: "mutations"},

		// Downloads
		{Name: "downloads list", Endpoint: "/downloads", Method: "POST", Args: nil, Description: "List all tracked file downloads with paths and sizes.", Category: "downloads"},
		{Name: "downloads scan", Endpoint: "/downloads/scan", Method: "POST", Args: nil, Description: "Scan for newly downloaded files.", Category: "downloads"},

		// Overrides
		{Name: "geo set", Endpoint: "/geo/set", Method: "POST", Args: []string{"lat", "lon", "accuracy (optional)"}, Description: "Spoof browser GPS coordinates.", Category: "overrides"},
		{Name: "geo clear", Endpoint: "/geo/clear", Method: "POST", Args: nil, Description: "Remove geolocation override.", Category: "overrides"},
		{Name: "ua set", Endpoint: "/ua/set", Method: "POST", Args: []string{"value"}, Description: "Set User-Agent. Presets: chrome-win, chrome-mac, firefox-win, safari-mac, iphone, android, googlebot.", Category: "overrides"},

		// Clipboard
		{Name: "clipboard read", Endpoint: "/clipboard", Method: "POST", Args: nil, Description: "Read the clipboard text content.", Category: "clipboard"},
		{Name: "clipboard write", Endpoint: "/clipboard/write", Method: "POST", Args: []string{"text"}, Description: "Write text to the clipboard.", Category: "clipboard"},

		// Debug & Monitoring
		{Name: "console", Endpoint: "/console", Method: "POST", Args: nil, Description: "Get captured console.log, console.warn, console.error messages from the page.", Category: "monitoring"},
		{Name: "network", Endpoint: "/network", Method: "POST", Args: nil, Description: "Get captured network request/response pairs with timing and status codes.", Category: "monitoring"},
		{Name: "debug", Endpoint: "/debug", Method: "POST", Args: nil, Description: "All-in-one debug bundle: snapshot + screenshot + console + network + security scan.", Category: "monitoring"},

		// Security
		{Name: "threats", Endpoint: "/threats", Method: "GET", Args: nil, Description: "Get all recorded AI recommendation poisoning detections.", Category: "security"},
		{Name: "threats stats", Endpoint: "/threats/stats", Method: "GET", Args: nil, Description: "Get aggregate threat statistics (total, unique domains, severity breakdown).", Category: "security"},
		{Name: "webhook", Endpoint: "/webhook", Method: "POST", Args: []string{"url"}, Description: "Set webhook URL for real-time threat detection notifications.", Category: "security"},

		// Blocker
		{Name: "blocker stats", Endpoint: "/blocker/stats", Method: "POST", Args: nil, Description: "Get request blocking statistics.", Category: "blocker"},
		{Name: "blocker add", Endpoint: "/blocker/add", Method: "POST", Args: []string{"url"}, Description: "Add a domain to the block list.", Category: "blocker"},
		{Name: "blocker remove", Endpoint: "/blocker/remove", Method: "POST", Args: []string{"url"}, Description: "Remove a domain from the block list.", Category: "blocker"},

		// Teaching
		{Name: "record start", Endpoint: "/record/start", Method: "POST", Args: []string{"title (optional)"}, Description: "Begin recording actions for a teaching lesson.", Category: "teaching"},
		{Name: "record stop", Endpoint: "/record/stop", Method: "POST", Args: nil, Description: "Stop recording and get the lesson plan.", Category: "teaching"},
		{Name: "record steps", Endpoint: "/record/steps", Method: "POST", Args: nil, Description: "Get current recorded steps without stopping.", Category: "teaching"},
		{Name: "record narrate", Endpoint: "/record/narrate", Method: "POST", Args: []string{"text"}, Description: "Add a narration/explanation step to the recording.", Category: "teaching"},
		{Name: "highlight", Endpoint: "/highlight", Method: "POST", Args: []string{"ref"}, Description: "Highlight an element with a glowing blue outline in headed mode. Used for teaching.", Category: "teaching"},

		// Session
		{Name: "session new", Endpoint: "/session/new", Method: "POST", Args: []string{"headed", "stealth"}, Description: "Create a new browser session.", Category: "session"},
		{Name: "session save", Endpoint: "/session/save", Method: "POST", Args: []string{"passphrase"}, Description: "Save session cookies encrypted with AES-256-GCM.", Category: "session"},
		{Name: "session restore", Endpoint: "/session/restore", Method: "POST", Args: []string{"passphrase"}, Description: "Restore saved session cookies.", Category: "session"},
	}

	categories := []string{
		"navigation", "interaction", "extraction", "tabs", "state",
		"frames", "dialogs", "mutations", "downloads", "overrides",
		"clipboard", "monitoring", "security", "blocker", "teaching", "session",
	}

	return CapabilitiesResponse{
		Version:       Version,
		Description:   "iTaK Browser - AI-native browser automation with built-in security, teaching support, and self-describing API.",
		Categories:    categories,
		Commands:      commands,
		TotalCommands: len(commands),
	}
}

// GetSystemPrompt returns the capabilities as a formatted system prompt
// that can be injected into an AI agent's context.
func GetSystemPrompt() string {
	caps := GetCapabilities()
	prompt := "# iTaK Browser v" + caps.Version + "\n\n"
	prompt += "You are connected to iTaK Browser, an AI-native browser automation tool.\n"
	prompt += "Use the session ID provided to interact with the browser.\n\n"
	prompt += "## Available Commands\n\n"

	currentCat := ""
	for _, c := range caps.Commands {
		if c.Category != currentCat {
			currentCat = c.Category
			prompt += "\n### " + currentCat + "\n"
		}
		args := ""
		if len(c.Args) > 0 {
			for i, a := range c.Args {
				if i > 0 {
					args += ", "
				}
				args += a
			}
			args = " (" + args + ")"
		}
		prompt += "- **" + c.Name + "**" + args + " - " + c.Description + "\n"
	}

	prompt += "\n## Key Concepts\n"
	prompt += "- Use `snapshot` to get element refs (e1, e2, ...). Use these refs with click, fill, etc.\n"
	prompt += "- Dialogs (alert/confirm/prompt) are auto-handled. Check `dialogs list` to see captured dialogs.\n"
	prompt += "- Console logs are captured automatically. Use `console` to read them.\n"
	prompt += "- Every navigation auto-scans for AI recommendation poisoning.\n"
	prompt += "- Use `record start` to begin recording a teaching lesson.\n"

	return prompt
}
