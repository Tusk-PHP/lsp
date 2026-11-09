package parser

import (
	"hash/fnv"
	"strconv"
	"strings"
)

// AnonymousClassName is the name PHP gives every anonymous class, and the Name
// of each one the parser reads.
const AnonymousClassName = "class@anonymous"

// IsAnonymousClass reports whether fqn names an anonymous class read by the
// parser.
func IsAnonymousClass(fqn string) bool {
	return strings.Contains(fqn, AnonymousClassName+"@")
}

// AnonymousClassAt returns the innermost anonymous class (`new class { ... }`)
// whose body holds the position, or nil. Inside it, `$this`, `self` and
// `static` mean it, not the class around it.
//
// The body runs from just after its `{` to the line before its `}`, so code on
// the closing line, as in `}, $this->x);`, and before the `{` on the opening
// line, as in `$this->bind(new class {`, stays with the class around it. A
// body written on one line is never matched.
func (f *FileNode) AnonymousClassAt(line, character int) *ClassNode {
	if f == nil {
		return nil
	}
	var best *ClassNode
	for i := range f.AnonymousClasses {
		c := &f.AnonymousClasses[i]
		afterOpen := line > c.BodyLine || (line == c.BodyLine && character > c.BodyCol)
		if afterOpen && line < c.EndLine && (best == nil || c.BodyLine > best.BodyLine || (c.BodyLine == best.BodyLine && c.BodyCol > best.BodyCol)) {
			best = c
		}
	}
	return best
}

// parseAnonymousClasses reads each `new class (...) extends ... implements ...
// { ... }` in the file, including those in function bodies and inside other
// anonymous classes. Their FullName is unique to the file and line, so they can
// be indexed without colliding, but no one can write it: they're never offered
// by name.
func (p *structParser) parseAnonymousClasses() {
	var fileHash string
	for i := 0; i < len(p.tokens); i++ {
		if p.tokens[i].Kind != TokenNew {
			continue
		}
		j := p.nextCode(i + 1)
		isReadonly := false
		if j < len(p.tokens) && p.tokens[j].Kind == TokenReadonly {
			isReadonly = true
			j = p.nextCode(j + 1)
		}
		if j >= len(p.tokens) || p.tokens[j].Kind != TokenClass {
			continue
		}
		if fileHash == "" {
			h := fnv.New32a()
			for _, line := range p.result.Lines {
				h.Write([]byte(line))
				h.Write([]byte{'\n'})
			}
			fileHash = strconv.FormatUint(uint64(h.Sum32()), 36)
		}
		cls := ClassDef{Name: AnonymousClassName, Line: p.tokens[j].Line, IsReadonly: isReadonly}
		name := AnonymousClassName + "@" + strconv.Itoa(cls.Line+1) + "@" + fileHash
		if p.result.Namespace != "" {
			cls.FullName = p.result.Namespace + "\\" + name
		} else {
			cls.FullName = name
		}
		p.pos = j + 1
		p.attributes = nil
		if p.peek().Kind == TokenOpenParen {
			p.skipParens()
		}
		if p.peek().Kind == TokenExtends {
			p.advance()
			cls.Extends = p.readTypeName()
		}
		if p.peek().Kind == TokenImplements {
			p.advance()
			for {
				name := p.readTypeName()
				if name == "" {
					break
				}
				cls.Implements = append(cls.Implements, name)
				if p.peek().Kind != TokenComma {
					break
				}
				p.advance()
			}
		}
		open, ok := p.expect(TokenOpenBrace)
		if !ok {
			continue
		}
		cls.BodyLine, cls.BodyCol = open.Line, open.Column
		cls.Methods, cls.Properties, cls.Constants, cls.Traits, cls.TraitUseDoc = p.parseClassBodyWithTraitDocs()
		if cls.IsReadonly {
			for k := range cls.Properties {
				cls.Properties[k].IsReadonly = true
			}
		}
		// The closing brace's line: what follows may be far below.
		if p.pos > 0 {
			cls.EndLine = p.tokens[p.pos-1].Line
		}
		p.result.AnonymousClasses = append(p.result.AnonymousClasses, cls)
	}
}

// nextCode returns the index of the first token at or after i that isn't a
// comment.
func (p *structParser) nextCode(i int) int {
	for i < len(p.tokens) && (p.tokens[i].Kind == TokenComment || p.tokens[i].Kind == TokenDocComment) {
		i++
	}
	return i
}

// skipParens skips a balanced parenthesised group starting at the current `(`.
func (p *structParser) skipParens() {
	depth := 0
	for p.peek().Kind != TokenEOF {
		switch p.advance().Kind {
		case TokenOpenParen:
			depth++
		case TokenCloseParen:
			depth--
			if depth == 0 {
				return
			}
		}
	}
}
