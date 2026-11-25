//go:build !wasip1

package models

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
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

func TestMySQLDataSourceReachesTheConfiguredHost(t *testing.T) {
	cases := []struct {
		host, port, password, wantAddr string
	}{
		{"127.0.0.1", "3306", "secret", "127.0.0.1:3306"},
		{"db.internal", "3307", "secret", "db.internal:3307"},
		{"::1", "3306", "secret", "[::1]:3306"},
		{"[::1]", "3306", "secret", "[::1]:3306"},
		{"fe80::1%en0", "3306", "secret", "[fe80::1%en0]:3306"},
		{"fd00::5", "3306", "secret", "[fd00::5]:3306"},
		{"db", "3306", "p@ss/w:rd", "db:3306"},
	}
	for _, c := range cases {
		cfg := &config.DatabaseConfig{Driver: "mysql", Host: c.host, Port: c.port, Username: "app", Password: c.password, Database: "shop"}
		driver, dsn, err := dataSource(cfg, 3*time.Second)
		if err != nil || driver != "mysql" {
			t.Fatalf("dataSource(%s) = %q, %v", c.host, driver, err)
		}
		parsed, err := mysql.ParseDSN(dsn)
		if err != nil {
			t.Errorf("the driver can't read the DSN for host %s: %v", c.host, err)
			continue
		}
		if parsed.Net != "tcp" || parsed.Addr != c.wantAddr || parsed.User != "app" || parsed.Passwd != c.password || parsed.DBName != "shop" || parsed.Timeout != 3*time.Second {
			t.Errorf("host %s: got net %q addr %q user %q password %q db %q timeout %v", c.host, parsed.Net, parsed.Addr, parsed.User, parsed.Passwd, parsed.DBName, parsed.Timeout)
		}
	}
}

func TestDataSourceRejectsAnUnknownDriver(t *testing.T) {
	if _, _, err := dataSource(&config.DatabaseConfig{Driver: "oracle"}, time.Second); err == nil {
		t.Error("an unsupported driver is an error")
	}
}

func TestPostgresDataSourceReachesTheConfiguredServer(t *testing.T) {
	cases := []struct {
		host, wantHost       string
		timeout, wantTimeout time.Duration
	}{
		{"db", "db", 3 * time.Second, 3 * time.Second},
		{"db", "db", 200 * time.Millisecond, time.Second},
	}
	for _, c := range cases {
		cfg := &config.DatabaseConfig{Driver: "pgsql", Host: c.host, Port: "5432", Username: "app", Password: "secret", Database: "shop"}
		driver, dsn, err := dataSource(cfg, c.timeout)
		if err != nil || driver != "postgres" {
			t.Fatalf("dataSource(pgsql %s, %v) = %q, %v", c.host, c.timeout, driver, err)
		}
		parsed, err := pq.NewConfig(dsn)
		if err != nil {
			t.Errorf("pq can't read the DSN for host %s: %v", c.host, err)
			continue
		}
		if parsed.Host != c.wantHost || parsed.Port != 5432 || parsed.User != "app" || parsed.Password != "secret" || parsed.Database != "shop" || parsed.ConnectTimeout != c.wantTimeout {
			t.Errorf("host %s, timeout %v: got host %q port %d user %q password %q dbname %q connect_timeout %v", c.host, c.timeout, parsed.Host, parsed.Port, parsed.User, parsed.Password, parsed.Database, parsed.ConnectTimeout)
		}
	}
}

func TestSQLiteDataSourceIsTheDatabasePath(t *testing.T) {
	cfg := &config.DatabaseConfig{Driver: "sqlite", Database: "/app/database/database.sqlite"}
	driver, dsn, err := dataSource(cfg, time.Second)
	if err != nil || driver != "sqlite" || dsn != cfg.Database {
		t.Errorf("dataSource(sqlite) = %q, %q, %v; want %q, %q", driver, dsn, err, "sqlite", cfg.Database)
	}
}

func TestOpenDatabaseConnectsThroughTheDataSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.sqlite")
	db, err := openDatabase(&config.DatabaseConfig{Driver: "sqlite", Database: path}, time.Second)
	if err != nil {
		t.Fatalf("openDatabase(sqlite %s) = %v", path, err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("closing %s: %v", path, err)
	}
	if _, err := openDatabase(&config.DatabaseConfig{Driver: "oracle"}, time.Second); err == nil {
		t.Error("openDatabase with an unsupported driver is an error")
	}
}
