// Package daemon - HTTP handlers for v0.3.0 features.
//
// What: Handler functions for Spotlight, Diff, StorageInspect, A11y,
//       Inspector, Waterfall, Autofill, TabStrip, ScreenRec, Translate.
// Why:  Keeps v0.3.0 handlers separate from the growing daemon.go.
// How:  Each handler follows the same decode -> get engine -> call method -> respond pattern.
package daemon

import (
	"net/http"
)

// ---- Spotlight ----

func (d *Daemon) handleSpotlight(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	sel := req.Selector
	if sel == "" { sel = req.Ref }
	if err := eng.Spotlight(r.Context(), sel, req.Label); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"spotlight": true})
}

func (d *Daemon) handleSpotlightClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.SpotlightClear(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

// ---- Visual Diff ----

func (d *Daemon) handleDiffSnapshot(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	name := req.Name
	if name == "" { name = "default" }
	if err := eng.DiffSnapshot(r.Context(), name); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"snapshot": name})
}

func (d *Daemon) handleDiffCompare(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	name := req.Name
	if name == "" { name = "default" }
	if err := eng.DiffCompare(r.Context(), name); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"compared": true})
}

func (d *Daemon) handleDiffClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.DiffClear(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

// ---- Storage Inspector ----

func (d *Daemon) handleStorageInspect(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.StorageInspect(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"opened": true})
}

func (d *Daemon) handleStorageInspectClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.StorageClose(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"closed": true})
}

// ---- Accessibility Audit ----

func (d *Daemon) handleA11yAudit(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.A11yAudit(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"audited": true})
}

func (d *Daemon) handleA11yClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.A11yClear(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}

// ---- Element Inspector ----

func (d *Daemon) handleInspectorOpen(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.InspectorOpen(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"opened": true})
}

func (d *Daemon) handleInspectorClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.InspectorClose(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"closed": true})
}

// ---- Performance Waterfall ----

func (d *Daemon) handleWaterfallOpen(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.WaterfallOpen(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"opened": true})
}

func (d *Daemon) handleWaterfallClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.WaterfallClose(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"closed": true})
}

// ---- Form Autofill ----

func (d *Daemon) handleAutofillSave(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	name := req.Name
	if name == "" { name = "default" }
	if err := eng.AutofillSave(r.Context(), name); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"saved": name})
}

func (d *Daemon) handleAutofillLoad(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	name := req.Name
	if name == "" { name = "default" }
	if err := eng.AutofillLoad(r.Context(), name); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"loaded": name})
}

func (d *Daemon) handleAutofillList(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	names, err := eng.AutofillList(r.Context())
	if err != nil { fail(w, err); return }
	ok(w, req.Session, map[string][]string{"profiles": names})
}

// ---- Tab Strip Dashboard ----

func (d *Daemon) handleTabStripOpen(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.TabDashboardOpen(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"opened": true})
}

func (d *Daemon) handleTabStripClose(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.TabDashboardClose(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"closed": true})
}

// ---- Screen Recording ----

func (d *Daemon) handleScreenRecStart(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	fps := req.FPS
	if fps <= 0 { fps = 2 }
	if err := eng.RecordingStart(r.Context(), fps); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]any{"recording": true, "fps": fps})
}

func (d *Daemon) handleScreenRecStop(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	count, err := eng.RecordingStop(r.Context())
	if err != nil { fail(w, err); return }
	ok(w, req.Session, map[string]any{"stopped": true, "frames": count})
}

func (d *Daemon) handleScreenRecPlay(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.RecordingInjectPlayer(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"player": true})
}

// ---- Page Translation ----

func (d *Daemon) handleTranslate(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	lang := req.Lang
	if lang == "" { lang = "es" }
	if err := eng.TranslateOverlay(r.Context(), lang); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]string{"language": lang})
}

func (d *Daemon) handleTranslateClear(w http.ResponseWriter, r *http.Request) {
	req, err := d.decode(r)
	if err != nil { fail(w, err); return }
	eng, err := d.get(req.Session)
	if err != nil { fail(w, err); return }
	if err := eng.TranslateClear(r.Context()); err != nil {
		fail(w, err); return
	}
	ok(w, req.Session, map[string]bool{"cleared": true})
}
