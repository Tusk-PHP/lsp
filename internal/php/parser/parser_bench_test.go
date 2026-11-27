package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func BenchmarkParse(b *testing.B) {
	sources := phpref.ReadAll(b, phpref.Corpus(b))
	size := 0
	for _, src := range sources {
		size += len(src)
	}
	b.Run("php/parser", func(b *testing.B) {
		b.SetBytes(int64(size))
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				Parse(src, token.Latest)
			}
		}
	})
}

func BenchmarkGrammarRangeParse(b *testing.B) {
	type input struct {
		src    string
		chosen token.Version
	}
	var clean, typing []input
	for _, src := range phpref.ReadAll(b, phpref.Corpus(b)) {
		if len(Parse(src, token.Latest).Errors) == 0 {
			clean = append(clean, input{src: src})
		}
		if half := src[:len(src)/2]; len(Parse(half, token.Latest).Errors) > 0 {
			typing = append(typing, input{src: half, chosen: AnyGrammar().Parse(src).Tree.Version})
		}
	}
	cases := []struct {
		name     string
		inputs   []input
		grammars func(input) GrammarRange
	}{
		{"clean/8.6", clean, func(input) GrammarRange { return OnlyGrammar(token.Latest) }},
		{"clean/7.4-8.6", clean, func(input) GrammarRange { return AnyGrammar() }},
		{"cut in half/8.6", typing, func(input) GrammarRange { return OnlyGrammar(token.Latest) }},
		{"cut in half/7.4-8.6, first parse", typing, func(input) GrammarRange { return AnyGrammar() }},
		{"cut in half/7.4-8.6, after the whole file", typing, func(in input) GrammarRange { return AnyGrammar().After(in.chosen) }},
	}
	for _, c := range cases {
		size := 0
		grammars := make([]GrammarRange, len(c.inputs))
		for i, in := range c.inputs {
			size += len(in.src)
			grammars[i] = c.grammars(in)
		}
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				for i, in := range c.inputs {
					grammars[i].Parse(in.src)
				}
			}
		})
	}
}
