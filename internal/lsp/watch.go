package lsp

import (
	"fmt"
	"sync"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

const watchedFilesDelay = 200 * time.Millisecond

type watchedFiles struct {
	mu      sync.Mutex
	paths   map[string]struct{}
	profile bool
	timer   *time.Timer
	refresh sync.Mutex
}

func (s *Server) watchFiles() {
	if !s.clientWatchesFiles || s.rootPath == "" {
		return
	}
	globs := workspace.WatchGlobs()
	watchers := make([]protocol.FileSystemWatcher, len(globs), len(globs)+1)
	for i, glob := range globs {
		watchers[i] = protocol.FileSystemWatcher{GlobPattern: glob}
	}
	watchers = append(watchers, protocol.FileSystemWatcher{GlobPattern: "**/*", Kind: protocol.WatchCreate | protocol.WatchDelete})
	s.sendServerRequest("client/registerCapability", protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:              "tusk-php/watched-files",
			Method:          "workspace/didChangeWatchedFiles",
			RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{Watchers: watchers},
		}},
	})
}

func (s *Server) handleDidChangeWatchedFiles(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidChangeWatchedFilesParams](msg)
	if !ok || s.workspace == nil {
		return
	}
	paths := make([]string, 0, len(params.Changes))
	for _, change := range params.Changes {
		if path := symbols.URIToPath(change.URI); path != "" {
			paths = append(paths, path)
		}
	}
	s.queueRefresh(paths, false)
}

func (s *Server) handleDidChangeConfiguration(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.DidChangeConfigurationParams](msg)
	if !ok || s.workspace == nil {
		return
	}
	settings, err := tuskSettings(params.Settings)
	if err != nil {
		s.logger.Printf("workspace/didChangeConfiguration: %v", err)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeError,
			"message": "Tusk PHP ignored the new settings: " + err.Error(),
		})
		return
	}
	if settings != nil {
		s.clientSettings.Store(settings)
	}
	s.queueRefresh(nil, true)
}

func (s *Server) queueRefresh(paths []string, profile bool) {
	w := &s.watch
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.paths == nil {
		w.paths = make(map[string]struct{})
	}
	for _, path := range paths {
		w.paths[path] = struct{}{}
	}
	w.profile = w.profile || profile
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(watchedFilesDelay, func() {
		s.goSafe("workspace.Refresh", s.refreshWorkspace)
	})
}

func (s *Server) takeRefresh() ([]string, bool) {
	w := &s.watch
	w.mu.Lock()
	defer w.mu.Unlock()
	paths := make([]string, 0, len(w.paths))
	for path := range w.paths {
		paths = append(paths, path)
	}
	profile := w.profile
	w.paths, w.profile = nil, false
	return paths, profile
}

func (s *Server) refreshWorkspace() {
	<-s.indexed
	s.watch.refresh.Lock()
	defer s.watch.refresh.Unlock()
	paths, profile := s.takeRefresh()
	if len(paths) == 0 && !profile {
		return
	}
	result, err := s.workspace.Refresh(workspace.Refresh{
		Paths:      paths,
		Profile:    profile,
		Config:     func() (*config.Config, error) { return s.loadConfig(), nil },
		EditorOwns: s.isOpenAt,
	})
	if err != nil {
		s.logger.Printf("workspace refresh: %v", err)
		s.sendNotification("window/logMessage", map[string]interface{}{
			"type":    protocol.MessageTypeWarning,
			"message": "PHP LSP: some changed files could not be re-indexed: " + err.Error(),
		})
	}
	if result.ProfileChanged {
		s.logInfo(fmt.Sprintf("PHP LSP: PHP %s (source: %s)", result.Profile.PHPVersion, result.ProfileSource))
	}
	s.handBackToEditor(result.EditorOpened)
	for _, uri := range result.Files {
		if affectsComponents(s.rootPath, uri) {
			s.session.Components().Invalidate()
			break
		}
	}
	if len(result.Files) > 0 || result.Composer || result.ProfileChanged {
		s.republishOpenDocuments()
	}
}

func (s *Server) handBackToEditor(uris []string) {
	for _, uri := range uris {
		if _, open := s.openDocument(uri); open {
			s.reindexOpenDocument(uri)
		}
	}
}

func (s *Server) isOpenAt(path string) bool {
	_, open := s.openDocument(symbols.PathToURI(path))
	return open
}

func (s *Server) openDocument(uri string) (string, bool) {
	s.docMu.RLock()
	defer s.docMu.RUnlock()
	source, open := s.documents[uri]
	return source, open
}
