package main

// lspbench — runs the same set of editor questions against several PHP language
// servers and reports how fast each one starts, how much memory it holds, how
// quickly it answers, and how many answers are right.
//
//	go run ./bench/lspbench -h

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type options struct {
	benchDir    string
	repoRoot    string
	serversFile string
	suiteDir    string
	outDir      string

	serverFilter    []string
	workspaceFilter []string
	featureFilter   []string
	caseFilter      string

	warmup       int
	repeat       int
	callTimeout  time.Duration
	diagTimeout  time.Duration
	readyTimeout time.Duration
	sampleEvery  time.Duration
	warmFS       bool
	verbose      bool
}

func main() {
	var (
		serversFlag  = flag.String("servers", "", "comma-separated server ids to run (default: all enabled)")
		wsFlag       = flag.String("workspaces", "", "comma-separated workspace names to run (default: all)")
		featureFlag  = flag.String("features", "", "comma-separated features to run (default: all)")
		caseFlag     = flag.String("cases", "", "only run cases whose id contains this substring")
		warmupFlag   = flag.Int("warmup", 1, "untimed calls before timing each case")
		repeatFlag   = flag.Int("repeat", 5, "timed calls per case")
		timeoutFlag  = flag.Duration("timeout", 20*time.Second, "per-request timeout")
		diagFlag     = flag.Duration("diagnostics-timeout", 15*time.Second, "how long to wait for pushed diagnostics")
		readyFlag    = flag.Duration("ready-timeout", 0, "override each server's readyTimeoutSec")
		sampleFlag   = flag.Duration("sample-interval", 250*time.Millisecond, "memory sampling interval")
		outFlag      = flag.String("out", "", "output directory (default: bench/results/<timestamp>)")
		noWarmFSFlag = flag.Bool("no-warm-fs", false, "skip warming the OS file cache before the first server")
		listFlag     = flag.Bool("list", false, "list servers and cases, then exit")
		renderFlag   = flag.String("render", "", "re-render report.md from an existing results.json, then exit")
		verboseFlag  = flag.Bool("v", false, "log every case as it runs")
		benchDirFlag = flag.String("bench-dir", "", "path to the bench/ directory (default: auto-detect)")
		serversFile  = flag.String("servers-file", "", "path to servers.json (default: <bench-dir>/servers.json)")
		suiteDirFlag = flag.String("suite-dir", "", "path to the suite directory (default: <bench-dir>/suite)")
	)
	flag.Parse()

	if *renderFlag != "" {
		if err := rerender(*renderFlag, *outFlag); err != nil {
			fatal(err)
		}
		return
	}

	benchDir, err := resolveBenchDir(*benchDirFlag)
	if err != nil {
		fatal(err)
	}
	opts := options{
		benchDir:        benchDir,
		repoRoot:        filepath.Dir(benchDir),
		serversFile:     orDefault(*serversFile, filepath.Join(benchDir, "servers.json")),
		suiteDir:        orDefault(*suiteDirFlag, filepath.Join(benchDir, "suite")),
		outDir:          *outFlag,
		serverFilter:    splitCSV(*serversFlag),
		workspaceFilter: splitCSV(*wsFlag),
		featureFilter:   splitCSV(*featureFlag),
		caseFilter:      *caseFlag,
		warmup:          *warmupFlag,
		repeat:          *repeatFlag,
		callTimeout:     *timeoutFlag,
		diagTimeout:     *diagFlag,
		readyTimeout:    *readyFlag,
		sampleEvery:     *sampleFlag,
		warmFS:          !*noWarmFSFlag,
		verbose:         *verboseFlag,
	}
	if opts.outDir == "" {
		opts.outDir = filepath.Join(benchDir, "results", time.Now().Format("20060102-150405"))
	}

	specs, err := LoadServers(opts.serversFile)
	if err != nil {
		fatal(err)
	}
	suites, err := LoadSuites(opts.suiteDir)
	if err != nil {
		fatal(err)
	}
	specs = filterServers(specs, opts.serverFilter)
	suites = filterSuites(suites, opts.workspaceFilter)

	if *listFlag {
		listAll(specs, suites, opts)
		return
	}
	if len(specs) == 0 {
		fatal(fmt.Errorf("no servers selected"))
	}
	if len(suites) == 0 {
		fatal(fmt.Errorf("no workspaces selected"))
	}

	rep := Report{
		GeneratedAt: time.Now(),
		Environment: collectEnvironment(),
		Config: map[string]any{
			"warmup":      opts.warmup,
			"repeat":      opts.repeat,
			"callTimeout": opts.callTimeout.String(),
			"diagTimeout": opts.diagTimeout.String(),
		},
	}

	for _, suite := range suites {
		root := filepath.Join(opts.repoRoot, suite.Root)
		if _, err := os.Stat(root); err != nil {
			fmt.Fprintf(os.Stderr, "skipping workspace %s: %v\n", suite.Workspace, err)
			continue
		}
		fileCount := countPHPFiles(root)
		if opts.warmFS {
			fmt.Printf("warming file cache for %s (%d PHP files)…\n", suite.Workspace, fileCount)
			warmFilesystem(root)
		}
		for _, spec := range specs {
			if spec.Disabled {
				fmt.Printf("skipping %s (disabled in servers.json)\n", spec.ID)
				continue
			}
			if !serverAvailable(spec, opts) {
				fmt.Printf("skipping %s: command not found (%s)\n", spec.ID, strings.Join(spec.Cmd, " "))
				continue
			}
			fmt.Printf("\n=== %s on %s ===\n", spec.ID, suite.Workspace)
			run := runServer(spec, suite, root, fileCount, opts)
			rep.Runs = append(rep.Runs, run)
			if run.Error != "" {
				fmt.Printf("  ! %s\n", run.Error)
			}
		}
	}

	jsonPath, mdPath, err := writeReport(opts.outDir, rep)
	if err != nil {
		fatal(err)
	}
	printConsole(rep)
	fmt.Printf("\nresults: %s\nreport:  %s\n", jsonPath, mdPath)
}

