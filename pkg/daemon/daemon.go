// Package daemon implements the persistent iTaK Browser daemon.
//
// What: A long-lived HTTP server that holds a Chrome instance between CLI calls.
// Why:  Starting Chrome on every CLI invocation adds 1-3 seconds of cold-start
//       latency. The daemon keeps Chrome alive so sequential agent commands
//       complete in milliseconds.
// How:  The daemon listens on a local Unix socket (or TCP loopback). The CLI
//       connects, sends a command JSON payload, receives a JSON result.
//       If the daemon is not running, the CLI starts it automatically.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/David2024patton/iTaKBrowser/pkg/browser"
)

const (
	// DefaultPort is the daemon's default TCP port.
	DefaultPort = 43721
	// Version matches the browser module version.
	Version = browser.Version
)

// Daemon holds the browser engine and exposes it over HTTP.
type Daemon struct {
	mu      sync.RWMutex
	engines map[string]*browser.Engine // session ID -> engine

	port      int
	server    *http.Server
	ledger    *browser.ThreatLedger
	startTime time.Time
	logger    *slog.Logger
}

// New creates a new Daemon.
func New(port int, logger *slog.Logger) *Daemon {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize the ThreatLedger in the user's data directory.
	ledgerDir := ".itak-browser"
	if home, err := os.UserHomeDir(); err == nil {
		ledgerDir = filepath.Join(home, ".itak-browser", "threats")
	}
	ledger, err := browser.NewThreatLedger(ledgerDir)
	if err != nil {
		logger.Warn("threatledger: failed to init, detections will not persist", "err", err)
	}

	return &Daemon{
		engines:   make(map[string]*browser.Engine),
		port:      port,
		ledger:    ledger,
		startTime: time.Now(),
		logger:    logger,
	}
}

