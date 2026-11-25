package symbols

func (idx *Index) RemoveFile(uri string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeFileSymbols(uri)
	if _, ok := idx.templateFiles[uri]; ok {
		delete(idx.templateFiles, uri)
		idx.files.remove(uri)
	}
}

func (idx *Index) FilesUnder(dir string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.files.under(dir)
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
