package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var jsonUnmarshaler = reflect.TypeFor[json.Unmarshaler]()

func unknownKeys(data []byte, t reflect.Type, prefix string) []error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || reflect.PointerTo(t).Implements(jsonUnmarshaler) {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var problems []error
	for _, key := range keys {
		field, ok := jsonField(t, key)
		if !ok {
			problems = append(problems, fmt.Errorf("unknown setting %q is ignored", prefix+key))
			continue
		}
		problems = append(problems, unknownKeys(object[key], field.Type, prefix+key+".")...)
	}
	return problems
}

func jsonField(t reflect.Type, key string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if strings.EqualFold(name, key) {
			return field, true
		}
	}
	return reflect.StructField{}, false
}
