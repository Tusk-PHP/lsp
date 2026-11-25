//go:build !wasip1

package models

import (
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/Tusk-PHP/lsp/internal/config"
)

func TestPostgresDSNKeepsEveryValueIntact(t *testing.T) {
	for _, value := range []string{"my secret", "it's", `back\slash`, `\'`, "a=b c=d", "", "'"} {
		cfg := &config.DatabaseConfig{Driver: "pgsql", Host: "db " + value, Port: "5433", Database: value + " db", Username: value + "user", Password: value}
		parsed, err := pq.NewConfig(postgresDSN(cfg, 1500*time.Millisecond))
		if err != nil {
			t.Fatalf("%q: pq rejects the DSN %q: %v", value, postgresDSN(cfg, time.Second), err)
		}
		if parsed.Host != cfg.Host || parsed.Port != 5433 || parsed.Database != cfg.Database || parsed.User != cfg.Username || parsed.Password != cfg.Password {
			t.Errorf("%q: pq read host=%q port=%d dbname=%q user=%q password=%q", value, parsed.Host, parsed.Port, parsed.Database, parsed.User, parsed.Password)
		}
		if parsed.SSLMode != pq.SSLModeDisable || parsed.ConnectTimeout != time.Second {
			t.Errorf("%q: sslmode=%q connect_timeout=%v", value, parsed.SSLMode, parsed.ConnectTimeout)
		}
	}
}
