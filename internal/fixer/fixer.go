package fixer

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/zenardi/gameperf/internal/analyzer"
)

// Result holds the outcome of attempting to apply a fix.
type Result struct {
	FindingID string
	Applied   bool
	Output    string
	Err       error
}

// ApplyAll attempts to auto-fix all fixable findings.
// Fixes that require root are attempted with sudo.
func ApplyAll(findings []analyzer.Finding, useSudo bool) []Result {
	var results []Result
	for _, f := range findings {
		if !f.AutoFixable {
			continue
		}
		results = append(results, Apply(f, useSudo))
	}
	return results
}

// Apply executes the auto-fix for a single finding.
func Apply(f analyzer.Finding, useSudo bool) Result {
	if !f.AutoFixable || len(f.AutoFixCmd) == 0 {
		return Result{FindingID: f.ID, Applied: false, Err: fmt.Errorf("not auto-fixable")}
	}

	cmd := f.AutoFixCmd
	if useSudo && cmd[0] != "sudo" {
		cmd = append([]string{"sudo"}, cmd...)
	}

	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		return Result{FindingID: f.ID, Applied: false, Output: output, Err: err}
	}
	return Result{FindingID: f.ID, Applied: true, Output: output}
}
