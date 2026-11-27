package lsp

import (
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// clientOptions are the client's initializationOptions: the settings config
// applies over the configuration file, and what only the language server
// uses.
type clientOptions struct {
	config.ClientOptions
	// ClientCommands lists the tuskPhpLsp.* commands the client handles itself
	// (e.g. by copying the result to the clipboard). Code actions that depend on
	// that are only offered for commands listed here.
	ClientCommands []protocol.CommandName `json:"clientCommands,omitempty"`
	// CoverageNotifications asks the server to watch for new coverage reports and
	// send tusk/coverageChanged, instead of only noticing them on the next request.
	CoverageNotifications bool `json:"coverageNotifications,omitempty"`
}

// initializeParams is the initialize request with its initializationOptions
// read as clientOptions.
type initializeParams struct {
	protocol.InitializeParams
	InitializationOptions *clientOptions `json:"initializationOptions,omitempty"`
}

// settings returns the options config applies, or nil when there are none.
func (o *clientOptions) settings() *config.ClientOptions {
	if o == nil {
		return nil
	}
	return &o.ClientOptions
}
