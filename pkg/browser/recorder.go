// Package browser - Action recorder for AI teaching sessions.
//
// What: Records every browser action the agent performs, with timestamps,
//       element refs, screenshots, and auto-generated narration.
// Why:  The teacher model needs to create step-by-step lessons. The recorder
//       captures every action as a structured step, so the AI can later
//       present them as a tutorial with "Step 1: Navigate to...",
//       "Step 2: Click the login button", etc.
// How:  Engine methods call recorder.Record() with action details. The
//       recorder builds a chronological list of TeachingStep entries
//       that can be exported as a lesson plan.
package browser

import (
	"fmt"
	"sync"
	"time"
)

// TeachingStep represents one recorded action in a teaching lesson.
type TeachingStep struct {
	StepNumber  int    `json:"step_number"`
	Action      string `json:"action"`       // open, click, fill, scroll, etc.
	Target      string `json:"target"`       // URL, element ref, or description
	Value       string `json:"value,omitempty"` // Text entered, key pressed, etc.
	Narration   string `json:"narration"`    // Auto-generated or manual explanation
	URL         string `json:"url"`          // Page URL at time of action
	Timestamp   string `json:"timestamp"`
	DurationMs  int64  `json:"duration_ms,omitempty"` // How long the action took
}

// Lesson is the complete recorded teaching session.
type Lesson struct {
	Title     string         `json:"title"`
	StartedAt string         `json:"started_at"`
	EndedAt   string         `json:"ended_at,omitempty"`
	Steps     []TeachingStep `json:"steps"`
	TotalSteps int           `json:"total_steps"`
}

// ActionRecorder captures browser actions for teaching playback.
type ActionRecorder struct {
	mu        sync.RWMutex
	recording bool
	title     string
	startedAt time.Time
	steps     []TeachingStep
	stepCount int
}

// NewActionRecorder creates a new recorder (not recording by default).
func NewActionRecorder() *ActionRecorder {
	return &ActionRecorder{
		steps: make([]TeachingStep, 0),
	}
}

// Start begins recording with an optional title.
func (ar *ActionRecorder) Start(title string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.recording = true
	ar.title = title
	ar.startedAt = time.Now()
	ar.steps = make([]TeachingStep, 0)
	ar.stepCount = 0
}

// Stop ends recording and returns the lesson.
func (ar *ActionRecorder) Stop() Lesson {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.recording = false
	lesson := Lesson{
		Title:      ar.title,
		StartedAt:  ar.startedAt.UTC().Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
		Steps:      make([]TeachingStep, len(ar.steps)),
		TotalSteps: len(ar.steps),
	}
	copy(lesson.Steps, ar.steps)
	return lesson
}

// IsRecording returns whether the recorder is active.
func (ar *ActionRecorder) IsRecording() bool {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	return ar.recording
}

// Steps returns current recorded steps without stopping.
func (ar *ActionRecorder) Steps() []TeachingStep {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	result := make([]TeachingStep, len(ar.steps))
	copy(result, ar.steps)
	return result
}

// Record adds an action to the lesson.
// Auto-generates narration based on the action type.
func (ar *ActionRecorder) Record(action, target, value, url string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if !ar.recording {
		return
	}
	ar.stepCount++
	narration := generateNarration(action, target, value)
	ar.steps = append(ar.steps, TeachingStep{
		StepNumber: ar.stepCount,
		Action:     action,
		Target:     target,
		Value:      value,
		Narration:  narration,
		URL:        url,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	})
}

// Narrate adds a manual explanation step (not tied to an action).
func (ar *ActionRecorder) Narrate(text, url string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if !ar.recording {
		return
	}
	ar.stepCount++
	ar.steps = append(ar.steps, TeachingStep{
		StepNumber: ar.stepCount,
		Action:     "narrate",
		Narration:  text,
		URL:        url,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	})
}

// generateNarration creates a human-readable description of what happened.
func generateNarration(action, target, value string) string {
	switch action {
	case "open":
		return fmt.Sprintf("Navigate to %s", target)
	case "click":
		return fmt.Sprintf("Click on element %s", target)
	case "dblclick":
		return fmt.Sprintf("Double-click on element %s", target)
	case "fill":
		display := value
		if len(display) > 50 {
			display = display[:50] + "..."
		}
		return fmt.Sprintf("Type \"%s\" into %s", display, target)
	case "press":
		return fmt.Sprintf("Press the %s key", value)
	case "scroll":
		return fmt.Sprintf("Scroll the page (offset: %s)", value)
	case "hover":
		return fmt.Sprintf("Hover over element %s", target)
	case "select":
		return fmt.Sprintf("Select option \"%s\" in %s", value, target)
	case "check":
		return fmt.Sprintf("Check the checkbox %s", target)
	case "uncheck":
		return fmt.Sprintf("Uncheck the checkbox %s", target)
	case "upload":
		return fmt.Sprintf("Upload file %s to %s", value, target)
	case "screenshot":
		return "Take a screenshot of the current page"
	case "snapshot":
		return "Capture the page structure for analysis"
	case "drag":
		return fmt.Sprintf("Drag element %s by %s pixels", target, value)
	default:
		return fmt.Sprintf("Perform %s on %s", action, target)
	}
}
