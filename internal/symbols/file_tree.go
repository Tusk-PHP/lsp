package symbols

import (
	"path"
	"strings"
)

const fileScheme = "file://"

type fileTree struct {
	files    map[string]map[string]struct{}
	children map[string]map[string]struct{}
}

func (t *fileTree) add(uri string) {
	dir, ok := uriDir(uri)
	if !ok {
		return
	}
	if t.files == nil {
		t.files = make(map[string]map[string]struct{})
		t.children = make(map[string]map[string]struct{})
	}
	if t.files[dir] == nil {
		t.files[dir] = make(map[string]struct{})
	}
	t.files[dir][uri] = struct{}{}
	for dir != "/" {
		parent := path.Dir(dir)
		if t.children[parent] == nil {
			t.children[parent] = make(map[string]struct{})
		}
		if _, linked := t.children[parent][dir]; linked {
			return
		}
		t.children[parent][dir] = struct{}{}
		dir = parent
	}
}

func (t *fileTree) remove(uri string) {
	dir, ok := uriDir(uri)
	if !ok || t.files[dir] == nil {
		return
	}
	delete(t.files[dir], uri)
	for dir != "/" && len(t.files[dir]) == 0 && len(t.children[dir]) == 0 {
		delete(t.files, dir)
		delete(t.children, dir)
		parent := path.Dir(dir)
		delete(t.children[parent], dir)
		dir = parent
	}
}

func (t *fileTree) under(dir string) []string {
	var out []string
	pending := []string{path.Clean(dir)}
	for len(pending) > 0 {
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for uri := range t.files[dir] {
			out = append(out, uri)
		}
		for child := range t.children[dir] {
			pending = append(pending, child)
		}
	}
	return out
}

func uriDir(uri string) (string, bool) {
	p, ok := strings.CutPrefix(uri, fileScheme)
	if !ok || !strings.HasPrefix(p, "/") {
		return "", false
	}
	return path.Dir(p), true
}
