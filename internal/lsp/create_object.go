package lsp

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// validObjectKinds lists the PHP object kinds supported by the create command.
var validObjectKinds = map[string]bool{
	"class":     true,
	"trait":     true,
	"interface": true,
	"enum":      true,
}

// buildCreateObjectEdit computes a WorkspaceEdit that creates a new PHP file
// containing a skeleton class/trait/interface/enum declaration. Returns nil
// when kind is invalid or name is empty.
//
// dirURI is the directory URI in which the file should be created.
// kind must be one of "class", "trait", "interface", "enum".
// name is the PHP type name (without extension).
// autoload is the PSR-4 mapping returned by composer.GetAutoloadPaths.
func buildCreateObjectEdit(dirURI, kind, name string, autoload []composer.AutoloadEntry) *protocol.WorkspaceEdit {
	if name == "" || !validObjectKinds[kind] {
		return nil
	}

	// Build the new file URI: join dirURI and name+".php", avoiding double slash.
	base := strings.TrimRight(dirURI, "/")
	newFileURI := base + "/" + name + ".php"

	// Derive the filesystem path from the URI to compute the namespace.
	filePath := symbols.URIToPath(newFileURI)

	// PathToNamespace returns the namespace WITHOUT the class name portion.
	ns := composer.PathToNamespace(filePath, autoload)

	// Render the PHP skeleton.
	skeleton := renderSkeleton(ns, kind, name)

	// Assemble the WorkspaceEdit using DocumentChanges (not Changes).
	ignoreIfExists := true
	edit := &protocol.WorkspaceEdit{
		DocumentChanges: []protocol.DocumentChange{
			{
				CreateFile: &protocol.CreateFile{
					Kind: "create",
					URI:  newFileURI,
					Options: &protocol.CreateFileOptions{
						IgnoreIfExists: ignoreIfExists,
					},
				},
			},
			{
				TextDocumentEdit: &protocol.TextDocumentEdit{
					TextDocument: protocol.VersionedTextDocumentIdentifier{
						URI:     newFileURI,
						Version: nil,
					},
					Edits: []protocol.TextEdit{
						{
							Range: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
							NewText: skeleton,
						},
					},
				},
			},
		},
	}

	return edit
}

// renderSkeleton produces the PHP file skeleton string.
func renderSkeleton(ns, kind, name string) string {
	var sb strings.Builder
	sb.WriteString("<?php\n")
	if ns != "" {
		sb.WriteString(fmt.Sprintf("\nnamespace %s;\n", ns))
	}
	sb.WriteString(fmt.Sprintf("\n%s %s\n{\n}\n", kind, name))
	return sb.String()
}
