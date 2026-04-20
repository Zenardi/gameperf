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
	ID          string   `json:"id"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    string   `json:"evidence,omitempty"`

	// Fix guidance
	AutoFixable  bool     `json:"auto_fixable"`
	AutoFixCmd   []string `json:"auto_fix_cmd,omitempty"`
	ManualFix    string   `json:"manual_fix,omitempty"`
	InGameFix    string   `json:"in_game_fix,omitempty"`
}

// String returns a short one-line summary.
func (f Finding) String() string {
	tag := strings.ToUpper(string(f.Severity))
	return fmt.Sprintf("[%s] %s: %s", tag, f.ID, f.Title)
}

// Report is the full output of an analysis session.
type Report struct {
	GameProcess string       `json:"game_process,omitempty"`
	Findings    []Finding    `json:"findings"`
	Applied     []AppliedFix `json:"applied,omitempty"`
}

// AppliedFix records a fix that was executed automatically.
type AppliedFix struct {
	FindingID string `json:"finding_id"`
	Command   string `json:"command"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
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