// Start begins serving the daemon HTTP API.
func (d *Daemon) Start() error {
	mux := http.NewServeMux()
	// Session management
	mux.HandleFunc("POST /session/new", d.handleSessionNew)
	mux.HandleFunc("POST /session/close", d.handleSessionClose)
	mux.HandleFunc("POST /session/save", d.handleSaveSession)
	mux.HandleFunc("POST /session/restore", d.handleRestoreSession)
	// Navigation
	mux.HandleFunc("POST /open", d.handleOpen)
	mux.HandleFunc("POST /back", d.handleBack)
	mux.HandleFunc("POST /forward", d.handleForward)
	mux.HandleFunc("POST /reload", d.handleReload)
	// Snapshot / Capture
	mux.HandleFunc("POST /snapshot", d.handleSnapshot)
	mux.HandleFunc("POST /screenshot", d.handleScreenshot)
	mux.HandleFunc("POST /pdf", d.handlePDF)
	// Interaction (v0.1)
	mux.HandleFunc("POST /click", d.handleClick)
	mux.HandleFunc("POST /fill", d.handleFill)
	mux.HandleFunc("POST /press", d.handlePress)
	mux.HandleFunc("POST /scroll", d.handleScroll)
	mux.HandleFunc("POST /eval", d.handleEval)
	// Interaction (v0.2 - headed mode expansion)
	mux.HandleFunc("POST /dblclick", d.handleDblClick)
	mux.HandleFunc("POST /hover", d.handleHover)
	mux.HandleFunc("POST /select", d.handleSelect)
	mux.HandleFunc("POST /check", d.handleCheck)
	mux.HandleFunc("POST /uncheck", d.handleUncheck)
	mux.HandleFunc("POST /upload", d.handleUpload)
	mux.HandleFunc("POST /mouse-move", d.handleMouseMove)
	mux.HandleFunc("POST /mouse-click", d.handleMouseClick)
	mux.HandleFunc("POST /resize", d.handleResize)
	// Data extraction (v0.1)
	mux.HandleFunc("POST /get-text", d.handleGetText)
	mux.HandleFunc("POST /get-html", d.handleGetHTML)
	mux.HandleFunc("POST /get-title", d.handleGetTitle)
	mux.HandleFunc("POST /get-url", d.handleGetURL)
	mux.HandleFunc("POST /wait-visible", d.handleWaitVisible)
	// Data extraction (v0.2 - agent debug features)
	mux.HandleFunc("POST /get-value", d.handleGetValue)
	mux.HandleFunc("POST /get-attr", d.handleGetAttr)
	mux.HandleFunc("POST /get-box", d.handleGetBox)
	mux.HandleFunc("POST /is-visible", d.handleIsVisible)
	mux.HandleFunc("POST /is-enabled", d.handleIsEnabled)
	mux.HandleFunc("POST /is-checked", d.handleIsChecked)
	mux.HandleFunc("POST /console", d.handleConsole)
	mux.HandleFunc("POST /network", d.handleNetwork)
	mux.HandleFunc("POST /debug", d.handleDebug)
	// Threat feed (public API for website polling)
	mux.HandleFunc("GET /threats", d.handleGetThreats)
	mux.HandleFunc("GET /threats/stats", d.handleGetThreatStats)
	// v0.2.1: Tabs, Metrics, Extract, Wait, Webhook, Blocker
	mux.HandleFunc("POST /tab/new", d.handleTabNew)
	mux.HandleFunc("POST /tab/switch", d.handleTabSwitch)
	mux.HandleFunc("POST /tab/close", d.handleTabClose)
	mux.HandleFunc("POST /tab/list", d.handleTabList)
	mux.HandleFunc("POST /metrics", d.handleMetrics)
	mux.HandleFunc("POST /get-links", d.handleGetLinks)
	mux.HandleFunc("POST /get-forms", d.handleGetForms)
	mux.HandleFunc("POST /wait-nav", d.handleWaitNav)
	mux.HandleFunc("POST /wait-idle", d.handleWaitIdle)
	mux.HandleFunc("POST /webhook", d.handleSetWebhook)
	mux.HandleFunc("POST /blocker/stats", d.handleBlockerStats)
	mux.HandleFunc("POST /blocker/add", d.handleBlockerAdd)
	mux.HandleFunc("POST /blocker/remove", d.handleBlockerRemove)
	// v0.2.2: Cookies, Storage, Dialogs, Mutations, Frames, Downloads, Overrides, Clipboard
	mux.HandleFunc("POST /cookies", d.handleCookies)
	mux.HandleFunc("POST /cookies/set", d.handleCookieSet)
	mux.HandleFunc("POST /cookies/delete", d.handleCookieDelete)
	mux.HandleFunc("POST /cookies/clear", d.handleCookieClear)
	mux.HandleFunc("POST /storage", d.handleStorage)
	mux.HandleFunc("POST /storage/set", d.handleStorageSet)
	mux.HandleFunc("POST /storage/clear", d.handleStorageClear)
	mux.HandleFunc("POST /dialogs", d.handleDialogs)
	mux.HandleFunc("POST /dialogs/mode", d.handleDialogMode)
	mux.HandleFunc("POST /mutations/start", d.handleMutationsStart)
	mux.HandleFunc("POST /mutations", d.handleMutationsGet)
	mux.HandleFunc("POST /mutations/stop", d.handleMutationsStop)
	mux.HandleFunc("POST /frames", d.handleFrames)
	mux.HandleFunc("POST /frames/content", d.handleFrameContent)
	mux.HandleFunc("POST /downloads", d.handleDownloads)
	mux.HandleFunc("POST /downloads/scan", d.handleDownloadScan)
	mux.HandleFunc("POST /geo/set", d.handleGeoSet)
	mux.HandleFunc("POST /geo/clear", d.handleGeoClear)
	mux.HandleFunc("POST /ua/set", d.handleUASet)
	mux.HandleFunc("POST /clipboard", d.handleClipboardRead)
	mux.HandleFunc("POST /clipboard/write", d.handleClipboardWrite)
	// v0.2.3: Capabilities, History, Recording, Teaching, Annotations, Styles, Drag
	mux.HandleFunc("GET /capabilities", d.handleCapabilities)
	mux.HandleFunc("GET /system-prompt", d.handleSystemPrompt)
	mux.HandleFunc("POST /history", d.handleHistory)
	mux.HandleFunc("POST /record/start", d.handleRecordStart)
	mux.HandleFunc("POST /record/stop", d.handleRecordStop)
	mux.HandleFunc("POST /record/steps", d.handleRecordSteps)
	mux.HandleFunc("POST /record/narrate", d.handleRecordNarrate)
	mux.HandleFunc("POST /highlight", d.handleHighlight)
	mux.HandleFunc("POST /annotate/circle", d.handleCircle)
	mux.HandleFunc("POST /annotate/arrow", d.handleArrow)
	mux.HandleFunc("POST /annotate/text", d.handleDrawText)
	mux.HandleFunc("POST /annotate/clear", d.handleClearAnnotations)
	mux.HandleFunc("POST /style", d.handleStyle)
	mux.HandleFunc("POST /drag", d.handleDrag)
	mux.HandleFunc("POST /toolbar", d.handleToolbarInject)
	mux.HandleFunc("POST /toolbar/remove", d.handleToolbarRemove)
	mux.HandleFunc("POST /toolbar/markup", d.handleToolbarMarkup)
	// v0.3.0: Spotlight, Diff, StorageInspect, A11y, Inspector, Waterfall, Autofill, TabStrip, ScreenRec, Translate
	mux.HandleFunc("POST /spotlight", d.handleSpotlight)
	mux.HandleFunc("POST /spotlight/clear", d.handleSpotlightClear)
	mux.HandleFunc("POST /diff/snapshot", d.handleDiffSnapshot)
	mux.HandleFunc("POST /diff/compare", d.handleDiffCompare)
	mux.HandleFunc("POST /diff/clear", d.handleDiffClear)
	mux.HandleFunc("POST /search", d.handleSearch)
	mux.HandleFunc("POST /storage-inspect", d.handleStorageInspect)
	mux.HandleFunc("POST /storage-inspect/close", d.handleStorageInspectClose)
	mux.HandleFunc("POST /a11y", d.handleA11yAudit)
	mux.HandleFunc("POST /a11y/clear", d.handleA11yClear)
	mux.HandleFunc("POST /inspector", d.handleInspectorOpen)
	mux.HandleFunc("POST /inspector/close", d.handleInspectorClose)
	mux.HandleFunc("POST /waterfall", d.handleWaterfallOpen)
	mux.HandleFunc("POST /waterfall/close", d.handleWaterfallClose)
	mux.HandleFunc("POST /autofill/save", d.handleAutofillSave)
	mux.HandleFunc("POST /autofill/load", d.handleAutofillLoad)
	mux.HandleFunc("POST /autofill/list", d.handleAutofillList)
	mux.HandleFunc("POST /tabstrip", d.handleTabStripOpen)
	mux.HandleFunc("POST /tabstrip/close", d.handleTabStripClose)
	mux.HandleFunc("POST /screenrec/start", d.handleScreenRecStart)
	mux.HandleFunc("POST /screenrec/stop", d.handleScreenRecStop)
	mux.HandleFunc("POST /screenrec/play", d.handleScreenRecPlay)
	mux.HandleFunc("POST /translate", d.handleTranslate)
	mux.HandleFunc("POST /translate/clear", d.handleTranslateClear)
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.HandleFunc("GET /sessions", d.handleListSessions)

	d.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", d.port),
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  300 * time.Second,
	}

	d.logger.Info("daemon listening", "addr", d.server.Addr)
	return d.server.ListenAndServe()
}

