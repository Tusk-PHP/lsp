package document

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

type Snapshot struct {
	URI     string
	Version int
	Text    string
}

type Store struct {
	mu   sync.RWMutex
	docs map[string]Snapshot
}

func NewStore() *Store {
	return &Store{docs: make(map[string]Snapshot)}
}

func (s *Store) Open(uri string, version int, text string) {
	s.put(Snapshot{URI: uri, Version: version, Text: text})
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
	if latest, ok := s.docs[uri]; !ok || latest != current {
		return "", fmt.Errorf("change to %s, version %d: the document changed while the edit was applied", uri, version)
	}
	s.docs[uri] = Snapshot{URI: uri, Version: version, Text: text}
	return text, nil
}

func (s *Store) Save(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := s.docs[uri]
	doc.URI = uri
	doc.Text = text
	s.docs[uri] = doc
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

func (s *Store) put(doc Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[doc.URI] = doc
}
