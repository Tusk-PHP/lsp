package lsp

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadyDoesNotWaitForTheDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted <- conn
		}
	}()

	root := t.TempDir()
	port := listener.Addr().(*net.TCPAddr).Port
	for path, content := range map[string]string{
		"composer.json":       `{"require": {"laravel/framework": "^11.0"}, "autoload": {"psr-4": {"App\\": "app/"}}}`,
		".env":                fmt.Sprintf("DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=%d\nDB_DATABASE=app\n", port),
		".tusk-php.json":      `{"db": {"connectTimeoutMs": 120000}}`,
		"app/Models/Post.php": "<?php\nnamespace App\\Models;\nclass Post {}\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})

	deadline := time.Now().Add(20 * time.Second)
	for !h.server.index.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("the index waited for a database that never answers before becoming ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case conn := <-accepted:
		conn.Close()
	case <-time.After(20 * time.Second):
		t.Fatal("the schema scan never asked the database, so this test no longer covers the wait")
	}
}