// Stop gracefully shuts down all sessions and the HTTP server.
func (d *Daemon) Stop() {
	d.mu.Lock()
	for id, eng := range d.engines {
		if err := eng.Close(context.Background()); err != nil {
			d.logger.Warn("close engine", "session", id, "err", err)
		}
	}
	d.engines = make(map[string]*browser.Engine)
	d.mu.Unlock()

	if d.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = d.server.Shutdown(ctx)
	}
}

// ---- HTTP Handlers ----

// cmdRequest is the standard request envelope for all daemon commands.
type cmdRequest struct {
	Session      string   `json:"session"`                   // Optional; creates new if empty
	URL          string   `json:"url,omitempty"`
	Ref          string   `json:"ref,omitempty"`
	Text         string   `json:"text,omitempty"`
	JS           string   `json:"js,omitempty"`
	Annotate     bool     `json:"annotate,omitempty"`
	Headed       bool     `json:"headed,omitempty"`           // Explicit headed (visible) mode
	Stealth      bool     `json:"stealth,omitempty"`
	ProfileDir   string   `json:"profile_dir,omitempty"`
	DeltaX       float64  `json:"delta_x,omitempty"`
	DeltaY       float64  `json:"delta_y,omitempty"`
	Selector     string   `json:"selector,omitempty"`
	Path         string   `json:"path,omitempty"`
	Passphrase   string   `json:"passphrase,omitempty"`
	WindowWidth  int      `json:"window_width,omitempty"`    // Viewport width (default 1920)
	WindowHeight int      `json:"window_height,omitempty"`   // Viewport height (default 1080)
	X            float64  `json:"x,omitempty"`               // Pixel X for mouse ops
	Y            float64  `json:"y,omitempty"`               // Pixel Y for mouse ops
	Button       string   `json:"button,omitempty"`          // Mouse button (left/right/middle)
	Values       []string `json:"values,omitempty"`          // Select option values
	Attr         string   `json:"attr,omitempty"`            // Attribute name
	Width        int      `json:"width,omitempty"`           // Resize width
	Height       int      `json:"height,omitempty"`          // Resize height
	// v0.2.2 fields
	Key          string   `json:"key,omitempty"`             // Cookie/storage key
	Value        string   `json:"value,omitempty"`           // Cookie/storage value
	Domain       string   `json:"domain,omitempty"`          // Cookie domain
	StorageType  string   `json:"storage_type,omitempty"`    // "local" or "session"
	Mode         string   `json:"mode,omitempty"`            // Dialog mode: accept/dismiss
	Clear        bool     `json:"clear,omitempty"`           // Clear mutations on read
	Index        int      `json:"index,omitempty"`           // Frame index
	Lat          float64  `json:"lat,omitempty"`             // Latitude
	Lon          float64  `json:"lon,omitempty"`             // Longitude
	Accuracy     float64  `json:"accuracy,omitempty"`        // Geo accuracy meters
	Title        string   `json:"title,omitempty"`           // Record lesson title
	Label        string   `json:"label,omitempty"`           // Annotation label
	Properties   []string `json:"properties,omitempty"`      // CSS properties
	// v0.3.0 fields
	Name         string   `json:"name,omitempty"`            // Profile/snapshot name
	Lang         string   `json:"lang,omitempty"`            // Translation target language
	FPS          int      `json:"fps,omitempty"`             // Screen recording FPS
}

