package symbols

type builtinFunctionSignature struct {
	Name       string
	Params     []ParamInfo
	ReturnType string
	Docs       builtinDocs
}

type builtinDoc struct {
	Since string
	Doc   string
}

type builtinDocs []builtinDoc

func (docs builtinDocs) docFor(version string) string {
	doc := ""
	for _, d := range docs {
		if d.Since != "" && compareBuiltinVersions(d.Since, version) > 0 {
			break
		}
		doc = d.Doc
	}
	return doc
}
