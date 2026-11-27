package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (s *Server) loadConfig() *config.Config {
	return s.readConfigFile().WithClientOptions(s.initOptions).WithClientOptions(s.clientSettings.Load())
}

func (s *Server) readConfigFile() *config.Config {
	cfg, err := config.LoadFromFile(filepath.Join(s.rootPath, config.FileName))
	if err != nil {
		return s.lastGoodConfigFile(err)
	}
	for _, problem := range s.configProblems.unreported(cfg.Problems()) {
		s.logger.Printf("Config problem: %v", problem)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeWarning,
			"message": "Tusk PHP skipped part of its configuration: " + problem.Error(),
		})
	}
	s.fileConfig.Store(cfg)
	return cfg
}

type reportedProblems struct {
	mu    sync.Mutex
	shown map[string]bool
}

func (r *reportedProblems) unreported(problems []error) []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := make(map[string]bool, len(problems))
	var fresh []error
	for _, problem := range problems {
		message := problem.Error()
		current[message] = true
		if !r.shown[message] {
			fresh = append(fresh, problem)
		}
	}
	r.shown = current
	return fresh
}

func (s *Server) lastGoodConfigFile(err error) *config.Config {
	s.logger.Printf("Config error: %v", err)
	last := s.fileConfig.Load()
	message := "Tusk PHP kept its previous configuration: " + err.Error()
	if last == nil {
		last = config.DefaultConfig()
		message = "Tusk PHP ignored its configuration: " + err.Error()
	}
	s.sendNotification("window/showMessage", map[string]interface{}{
		"type":    protocol.MessageTypeError,
		"message": message,
	})
	return last
}

func tuskSettings(raw json.RawMessage) (*config.ClientOptions, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, nil
	}
	var opts config.ClientOptions
	if err := json.Unmarshal(trimmed, &opts); err != nil {
		return nil, fmt.Errorf("settings are not Tusk PHP options: %w", err)
	}
	if reflect.ValueOf(opts).IsZero() {
		return nil, nil
	}
	return &opts, nil
}
