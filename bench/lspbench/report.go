package main

// report.go — turns raw run data into a JSON artifact and a Markdown report.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// ServerRun is everything measured for one server against one workspace.
type ServerRun struct {
	ServerID   string   `json:"serverId"`
	ServerName string   `json:"serverName"`
	Version    string   `json:"version"`
	License    string   `json:"license"`
	Command    []string `json:"command"`
	Workspace  string   `json:"workspace"`
	PHPFiles   int      `json:"phpFiles"`

	StartupMs     float64 `json:"startupMs"`               // spawn -> initialize response
	ReadyMs       float64 `json:"readyMs"`                 // spawn -> first correct cross-file answer
	ReadySignalMs float64 `json:"readySignalMs,omitempty"` // spawn -> native "indexing done" signal
	ReadySignal   string  `json:"readySignal,omitempty"`
	IndexPeakMB   float64 `json:"indexPeakMB"` // peak RSS at the moment readiness was reached
	PeakRSSMB     float64 `json:"peakRssMB"`   // peak RSS across the whole run
	CPUSeconds    float64 `json:"cpuSeconds"`

	// Capabilities lists the providers the server advertised, so a case scored
	// "unsupported" can be audited against what the server actually claimed.
	Capabilities []string     `json:"capabilities,omitempty"`
	Cases        []CaseResult `json:"cases"`
	Error        string       `json:"error,omitempty"`
	StderrTail   string       `json:"stderrTail,omitempty"`
	Notes        string       `json:"notes,omitempty"`
}

// Report is the full benchmark artifact.
type Report struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Environment map[string]string `json:"environment"`
	Config      map[string]any    `json:"config"`
	Runs        []ServerRun       `json:"runs"`
}

type featureStat struct {
	Pass, Fail, Unsupported, Skipped, Errored int
	Latencies                                 []float64
}

func (f featureStat) graded() int { return f.Pass + f.Fail }

func statsByFeature(run ServerRun) map[string]*featureStat {
	out := map[string]*featureStat{}
	for _, c := range run.Cases {
		st, ok := out[c.Feature]
		if !ok {
			st = &featureStat{}
			out[c.Feature] = st
		}
		switch c.Status {
		case StatusPass:
			st.Pass++
		case StatusFail:
			st.Fail++
		case StatusUnsupported:
			st.Unsupported++
		case StatusSkipped:
			st.Skipped++
		default:
			st.Errored++
		}
		// Only passing cases contribute to latency. A server that answers
		// "nothing" instantly is not fast, and averaging its non-answers in
		// would reward exactly the wrong thing.
		if c.Status == StatusPass {
			st.Latencies = append(st.Latencies, c.LatencyMs...)
		}
	}
	return out
}

func collectEnvironment() map[string]string {
	env := map[string]string{
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"cpus":      fmt.Sprintf("%d", runtime.NumCPU()),
		"goVersion": runtime.Version(),
	}
	if out, err := exec.Command("php", "-r", "echo PHP_VERSION;").Output(); err == nil {
		env["php"] = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("node", "--version").Output(); err == nil {
		env["node"] = strings.TrimSpace(string(out))
	}
	if host, err := os.Hostname(); err == nil {
		env["host"] = host
	}
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			env["cpu"] = strings.TrimSpace(string(out))
		}
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			env["memBytes"] = strings.TrimSpace(string(out))
		}
	case "linux":
		if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "model name") {
					if i := strings.Index(line, ":"); i >= 0 {
						env["cpu"] = strings.TrimSpace(line[i+1:])
					}
					break
				}
			}
		}
	}
	return env
}

func writeReport(dir string, rep Report) (jsonPath, mdPath string, err error) {
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	jsonPath = filepath.Join(dir, "results.json")
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err = os.WriteFile(jsonPath, b, 0o644); err != nil {
		return "", "", err
	}
	mdPath = filepath.Join(dir, "report.md")
	if err = os.WriteFile(mdPath, []byte(renderMarkdown(rep)), 0o644); err != nil {
		return "", "", err
	}
	return jsonPath, mdPath, nil
}

