package symbols

type TemplateFallback struct {
	Params  []string
	Methods map[string]string
}

func (idx *Index) SetTemplateFallbacks(fallbacks map[string]TemplateFallback) {
	idx.mu.Lock()
	idx.templateFallbacks = fallbacks
	idx.mu.Unlock()
}

func (idx *Index) TemplateFallbackOf(fqn string) (TemplateFallback, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	f, ok := idx.templateFallbacks[fqn]
	return f, ok
}
