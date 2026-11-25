package document

import (
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
}

type Store struct {
	mu       sync.RWMutex
	docs     map[string]Snapshot
	revision uint64
}

func NewStore() *Store {
	return &Store{docs: make(map[string]Snapshot)}
}

func (s *Store) Open(uri string, version int, text string) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(Snapshot{URI: uri, Version: version, Text: text})
}

func (s *Store) Change(uri string, version int, enc Encoding, changes []Change) (string, error) {
	current, ok := s.Get(uri)
	if !ok {
		return "", fmt.Errorf("change to %s, which is not open", uri)
	}
	text, err := Apply(current.Text, enc, changes)
	if err != nil {
		return "", fmt.Errorf("change to %s, version %d: %w", uri, version, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if latest, ok := s.docs[uri]; !ok || latest.revision != current.revision {
		return "", fmt.Errorf("change to %s, version %d: the document changed while the edit was applied", uri, version)
	}
	s.putLocked(Snapshot{URI: uri, Version: version, Text: text})
	return text, nil
}

func (s *Store) Save(uri, text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, open := s.docs[uri]
	if !open {
		return false
	}
	if doc.Text != text {
		doc.Text = text
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

func (s *Store) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
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
