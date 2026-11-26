package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type GrammarRange struct {
	oldest      token.Version
	belowLatest token.Version
	chosen      token.Version
	hasChosen   bool
}

func AnyGrammar() GrammarRange {
	return GrammarRange{}
}

func OnlyGrammar(version token.Version) GrammarRange {
	return GrammarsBetween(version, version)
}

func GrammarsBetween(a, b token.Version) GrammarRange {
	return GrammarRange{oldest: min(a, b), belowLatest: token.Latest - max(a, b)}
}

func (r GrammarRange) Oldest() token.Version {
	return r.oldest
}

func (r GrammarRange) Newest() token.Version {
	return token.Latest - r.belowLatest
}

func (r GrammarRange) Admits(version token.Version) bool {
	return r.oldest <= version && version <= r.Newest()
}

func (r GrammarRange) After(chosen token.Version) GrammarRange {
	if !r.Admits(chosen) {
		return GrammarRange{oldest: r.oldest, belowLatest: r.belowLatest}
	}
	r.chosen, r.hasChosen = chosen, true
	return r
}

func (r GrammarRange) String() string {
	if r.oldest == r.Newest() {
		return r.oldest.String()
	}
	return r.oldest.String() + "-" + r.Newest().String()
}

func (r GrammarRange) Parse(src string) *ast.File {
	return r.pick(src, Parse)
}

func (r GrammarRange) ParseCode(src string) *ast.File {
	return r.pick(src, ParseCode)
}

func (r GrammarRange) pick(src string, parse func(string, token.Version) *ast.File) *ast.File {
	if r.hasChosen {
		return r.keepChosen(src, parse)
	}
	newest := parse(src, r.Newest())
	if newest.Clean() {
		return newest
	}
	for version := r.Newest(); version > r.oldest; {
		version--
		if tree := parse(src, version); tree.Clean() {
			return tree
		}
	}
	return newest
}

func (r GrammarRange) keepChosen(src string, parse func(string, token.Version) *ast.File) *ast.File {
	tree := parse(src, r.chosen)
	if tree.Clean() || r.chosen == r.Newest() {
		return tree
	}
	if newest := parse(src, r.Newest()); newest.Clean() {
		return newest
	}
	return tree
}
