package workspace

import "sync"

type projectFiles struct {
	mu       sync.Mutex
	limit    int
	walked   bool
	admitted map[string]struct{}
	indexed  int
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
	p.indexed = count
	return count
}

func (p *projectFiles) admitCreated(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.walked {
		return p.admitLocked(path)
	}
	if p.limit > 0 && p.indexed >= p.limit {
		return false
	}
	p.indexed++
	return true
}

func (p *projectFiles) release(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.walked {
		delete(p.admitted, path)
		return
	}
	p.indexed--
}

func (p *projectFiles) rescope(limit, indexed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limit = limit
	p.walked = true
	p.admitted = nil
	p.indexed = indexed
}
