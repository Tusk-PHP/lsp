package coverage

import (
	"encoding/xml"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// refreshInterval limits how often RefreshAsync looks for a newer report.
const refreshInterval = 2 * time.Second

// maxRootSamples limits how many report files reportRoot checks. Only deleted
// files make it look past the first one.
const maxRootSamples = 20

// commonReportPaths are checked when phpunit.xml doesn't name a Clover report.
var commonReportPaths = []string{
	"clover.xml",
	"build/logs/clover.xml",
	"build/coverage/clover.xml",
	"coverage/clover.xml",
	"tests/coverage/clover.xml",
}

// snapshot is one loaded report. It's replaced whole, never changed in place.
type snapshot struct {
	path    string
	modTime time.Time
	report  *Report
	prefix  string // what report paths have in place of the workspace root, with a trailing slash
}

// Store holds the workspace's latest coverage report and notices newer ones.
type Store struct {
	rootPath   string
	configured string // report path from config; replaces discovery when set
	logger     *log.Logger

	current    atomic.Pointer[snapshot]
	refreshMu  sync.Mutex
	refreshing atomic.Bool
	lastCheck  atomic.Int64

	onChange func()

	// The last report that failed to parse, so it's only retried once it changes.
	failedPath    string
	failedModTime time.Time

	stop     chan struct{}
	stopOnce sync.Once
}

// NewStore creates an empty store. Nothing is read until Refresh is called.
func NewStore(rootPath, configuredPath string, logger *log.Logger) *Store {
	return &Store{rootPath: rootPath, configured: configuredPath, logger: logger, stop: make(chan struct{})}
}

// OnChange sets a function to call after a different report loads or the report
// goes away. Set it before the first Refresh.
func (s *Store) OnChange(fn func()) {
	s.onChange = fn
}

// Refresh loads the newest report if it changed since the last load. It reads
// from disk, so callers on a request path should use RefreshAsync.
func (s *Store) Refresh() {
	if s.refresh() && s.onChange != nil {
		s.onChange()
	}
}

// refresh reports whether the loaded report changed.
func (s *Store) refresh() bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.lastCheck.Store(time.Now().UnixNano())
	if s.rootPath == "" {
		return false
	}

	reportPath, modTime := s.findReport()
	if reportPath == "" {
		return s.current.Swap(nil) != nil
	}
	if cur := s.current.Load(); cur != nil && cur.path == reportPath && cur.modTime.Equal(modTime) {
		return false
	}
	if s.failedPath == reportPath && s.failedModTime.Equal(modTime) {
		return false
	}

	f, err := os.Open(reportPath)
	if err != nil {
		s.logf("coverage: can't open %s: %v", reportPath, err)
		return false
	}
	defer f.Close()

	report, err := ParseClover(f)
	if err != nil {
		// A test run may still be writing the report; keep what we have and
		// retry when the file next changes.
		s.logf("coverage: can't read %s: %v", reportPath, err)
		s.failedPath, s.failedModTime = reportPath, modTime
		return false
	}
	s.current.Store(&snapshot{path: reportPath, modTime: modTime, report: report, prefix: s.reportRoot(report)})
	return true
}

// Watch refreshes on a timer until Stop is called, for clients that want to hear
// about a new report without asking for coverage first.
func (s *Store) Watch() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.Refresh()
		}
	}
}

// Stop ends Watch.
func (s *Store) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// RefreshAsync checks for a newer report in the background, at most once every
// refreshInterval. It never blocks the caller.
func (s *Store) RefreshAsync() {
	if time.Now().UnixNano()-s.lastCheck.Load() < int64(refreshInterval) {
		return
	}
	if !s.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refreshing.Store(false)
		defer func() {
			if r := recover(); r != nil {
				s.logf("coverage: refresh panic: %v", r)
			}
		}()
		s.Refresh()
	}()
}

// ReportInfo describes the loaded report. ok is false when there isn't one.
func (s *Store) ReportInfo() (reportPath string, generated time.Time, ok bool) {
	cur := s.current.Load()
	if cur == nil {
		return "", time.Time{}, false
	}
	return cur.path, cur.modTime, true
}

