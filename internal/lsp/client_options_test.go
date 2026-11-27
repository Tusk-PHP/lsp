package lsp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// One initializationOptions object carries both the settings config applies
// and the options only the server reads.
func TestInitializeParamsReadTheClientOptions(t *testing.T) {
	var params initializeParams
	raw := `{"rootUri": "file:///app", "initializationOptions": {"phpVersion": "8.3", "inlayHints": {"enabled": false}, "clientCommands": ["tuskPhpLsp.copyToClipboard"], "coverageNotifications": true}}`
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatal(err)
	}
	opts := params.InitializationOptions
	if params.RootURI != "file:///app" || opts == nil {
		t.Fatalf("got %+v", params)
	}
	if s := opts.settings(); s.PHPVersion != "8.3" || s.InlayHints == nil || *s.InlayHints.Enabled {
		t.Errorf("settings: %+v", s)
	}
	if !slices.Equal(opts.ClientCommands, []protocol.CommandName{"tuskPhpLsp.copyToClipboard"}) || !opts.CoverageNotifications {
		t.Errorf("server options: %+v", opts)
	}

	var none initializeParams
	if err := json.Unmarshal([]byte(`{"rootUri": "file:///app"}`), &none); err != nil {
		t.Fatal(err)
	}
	if none.InitializationOptions.settings() != nil {
		t.Error("no initializationOptions give no settings")
	}
	if err := json.Unmarshal([]byte(`{"initializationOptions": {"phpVersion": 8}}`), &none); err == nil {
		t.Error("a mistyped option still fails the request")
	}
}
