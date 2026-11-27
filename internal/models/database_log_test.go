//go:build !wasip1

package models

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestALiveSchemaThatCannotBeReadLogsNoConfiguredValue(t *testing.T) {
	port := closedPort(t)
	cases := map[string]map[string]string{
		"mysql":       {"DB_CONNECTION": "mysql", "DB_HOST": "127.0.0.1", "DB_PORT": port, "DB_DATABASE": "secret_db_name", "DB_USERNAME": "secret_user", "DB_PASSWORD": "secret_password"},
		"pgsql":       {"DB_CONNECTION": "pgsql", "DB_HOST": "127.0.0.1", "DB_PORT": port, "DB_DATABASE": "secret_db_name", "DB_USERNAME": "secret_user", "DB_PASSWORD": "secret_password"},
		"sqlite":      {"DB_CONNECTION": "sqlite", "DB_DATABASE": filepath.Join(t.TempDir(), "secret_dir", "secret_db_name.sqlite")},
		"unsupported": {"DB_CONNECTION": "secret_driver", "DB_HOST": "127.0.0.1", "DB_PORT": port, "DB_DATABASE": "secret_db_name"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			var dotenv strings.Builder
			for key, value := range env {
				fmt.Fprintf(&dotenv, "%s=%s\n", key, value)
			}
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(dotenv.String()), 0o644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if _, err := ScanSchema(root, "laravel", nil, log.New(&out, "", 0), NewSchemaCache()); err != nil {
				t.Fatal(err)
			}
			logged := out.String()
			if !strings.Contains(logged, "Live database schema not read") {
				t.Fatalf("the failure is logged: %q", logged)
			}
			for key, value := range env {
				if strings.Contains(logged, value) {
					t.Errorf("the log shows %s: %q", key, logged)
				}
			}
		})
	}
}

func TestALiveSchemaThatIsReadLogsNoConfiguredValue(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "secret_db_name.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_CONNECTION=sqlite\nDB_DATABASE="+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	schema, err := ScanSchema(root, "laravel", nil, log.New(&out, "", 0), NewSchemaCache())
	if err != nil || schema.Source != SchemaSourceLive {
		t.Fatalf("the live schema is read: %+v, %v", schema, err)
	}
	if logged := out.String(); !strings.Contains(logged, "Connected") || strings.Contains(logged, "secret_db_name") {
		t.Errorf("the connection is logged without the database: %q", logged)
	}
}

func TestLiveSchemaErrorsKeepOnlyTheDriversErrorClass(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&mysql.MySQLError{Number: 1045, Message: "Access denied for user 'secret_user'@'10.1.2.3' (using password: YES)"}, "could not connect: MySQL error 1045"},
		{&pq.Error{Code: "28P01", Message: `password authentication failed for user "secret_user"`}, "could not connect: PostgreSQL error 28P01"},
		{&net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 5432}, Err: errors.New("connection refused")}, "could not connect: network error on dial"},
		{&net.DNSError{Name: "secret-host.internal", Err: "no such host"}, "could not connect: host not found"},
		{errors.New("secret_user secret_password"), "could not connect: driver error"},
	}
	for _, tc := range cases {
		err := &liveSchemaError{step: "connect", err: fmt.Errorf("ping: %w", tc.err)}
		if got := err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
		if !errors.Is(err, tc.err) {
			t.Errorf("the driver's error stays reachable: %v", tc.err)
		}
	}
}
