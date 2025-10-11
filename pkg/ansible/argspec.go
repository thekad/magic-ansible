// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"sort"
	"strings"
)

// ArgumentSpec represents the argument specification for an Ansible module
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_program_flow_modules.html#argument-spec
type ArgumentSpec struct {
	// Arguments is the main argument specification dictionary
	Arguments map[string]*Option

	// Dependencies is the top-level dependency specification
	Dependencies *Dependency
}

// NewArgSpecFromOptions creates an ArgumentSpec from a map of Option structs
// This constructor converts Ansible module options to argument specification format
// suitable for Python argument spec generation
func NewArgSpecFromOptions(options map[string]*Option, topLevelDependency *Dependency) *ArgumentSpec {
	argSpec := &ArgumentSpec{
		Arguments: make(map[string]*Option),
	}

	argSpec.Dependencies = topLevelDependency

	if options == nil {
		return argSpec
	}

	// Add each option directly to the ArgumentSpec
	for name, option := range options {
		if option.OutputOnly() {
			continue // Skip output-only options
		}
		argSpec.Arguments[name] = option
	}

	return argSpec
}

// ToString generates the Python argument specification code for the ArgumentSpec
// This method outputs proper Python dict() constructor syntax that can be used directly
// in Ansible module code as the argument_spec parameter for AnsibleModule()
func (as *ArgumentSpec) ToString() string {
	if as == nil || len(as.Arguments) == 0 {
		return "argument_spec = dict()"
	}

	var builder strings.Builder
	builder.WriteString("argument_spec=dict(\n")

	// Sort argument names with priority ordering for readability
	argNames := make([]string, 0, len(as.Arguments))
	for name := range as.Arguments {
		argNames = append(argNames, name)
	}

	// Custom sort: name first, then state, then alphabetical
	sort.Slice(argNames, func(i, j int) bool {
		a, b := argNames[i], argNames[j]

		// name always comes first
		if a == "name" {
			return true
		}
		if b == "name" {
			return false
		}

		// state comes second (after name)
		if a == "state" {
			return true
		}
		if b == "state" {
			return false
		}

		// Everything else in alphabetical order
		return a < b
	})

	// Generate argument specifications
	for i, argName := range argNames {
		option := as.Arguments[argName]
		builder.WriteString(fmt.Sprintf("    %s=dict(\n", pythonIdentifier(argName)))

		// Add type
		builder.WriteString(fmt.Sprintf("        type=%s,\n", pythonQuote(option.Type.String())))

		// Add required
		if option.Required {
			builder.WriteString("        required=True,\n")
		}

		// Add default
		if option.Default != nil {
			builder.WriteString(fmt.Sprintf("        default=%s,\n", pythonValue(option.Default)))
		}

		// Add choices
		if len(option.Choices) > 0 {
			builder.WriteString(fmt.Sprintf("        choices=%s,\n", pythonList(option.Choices)))
		}

		// Add elements for list types
		if option.Elements != "" {
			builder.WriteString(fmt.Sprintf("        elements=%s,\n", pythonQuote(option.Elements.String())))
		}

		// Add no_log
		if option.NoLog {
			builder.WriteString("        no_log=True,\n")
		}

		// Add nested options
		if len(option.Suboptions) > 0 {
			builder.WriteString("        options=dict(\n")
			as.writeNestedOptions(&builder, option.Suboptions, "            ")
			builder.WriteString("        ),\n")
		}

		// Add dependency constraints for this argument
		as.writeArgumentConstraints(&builder, option, "        ")

		builder.WriteString("    )")

		// Add comma if not the last argument
		if i < len(argNames)-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}

	builder.WriteString(")")

	// Add module-level constraints if any
	moduleConstraints := as.buildModuleConstraints()
	if moduleConstraints != "" {
		builder.WriteString(",\n")
		builder.WriteString(moduleConstraints)
	}

	return builder.String()
}

