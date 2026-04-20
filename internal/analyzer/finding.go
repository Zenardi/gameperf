package analyzer

import (
	"fmt"
	"strings"
)

// Severity of a finding.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Finding represents a detected performance issue.
type Finding struct {
	ID          string
	Severity    Severity
	Title       string
	Description string
	Evidence    string // raw metric data that triggered this finding

	// Fix guidance
	AutoFixable  bool
	AutoFixCmd   []string // shell command to apply the fix automatically (may require sudo)
	ManualFix    string   // human-readable instructions for fixes that can't be automated
	InGameFix    string   // steps to take inside the game settings
}

// String returns a short one-line summary.
func (f Finding) String() string {
	tag := strings.ToUpper(string(f.Severity))
	return fmt.Sprintf("[%s] %s: %s", tag, f.ID, f.Title)
}

// Report is the full output of an analysis session.
type Report struct {
	GameProcess string
	Findings    []Finding
	Applied     []AppliedFix
}

// AppliedFix records a fix that was executed automatically.
type AppliedFix struct {
	FindingID string
	Command   string
	Output    string
	Error     string
}

// HasCritical returns true if any finding is critical severity.
func (r *Report) HasCritical() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// FindingsByID returns the finding with the given ID, or nil.
func (r *Report) FindingByID(id string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i]
		}
	}
	return nil
}
