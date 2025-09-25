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
	Arguments map[string]*ArgumentOption `json:"arguments"`

	// MutuallyExclusive lists groups of arguments that cannot be used together
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`

	// RequiredTogether lists groups of arguments that must be used together
	RequiredTogether [][]string `json:"required_together,omitempty"`

	// RequiredOneOf lists groups where at least one argument must be specified
	RequiredOneOf [][]string `json:"required_one_of,omitempty"`

	// RequiredIf lists conditional requirements
	RequiredIf [][]interface{} `json:"required_if,omitempty"`

	// RequiredBy maps arguments to other arguments they require
	RequiredBy map[string][]string `json:"required_by,omitempty"`
}

// ArgumentOption represents a single argument in the argument spec
type ArgumentOption struct {
	// Type specifies the expected data type
	Type string `json:"type,omitempty"`

	// Required indicates if this argument is mandatory
	Required bool `json:"required,omitempty"`

	// Default provides the default value
	Default interface{} `json:"default,omitempty"`

	// Choices lists valid values for the argument
	Choices []string `json:"choices,omitempty"`

	// Elements specifies the type of list elements (when type="list")
	Elements string `json:"elements,omitempty"`

	// Options defines nested arguments (when type="dict")
	Options map[string]*ArgumentOption `json:"options,omitempty"`

	// NoLog indicates sensitive data that should not be logged
	NoLog bool `json:"no_log,omitempty"`

	// Aliases provides alternative names for this argument
	Aliases []string `json:"aliases,omitempty"`

	// Deprecated marks this argument as deprecated
	Deprecated map[string]interface{} `json:"deprecated,omitempty"`

	// MutuallyExclusive for nested options
	MutuallyExclusive [][]string `json:"mutually_exclusive,omitempty"`

	// RequiredTogether for nested options
	RequiredTogether [][]string `json:"required_together,omitempty"`

	// RequiredOneOf for nested options
	RequiredOneOf [][]string `json:"required_one_of,omitempty"`

	// RequiredIf for nested options
	RequiredIf [][]interface{} `json:"required_if,omitempty"`

	// RequiredBy for nested options
	RequiredBy map[string][]string `json:"required_by,omitempty"`
}

// NewArgSpecFromOptions creates an ArgumentSpec from existing Ansible options
// This constructor converts the already-parsed Ansible module options to
// argument specification format suitable for Python argument spec generation
func NewArgSpecFromOptions(options map[string]*Option) *ArgumentSpec {
	if options == nil {
		return &ArgumentSpec{
			Arguments: make(map[string]*ArgumentOption),
		}
	}

	argSpec := &ArgumentSpec{
		Arguments: make(map[string]*ArgumentOption),
	}

	// Convert each option to an argument option
	for optionName, option := range options {
		argOption := &ArgumentOption{
			Type:     option.Type.String(),
			Required: option.Required,
			Default:  option.Default,
			Choices:  option.Choices,
		}

		// Convert elements type
		if option.Elements != "" {
			argOption.Elements = option.Elements.String()
		}

		// Convert suboptions recursively
		if len(option.Suboptions) > 0 {
			argOption.Options = convertSuboptionsToArgOptions(option.Suboptions)
		}

		// Copy constraint fields from option to argument option
		if len(option.MutuallyExclusive) > 0 {
			argOption.MutuallyExclusive = option.MutuallyExclusive
		}
		if len(option.RequiredTogether) > 0 {
			argOption.RequiredTogether = option.RequiredTogether
		}
		if len(option.RequiredOneOf) > 0 {
			argOption.RequiredOneOf = option.RequiredOneOf
		}
		if len(option.RequiredIf) > 0 {
			argOption.RequiredIf = option.RequiredIf
		}
		if len(option.RequiredBy) > 0 {
			argOption.RequiredBy = option.RequiredBy
		}

		// Set no_log for sensitive fields (basic heuristic)
		if isSensitiveField(optionName) {
			argOption.NoLog = true
		}

		argSpec.Arguments[optionName] = argOption
	}

	return argSpec
}

// convertSuboptionsToArgOptions recursively converts Ansible suboptions to argument options
func convertSuboptionsToArgOptions(suboptions map[string]*Option) map[string]*ArgumentOption {
	if suboptions == nil {
		return nil
	}

	argOptions := make(map[string]*ArgumentOption)

	for suboptionName, suboption := range suboptions {
		argOption := &ArgumentOption{
			Type:     suboption.Type.String(),
			Required: suboption.Required,
			Default:  suboption.Default,
			Choices:  suboption.Choices,
		}

		// Convert elements type
		if suboption.Elements != "" {
			argOption.Elements = suboption.Elements.String()
		}

		// Convert nested suboptions recursively
		if len(suboption.Suboptions) > 0 {
			argOption.Options = convertSuboptionsToArgOptions(suboption.Suboptions)
		}

		// Copy constraint fields from suboption to argument option
		if len(suboption.MutuallyExclusive) > 0 {
			argOption.MutuallyExclusive = suboption.MutuallyExclusive
		}
		if len(suboption.RequiredTogether) > 0 {
			argOption.RequiredTogether = suboption.RequiredTogether
		}
		if len(suboption.RequiredOneOf) > 0 {
			argOption.RequiredOneOf = suboption.RequiredOneOf
		}
		if len(suboption.RequiredIf) > 0 {
			argOption.RequiredIf = suboption.RequiredIf
		}
		if len(suboption.RequiredBy) > 0 {
			argOption.RequiredBy = suboption.RequiredBy
		}

		// Set no_log for sensitive fields
		if isSensitiveField(suboptionName) {
			argOption.NoLog = true
		}

		argOptions[suboptionName] = argOption
	}

	return argOptions
}

// isSensitiveField determines if a field should be marked as no_log
func isSensitiveField(fieldName string) bool {
	sensitivePatterns := []string{
		"password", "secret", "key", "token", "credential", "auth",
		"private", "cert", "certificate", "passphrase", "pin",
	}

	lowerName := strings.ToLower(fieldName)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerName, pattern) {
			return true
		}
	}
	return false
}

// ToString generates the Python argument specification code for the ArgumentSpec
// This method outputs proper Python dict() constructor syntax that can be used directly
// in Ansible module code as the argument_spec parameter for AnsibleModule()
func (as *ArgumentSpec) ToString() string {
	if as == nil || len(as.Arguments) == 0 {
		return "argument_spec = dict()"
	}

	var builder strings.Builder
	builder.WriteString("argument_spec = dict(\n")

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

		// Add aliases
		if len(argOption.Aliases) > 0 {
			builder.WriteString(fmt.Sprintf("        aliases=%s,\n", pythonList(argOption.Aliases)))
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
		if option.Required {
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

		// Add aliases
		if len(option.Aliases) > 0 {
			builder.WriteString(fmt.Sprintf("%s    aliases=%s,\n", indent, pythonList(option.Aliases)))
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
	if len(option.RequiredOneOf) > 0 {
		builder.WriteString(fmt.Sprintf("%srequired_one_of=%s,\n", indent, pythonListOfLists(option.RequiredOneOf)))
	}
	if len(option.RequiredIf) > 0 {
		builder.WriteString(fmt.Sprintf("%srequired_if=%s,\n", indent, pythonRequiredIf(option.RequiredIf)))
	}
	if len(option.RequiredBy) > 0 {
		builder.WriteString(fmt.Sprintf("%srequired_by=%s,\n", indent, pythonRequiredBy(option.RequiredBy)))
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
	if len(as.RequiredOneOf) > 0 {
		constraints = append(constraints, fmt.Sprintf("required_one_of=%s", pythonListOfLists(as.RequiredOneOf)))
	}
	if len(as.RequiredIf) > 0 {
		constraints = append(constraints, fmt.Sprintf("required_if=%s", pythonRequiredIf(as.RequiredIf)))
	}
	if len(as.RequiredBy) > 0 {
		constraints = append(constraints, fmt.Sprintf("required_by=%s", pythonRequiredBy(as.RequiredBy)))
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
	escaped := strings.ReplaceAll(s, "'", "\\'")
	return fmt.Sprintf("'%s'", escaped)
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

// pythonRequiredIf converts required_if constraints to Python format
func pythonRequiredIf(items [][]interface{}) string {
	if len(items) == 0 {
		return "[]"
	}

	var constraints []string
	for _, constraint := range items {
		var parts []string
		for _, part := range constraint {
			parts = append(parts, pythonValue(part))
		}
		constraints = append(constraints, fmt.Sprintf("[%s]", strings.Join(parts, ", ")))
	}
	return fmt.Sprintf("[%s]", strings.Join(constraints, ", "))
}

// pythonRequiredBy converts required_by constraints to Python dict format
func pythonRequiredBy(items map[string][]string) string {
	if len(items) == 0 {
		return "dict()"
	}

	var pairs []string
	// Sort keys for consistent output
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := items[key]
		pairs = append(pairs, fmt.Sprintf("%s=%s", pythonIdentifier(key), pythonList(value)))
	}
	return fmt.Sprintf("dict(%s)", strings.Join(pairs, ", "))
}
