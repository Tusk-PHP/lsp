package lsp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var uriFields = map[string]bool{"uri": true, "rootUri": true, "newUri": true, "oldUri": true}

type respelledURI struct {
	client, canonical string
}

func canonicalizeURIs(v reflect.Value, out []respelledURI) []respelledURI {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			out = canonicalizeURIs(v.Elem(), out)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			out = canonicalizeURIs(v.Index(i), out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			value := v.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if value.Kind() == reflect.String && uriFields[name] {
				if canonical := symbols.CanonicalURI(value.String()); canonical != value.String() {
					out = append(out, respelledURI{client: value.String(), canonical: canonical})
					value.SetString(canonical)
				}
				continue
			}
			out = canonicalizeURIs(value, out)
		}
	}
	return out
}

type clientURIs struct {
	mu       sync.RWMutex
	spelling map[string][]byte
}

func (c *clientURIs) remember(uris []respelledURI) {
	if len(uris) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.spelling == nil {
		c.spelling = make(map[string][]byte)
	}
	for _, uri := range uris {
		c.spelling[uri.canonical] = quoted(uri.client)
	}
}

func (c *clientURIs) forget(canonical string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.spelling, canonical)
}

func (c *clientURIs) respell(body []byte) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for canonical, client := range c.spelling {
		if from := quoted(canonical); bytes.Contains(body, from) {
			body = bytes.ReplaceAll(body, from, client)
		}
	}
	return body
}

func quoted(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}
