package symbols

type builtinFunctionSignature struct {
	Name       string
	Params     []ParamInfo
	ReturnType string
	Docs       []builtinDoc
}

type builtinDoc struct {
	Since string
	Doc   string
}

func (s *builtinFunctionSignature) docFor(version string) string {
	doc := ""
	for _, d := range s.Docs {
		if d.Since != "" && compareBuiltinVersions(d.Since, version) > 0 {
			break
		}
		doc = d.Doc
	}
	return doc
}
