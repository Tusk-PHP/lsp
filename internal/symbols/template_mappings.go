package symbols

type TemplateMapping struct {
	Params  []string
	Methods map[string]string
}

func (idx *Index) SetTemplateMappings(mappings map[string]TemplateMapping) {
	idx.mu.Lock()
	idx.templateMappings = mappings
	idx.mu.Unlock()
}

func (idx *Index) TemplateMappingOf(fqn string) (TemplateMapping, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	m, ok := idx.templateMappings[fqn]
	return m, ok
}
