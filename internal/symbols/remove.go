package symbols

func (idx *Index) RemoveFile(uri string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeFileSymbols(uri)
	delete(idx.templateFiles, uri)
}

func (idx *Index) HasFile(uri string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if _, ok := idx.fileSymbols[uri]; ok {
		return true
	}
	_, ok := idx.templateFiles[uri]
	return ok
}