// writeNestedOptions recursively writes nested argument options using dict() constructor
func (as *ArgumentSpec) writeNestedOptions(builder *strings.Builder, options map[string]*Option, indent string) {
	// Sort option names for consistent output
	optionNames := make([]string, 0, len(options))
	for name := range options {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)

	for i, optionName := range optionNames {
		option := options[optionName]
		builder.WriteString(fmt.Sprintf("%s%s=dict(\n", indent, pythonIdentifier(optionName)))

		// Add type
		if option.Type != "" {
			builder.WriteString(fmt.Sprintf("%s    type=%s,\n", indent, pythonQuote(option.Type.String())))
		}

		// Add required
		if option.Required && option.Default == nil {
			builder.WriteString(fmt.Sprintf("%s    required=True,\n", indent))
		}

		// Add default
		if option.Default != nil {
			builder.WriteString(fmt.Sprintf("%s    default=%s,\n", indent, pythonValue(option.Default)))
		}

		// Add choices
		if len(option.Choices) > 0 {
			builder.WriteString(fmt.Sprintf("%s    choices=%s,\n", indent, pythonList(option.Choices)))
		}

		// Add elements for list types
		if option.Elements != "" {
			builder.WriteString(fmt.Sprintf("%s    elements=%s,\n", indent, pythonQuote(option.Elements.String())))
		}

		// Add no_log
		if option.NoLog {
			builder.WriteString(fmt.Sprintf("%s    no_log=True,\n", indent))
		}

		// Add nested options recursively
		if len(option.Suboptions) > 0 {
			builder.WriteString(fmt.Sprintf("%s    options=dict(\n", indent))
			as.writeNestedOptions(builder, option.Suboptions, indent+"        ")
			builder.WriteString(fmt.Sprintf("%s    ),\n", indent))
		}

		// Add dependency constraints for this nested option
		as.writeArgumentConstraints(builder, option, indent+"    ")

		builder.WriteString(fmt.Sprintf("%s)", indent))

		// Add comma if not the last option
		if i < len(optionNames)-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}
}

// writeArgumentConstraints writes dependency constraints for an argument using dict() constructor syntax
func (as *ArgumentSpec) writeArgumentConstraints(builder *strings.Builder, option *Option, indent string) {
	if option.Dependency != nil {
		if len(option.Dependency.MutuallyExclusive) > 0 {
			builder.WriteString(fmt.Sprintf("%smutually_exclusive=%s,\n", indent, pythonListOfLists(option.Dependency.MutuallyExclusive)))
		}
		if len(option.Dependency.RequiredTogether) > 0 {
			builder.WriteString(fmt.Sprintf("%srequired_together=%s,\n", indent, pythonListOfLists(option.Dependency.RequiredTogether)))
		}
	}
}

// buildModuleConstraints builds module-level constraint parameters
func (as *ArgumentSpec) buildModuleConstraints() string {
	var constraints []string

	if as.Dependencies == nil {
		return ""
	}
	if len(as.Dependencies.MutuallyExclusive) > 0 {
		constraints = append(constraints, fmt.Sprintf("mutually_exclusive=%s", pythonListOfLists(as.Dependencies.MutuallyExclusive)))
	}
	if len(as.Dependencies.RequiredTogether) > 0 {
		constraints = append(constraints, fmt.Sprintf("required_together=%s", pythonListOfLists(as.Dependencies.RequiredTogether)))
	}
	if len(constraints) == 0 {
		return ""
	}

	return strings.Join(constraints, ",\n")
}

// Python formatting helper functions

// pythonIdentifier formats a string as a Python identifier for use in dict() constructor
// If the string is not a valid Python identifier, it falls back to quoted string
func pythonIdentifier(s string) string {
	// Check if it's a valid Python identifier (simplified check)
	// Valid identifiers start with letter/underscore and contain only letters/digits/underscores
	if len(s) == 0 {
		return pythonQuote(s)
	}

	// Check first character
	first := s[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return pythonQuote(s)
	}

	// Check remaining characters
	for i := 1; i < len(s); i++ {
		char := s[i]
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_') {
			return pythonQuote(s)
		}
	}

	// Check if it's a Python keyword (basic list)
	keywords := []string{
		"False", "None", "True", "and", "as", "assert", "break", "class", "continue",
		"def", "del", "elif", "else", "except", "finally", "for", "from", "global",
		"if", "import", "in", "is", "lambda", "nonlocal", "not", "or", "pass",
		"raise", "return", "try", "while", "with", "yield",
	}

	for _, keyword := range keywords {
		if s == keyword {
			return pythonQuote(s)
		}
	}

	return s
}

// pythonQuote adds single quotes around a string for Python
func pythonQuote(s string) string {
	// Escape single quotes in the string
	escaped := strings.ReplaceAll(s, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

// pythonValue converts a Go value to its Python representation
func pythonValue(value interface{}) string {
	if value == nil {
		return "None"
	}

	switch v := value.(type) {
	case string:
		return pythonQuote(v)
	case bool:
		if v {
			return "True"
		}
		return "False"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	default:
		return pythonQuote(fmt.Sprintf("%v", v))
	}
}

// pythonList converts a slice of strings to a Python list
func pythonList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}

	var quoted []string
	for _, item := range items {
		quoted = append(quoted, pythonQuote(item))
	}
	return fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
}

// pythonListOfLists converts a slice of string slices to a Python list of lists
func pythonListOfLists(items [][]string) string {
	if len(items) == 0 {
		return "[]"
	}

	var lists []string
	for _, subList := range items {
		lists = append(lists, pythonList(subList))
	}
	return fmt.Sprintf("[%s]", strings.Join(lists, ", "))
}