func renderMarkdown(rep Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PHP language server benchmark\n\n")
	fmt.Fprintf(&b, "Generated %s\n\n", rep.GeneratedAt.Format(time.RFC3339))

	b.WriteString("## Environment\n\n")
	keys := sortedKeys(rep.Environment)
	b.WriteString("| key | value |\n|---|---|\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "| %s | %s |\n", k, rep.Environment[k])
	}
	b.WriteString("\n")

	b.WriteString("## Servers under test\n\n")
	b.WriteString("| server | version | license | command |\n|---|---|---|---|\n")
	seen := map[string]bool{}
	for _, r := range rep.Runs {
		if seen[r.ServerID] {
			continue
		}
		seen[r.ServerID] = true
		fmt.Fprintf(&b, "| %s | %s | %s | `%s` |\n", r.ServerName, dash(r.Version), dash(r.License), strings.Join(r.Command, " "))
	}
	b.WriteString("\n")

	for _, ws := range workspacesIn(rep) {
		runs := runsFor(rep, ws)
		fmt.Fprintf(&b, "## Workspace: %s\n\n", ws)
		if len(runs) > 0 {
			fmt.Fprintf(&b, "%d PHP files (including vendor).\n\n", runs[0].PHPFiles)
		}

		b.WriteString("### Startup and footprint\n\n")
		b.WriteString("| server | initialize | ready (first correct cross-file answer) | RSS at ready | peak RSS | CPU |\n|---|--:|--:|--:|--:|--:|\n")
		for _, r := range runs {
			if r.Error != "" {
				fmt.Fprintf(&b, "| %s | _failed: %s_ | | | | |\n", r.ServerName, truncate(r.Error, 80))
				continue
			}
			fmt.Fprintf(&b, "| %s | %.0f ms | %s | %.0f MB | %.0f MB | %.1f s |\n",
				r.ServerName, r.StartupMs, readyCell(r), r.IndexPeakMB, r.PeakRSSMB, r.CPUSeconds)
		}
		b.WriteString("\n")

		features := featuresIn(runs)

		b.WriteString("### Correctness (cases passed / cases graded)\n\n")
		b.WriteString("| feature | " + strings.Join(names(runs), " | ") + " |\n")
		b.WriteString(strings.Repeat("|---", len(runs)+1) + "|\n")
		statsPerRun := make([]map[string]*featureStat, len(runs))
		for i, r := range runs {
			statsPerRun[i] = statsByFeature(r)
		}
		for _, f := range features {
			fmt.Fprintf(&b, "| %s ", f)
			for i := range runs {
				st := statsPerRun[i][f]
				b.WriteString("| " + correctnessCell(st) + " ")
			}
			b.WriteString("|\n")
		}
		fmt.Fprintf(&b, "| **total** ")
		for i := range runs {
			total := &featureStat{}
			for _, st := range statsPerRun[i] {
				total.Pass += st.Pass
				total.Fail += st.Fail
				total.Unsupported += st.Unsupported
				total.Skipped += st.Skipped
				total.Errored += st.Errored
			}
			b.WriteString("| **" + correctnessCell(total) + "** ")
		}
		b.WriteString("|\n\n")

		b.WriteString("### Latency of correct answers, p50 / p95 in ms\n\n")
		b.WriteString("| feature | " + strings.Join(names(runs), " | ") + " |\n")
		b.WriteString(strings.Repeat("|---", len(runs)+1) + "|\n")
		for _, f := range features {
			fmt.Fprintf(&b, "| %s ", f)
			for i := range runs {
				st := statsPerRun[i][f]
				if st == nil || len(st.Latencies) == 0 {
					b.WriteString("| — ")
					continue
				}
				fmt.Fprintf(&b, "| %.1f / %.1f ", percentile(st.Latencies, 0.5), percentile(st.Latencies, 0.95))
			}
			b.WriteString("|\n")
		}
		b.WriteString("\n")

		b.WriteString("<details>\n<summary>Per-case results</summary>\n\n")
		b.WriteString("| case | " + strings.Join(names(runs), " | ") + " |\n")
		b.WriteString(strings.Repeat("|---", len(runs)+1) + "|\n")
		for _, id := range caseIDsIn(runs) {
			fmt.Fprintf(&b, "| `%s` ", id)
			for _, r := range runs {
				b.WriteString("| " + statusIcon(findCase(r, id)) + " ")
			}
			b.WriteString("|\n")
		}
		b.WriteString("\n</details>\n\n")
	}

	b.WriteString("## Reading these numbers\n\n")
	b.WriteString("- **ready** is time from process spawn until the server first answers a cross-file question correctly. It is the only start-up number comparable across servers, because each one signals \"indexed\" differently (or not at all).\n")
	b.WriteString("- **Correctness** counts only cases the server advertises support for. `n/a` means the capability was not advertised; those cases are excluded from the denominator rather than counted as failures.\n")
	b.WriteString("- `err` means the server advertised the capability and then failed to answer — usually no reply at all before the request timeout. The advertised capability list is recorded per run in `results.json`, so the distinction between \"did not claim it\" and \"claimed it and did not deliver\" is auditable.\n")
	b.WriteString("- **Latency** covers only the cases a server answered correctly, measured after readiness with warm caches. A server that returns nothing does so very quickly; counting that as speed would reward the wrong thing.\n")
	b.WriteString("- Memory and CPU cover the server process and its children. A Node or PHP server carries its runtime in these numbers; that is a real cost to the user, but it is not a like-for-like comparison of indexing strategy.\n")
	b.WriteString("- The case suite was written against this repository's fixtures. It covers what these fixtures exercise, not the whole of PHP.\n")
	return b.String()
}

