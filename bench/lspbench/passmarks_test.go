package main

import (
	"strings"
	"testing"
)

func TestCheckPassMarks(t *testing.T) {
	rep := Report{Runs: []ServerRun{{
		ServerID:  "tusk-php",
		Workspace: "sylius",
		ReadyMs:   900,
		PeakRSSMB: 700,
		Cases: []CaseResult{
			{Feature: "hover", Status: StatusPass, LatencyMs: []float64{2, 3}},
			{Feature: "hover", Status: StatusFail},
			{Feature: "definition", Status: StatusPass, LatencyMs: []float64{40}},
		},
	}}}
	pm := PassMarks{Server: "tusk-php", Workspaces: map[string]WorkspaceMark{
		"sylius": {MinPass: 2, MaxReadyMs: 1000, MaxPeakRSSMB: 800, MaxP95Ms: map[string]float64{"hover": 10, "definition": 50}},
	}}
	if misses := checkPassMarks(rep, pm, nil); len(misses) != 0 {
		t.Fatalf("expected every mark met, got %v", misses)
	}

	pm.Workspaces["sylius"] = WorkspaceMark{MinPass: 3, MaxReadyMs: 500, MaxPeakRSSMB: 600, MaxP95Ms: map[string]float64{"definition": 20}}
	misses := checkPassMarks(rep, pm, nil)
	if len(misses) != 4 {
		t.Fatalf("expected 4 misses, got %v", misses)
	}
	for i, want := range []string{"2 cases passed", "ready in 900 ms", "peak RSS 700 MB", "definition p95 40.0 ms"} {
		if !strings.Contains(misses[i], want) {
			t.Errorf("miss %d = %q, want it to mention %q", i, misses[i], want)
		}
	}

	pm.Workspaces["mageos"] = WorkspaceMark{MinPass: 1}
	if misses := checkPassMarks(rep, pm, []string{"mageos"}); len(misses) != 1 || !strings.Contains(misses[0], "no run") {
		t.Fatalf("a workspace with marks but no run must miss, got %v", misses)
	}
}