// Files returns every file in the loaded report, sorted by path.
func (s *Store) Files() []*File {
	cur := s.current.Load()
	if cur == nil {
		return nil
	}
	files := make([]*File, 0, len(cur.report.Files))
	for _, f := range cur.report.Files {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

// File returns the coverage of a source file, given its path on this machine.
// stale is true when the file changed after the report was written, so its line
// numbers may no longer match.
func (s *Store) File(sourcePath string) (file *File, stale bool) {
	cur := s.current.Load()
	if cur == nil || sourcePath == "" {
		return nil, false
	}
	file = cur.report.Files[filepath.ToSlash(sourcePath)]
	if rel := s.relativeToRoot(sourcePath); file == nil && rel != "" && cur.prefix != "" {
		file = cur.report.Files[cur.prefix+rel]
	}
	if file == nil {
		return nil, false
	}
	if info, err := os.Stat(sourcePath); err == nil && info.ModTime().After(cur.modTime) {
		stale = true
	}
	return file, stale
}

// Method returns the coverage of the named method or function in a source file.
// line is its 1-based declaration line, used to tell apart same-named methods of
// different classes in one file. Methods with no executable lines return nil.
func (s *Store) Method(sourcePath, name string, line int) (method *Method, stale bool) {
	file, stale := s.File(sourcePath)
	if file == nil {
		return nil, false
	}
	bestDistance := math.MaxInt
	for i := range file.Methods {
		m := &file.Methods[i]
		if !strings.EqualFold(m.Name, name) || m.Executable == 0 {
			continue
		}
		distance := m.Line - line
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance {
			method, bestDistance = m, distance
		}
	}
	if method == nil {
		return nil, false
	}
	return method, stale
}

// Relative returns a file's path relative to the workspace, as seen on this
// machine, or its path in the report if it's outside the workspace.
func (s *Store) Relative(f *File) string {
	reportPath := filepath.ToSlash(f.Path)
	if cur := s.current.Load(); cur != nil && cur.prefix != "" && strings.HasPrefix(reportPath, cur.prefix) {
		return strings.TrimPrefix(reportPath, cur.prefix)
	}
	return reportPath
}

// LocalPath returns where a report's file is on this machine, or "" if it isn't
// here, such as a file from a container path outside the workspace.
func (s *Store) LocalPath(f *File) string {
	p := filepath.FromSlash(s.Relative(f))
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.rootPath, p)
	}
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		return ""
	}
	return p
}

// reportRoot works out what report paths have in place of the workspace root.
// Reports written inside a container say /var/www/html/app/User.php for
// <root>/app/User.php, so this looks for the longest ending of a report path
// that exists under the workspace. Longest wins: <root>/User.php also exists in
// some projects, and matching it would put the root one directory too deep.
func (s *Store) reportRoot(report *Report) string {
	paths := make([]string, 0, len(report.Files))
	for p := range report.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) > maxRootSamples {
		paths = paths[:maxRootSamples]
	}

	for _, p := range paths {
		rest := strings.TrimLeft(p, "/")
		for rest != "" {
			if info, err := os.Stat(filepath.Join(s.rootPath, filepath.FromSlash(rest))); err == nil && !info.IsDir() {
				return strings.TrimSuffix(p, rest)
			}
			i := strings.Index(rest, "/")
			if i < 0 {
				break
			}
			rest = rest[i+1:]
		}
	}
	return ""
}

func (s *Store) relativeToRoot(sourcePath string) string {
	rel, err := filepath.Rel(s.rootPath, sourcePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// findReport returns the report to load: the configured path if there is one,
// or else the most recently written of the discovered candidates.
func (s *Store) findReport() (string, time.Time) {
	if s.configured != "" {
		p := s.absolute(s.configured)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, info.ModTime()
		}
		return "", time.Time{}
	}

	var newest string
	var newestTime time.Time
	for _, candidate := range s.candidates() {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = candidate, info.ModTime()
		}
	}
	return newest, newestTime
}

func (s *Store) candidates() []string {
	candidates := make([]string, 0, len(commonReportPaths)+2)
	configs, _ := filepath.Glob(filepath.Join(s.rootPath, "phpunit*.xml*"))
	for _, config := range configs {
		data, err := os.ReadFile(config)
		if err != nil {
			continue
		}
		for _, output := range cloverOutputs(data) {
			candidates = append(candidates, s.absolute(output))
		}
	}
	for _, p := range commonReportPaths {
		candidates = append(candidates, s.absolute(p))
	}
	return candidates
}

func (s *Store) absolute(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.rootPath, p)
}

// cloverOutputs returns the Clover report paths named in a PHPUnit config, from
// <coverage><report><clover outputFile="..."> and PHPUnit 9's older
// <logging><log type="coverage-clover" target="...">.
func cloverOutputs(data []byte) []string {
	var cfg struct {
		Clover []struct {
			OutputFile string `xml:"outputFile,attr"`
		} `xml:"coverage>report>clover"`
		Logs []struct {
			Type   string `xml:"type,attr"`
			Target string `xml:"target,attr"`
		} `xml:"logging>log"`
	}
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	var outputs []string
	for _, c := range cfg.Clover {
		if c.OutputFile != "" {
			outputs = append(outputs, path.Clean(c.OutputFile))
		}
	}
	for _, l := range cfg.Logs {
		if l.Type == "coverage-clover" && l.Target != "" {
			outputs = append(outputs, path.Clean(l.Target))
		}
	}
	return outputs
}

func (s *Store) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
