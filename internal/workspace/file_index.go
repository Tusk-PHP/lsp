package workspace

import "github.com/Tusk-PHP/lsp/internal/symbols"

// IndexFile indexes uri's new text source and re-analyses what it declares.
// castDependents are the other model files whose attribute cast types were
// read from a class uri declares, before or after the edit: pass them to
// RefreshModels, in the background.
func (ws *Bootstrapped) IndexFile(uri, source string) (castDependents []string) {
	ws.ViewData.IndexFile(uri, source)
	if IsBlade(uri) {
		ws.Index.AddTemplateFile(uri)
		return nil
	}
	if IsIDEHelperFile(symbols.URIToPath(uri)) {
		ws.Index.IndexIDEHelperFile(uri, source)
	} else {
		tracked := ws.castDependents.tracking()
		var before []string
		if tracked {
			before = declaredClasses(ws.Index, uri)
		}
		ws.Index.IndexFile(uri, source)
		ws.UpdateFile(uri, source)
		if tracked {
			castDependents = ws.castDependents.of(append(before, declaredClasses(ws.Index, uri)...), uri)
		}
	}
	if ws.RouteIndex != nil {
		ws.RouteIndex.IndexFile(uri, source)
	}
	return castDependents
}

// RemoveFile drops uri from the index. castDependents are as for IndexFile.
func (ws *Bootstrapped) RemoveFile(uri string) (castDependents []string) {
	if ws.castDependents.tracking() {
		castDependents = ws.castDependents.of(declaredClasses(ws.Index, uri), uri)
	}
	ws.Index.RemoveFile(uri)
	ws.castDependents.set(uri, nil)
	ws.ViewData.IndexFile(uri, "")
	ws.SymfonyCommands.UpdateFile(uri, "")
	if ws.RouteIndex != nil {
		ws.RouteIndex.IndexFile(uri, "")
	}
	return castDependents
}
