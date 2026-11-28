package document

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

type Snapshot struct {
	URI      string
	Version  int
	Text     string
	revision uint64
	diverged bool
}

func (d Snapshot) Diverged() bool {
	return d.diverged
}

var ErrDiverged = errors.New("the document's text diverged from the editor's; waiting for its full text")

type Store struct {
	mu       sync.RWMutex
	docs     map[string]Snapshot
	files    map[string]openFile
	uris     map[string]string
	revision uint64
	pathOf   func(uri string) string
}

type openFile struct {
	path    string
	unsaved bool
}

func NewStore(pathOf func(uri string) string) *Store {
	return &Store{docs: make(map[string]Snapshot), files: make(map[string]openFile), uris: make(map[string]string), pathOf: pathOf}
}

func (s *Store) Open(uri string, version int, text string, unsaved bool) Snapshot {
	path := s.pathOf(uri)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[uri] = openFile{path: path, unsaved: unsaved}
	s.uris[path] = uri
	return s.putLocked(Snapshot{URI: uri, Version: version, Text: text})
}

func (s *Store) Change(uri string, version int, enc Encoding, changes []Change) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.docs[uri]
	if !ok {
		return Snapshot{}, fmt.Errorf("change to %s, which is not open", uri)
	}
	if current.diverged && lastFullText(changes) < 0 {
		return Snapshot{}, fmt.Errorf("change to %s, version %d: %w", uri, version, ErrDiverged)
	}
	text, err := Apply(current.Text, enc, changes)
	if err != nil {
		current.diverged = true
		s.putLocked(current)
		return Snapshot{}, fmt.Errorf("change to %s, version %d: %w", uri, version, err)
	}
	file := s.files[uri]
	file.unsaved = true
	s.files[uri] = file
	return s.putLocked(Snapshot{URI: uri, Version: version, Text: text}), nil
}

func (s *Store) ShowDiverged(uri, text string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, open := s.docs[uri]
	if !open || !doc.diverged {
		return Snapshot{}, false
	}
	doc.Text = text
	return s.putLocked(doc), true
}

func (s *Store) MarkSaved(uri string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.markSavedLocked(uri)
}

func (s *Store) Save(uri, text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.markSavedLocked(uri) {
		return false
	}
	doc := s.docs[uri]
	if doc.Text != text || doc.diverged {
		doc.Text = text
		doc.diverged = false
		s.putLocked(doc)
	}
	return true
}

func (s *Store) Current(doc Snapshot) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	latest, open := s.docs[doc.URI]
	return open && latest.revision == doc.revision
}

func (s *Store) markSavedLocked(uri string) bool {
	file, open := s.files[uri]
	if !open {
		return false
	}
	file.unsaved = false
	s.files[uri] = file
	return true
}

func (s *Store) Close(uri string) (unsaved bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.files[uri]
	if s.uris[file.path] == uri {
		delete(s.uris, file.path)
	}
	delete(s.files, uri)
	delete(s.docs, uri)
	return file.unsaved
}

func (s *Store) URIAt(path string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uri, open := s.uris[path]
	return uri, open
}

func (s *Store) Get(uri string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	return doc, ok
}

func (s *Store) Text(uri string) (string, bool) {
	doc, ok := s.Get(uri)
	return doc.Text, ok
}

func (s *Store) All() []Snapshot {
	s.mu.RLock()
	all := make([]Snapshot, 0, len(s.docs))
	for _, doc := range s.docs {
		all = append(all, doc)
	}
	s.mu.RUnlock()
	slices.SortFunc(all, func(a, b Snapshot) int { return strings.Compare(a.URI, b.URI) })
	return all
}

func (s *Store) putLocked(doc Snapshot) Snapshot {
	s.revision++
	doc.revision = s.revision
	s.docs[doc.URI] = doc
	return doc
}
