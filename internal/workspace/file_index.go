package workspace

import "github.com/Tusk-PHP/lsp/internal/symbols"

func (ws *Bootstrapped) IndexFile(uri, source string) {
	ws.ViewData.IndexFile(uri, source)
	if IsBlade(uri) {
		ws.Index.AddTemplateFile(uri)
		return
	}
	if IsIDEHelperFile(symbols.URIToPath(uri)) {
		ws.Index.IndexIDEHelperFile(uri, source)
	} else {
		ws.Index.IndexFile(uri, source)
		ws.UpdateFile(uri, source)
	}
	if ws.RouteIndex != nil {
		ws.RouteIndex.IndexFile(uri, source)
	}
}

func (ws *Bootstrapped) RemoveFile(uri string) {
	ws.Index.RemoveFile(uri)
	ws.ViewData.IndexFile(uri, "")
	ws.SymfonyCommands.UpdateFile(uri, "")
	if ws.RouteIndex != nil {
		ws.RouteIndex.IndexFile(uri, "")
	}
}
