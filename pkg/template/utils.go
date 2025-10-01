// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"cmp"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	gotpl "text/template"
	"time"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/thekad/magic-ansible/pkg/ansible"
)

func funcMap() gotpl.FuncMap {
	funcMap := gotpl.FuncMap{
		// Comparison functions
		"eq":    eqFunc,
		"gt":    gtFunc,
		"gte":   gteFunc,
		"isNil": isNilFunc,
		"len":   lenFunc,
		"lt":    ltFunc,
		"lte":   lteFunc,
		"ne":    neFunc,
		// String functions
		"concat":        concatFunc,
		"indent":        indentFunc,
		"lines":         splitLinesFunc,
		"now":           time.Now,
		"singular":      ansible.Singular,
		"streq":         strings.EqualFold,
		"trim":          strings.Trim,
		"trimSpace":     strings.TrimSpace,
		"resource_name": resourceNameFunc, // this is cheating a bit, but it's useful for templates
		"tojson":        tojsonFunc,
		// property functions
		"sortProperties":   sortPropertiesFunc,
		"selectProperties": selectPropertiesFunc,
		// misc functions
		"list":              listFunc, // for passing arguments to template fragments
		"classOrType":       classOrTypeFunc,
		"mmv1TypeToAnsible": ansible.MapMmv1ToAnsible,
		"toJinja":           goTplToJinjaFunc,
	}
	// Copy google template functions
	maps.Copy(funcMap, google.TemplateFunctions)

	return funcMap
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

// listFunc creates a slice from the provided arguments
// Usage in templates: {{ list "arg1" "arg2" "arg3" }} or {{ list $var1 $var2 }}
func listFunc(items ...interface{}) []interface{} {
	return items
}

// concatFunc concatenates the provided arguments into a single string
// Usage in templates: {{ concat "arg1" "arg2" "arg3" }} or {{ concat $var1 $var2 }}
func concatFunc(items ...string) string {
	return strings.Join(items, "")
}

// isNilFunc checks if the value is nil
// Usage in templates: {{ isNil .SomeVariable }}
func isNilFunc(v interface{}) bool {
	return v == nil
}

// selectPropertiesFunc filters properties based on a string predicate
// Usage in templates: {{ selectProperties $properties "not output" }} or {{ selectProperties $properties "output" }}
func selectPropertiesFunc(properties []*api.Type, predicate string) []*api.Type {
	switch strings.ToLower(strings.TrimSpace(predicate)) {
	case "output":
		return google.Select(properties, func(p *api.Type) bool {
			return p.Output
		})
	case "not output", "!output":
		return google.Select(properties, func(p *api.Type) bool {
			return !p.Output
		})
	case "required":
		return google.Select(properties, func(p *api.Type) bool {
			return p.Required
		})
	case "not required", "!required", "optional":
		return google.Select(properties, func(p *api.Type) bool {
			return !p.Required
		})
	default:
		// If predicate is not recognized, return all properties
		return properties
	}
}

// resourceNameFunc returns a constant string "{{ resource_name }}"
func resourceNameFunc(resource *api.Resource) string {
	return "resource_name"
}

// tojsonFunc returns the JSON representation of the given value
// Usage in templates: {{ .SomeVariable | tojson }}
func tojsonFunc(v interface{}) string {
	json, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(json)
}

// classOrTypeFunc returns None if the property is nil, class name if it's a nested object, or a python/ansible type otherwise
// Usage in templates: {{ .Property | classOrType }}
func classOrTypeFunc(property *api.Type) string {
	if property == nil {
		return "None"
	}
	if property.IsA("NestedObject") {
		return google.Camelize(property.Name, "upper")
	}

	return ansible.MapMmv1ToAnsible(property).String()
}

func goTplToJinjaFunc(tpl string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tpl, "{{", "{"), "}}", "}")
}
