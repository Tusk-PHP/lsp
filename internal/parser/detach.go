package parser

import (
	"reflect"
	"strings"
	"sync"
)

func (f *FileNode) Detach() {
	var fields []reflect.Value
	size := 0
	collectStrings(reflect.ValueOf(f).Elem(), map[uintptr]bool{}, func(s reflect.Value) {
		fields = append(fields, s)
		size += s.Len()
	})
	var arena strings.Builder
	arena.Grow(size)
	for _, s := range fields {
		arena.WriteString(s.String())
	}
	copied := arena.String()
	for _, s := range fields {
		n := s.Len()
		s.SetString(copied[:n])
		copied = copied[n:]
	}
}

var (
	stringFields sync.Map
	stringTypes  sync.Map
)

func typeHoldsStrings(t reflect.Type) bool {
	if cached, ok := stringTypes.Load(t); ok {
		return cached.(bool)
	}
	holds := holdsStrings(t, map[reflect.Type]bool{})
	stringTypes.Store(t, holds)
	return holds
}

func holdsStrings(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if visiting[t] {
		return false
	}
	visiting[t] = true
	switch t.Kind() {
	case reflect.String:
		return true
	case reflect.Struct:
		for i := range t.NumField() {
			if holdsStrings(t.Field(i).Type, visiting) {
				return true
			}
		}
	case reflect.Slice, reflect.Array, reflect.Pointer:
		return holdsStrings(t.Elem(), visiting)
	}
	return false
}

func fieldsWithStrings(t reflect.Type) []int {
	if cached, ok := stringFields.Load(t); ok {
		return cached.([]int)
	}
	var fields []int
	for i := range t.NumField() {
		if typeHoldsStrings(t.Field(i).Type) {
			fields = append(fields, i)
		}
	}
	stringFields.Store(t, fields)
	return fields
}

func collectStrings(v reflect.Value, seen map[uintptr]bool, visit func(reflect.Value)) {
	switch v.Kind() {
	case reflect.String:
		if v.Len() > 0 {
			visit(v)
		}
	case reflect.Struct:
		for _, i := range fieldsWithStrings(v.Type()) {
			collectStrings(v.Field(i), seen, visit)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			collectStrings(v.Index(i), seen, visit)
		}
	case reflect.Pointer:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		collectStrings(v.Elem(), seen, visit)
	}
}