func readyCell(r ServerRun) string {
	if r.ReadyMs <= 0 {
		return "_never_"
	}
	s := fmt.Sprintf("%.0f ms", r.ReadyMs)
	if r.ReadySignalMs > 0 {
		s += fmt.Sprintf(" (own signal: %.0f ms)", r.ReadySignalMs)
	}
	return s
}

func correctnessCell(st *featureStat) string {
	if st == nil {
		return "—"
	}
	if st.graded() == 0 {
		if st.Unsupported > 0 {
			return fmt.Sprintf("n/a (%d)", st.Unsupported)
		}
		if st.Errored > 0 {
			return fmt.Sprintf("err (%d)", st.Errored)
		}
		return "—"
	}
	cell := fmt.Sprintf("%d/%d", st.Pass, st.graded())
	if st.Unsupported > 0 {
		cell += fmt.Sprintf(" +%d n/a", st.Unsupported)
	}
	if st.Errored > 0 {
		cell += fmt.Sprintf(" +%d err", st.Errored)
	}
	return cell
}

func statusIcon(c *CaseResult) string {
	if c == nil {
		return "—"
	}
	switch c.Status {
	case StatusPass:
		return "✅"
	case StatusFail:
		return "❌"
	case StatusUnsupported:
		return "n/a"
	case StatusSkipped:
		return "skip"
	default:
		return "⚠️"
	}
}

func findCase(r ServerRun, id string) *CaseResult {
	for i := range r.Cases {
		if r.Cases[i].ID == id {
			return &r.Cases[i]
		}
	}
	return nil
}

func names(runs []ServerRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ServerName
	}
	return out
}

func workspacesIn(rep Report) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rep.Runs {
		if !seen[r.Workspace] {
			seen[r.Workspace] = true
			out = append(out, r.Workspace)
		}
	}
	return out
}

func runsFor(rep Report, ws string) []ServerRun {
	var out []ServerRun
	for _, r := range rep.Runs {
		if r.Workspace == ws {
			out = append(out, r)
		}
	}
	return out
}

func featuresIn(runs []ServerRun) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range runs {
		for _, c := range r.Cases {
			if !seen[c.Feature] {
				seen[c.Feature] = true
				out = append(out, c.Feature)
			}
		}
	}
	sort.Strings(out)
	return out
}

func caseIDsIn(runs []ServerRun) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range runs {
		for _, c := range r.Cases {
			if !seen[c.ID] {
				seen[c.ID] = true
				out = append(out, c.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// printConsole writes a compact summary for the terminal.
func printConsole(rep Report) {
	for _, ws := range workspacesIn(rep) {
		runs := runsFor(rep, ws)
		fmt.Printf("\nworkspace: %s\n", ws)
		fmt.Printf("%-16s %10s %10s %9s %9s   %s\n", "server", "init", "ready", "rss@ready", "peak", "correctness")
		for _, r := range runs {
			if r.Error != "" {
				fmt.Printf("%-16s  failed: %s\n", r.ServerID, truncate(r.Error, 90))
				continue
			}
			total := &featureStat{}
			for _, st := range statsByFeature(r) {
				total.Pass += st.Pass
				total.Fail += st.Fail
				total.Unsupported += st.Unsupported
				total.Errored += st.Errored
			}
			ready := "never"
			if r.ReadyMs > 0 {
				ready = fmt.Sprintf("%.0fms", r.ReadyMs)
			}
			fmt.Printf("%-16s %9.0fms %10s %8.0fMB %8.0fMB   %s\n",
				r.ServerID, r.StartupMs, ready, r.IndexPeakMB, r.PeakRSSMB, correctnessCell(total))
		}
	}
}
