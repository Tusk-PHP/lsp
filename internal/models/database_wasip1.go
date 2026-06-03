//go:build wasip1

package models

import (
	"errors"
	"log"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
)

// errLiveDBUnsupported is returned by the wasm build's scanLiveSchema: live
// database introspection needs native networking and SQL drivers that are
// unavailable under wasip1/wasm (browser). ScanSchema falls back to
// migration-derived schema where available.
var errLiveDBUnsupported = errors.New("models: live database introspection unavailable in wasm build")

func scanLiveSchema(_ *config.DatabaseConfig, _ time.Duration, _ *SchemaCache, _ *log.Logger) (*Schema, error) {
	return nil, errLiveDBUnsupported
}