// cmdResponse is the standard response envelope.
type cmdResponse struct {
	OK      bool        `json:"ok"`
	Session string      `json:"session,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, session string, data interface{}) {
	writeJSON(w, http.StatusOK, cmdResponse{OK: true, Session: session, Data: data})
}

func fail(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, cmdResponse{OK: false, Error: err.Error()})
}

func (d *Daemon) decode(r *http.Request) (cmdRequest, error) {
	var req cmdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("decode request: %w", err)
	}
	return req, nil
}

// getOrCreate returns an existing engine or creates a new one.
//
// What: Session lookup or creation.
// Why:  The daemon manages N sessions. Agents create them once and reuse.
// How:  If session ID matches an existing engine, return it. Otherwise
//       create a new one using the request's headed/stealth/viewport settings.
func (d *Daemon) getOrCreate(req cmdRequest) (*browser.Engine, string, error) {
	if req.Session != "" {
		d.mu.RLock()
		eng, exists := d.engines[req.Session]
		d.mu.RUnlock()
		if exists {
			return eng, req.Session, nil
		}
	}

	// Default: headless unless explicitly headed.
	headless := !req.Headed

	// Viewport defaults.
	ww := req.WindowWidth
	wh := req.WindowHeight
	if ww <= 0 {
		ww = 1920
	}
	if wh <= 0 {
		wh = 1080
	}

	opts := browser.Options{
		SessionID:    req.Session,
		ProfileDir:   req.ProfileDir,
		Headless:     headless,
		Stealth:      req.Stealth,
		WindowWidth:  ww,
		WindowHeight: wh,
	}

	eng, err := browser.New(opts)
	if err != nil {
		return nil, "", err
	}

	d.mu.Lock()
	d.engines[eng.SessionID()] = eng
	d.mu.Unlock()

	return eng, eng.SessionID(), nil
}

func (d *Daemon) get(sessionID string) (*browser.Engine, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	eng, ok := d.engines[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return eng, nil
}

// handleSessionNew creates a new browser session and returns its ID.
func (d *Daemon) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}

	eng, sessionID, err := d.getOrCreate(req)
	if err != nil {
		fail(w, err)
		return
	}
	_ = eng
	ok(w, sessionID, map[string]string{"session": sessionID})
}

// handleSessionClose closes a session and removes it from the daemon.
func (d *Daemon) handleSessionClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}

	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}

	if err := eng.Close(r.Context()); err != nil {
		fail(w, err)
		return
	}

	d.mu.Lock()
	delete(d.engines, req.Session)
	d.mu.Unlock()

	ok(w, req.Session, map[string]bool{"closed": true})
}

func (d *Daemon) handleOpen(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, sid, err := d.getOrCreate(req)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Open(r.Context(), req.URL); err != nil {
		fail(w, err)
		return
	}

	// Auto-scan for AI recommendation poisoning after navigation.
	scan := eng.AutoScan(r.Context())
	response := map[string]interface{}{"scan_result": scan}
	if !scan.Clean && d.ledger != nil {
		d.ledger.Record(req.URL, sid, browser.ContentGuardResult{
			Clean: scan.Clean, Warnings: scan.Warnings,
		}, "")
		response["poisoning_warning"] = "AI recommendation poisoning detected on this page"
	}
	ok(w, sid, response)
}

func (d *Daemon) handleSearch(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, sid, err := d.getOrCreate(req)
	if err != nil {
		fail(w, err)
		return
	}
	
	// Fast VPS SearXNG instance bypasses Google Bot Detection and aggregates 10+ engines
	query := url.QueryEscape(req.Text)
	searchURL := fmt.Sprintf("http://resources-searxng-82271d-145-79-2-67.traefik.me/search?q=%s", query)
	
	if err := eng.Open(r.Context(), searchURL); err != nil {
		fail(w, err)
		return
	}
	ok(w, sid, nil)
}

func (d *Daemon) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	snap, err := eng.Snapshot(r.Context())
	if err != nil {
		fail(w, err)
		return
	}

	// Auto-scan for AI recommendation poisoning on snapshot.
	scan := eng.AutoScan(r.Context())
	if !scan.Clean && d.ledger != nil {
		d.ledger.Record(snap.URL, req.Session, browser.ContentGuardResult{
			Clean: scan.Clean, Warnings: scan.Warnings,
		}, snap.Tree)
	}

	// Wrap original snap with scan result.
	result := map[string]interface{}{
		"snapshot":    snap,
		"scan_result": scan,
	}
	ok(w, req.Session, result)
}

func (d *Daemon) handleClick(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Click(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleFill(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Fill(r.Context(), req.Ref, req.Text); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := eng.Screenshot(r.Context(), req.Annotate)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, result)
}

func (d *Daemon) handleEval(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := eng.Eval(r.Context(), req.JS)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, result)
}

func (d *Daemon) handleGetText(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	text, err := eng.GetText(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, text)
}

func (d *Daemon) handleGetHTML(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	html, err := eng.GetHTML(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, html)
}

func (d *Daemon) handlePress(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Press(r.Context(), req.Text); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleBack(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Back(r.Context()); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleForward(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Forward(r.Context()); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleReload(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Reload(r.Context()); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleScroll(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Scroll(r.Context(), req.DeltaX, req.DeltaY); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleWaitVisible(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.WaitVisible(r.Context(), req.Selector); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleGetTitle(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	title, err := eng.GetTitle(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, title)
}

func (d *Daemon) handleGetURL(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	url, err := eng.GetURL(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, url)
}

func (d *Daemon) handlePDF(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.PDF(r.Context(), req.Path); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, map[string]string{"path": req.Path})
}

func (d *Daemon) handleSaveSession(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.SaveSession(req.Passphrase); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, map[string]bool{"saved": true})
}

func (d *Daemon) handleRestoreSession(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, sid, err := d.getOrCreate(req)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.RestoreSession(req.Passphrase); err != nil {
		fail(w, err)
		return
	}
	ok(w, sid, map[string]bool{"restored": true})
}

// handleHealth returns daemon health JSON.
func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	numSessions := len(d.engines)
	d.mu.RUnlock()

	ok(w, "", map[string]interface{}{
		"status":   "healthy",
		"module":   "browser-daemon",
		"version":  Version,
		"uptime":   time.Since(d.startTime).Truncate(time.Second).String(),
		"sessions": numSessions,
	})
}

// handleListSessions returns all active session IDs.
func (d *Daemon) handleListSessions(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sessions := make([]map[string]interface{}, 0, len(d.engines))
	for id, eng := range d.engines {
		mode := "headless"
		if eng.IsHeaded() {
			mode = "headed"
		}
		sessions = append(sessions, map[string]interface{}{
			"id":     id,
			"uptime": eng.Uptime().Truncate(time.Second).String(),
			"mode":   mode,
		})
	}
	ok(w, "", sessions)
}

// ---- Interaction v0.2 Handlers ----

func (d *Daemon) handleDblClick(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.DblClick(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleHover(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Hover(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleSelect(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Select(r.Context(), req.Ref, req.Values); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleCheck(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Check(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleUncheck(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Uncheck(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleUpload(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Upload(r.Context(), req.Ref, req.Path); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleMouseMove(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.MouseMove(r.Context(), req.X, req.Y); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleMouseClick(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	btn := req.Button
	if btn == "" {
		btn = "left"
	}
	if err := eng.MouseClick(r.Context(), req.X, req.Y, btn); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleResize(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.Resize(r.Context(), req.Width, req.Height); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

// ---- Data Extraction v0.2 Handlers ----

func (d *Daemon) handleGetValue(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	val, err := eng.GetValue(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, val)
}

func (d *Daemon) handleGetAttr(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	val, err := eng.GetAttribute(r.Context(), req.Ref, req.Attr)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, val)
}

func (d *Daemon) handleGetBox(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	box, err := eng.GetBoundingBox(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, box)
}

func (d *Daemon) handleIsVisible(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	visible, err := eng.IsVisible(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, visible)
}

func (d *Daemon) handleIsEnabled(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	enabled, err := eng.IsEnabled(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, enabled)
}

func (d *Daemon) handleIsChecked(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	checked, err := eng.IsChecked(r.Context(), req.Ref)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, checked)
}

func (d *Daemon) handleConsole(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, eng.ConsoleMessages())
}

func (d *Daemon) handleNetwork(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, eng.NetworkRequests())
}

func (d *Daemon) handleDebug(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	bundle, err := eng.Debug(r.Context())
	if err != nil {
		fail(w, err)
		return
	}

	// Auto-record to ThreatLedger if poisoning was detected.
	if d.ledger != nil && bundle.ScanReport != nil && !bundle.ScanReport.Clean {
		snippet := bundle.Tree
		if err := d.ledger.Record(bundle.URL, req.Session, *bundle.ScanReport, snippet); err != nil {
			d.logger.Warn("threatledger: failed to record", "err", err)
		} else {
			d.logger.Info("threatledger: poisoning recorded",
				"url", bundle.URL,
				"warnings", len(bundle.ScanReport.Warnings))
		}
	}

	ok(w, req.Session, bundle)
}

// ---- Threat Feed (public API for website polling) ----

// handleGetThreats returns all recorded poisoning detections.
// GET /threats -- designed to be polled by an external website.
func (d *Daemon) handleGetThreats(w http.ResponseWriter, r *http.Request) {
	// CORS headers so any website can poll this endpoint.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if d.ledger == nil {
		ok(w, "", []browser.ThreatEntry{})
		return
	}
	ok(w, "", d.ledger.Entries())
}

// handleGetThreatStats returns aggregate threat statistics.
// GET /threats/stats -- summary endpoint for dashboard display.
func (d *Daemon) handleGetThreatStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if d.ledger == nil {
		ok(w, "", browser.ThreatStats{BySeverity: map[string]int{}})
		return
	}
	ok(w, "", d.ledger.Stats())
}

// ---- v0.2.1 Handlers: Tabs, Metrics, Extract, Wait, Webhook, Blocker ----

func (d *Daemon) handleTabNew(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	tabID, err := eng.TabNew(r.Context(), req.URL)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, map[string]string{"tab_id": tabID})
}

func (d *Daemon) handleTabSwitch(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.TabSwitch(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleTabClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	if err := eng.TabClose(r.Context(), req.Ref); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, nil)
}

func (d *Daemon) handleTabList(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, eng.TabList(r.Context()))
}

func (d *Daemon) handleMetrics(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	metrics, err := eng.Metrics(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, metrics)
}

func (d *Daemon) handleGetLinks(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	links, err := eng.Links(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, links)
}

func (d *Daemon) handleGetForms(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	forms, err := eng.Forms(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, forms)
}

func (d *Daemon) handleWaitNav(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	timeout := 30 * time.Second
	if err := eng.WaitNavigation(r.Context(), timeout); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, map[string]bool{"navigated": true})
}

func (d *Daemon) handleWaitIdle(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	idle := 2 * time.Second
	timeout := 30 * time.Second
	if err := eng.WaitNetworkIdle(r.Context(), idle, timeout); err != nil {
		fail(w, err)
		return
	}
	ok(w, req.Session, map[string]bool{"idle": true})
}

func (d *Daemon) handleSetWebhook(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	if d.ledger == nil {
		fail(w, fmt.Errorf("threat ledger not initialized"))
		return
	}
	d.ledger.SetWebhook(req.URL)
	ok(w, "", map[string]string{"webhook": req.URL})
}

func (d *Daemon) handleBlockerStats(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	stats := eng.BlockerStats()
	if stats == nil {
		ok(w, req.Session, map[string]string{"status": "blocker not enabled"})
		return
	}
	ok(w, req.Session, stats)
}

func (d *Daemon) handleBlockerAdd(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	stats := eng.BlockerStats()
	if stats == nil {
		fail(w, fmt.Errorf("blocker not enabled - use --block-ads flag"))
		return
	}
	// Use the URL field as the domain to block.
	eng.AddBlockDomain(req.URL)
	ok(w, req.Session, map[string]string{"blocked": req.URL})
}

func (d *Daemon) handleBlockerRemove(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil {
		fail(w, err)
		return
	}
	eng, err := d.get(req.Session)
	if err != nil {
		fail(w, err)
		return
	}
	stats := eng.BlockerStats()
	if stats == nil {
		fail(w, fmt.Errorf("blocker not enabled - use --block-ads flag"))
		return
	}
	eng.RemoveBlockDomain(req.URL)
	ok(w, req.Session, map[string]string{"unblocked": req.URL})
}

// ---- v0.2.2 Handlers ----

func (d *Daemon) handleCookies(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	cookies, err := eng.Cookies(r.Context())
	if err != nil { fail(w, err); return }
	ok(w, req.Session, cookies)
}

func (d *Daemon) handleCookieSet(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	path := req.Path
	if path == "" { path = "/" }
	if err := eng.SetCookie(r.Context(), req.Key, req.Value, req.Domain, path); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"set": req.Key})
}

func (d *Daemon) handleCookieDelete(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.DeleteCookie(r.Context(), req.Key); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"deleted": req.Key})
}

func (d *Daemon) handleCookieClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.ClearCookies(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

func (d *Daemon) handleStorage(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	st := req.StorageType
	if st == "" { st = "local" }
	entries, err := eng.Storage(r.Context(), st)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, entries)
}

func (d *Daemon) handleStorageSet(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	st := req.StorageType
	if st == "" { st = "local" }
	if err := eng.SetStorageItem(r.Context(), st, req.Key, req.Value); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"set": req.Key})
}

func (d *Daemon) handleStorageClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	st := req.StorageType
	if st == "" { st = "local" }
	if err := eng.ClearStorageAll(r.Context(), st); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

func (d *Daemon) handleDialogs(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, eng.DialogEntries())
}

func (d *Daemon) handleDialogMode(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	eng.SetDialogMode(req.Mode)
	ok(w, req.Session, map[string]string{"mode": req.Mode})
}

func (d *Daemon) handleMutationsStart(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.StartMutations(r.Context(), req.Selector); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"observing": true})
}

func (d *Daemon) handleMutationsGet(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	muts, err := eng.GetMutations(r.Context(), req.Clear)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, muts)
}

func (d *Daemon) handleMutationsStop(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.StopMutations(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"stopped": true})
}

func (d *Daemon) handleFrames(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	frames, err := eng.FrameList(r.Context())
	if err != nil { fail(w, err); return }
	ok(w, req.Session, frames)
}

func (d *Daemon) handleFrameContent(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	content, err := eng.FrameContent(r.Context(), req.Index)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, map[string]string{"content": content})
}

func (d *Daemon) handleDownloads(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, map[string]interface{}{
		"downloads": eng.DownloadEntries(),
		"directory": eng.DownloadDir(),
	})
}

func (d *Daemon) handleDownloadScan(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	newFiles := eng.DownloadScan()
	ok(w, req.Session, map[string]interface{}{
		"new_files": newFiles,
		"count":     len(newFiles),
	})
}

func (d *Daemon) handleGeoSet(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.SetGeo(r.Context(), req.Lat, req.Lon, req.Accuracy); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]interface{}{"lat": req.Lat, "lon": req.Lon})
}

func (d *Daemon) handleGeoClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.ClearGeo(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

func (d *Daemon) handleUASet(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	ua := req.Value
	if ua == "" { ua = req.Text }
	if err := eng.SetUA(r.Context(), ua); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"user_agent": ua})
}

func (d *Daemon) handleClipboardRead(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	text, err := eng.ClipboardRead(r.Context())
	if err != nil { fail(w, err); return }
	ok(w, req.Session, map[string]string{"text": text})
}

func (d *Daemon) handleClipboardWrite(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.ClipboardWrite(r.Context(), req.Text); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"written": true})
}

// ---- v0.2.3 Handlers ----

func (d *Daemon) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	caps := browser.GetCapabilities()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(caps)
}

func (d *Daemon) handleSystemPrompt(w http.ResponseWriter, _ *http.Request) {
	prompt := browser.GetSystemPrompt()
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(prompt))
}

func (d *Daemon) handleHistory(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, eng.History())
}

func (d *Daemon) handleRecordStart(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	title := req.Title
	if title == "" { title = "Untitled Lesson" }
	eng.RecordStart(title)
	ok(w, req.Session, map[string]string{"recording": title})
}

func (d *Daemon) handleRecordStop(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	lesson := eng.RecordStop()
	ok(w, req.Session, lesson)
}

func (d *Daemon) handleRecordSteps(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, eng.RecordSteps())
}

func (d *Daemon) handleRecordNarrate(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	eng.RecordNarrate(req.Text)
	ok(w, req.Session, map[string]bool{"narrated": true})
}

func (d *Daemon) handleHighlight(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.Highlight(r.Context(), req.Ref); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"highlighted": true})
}

func (d *Daemon) handleCircle(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.CircleRef(r.Context(), req.Ref, req.Label); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"circled": true})
}

func (d *Daemon) handleArrow(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.ArrowRef(r.Context(), req.Ref, req.Label); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"arrowed": true})
}

func (d *Daemon) handleDrawText(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.DrawTextAt(r.Context(), int(req.X), int(req.Y), req.Text); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"drawn": true})
}

func (d *Daemon) handleClearAnnotations(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.ClearAnnotations(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

func (d *Daemon) handleStyle(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	styles, err := eng.Styles(r.Context(), req.Ref, req.Properties)
	if err != nil { fail(w, err); return }
	ok(w, req.Session, styles)
}

func (d *Daemon) handleDrag(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	selector := fmt.Sprintf("[data-itak-ref='%s']", req.Ref)
	if err := eng.DragDrop(r.Context(), selector, req.DeltaX, req.DeltaY); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"dragged": true})
}

// ---- Toolbar Handlers ----

func (d *Daemon) handleToolbarInject(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.InjectToolbar(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"injected": true})
}

func (d *Daemon) handleToolbarRemove(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.RemoveToolbar(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"removed": true})
}

func (d *Daemon) handleToolbarMarkup(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	data, err := eng.GetAnnotatedScreenshot(r.Context())
	if err != nil { fail(w, err); return }
	if data == "" {
		fail(w, fmt.Errorf("no annotated screenshot available (user has not sent one yet)")); return
	}
	ok(w, req.Session, map[string]string{"screenshot": data})
}
