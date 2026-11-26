package symbols

import "path"

type fileTree struct {
	windows  bool
	files    map[string]map[string]struct{}
	children map[string]map[string]struct{}
}

func (t *fileTree) add(uri string) {
	dir, ok := t.dirOf(uri)
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
	for parent := path.Dir(dir); parent != dir; dir, parent = parent, path.Dir(parent) {
		if t.children[parent] == nil {
			t.children[parent] = make(map[string]struct{})
		}
		if _, linked := t.children[parent][dir]; linked {
			return
		}
		t.children[parent][dir] = struct{}{}
	}
}

func (t *fileTree) remove(uri string) {
	dir, ok := t.dirOf(uri)
	if !ok || t.files[dir] == nil {
		return
	}
	delete(t.files[dir], uri)
	for parent := path.Dir(dir); parent != dir && len(t.files[dir]) == 0 && len(t.children[dir]) == 0; dir, parent = parent, path.Dir(parent) {
		delete(t.files, dir)
		delete(t.children, dir)
		delete(t.children[parent], dir)
	}
}

func (t *fileTree) under(dir string) []string {
	var out []string
	pending := []string{slashPath(dir, t.windows)}
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

func (t *fileTree) dirOf(uri string) (string, bool) {
	p, err := uriSlashPath(uri, t.windows)
	if err != nil {
		return "", false
	}
	return path.Dir(p), true
}
