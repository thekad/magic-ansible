// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	gotpl "text/template"
	"time"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
)

func funcMap() gotpl.FuncMap {
	return gotpl.FuncMap{
		"eq":             eqFunc,
		"gt":             gtFunc,
		"gte":            gteFunc,
		"indent":         indentFunc,
		"len":            lenFunc,
		"lines":          splitLinesFunc,
		"lt":             ltFunc,
		"lte":            lteFunc,
		"ne":             neFunc,
		"now":            time.Now,
		"sortProperties": sortPropertiesFunc,
		"split":          strings.Split,
		"streq":          strings.EqualFold,
		"trim":           strings.Trim,
		"trimSpace":      strings.TrimSpace,
		"underscore":     google.Underscore,
	}
}

func lenFunc(v interface{}) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len()
	default:
		return 0
	}
}

func splitLinesFunc(s string) []string {
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
}

func sortPropertiesFunc(props []*api.Type) []*api.Type {
	slices.SortFunc(props, func(a, b *api.Type) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return props
}

// Comparison functions for templates
func gtFunc(a, b interface{}) bool {
	return compareValues(b, a) > 0
}

func ltFunc(a, b interface{}) bool {
	return compareValues(b, a) < 0
}

func gteFunc(a, b interface{}) bool {
	return compareValues(b, a) >= 0
}

func lteFunc(a, b interface{}) bool {
	return compareValues(b, a) <= 0
}

func eqFunc(a, b interface{}) bool {
	return compareValues(b, a) == 0
}

func neFunc(a, b interface{}) bool {
	return compareValues(b, a) != 0
}

func compareValues(a, b interface{}) int {
	// Convert to int64 for comparison
	aVal := toInt64(a)
	bVal := toInt64(b)

	if aVal < bVal {
		return -1
	} else if aVal > bVal {
		return 1
	}
	return 0
}

func toInt64(v interface{}) int64 {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return int64(rv.Float())
	default:
		return 0
	}
}

// indentFunc indents each line of the given text by the specified number of spaces
// Usage in templates: {{ "some text" | indent 4 }} or {{ .SomeMultilineText | indent 8 }}
func indentFunc(spaces int, first bool, text string) string {
	if spaces < 0 {
		spaces = 0
	}

	if text == "" {
		return text
	}

	// Create the indentation string
	indentation := strings.Repeat(" ", spaces)

	// Split text into lines
	lines := strings.Split(text, "\n")

	skip := !first
	// Indent each line
	indentedLines := make([]string, len(lines))
	for i, line := range lines {
		if skip {
			indentedLines[i] = line
			skip = false
			continue
		}
		indentedLines[i] = indentation + line
	}

	// Join lines back together
	return strings.Join(indentedLines, "\n")
}