// runServer launches one server against one workspace and runs the suite.
func runServer(spec ServerSpec, suite Suite, root string, fileCount int, opts options) ServerRun {
	run := ServerRun{
		ServerID:   spec.ID,
		ServerName: orDefault(spec.Name, spec.ID),
		License:    spec.License,
		Workspace:  suite.Workspace,
		PHPFiles:   fileCount,
		Notes:      spec.Notes,
	}
	storage := filepath.Join(opts.benchDir, ".work", spec.ID, suite.Workspace)
	vars := map[string]string{
		"root":      opts.repoRoot,
		"bench":     opts.benchDir,
		"tools":     filepath.Join(opts.benchDir, ".tools"),
		"workspace": root,
		"storage":   storage,
	}
	if err := resetPaths(spec, vars, opts.benchDir); err != nil {
		run.Error = err.Error()
		return run
	}
	if err := os.MkdirAll(storage, 0o755); err != nil {
		run.Error = err.Error()
		return run
	}

	argv := expandSlice(spec.Cmd, vars)
	run.Command = argv
	run.Version = serverVersion(spec, vars)

	env := os.Environ()
	for k, v := range spec.Env {
		env = append(env, k+"="+expandVars(v, vars))
	}

	spawn := time.Now()
	client, err := StartClient(argv, root, env)
	if err != nil {
		run.Error = fmt.Sprintf("start: %v", err)
		return run
	}
	defer client.Shutdown()
	sampler := startMemSampler(client.PID(), opts.sampleEvery)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	initRes, err := client.Call(ctx, "initialize", initializeParams(root, expandMap(spec.InitOptions, vars)))
	cancel()
	if err != nil {
		run.Error = fmt.Sprintf("initialize: %v — stderr: %s", err, truncate(client.Stderr(), 400))
		peak, cpu := sampler.Stop()
		run.PeakRSSMB, run.CPUSeconds = peak, cpu
		return run
	}
	run.StartupMs = msSince(spawn)

	var init struct {
		Capabilities map[string]interface{} `json:"capabilities"`
		ServerInfo   struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(initRes, &init)
	if run.Version == "" && init.ServerInfo.Version != "" {
		run.Version = init.ServerInfo.Version
	}
	run.Capabilities = advertisedProviders(init.Capabilities)
	_ = client.Notify("initialized", map[string]interface{}{})

	sess := &session{
		client:   client,
		root:     root,
		caps:     init.Capabilities,
		open:     map[string]bool{},
		srcCache: map[string]string{},
		timeout:  opts.callTimeout,
		diagWait: opts.diagTimeout,
	}
	for _, f := range suite.OpenFiles {
		if err := sess.didOpen(f); err != nil && opts.verbose {
			fmt.Printf("  open %s: %v\n", f, err)
		}
	}

	readyTimeout := time.Duration(spec.ReadyTimeoutSec) * time.Second
	if opts.readyTimeout > 0 {
		readyTimeout = opts.readyTimeout
	}
	if probe, ok := findCaseSpec(suite, suite.ReadyProbe); ok {
		if waitReady(sess, probe, suite.Workspace, readyTimeout, opts.verbose) {
			run.ReadyMs = msSince(spawn)
			run.IndexPeakMB = sampler.Peak()
		} else {
			fmt.Printf("  ! never became ready within %s (probe %s)\n", readyTimeout, suite.ReadyProbe)
		}
	} else if suite.ReadyProbe != "" {
		run.Error = fmt.Sprintf("readyProbe %q not found in suite", suite.ReadyProbe)
	}
	if sig, at, ok := firstReadySignal(spec, client); ok {
		run.ReadySignal = sig
		run.ReadySignalMs = float64(at.Sub(spawn).Microseconds()) / 1000
	}

	for _, c := range suite.Cases {
		if !caseSelected(c, opts) {
			continue
		}
		if contains(spec.SkipFeatures, c.Feature) || contains(c.SkipServers, spec.ID) {
			run.Cases = append(run.Cases, CaseResult{
				ID: c.ID, Feature: c.Feature, Workspace: suite.Workspace, Tags: c.Tags,
				Status: StatusSkipped, Detail: "excluded for this server",
			})
			continue
		}
		res := sess.Run(c, suite.Workspace, opts.warmup, opts.repeat)
		if opts.verbose {
			fmt.Printf("  %-6s %-42s %s\n", res.Status, res.ID, res.Detail)
		}
		run.Cases = append(run.Cases, res)
	}

	// A server that never pushed a single diagnostic does not do diagnostics;
	// that is a missing capability, not a set of wrong answers.
	if !sess.sawDiags {
		for i := range run.Cases {
			if run.Cases[i].Feature == "diagnostics" && run.Cases[i].Status != StatusSkipped {
				run.Cases[i].Status = StatusUnsupported
				run.Cases[i].Detail = "server published no diagnostics"
			}
		}
	}

	// When cases errored, whatever the server said on stderr is usually the
	// explanation, so keep the tail of it with the results.
	for _, c := range run.Cases {
		if c.Status == StatusError {
			if tail := strings.TrimSpace(client.Stderr()); tail != "" {
				if len(tail) > 2000 {
					tail = "…" + tail[len(tail)-2000:]
				}
				run.StderrTail = tail
			}
			break
		}
	}

	peak, cpu := sampler.Stop()
	run.PeakRSSMB, run.CPUSeconds = peak, cpu
	if run.IndexPeakMB == 0 {
		run.IndexPeakMB = peak
	}
	return run
}

// waitReady polls the probe case until it passes. This is the only start-up
// measurement that means the same thing for every server: "answers a
// cross-file question correctly".
func waitReady(sess *session, probe Case, workspace string, timeout time.Duration, verbose bool) bool {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		res := sess.Run(probe, workspace, 0, 1)
		if res.Status == StatusPass {
			return true
		}
		if res.Status == StatusUnsupported {
			return false
		}
		if verbose && attempt%10 == 0 {
			fmt.Printf("  … not ready after %d probes (%s: %s)\n", attempt, res.Status, truncate(res.Detail, 80))
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func firstReadySignal(spec ServerSpec, client *Client) (string, time.Time, bool) {
	if len(spec.ReadySignals) == 0 {
		return "", time.Time{}, false
	}
	for _, n := range client.Notifications() {
		for i := range spec.ReadySignals {
			if spec.ReadySignals[i].Matches(n) {
				label := spec.ReadySignals[i].Method
				if label == "" {
					label = spec.ReadySignals[i].Pattern
				}
				return label, n.At, true
			}
		}
	}
	return "", time.Time{}, false
}

func initializeParams(root string, initOptions map[string]interface{}) map[string]interface{} {
	uri := pathToURI(root)
	return map[string]interface{}{
		"processId":             os.Getpid(),
		"clientInfo":            map[string]interface{}{"name": "lspbench", "version": "1"},
		"locale":                "en",
		"rootPath":              root,
		"rootUri":               uri,
		"initializationOptions": initOptions,
		"workspaceFolders": []map[string]interface{}{
			{"uri": uri, "name": filepath.Base(root)},
		},
		"capabilities": map[string]interface{}{
			"workspace": map[string]interface{}{
				"workspaceFolders":      true,
				"configuration":         true,
				"applyEdit":             true,
				"didChangeWatchedFiles": map[string]interface{}{"dynamicRegistration": true},
				"symbol": map[string]interface{}{
					"symbolKind": map[string]interface{}{"valueSet": intRange(1, 26)},
				},
			},
			"textDocument": map[string]interface{}{
				"synchronization": map[string]interface{}{
					"didSave": true, "willSave": false, "dynamicRegistration": true,
				},
				"publishDiagnostics": map[string]interface{}{
					"relatedInformation":     true,
					"versionSupport":         true,
					"codeDescriptionSupport": true,
					"tagSupport":             map[string]interface{}{"valueSet": []int{1, 2}},
				},
				"completion": map[string]interface{}{
					"dynamicRegistration": true,
					"contextSupport":      true,
					"completionItem": map[string]interface{}{
						"snippetSupport":       true,
						"documentationFormat":  []string{"markdown", "plaintext"},
						"deprecatedSupport":    true,
						"insertReplaceSupport": true,
						"labelDetailsSupport":  true,
						"resolveSupport":       map[string]interface{}{"properties": []string{"documentation", "detail"}},
					},
					"completionItemKind": map[string]interface{}{"valueSet": intRange(1, 25)},
				},
				"hover": map[string]interface{}{
					"dynamicRegistration": true,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"signatureHelp": map[string]interface{}{
					"dynamicRegistration": true,
					"signatureInformation": map[string]interface{}{
						"documentationFormat":    []string{"markdown", "plaintext"},
						"parameterInformation":   map[string]interface{}{"labelOffsetSupport": true},
						"activeParameterSupport": true,
					},
				},
				"definition":     map[string]interface{}{"linkSupport": true},
				"typeDefinition": map[string]interface{}{"linkSupport": true},
				"implementation": map[string]interface{}{"linkSupport": true},
				"references":     map[string]interface{}{"dynamicRegistration": true},
				"documentSymbol": map[string]interface{}{
					"hierarchicalDocumentSymbolSupport": true,
					"symbolKind":                        map[string]interface{}{"valueSet": intRange(1, 26)},
				},
				"codeAction": map[string]interface{}{
					"codeActionLiteralSupport": map[string]interface{}{
						"codeActionKind": map[string]interface{}{"valueSet": []string{"quickfix", "refactor", "source"}},
					},
				},
				"rename":         map[string]interface{}{"prepareSupport": true},
				"foldingRange":   map[string]interface{}{"lineFoldingOnly": true},
				"inlayHint":      map[string]interface{}{"dynamicRegistration": true},
				"semanticTokens": map[string]interface{}{"requests": map[string]interface{}{"full": true}, "formats": []string{"relative"}, "tokenTypes": []string{}, "tokenModifiers": []string{}},
			},
			"window": map[string]interface{}{
				"workDoneProgress": true,
				"showMessage":      map[string]interface{}{},
				"showDocument":     map[string]interface{}{"support": true},
			},
			"general": map[string]interface{}{
				"positionEncodings": []string{"utf-16"},
			},
		},
	}
}

// advertisedProviders lists the "*Provider" capabilities a server turned on.
func advertisedProviders(caps map[string]interface{}) []string {
	var out []string
	for k, v := range caps {
		if !strings.HasSuffix(k, "Provider") {
			continue
		}
		if b, isBool := v.(bool); isBool && !b {
			continue
		}
		if v == nil {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func intRange(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resetPaths deletes each server's cache directory so nobody benefits from a
// warm on-disk index. Paths are confined to bench/.work as a guard against a
// typo in servers.json deleting something real.
func resetPaths(spec ServerSpec, vars map[string]string, benchDir string) error {
	guard, err := filepath.Abs(filepath.Join(benchDir, ".work"))
	if err != nil {
		return err
	}
	for _, p := range spec.ResetPaths {
		abs, err := filepath.Abs(expandVars(p, vars))
		if err != nil {
			return err
		}
		if abs != guard && !strings.HasPrefix(abs, guard+string(filepath.Separator)) {
			return fmt.Errorf("server %s: resetPath %q is outside %s; refusing to delete", spec.ID, abs, guard)
		}
		if err := os.RemoveAll(abs); err != nil {
			return fmt.Errorf("server %s: reset %s: %w", spec.ID, abs, err)
		}
	}
	return nil
}

func serverVersion(spec ServerSpec, vars map[string]string) string {
	if len(spec.VersionCmd) == 0 {
		return ""
	}
	argv := expandSlice(spec.VersionCmd, vars)
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncate(line, 80)
		}
	}
	return ""
}

func serverAvailable(spec ServerSpec, opts options) bool {
	if len(spec.Cmd) == 0 {
		return false
	}
	vars := map[string]string{
		"root":  opts.repoRoot,
		"bench": opts.benchDir,
		"tools": filepath.Join(opts.benchDir, ".tools"),
	}
	bin := expandVars(spec.Cmd[0], vars)
	if strings.ContainsRune(bin, filepath.Separator) {
		if info, err := os.Stat(bin); err != nil || info.IsDir() {
			return false
		}
	} else if _, err := exec.LookPath(bin); err != nil {
		return false
	}
	// An interpreted server ("php .../phpactor.phar") is only installed when
	// the script it runs is there too.
	for _, arg := range expandSlice(spec.Cmd[1:], vars) {
		if strings.Contains(arg, "{{") || !strings.HasPrefix(arg, string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(arg); err != nil {
			return false
		}
	}
	return true
}

func findCaseSpec(suite Suite, id string) (Case, bool) {
	for _, c := range suite.Cases {
		if c.ID == id {
			return c, true
		}
	}
	return Case{}, false
}

func caseSelected(c Case, opts options) bool {
	if len(opts.featureFilter) > 0 && !contains(opts.featureFilter, c.Feature) {
		return false
	}
	if opts.caseFilter != "" && !strings.Contains(c.ID, opts.caseFilter) {
		return false
	}
	return true
}

func filterServers(specs []ServerSpec, ids []string) []ServerSpec {
	if len(ids) == 0 {
		return specs
	}
	var out []ServerSpec
	for _, s := range specs {
		if contains(ids, s.ID) {
			out = append(out, s)
		}
	}
	return out
}

func filterSuites(suites []Suite, names []string) []Suite {
	if len(names) == 0 {
		return suites
	}
	var out []Suite
	for _, s := range suites {
		if contains(names, s.Workspace) {
			out = append(out, s)
		}
	}
	return out
}

func listAll(specs []ServerSpec, suites []Suite, opts options) {
	fmt.Println("servers:")
	for _, s := range specs {
		state := "ok"
		if s.Disabled {
			state = "disabled"
		} else if !serverAvailable(s, opts) {
			state = "not installed"
		}
		fmt.Printf("  %-14s %-14s %s\n", s.ID, state, strings.Join(s.Cmd, " "))
	}
	for _, suite := range suites {
		fmt.Printf("\nworkspace %s (%s), %d cases, probe=%s\n", suite.Workspace, suite.Root, len(suite.Cases), suite.ReadyProbe)
		byFeature := map[string]int{}
		for _, c := range suite.Cases {
			byFeature[c.Feature]++
		}
		for _, f := range sortedCountKeys(byFeature) {
			fmt.Printf("  %-16s %d\n", f, byFeature[f])
		}
	}
}

func sortedCountKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// warmFilesystem reads every PHP file once so the first server measured does
// not pay for a cold page cache the others avoid.
func warmFilesystem(root string) {
	buf := make([]byte, 32*1024)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".php") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		for {
			if _, err := f.Read(buf); err != nil {
				break
			}
		}
		_ = f.Close()
		return nil
	})
}

func countPHPFiles(root string) int {
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".php") {
			count++
		}
		return nil
	})
	return count
}

func resolveBenchDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "bench", "servers.json")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "bench"), nil
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return "", fmt.Errorf("could not find bench/servers.json above %s; pass -bench-dir", wd)
}

// rerender rebuilds report.md from a stored results.json, so the report format
// can change without re-running an hour of benchmarks.
func rerender(resultsPath, outDir string) error {
	b, err := os.ReadFile(resultsPath)
	if err != nil {
		return err
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		return fmt.Errorf("%s: %w", resultsPath, err)
	}
	if outDir == "" {
		outDir = filepath.Dir(resultsPath)
	}
	_, mdPath, err := writeReport(outDir, rep)
	if err != nil {
		return err
	}
	printConsole(rep)
	fmt.Printf("\nreport: %s\n", mdPath)
	return nil
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "lspbench: %v\n", err)
	os.Exit(1)
}
