package main

// passmarks.go — holds a run to the numbers in bench/passmarks.json, so CI can
// fail when a reference app loses correct answers, gets slower or heavier.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// PassMarks are the bars one server must clear, per workspace.
type PassMarks struct {
	Server     string                   `json:"server"`
	Comment    string                   `json:"comment,omitempty"`
	Workspaces map[string]WorkspaceMark `json:"workspaces"`
}

// WorkspaceMark is the bar for one workspace. A zero value isn't checked.
type WorkspaceMark struct {
	MinPass      int                `json:"minPass"`      // cases passed
	MaxReadyMs   float64            `json:"maxReadyMs"`   // spawn to first correct cross-file answer
	MaxPeakRSSMB float64            `json:"maxPeakRssMB"` // peak RSS across the run
	MaxP95Ms     map[string]float64 `json:"maxP95Ms"`     // per feature, over correct answers
}

func loadPassMarks(path string) (PassMarks, error) {
	var pm PassMarks
	b, err := os.ReadFile(path)
	if err != nil {
		return pm, err
	}
	if err := json.Unmarshal(b, &pm); err != nil {
		return pm, fmt.Errorf("%s: %w", path, err)
	}
	return pm, nil
}

// checkPassMarks returns one line per bar the report misses. A workspace with
// a bar that the report has no run for is a miss too, so a workspace that
// silently stopped running (a failed fetch) can't pass.
func checkPassMarks(rep Report, pm PassMarks, workspaces []string) []string {
	var misses []string
	names := make([]string, 0, len(pm.Workspaces))
	for ws := range pm.Workspaces {
		if len(workspaces) == 0 || contains(workspaces, ws) {
			names = append(names, ws)
		}
	}
	sort.Strings(names)
	for _, ws := range names {
		mark := pm.Workspaces[ws]
		var run *ServerRun
		for i := range rep.Runs {
			if rep.Runs[i].ServerID == pm.Server && rep.Runs[i].Workspace == ws {
				run = &rep.Runs[i]
			}
		}
		if run == nil {
			misses = append(misses, fmt.Sprintf("%s: no run of %s", ws, pm.Server))
			continue
		}
		if run.Error != "" {
			misses = append(misses, fmt.Sprintf("%s: run failed: %s", ws, run.Error))
			continue
		}
		stats := statsByFeature(*run)
		pass := 0
		for _, st := range stats {
			pass += st.Pass
		}
		if pass < mark.MinPass {
			misses = append(misses, fmt.Sprintf("%s: %d cases passed, the pass mark is %d", ws, pass, mark.MinPass))
		}
		if mark.MaxReadyMs > 0 && (run.ReadyMs <= 0 || run.ReadyMs > mark.MaxReadyMs) {
			misses = append(misses, fmt.Sprintf("%s: ready in %.0f ms, the pass mark is %.0f ms", ws, run.ReadyMs, mark.MaxReadyMs))
		}
		if mark.MaxPeakRSSMB > 0 && run.PeakRSSMB > mark.MaxPeakRSSMB {
			misses = append(misses, fmt.Sprintf("%s: peak RSS %.0f MB, the pass mark is %.0f MB", ws, run.PeakRSSMB, mark.MaxPeakRSSMB))
		}
		features := make([]string, 0, len(mark.MaxP95Ms))
		for f := range mark.MaxP95Ms {
			features = append(features, f)
		}
		sort.Strings(features)
		for _, f := range features {
			st := stats[f]
			if st == nil || len(st.Latencies) == 0 {
				continue
			}
			if p95 := percentile(st.Latencies, 0.95); p95 > mark.MaxP95Ms[f] {
				misses = append(misses, fmt.Sprintf("%s: %s p95 %.1f ms, the pass mark is %.0f ms", ws, f, p95, mark.MaxP95Ms[f]))
			}
		}
	}
	return misses
}

// enforcePassMarks prints the misses and exits non-zero when there are any.
func enforcePassMarks(rep Report, path string, workspaces []string) {
	pm, err := loadPassMarks(path)
	if err != nil {
		fatal(err)
	}
	misses := checkPassMarks(rep, pm, workspaces)
	if len(misses) == 0 {
		fmt.Printf("\npass marks: all met (%s)\n", path)
		return
	}
	fmt.Printf("\npass marks: %d missed (%s)\n", len(misses), path)
	for _, m := range misses {
		fmt.Printf("  - %s\n", m)
	}
	os.Exit(1)
}
