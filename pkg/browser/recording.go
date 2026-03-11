// Package browser - Screen Recording via periodic CDP screenshots.
//
// What: Captures periodic screenshots during a browser session and stores
//       them as frames. The frames can be exported as an animated sequence.
// Why:  Teaching requires video-like recordings. CDP screenshot frames
//       give us a portable way to capture what's on screen.
// How:  A goroutine captures screenshots at configurable FPS. Frames are
//       stored as base64 PNGs. Export concatenates them into a filmstrip
//       or provides raw frame data for external video encoding.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// RecordingState manages an active screen recording session.
type RecordingState struct {
	mu       sync.Mutex
	active   bool
	frames   []recordingFrame
	cancel   context.CancelFunc
	startAt  time.Time
	fps      int
}

type recordingFrame struct {
	Timestamp float64 `json:"timestamp"` // Seconds since recording start.
	Data      string  `json:"data"`      // base64 PNG data URL.
}

// RecordingStart begins capturing screenshots at the given FPS.
func (e *Engine) RecordingStart(ctx context.Context, fps int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if fps <= 0 || fps > 10 {
		fps = 2 // Default 2 FPS for reasonable file sizes.
	}

	// Initialize or reset recording state.
	if e.recording == nil {
		e.recording = &RecordingState{}
	}
	e.recording.mu.Lock()
	if e.recording.active {
		e.recording.mu.Unlock()
		return fmt.Errorf("recording already active")
	}
	e.recording.active = true
	e.recording.frames = nil
	e.recording.fps = fps
	e.recording.startAt = time.Now()
	e.recording.mu.Unlock()

	// Start capture goroutine.
	rctx, cancel := context.WithCancel(context.Background())
	e.recording.cancel = cancel

	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(fps))
		defer ticker.Stop()
		for {
			select {
			case <-rctx.Done():
				return
			case <-ticker.C:
				var buf []byte
				if err := chromedp.Run(e.browserCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
					continue
				}
				b64 := "data:image/png;base64," + base64Encode(buf)
				e.recording.mu.Lock()
				e.recording.frames = append(e.recording.frames, recordingFrame{
					Timestamp: time.Since(e.recording.startAt).Seconds(),
					Data:      b64,
				})
				e.recording.mu.Unlock()
			}
		}
	}()

	return nil
}

// RecordingStop stops the recording and returns the frame count.
func (e *Engine) RecordingStop(ctx context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.recording == nil || !e.recording.active {
		return 0, fmt.Errorf("no active recording")
	}

	e.recording.cancel()
	e.recording.mu.Lock()
	e.recording.active = false
	count := len(e.recording.frames)
	e.recording.mu.Unlock()

	return count, nil
}

// RecordingFrames returns the recorded frames as JSON.
func (e *Engine) RecordingFrames(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.recording == nil {
		return "[]", nil
	}

	e.recording.mu.Lock()
	frames := e.recording.frames
	e.recording.mu.Unlock()

	data, err := json.Marshal(frames)
	if err != nil {
		return "", fmt.Errorf("marshal frames: %w", err)
	}
	return string(data), nil
}

// RecordingInjectPlayer injects an in-page video player that plays back
// the recorded frames as a slideshow.
func (e *Engine) RecordingInjectPlayer(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.recording == nil || len(e.recording.frames) == 0 {
		return fmt.Errorf("no frames to play")
	}

	e.recording.mu.Lock()
	frames := e.recording.frames
	fps := e.recording.fps
	e.recording.mu.Unlock()

	framesJSON, err := json.Marshal(frames)
	if err != nil {
		return fmt.Errorf("marshal frames: %w", err)
	}

	js := fmt.Sprintf(`
(function() {
	const old = document.getElementById('itak-player');
	if (old) old.remove();

	const frames = %s;
	const fps = %d;
	let current = 0;
	let playing = false;
	let interval = null;

	const overlay = document.createElement('div');
	overlay.id = 'itak-player';
	overlay.style.cssText =
		'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:999985;' +
		'background:#0f172a;display:flex;flex-direction:column;';

	// Controls bar.
	const controls = document.createElement('div');
	controls.style.cssText =
		'display:flex;align-items:center;gap:12px;padding:10px 16px;' +
		'background:#1e293b;border-bottom:1px solid rgba(255,255,255,0.06);flex-shrink:0;' +
		'font:13px system-ui;color:#e2e8f0;';

	const playBtn = document.createElement('button');
	playBtn.textContent = '\u25B6 Play';
	playBtn.style.cssText =
		'padding:6px 16px;border:none;border-radius:6px;' +
		'background:linear-gradient(135deg,#2563eb,#1d4ed8);' +
		'color:#fff;cursor:pointer;font:600 12px system-ui;';
	playBtn.onclick = () => {
		if (playing) { clearInterval(interval); playing = false; playBtn.textContent = '\u25B6 Play'; }
		else { interval = setInterval(next, 1000/fps); playing = true; playBtn.textContent = '\u23F8 Pause'; }
	};
	controls.appendChild(playBtn);

	const counter = document.createElement('span');
	counter.style.color = '#94a3b8';
	counter.textContent = '1 / ' + frames.length;
	controls.appendChild(counter);

	const slider = document.createElement('input');
	slider.type = 'range'; slider.min = '0'; slider.max = '' + (frames.length - 1); slider.value = '0';
	slider.style.cssText = 'flex:1;accent-color:#3b82f6;';
	slider.oninput = () => { current = parseInt(slider.value); show(); };
	controls.appendChild(slider);

	const timeLabel = document.createElement('span');
	timeLabel.style.cssText = 'color:#64748b;min-width:50px;text-align:right;font-size:11px;';
	controls.appendChild(timeLabel);

	const closeBtn = document.createElement('button');
	closeBtn.textContent = '\u2716 Close';
	closeBtn.style.cssText =
		'padding:6px 12px;border:1px solid rgba(239,68,68,0.3);border-radius:6px;' +
		'background:rgba(239,68,68,0.1);color:#ef4444;cursor:pointer;font:500 12px system-ui;';
	closeBtn.onclick = () => { if(interval) clearInterval(interval); overlay.remove(); };
	controls.appendChild(closeBtn);

	overlay.appendChild(controls);

	// Image display.
	const img = document.createElement('img');
	img.style.cssText = 'flex:1;object-fit:contain;background:#000;';
	overlay.appendChild(img);
	document.body.appendChild(overlay);

	function show() {
		if (current >= frames.length) { current = 0; if(playing) { clearInterval(interval); playing = false; playBtn.textContent = '\u25B6 Play'; } }
		img.src = frames[current].data;
		counter.textContent = (current+1) + ' / ' + frames.length;
		slider.value = '' + current;
		timeLabel.textContent = frames[current].timestamp.toFixed(1) + 's';
	}
	function next() { current++; show(); }

	show();
	return true;
})()
`, string(framesJSON), fps)

	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}
