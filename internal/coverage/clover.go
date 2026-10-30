// Package coverage reads test coverage reports written by PHP test runners
// (PHPUnit, Pest) so other providers can show them. It never runs tests.
package coverage

import (
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strconv"
)

// ErrNotClover means the file parsed as XML but isn't a Clover report, such as
// a Cobertura report, which shares the <coverage> root element.
var ErrNotClover = errors.New("not a Clover coverage report")

// Method is the coverage of one method or function.
type Method struct {
	Name       string
	Line       int // 1-based line of the declaration, as written in the report
	EndLine    int // 1-based last executable line assigned to it, or Line if it has none
	Hits       int
	Complexity int
	CRAP       float64
	Executable int
	Covered    int
	Uncovered  []int // 1-based executable lines no test reached
}

// File is the coverage of one source file.
type File struct {
	Path      string // as written in the report; may be a path inside a container
	Methods   []Method
	Lines     map[int]int // 1-based executable line → hit count
	Uncovered []int       // sorted 1-based executable lines no test reached
}

// Covered returns how many executable lines were reached.
func (f *File) Covered() int {
	return len(f.Lines) - len(f.Uncovered)
}

// Report is a parsed coverage report.
type Report struct {
	Files map[string]*File // keyed by slash-separated path
}

// ParseClover reads a Clover XML report, as written by --coverage-clover.
func ParseClover(r io.Reader) (*Report, error) {
	report := &Report{Files: map[string]*File{}}
	dec := xml.NewDecoder(r)
	sawProject := false
	var current *File

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "project":
				sawProject = true
			case "file":
				current = nil
				if name := attr(el, "name"); name != "" {
					current = &File{Path: name, Lines: map[int]int{}}
				}
			case "line":
				if current != nil {
					addLine(current, el)
				}
			}
		case xml.EndElement:
			if el.Name.Local == "file" && current != nil {
				finishFile(current)
				report.Files[filepath.ToSlash(current.Path)] = current
				current = nil
			}
		}
	}

	if !sawProject {
		return nil, ErrNotClover
	}
	return report, nil
}

func addLine(f *File, el xml.StartElement) {
	num, err := strconv.Atoi(attr(el, "num"))
	if err != nil {
		return
	}
	count, _ := strconv.Atoi(attr(el, "count"))

	if attr(el, "type") == "method" {
		complexity, _ := strconv.Atoi(attr(el, "complexity"))
		crap, _ := strconv.ParseFloat(attr(el, "crap"), 64)
		f.Methods = append(f.Methods, Method{
			Name:       attr(el, "name"),
			Line:       num,
			EndLine:    num,
			Hits:       count,
			Complexity: complexity,
			CRAP:       crap,
		})
		return
	}
	f.Lines[num] = count
}

// finishFile assigns each executable line to the method declared closest above
// it, since Clover lists a method's lines after it without marking where it ends.
func finishFile(f *File) {
	sort.SliceStable(f.Methods, func(i, j int) bool { return f.Methods[i].Line < f.Methods[j].Line })

	lines := make([]int, 0, len(f.Lines))
	for line := range f.Lines {
		lines = append(lines, line)
	}
	sort.Ints(lines)

	for _, line := range lines {
		count := f.Lines[line]
		if count == 0 {
			f.Uncovered = append(f.Uncovered, line)
		}
		i := sort.Search(len(f.Methods), func(i int) bool { return f.Methods[i].Line > line }) - 1
		if i < 0 {
			continue
		}
		m := &f.Methods[i]
		m.Executable++
		m.EndLine = line
		if count > 0 {
			m.Covered++
		} else {
			m.Uncovered = append(m.Uncovered, line)
		}
	}
}

func attr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
