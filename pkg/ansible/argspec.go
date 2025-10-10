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
	Arguments map[string]*ArgumentOption

	// MutuallyExclusive lists groups of arguments that cannot be used together
	MutuallyExclusive [][]string

	// RequiredTogether lists groups of arguments that must be used together
	RequiredTogether [][]string
}

// ArgumentOption represents a single argument in the argument spec
type ArgumentOption struct {
	// Type specifies the expected data type
	Type string

	// Required indicates if this argument is mandatory
	Required bool

	// Default provides the default value
	Default interface{}

	// Choices lists valid values for the argument
	Choices []string

	// Elements specifies the type of list elements (when type="list")
	Elements string

	// Options defines nested arguments (when type="dict")
	Options map[string]*ArgumentOption

	// NoLog indicates sensitive data that should not be logged
	NoLog bool

	// MutuallyExclusive for nested options
	MutuallyExclusive [][]string

	// RequiredTogether for nested options
	RequiredTogether [][]string
}

// NewArgSpecFromOptions creates an ArgumentSpec from a map of Option structs
// This constructor converts Ansible module options to argument specification format
// suitable for Python argument spec generation
func NewArgSpecFromOptions(options map[string]*Option, topLevelDepdenncy *Dependencies) *ArgumentSpec {
	argSpec := &ArgumentSpec{
		Arguments: make(map[string]*ArgumentOption),
	}

	if topLevelDepdenncy != nil {
		argSpec.MutuallyExclusive = topLevelDepdenncy.MutuallyExclusive
		argSpec.RequiredTogether = topLevelDepdenncy.RequiredTogether
	}

	if options == nil {
		return argSpec
	}

	// Convert each option to an ArgumentOption
	for name, option := range options {
		if option.OutputOnly() {
			continue // Skip output-only options
		}
		argSpec.Arguments[name] = convertOptionToArgumentOption(option)
	}

	return argSpec
}

// convertOptionToArgumentOption converts a single Option to an ArgumentOption
func convertOptionToArgumentOption(option *Option) *ArgumentOption {
	if option == nil {
		return nil
	}

	argOption := &ArgumentOption{
		Type:     option.Type.String(),
		Required: option.Required,
		Default:  option.Default,
		Choices:  option.Choices,
		NoLog:    option.NoLog,
	}

	if option.Dependencies != nil {
		argOption.MutuallyExclusive = option.Dependencies.MutuallyExclusive
		argOption.RequiredTogether = option.Dependencies.RequiredTogether
	}

	// Handle list element types
	if option.Elements != "" {
		argOption.Elements = option.Elements.String()
	}

	// Handle nested options (suboptions)
	if len(option.Suboptions) > 0 {
		argOption.Options = make(map[string]*ArgumentOption)
		for subName, subOption := range option.Suboptions {
			if !subOption.OutputOnly() {
				argOption.Options[subName] = convertOptionToArgumentOption(subOption)
			}
		}
	}

	return argOption
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
		argOption := as.Arguments[argName]
		builder.WriteString(fmt.Sprintf("    %s=dict(\n", pythonIdentifier(argName)))

		// Add type
		if argOption.Type != "" {
			builder.WriteString(fmt.Sprintf("        type=%s,\n", pythonQuote(argOption.Type)))
		}

		// Add required
		if argOption.Required {
			builder.WriteString("        required=True,\n")
		}

		// Add default
		if argOption.Default != nil {
			builder.WriteString(fmt.Sprintf("        default=%s,\n", pythonValue(argOption.Default)))
		}

		// Add choices
		if len(argOption.Choices) > 0 {
			builder.WriteString(fmt.Sprintf("        choices=%s,\n", pythonList(argOption.Choices)))
		}

		// Add elements for list types
		if argOption.Elements != "" {
			builder.WriteString(fmt.Sprintf("        elements=%s,\n", pythonQuote(argOption.Elements)))
		}

		// Add no_log
		if argOption.NoLog {
			builder.WriteString("        no_log=True,\n")
		}

		// Add nested options
		if len(argOption.Options) > 0 {
			builder.WriteString("        options=dict(\n")
			as.writeNestedOptions(&builder, argOption.Options, "            ")
			builder.WriteString("        ),\n")
		}

		// Add dependency constraints for this argument
		as.writeArgumentConstraints(&builder, argOption, "        ")

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
func (as *ArgumentSpec) writeNestedOptions(builder *strings.Builder, options map[string]*ArgumentOption, indent string) {
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
			builder.WriteString(fmt.Sprintf("%s    type=%s,\n", indent, pythonQuote(option.Type)))
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
			builder.WriteString(fmt.Sprintf("%s    elements=%s,\n", indent, pythonQuote(option.Elements)))
		}

		// Add no_log
		if option.NoLog {
			builder.WriteString(fmt.Sprintf("%s    no_log=True,\n", indent))
		}

		// Add nested options recursively
		if len(option.Options) > 0 {
			builder.WriteString(fmt.Sprintf("%s    options=dict(\n", indent))
			as.writeNestedOptions(builder, option.Options, indent+"        ")
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
func (as *ArgumentSpec) writeArgumentConstraints(builder *strings.Builder, option *ArgumentOption, indent string) {
	if len(option.MutuallyExclusive) > 0 {
		builder.WriteString(fmt.Sprintf("%smutually_exclusive=%s,\n", indent, pythonListOfLists(option.MutuallyExclusive)))
	}
	if len(option.RequiredTogether) > 0 {
		builder.WriteString(fmt.Sprintf("%srequired_together=%s,\n", indent, pythonListOfLists(option.RequiredTogether)))
	}
}

// buildModuleConstraints builds module-level constraint parameters
func (as *ArgumentSpec) buildModuleConstraints() string {
	var constraints []string

	if len(as.MutuallyExclusive) > 0 {
		constraints = append(constraints, fmt.Sprintf("mutually_exclusive=%s", pythonListOfLists(as.MutuallyExclusive)))
	}
	if len(as.RequiredTogether) > 0 {
		constraints = append(constraints, fmt.Sprintf("required_together=%s", pythonListOfLists(as.RequiredTogether)))
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
