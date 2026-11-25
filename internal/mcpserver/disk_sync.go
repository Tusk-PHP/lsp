package mcpserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fileStamp struct {
	modTime int64
	size    int64
}

type diskState struct {
	mu    sync.Mutex
	files map[string]fileStamp
}

func scanDisk(ws *workspace.Bootstrapped) map[string]fileStamp {
	files := make(map[string]fileStamp)
	ws.WalkSources(ws.RootPath, func(path string, info fs.FileInfo) error {
		files[path] = stampOf(info)
		return nil
	})
	for _, path := range append(ws.ComposerStatePaths(), ws.ConfigPath()) {
		if info, err := os.Stat(path); err == nil {
			files[path] = stampOf(info)
		}
	}
	return files
}

func stampOf(info fs.FileInfo) fileStamp {
	return fileStamp{modTime: info.ModTime().UnixNano(), size: info.Size()}
}

func changedFiles(before, after map[string]fileStamp) []string {
	var changed []string
	for path, stamp := range after {
		if old, ok := before[path]; !ok || old != stamp {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	return changed
}

func (s *Service) syncBeforeReads(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if readsIndex(method, req) {
			if err := s.syncWithDisk(ctx); err != nil {
				return nil, err
			}
		}
		return next(ctx, method, req)
	}
}

func readsIndex(method string, req mcp.Request) bool {
	switch method {
	case "resources/read":
		return true
	case "tools/call":
		call, ok := req.(*mcp.CallToolRequest)
		return !ok || call.Params.Name != reindexTool
	}
	return false
}

func (s *Service) syncWithDisk(ctx context.Context) error {
	s.disk.mu.Lock()
	defer s.disk.mu.Unlock()
	ws := s.runtime().Workspace
	now := scanDisk(ws)
	changed := changedFiles(s.disk.files, now)
	if len(changed) == 0 {
		return nil
	}
	if slices.ContainsFunc(changed, func(path string) bool { return slices.Contains(ws.VendorStatePaths(), path) }) {
		if _, err := s.rebuildLocked(ctx); err != nil {
			return fmt.Errorf("rebuild the index after a Composer change: %w", err)
		}
		return nil
	}
	result, err := ws.Refresh(workspace.Refresh{Paths: changed, Config: s.loadConfig})
	if result.ProfileChanged {
		s.profileSource.Store(&result.ProfileSource)
	}
	if err != nil {
		return fmt.Errorf("re-index the files changed on disk: %w", err)
	}
	s.disk.files = now
	return nil
}

func (s *Service) loadConfig() (*config.Config, error) {
	return loadConfig(s.RootPath, s.logger)
}
