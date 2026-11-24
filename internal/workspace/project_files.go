package workspace

import "sync"

type projectFiles struct {
	mu       sync.Mutex
	limit    int
	walked   bool
	admitted map[string]struct{}
}

func newProjectFiles(limit int) *projectFiles {
	return &projectFiles{limit: limit, admitted: map[string]struct{}{}}
}

func (p *projectFiles) begin() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.walked {
		p.walked = false
		p.admitted = map[string]struct{}{}
	}
}

func (p *projectFiles) admit(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.admitLocked(path)
}

func (p *projectFiles) admitLoaded(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.walked && p.admitLocked(path)
}

func (p *projectFiles) admitLocked(path string) bool {
	if _, ok := p.admitted[path]; ok {
		return true
	}
	if p.limit > 0 && len(p.admitted) >= p.limit {
		return false
	}
	p.admitted[path] = struct{}{}
	return true
}

func (p *projectFiles) finish() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := len(p.admitted)
	p.walked = true
	p.admitted = nil
	return count
}
