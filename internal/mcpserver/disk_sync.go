package mcpserver

import (
	"context"
	"errors"
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

func buildWorkspace(ctx context.Context, opts workspace.Options) (*workspace.Bootstrapped, map[string]fileStamp, error) {
	ws := workspace.New(opts)
	files := scanDisk(ws)
	if err := ws.Load(ctx); err != nil {
		return nil, nil, err
	}
	return ws, files, nil
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
	vendorState := ws.VendorStatePaths()
	if slices.ContainsFunc(changed, func(path string) bool { return slices.Contains(vendorState, path) }) {
		if _, err := s.rebuildLocked(ctx); err != nil {
			return fmt.Errorf("rebuild the index after a Composer change: %w", err)
		}
		return nil
	}
	result, err := ws.Refresh(workspace.Refresh{Paths: changed, Config: s.loadConfig})
	if result.ProfileChanged {
		s.profileSource.Store(&result.ProfileSource)
	}
	if configErr := (*workspace.ConfigError)(nil); errors.As(err, &configErr) {
		return fmt.Errorf("re-read the project configuration: %w", configErr)
	}
	if err != nil {
		s.logger.Warn("some files changed on disk could not be re-indexed; they are tried again on the next call", "error", err)
	}
	s.disk.files = retryFailed(now, s.disk.files, result.Failed)
	return nil
}

func retryFailed(now, before map[string]fileStamp, failed []string) map[string]fileStamp {
	for _, path := range failed {
		if stamp, ok := before[path]; ok {
			now[path] = stamp
		} else {
			delete(now, path)
		}
	}
	return now
}

func (s *Service) loadConfig() (*config.Config, error) {
	return loadConfig(s.RootPath, s.logger)
}
