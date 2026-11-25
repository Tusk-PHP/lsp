package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"

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
	for _, problem := range cfg.Problems() {
		s.logger.Printf("Config problem: %v", problem)
		s.sendNotification("window/showMessage", map[string]interface{}{
			"type":    protocol.MessageTypeWarning,
			"message": "Tusk PHP skipped part of its configuration: " + problem.Error(),
		})
	}
	s.fileConfig.Store(cfg)
	return cfg
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

func tuskSettings(raw json.RawMessage) (*protocol.InitializationOptions, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, nil
	}
	var opts protocol.InitializationOptions
	if err := json.Unmarshal(trimmed, &opts); err != nil {
		return nil, fmt.Errorf("settings are not Tusk PHP options: %w", err)
	}
	if reflect.ValueOf(opts).IsZero() {
		return nil, nil
	}
	return &opts, nil
}
