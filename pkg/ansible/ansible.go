// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"sort"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	mmv1resource "github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"github.com/thekad/magic-ansible/pkg/api"
)

// Type represents the data types supported by Ansible modules
type Type string

// Ansible module data types as defined in the official documentation
const (
	TypeStr     Type = "str"
	TypeInt     Type = "int"
	TypeBool    Type = "bool"
	TypeList    Type = "list"
	TypeDict    Type = "dict"
	TypePath    Type = "path"
	TypeRaw     Type = "raw"
	TypeJsonarg Type = "jsonarg"
	TypeBytes   Type = "bytes"
	TypeBits    Type = "bits"
	TypeFloat   Type = "float"
)

// String returns the string representation of the AnsibleType
func (t Type) String() string {
	return string(t)
}

// MapMmv1ToAnsible maps magic-modules API types to Ansible module types
// Returns AnsibleType enum and error for better error handling
func MapMmv1ToAnsible(property *mmv1api.Type) Type {
	if property == nil {
		return ""
	}

	switch property.Type {
	case "String":
		return TypeStr
	case "Integer":
		return TypeInt
	case "Boolean":
		return TypeBool
	case "NestedObject":
		return TypeDict
	case "KeyValueAnnotations":
		return TypeDict
	case "KeyValueLabels":
		return TypeDict
	case "KeyValuePairs":
		return TypeDict
	case "Array":
		return TypeList
	case "Enum":
		return TypeStr
	case "ResourceRef":
		return TypeDict
	case "Fingerprint":
		return TypeStr
	default:
		log.Warn().Msgf("unknown API type '%s' defaulting to string", property.Type)
		return TypeStr
	}
}

type Module struct {
	Name              string
	MutuallyExclusive [][]string
	RequiredTogether  [][]string
	RequiredOneOf     [][]string
	Resource          *api.Resource
	MinVersion        string
	Options           map[string]*Option
	Documentation     *Documentation
	Returns           *ReturnBlock
	Examples          *Examples
	ArgumentSpec      *ArgumentSpec
	OperationConfigs  map[string]*OperationConfig
}

func NewFromResource(resource *api.Resource) *Module {
	m := &Module{
		Name:             resource.AnsibleName(),
		Resource:         resource,
		Options:          NewOptionsFromMmv1(resource.Mmv1),
		Examples:         NewExamplesFromMmv1(resource.Mmv1),
		Returns:          NewReturnBlockFromMmv1(resource.Mmv1),
		ArgumentSpec:     NewArgSpecFromMmv1(resource.Mmv1),
		OperationConfigs: NewOperationConfigsFromMmv1(resource.Mmv1),
	}
	log.Info().Msgf("creating documentation for %s", resource.AnsibleName())
	// documentation should exclude output-only options
	docOptions := make(map[string]*Option, 0)
	for name, option := range m.Options {
		if option.OutputOnly() && option.Name != "state" {
			continue
		}
		docOptions[name] = option
	}
	m.Documentation = NewDocumentationFromOptions(resource, docOptions)

	return m
}

// Option represents a single option in the Ansible module documentation
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#documentation-block
type Option struct {
	// Name is the name of the option
	Name string `yaml:"-"`

	// Parent is a reference to the parent option
	Parent *Option `yaml:"-"`

	// Mmv1 is a reference to the original MMv1 property
	Mmv1 *mmv1api.Type `yaml:"-"`

	// Description is required - explanation of what this option does
	// Can be a string or list of strings (each string is one paragraph)
	Description []string `yaml:"description"`

	// Type is optional - data type of the option
	// Uses AnsibleType enum for type safety
	Type Type `yaml:"type,omitempty"`

	// Default is optional - default value for the option
	Default interface{} `yaml:"default,omitempty"`

	// Required is optional - whether this option is required
	// Defaults to false if not specified
	Required bool `yaml:"required,omitempty"`

	// Choices is optional - list of valid values for this option
	Choices []string `yaml:"choices,omitempty"`

	// Elements is optional - if type='list', specifies the data type of list elements
	Elements Type `yaml:"elements,omitempty"`

	// Suboptions is optional - for complex types (dict), defines nested options
	Suboptions map[string]*Option `yaml:"suboptions,omitempty"`
}

func (o *Option) OutputOnly() bool {
	if o.Parent != nil {
		return o.Parent.OutputOnly()
	}

	return o.Mmv1 != nil && o.Mmv1.Output
}

func (o *Option) SortedSuboptions() []*Option {
	return sortedOptions(o.Suboptions)
}

func (o *Option) OutputSuboptions() []*Option {
	options := make([]*Option, 0)

	if o.Suboptions == nil {
		return options
	}
	for _, option := range o.SortedSuboptions() {
		if option.OutputOnly() {
			options = append(options, option)
		}
	}

	return options
}

func (o *Option) InputSuboptions() []*Option {
	options := make([]*Option, 0)

	if o.Suboptions == nil {
		return options
	}
	for _, option := range o.SortedSuboptions() {
		if !option.OutputOnly() {
			options = append(options, option)
		}
	}

	return options
}

func (o *Option) IsList() bool {
	return o.Type == TypeList
}

func (o *Option) IsNestedObject() bool {
	return o.Mmv1.IsA("NestedObject")
}

func (o *Option) IsNestedList() bool {
	return o.IsList() && o.ElementsAre("NestedObject")
}

func (o *Option) AnsibleName() string {
	return google.Underscore(o.Name)
}

func (o *Option) ClassName() string {
	if o.IsNestedList() {
		if o.Parent != nil {
			return o.Parent.ClassName() + google.Camelize(strings.TrimSuffix(o.Name, "s"), "upper")
		}
	}
	if o.IsNestedObject() {
		if o.Parent != nil {
			return o.Parent.ClassName() + google.Camelize(o.Name, "upper")
		}
	}

	return google.Camelize(o.Name, "upper")
}

func (o *Option) ElementsAre(q string) bool {
	return o.Mmv1.ItemType.IsA(q)
}

// NewOptionsFromMmv1 creates a map of Ansible options from a magic-modules API Resource
// This constructor extracts user properties from the API Resource and converts them
// to Ansible module options following the documentation format
func NewOptionsFromMmv1(resource *mmv1api.Resource) map[string]*Option {
	if resource == nil {
		return nil
	}

	// Process all user properties from the API Resource
	options := convertPropertiesToOptions(resource.AllUserProperties(), nil)

	// Always add the standard 'state' option for GCP resources
	options["state"] = &Option{
		Name: "state",
		Description: []string{
			"Whether the resource should exist in GCP.",
		},
		Type:    TypeStr,
		Default: "present",
		Choices: []string{"present", "absent"},
	}

	return options
}

// convertPropertiesToOptions converts MMv1 properties to Ansible options
func convertPropertiesToOptions(properties []*mmv1api.Type, parent *Option) map[string]*Option {
	if properties == nil {
		return nil
	}

	options := map[string]*Option{}

	for _, property := range properties {

		// Create the option
		option := &Option{
			Name:        property.Name,
			Mmv1:        property,
			Parent:      parent,
			Description: parsePropertyDescription(property),
			Type:        MapMmv1ToAnsible(property),
			Required:    property.Required,
			Default:     property.DefaultValue,
			Choices:     property.EnumValues,
		}

		log.Debug().Msgf("converted property %s (parent: %v, class name: %s)", property.Name, parent, option.ClassName())

		// Handle list element types
		if option.Type == TypeList && property.ItemType != nil {
			option.Elements = MapMmv1ToAnsible(property.ItemType)

			// If the list contains nested objects, create suboptions for the element type
			if property.ItemType.Type == "NestedObject" && property.ItemType.Properties != nil {
				option.Suboptions = convertPropertiesToOptions(property.ItemType.Properties, option)
			}
		}

		// Handle nested dictionary objects (direct suboptions)
		if option.Type == TypeDict && property.Properties != nil {
			option.Suboptions = convertPropertiesToOptions(property.Properties, option)
		}

		options[option.AnsibleName()] = option
	}

	return options
}

func (m *Module) String() string {
	return m.Resource.AnsibleName()
}

// CustomCode returns the custom code (if any) defined in the API Resource YAML file
func (m *Module) CustomCode() mmv1resource.CustomCode {
	return m.Resource.Mmv1.CustomCode
}

func (m *Module) UrlParamOnlyProperties() []*mmv1api.Type {
	return google.Select(m.Resource.Mmv1.AllUserProperties(), func(p *mmv1api.Type) bool {
		return p.UrlParamOnly
	})
}

func (m *Module) BaseUrl() string {
	productVersions := m.Resource.Parent.Mmv1.Versions
	for _, version := range productVersions {
		if version.Name == m.Resource.MinVersion() {
			return version.BaseUrl
		}
	}

	return ""
}

func (m *Module) ModuleClass() string {
	return google.Camelize(m.Resource.Parent.Mmv1.Name, "upper")
}

func (m *Module) Kind() string {
	return fmt.Sprintf("%s#%s", google.Camelize(m.Resource.Parent.Name, "lower"), google.Camelize(m.Resource.Mmv1.Name, "lower"))
}

func (m *Module) Scopes() []string {
	return m.Resource.Parent.Mmv1.Scopes
}

func (m *Module) GetAsync() *mmv1api.Async {
	return m.Resource.Mmv1.GetAsync()
}

func (m *Module) ProductName() string {
	return m.Resource.Parent.Mmv1.Name
}

func (m *Module) AllMmv1BodyOptions() []*Option {
	opts := make([]*Option, 0)
	for _, option := range sortedOptions(m.Options) {
		// we only care about options that have an mmv1 attached to them
		if option.Mmv1 == nil || option.Mmv1.UrlParamOnly {
			continue
		}
		opts = append(opts, option)
	}
	return opts
}

func (m *Module) OutputOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return o.OutputOnly()
	})
}

func (m *Module) InputOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return !o.OutputOnly()
	})
}

func (m *Module) UrlParamOnlyOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return o.Mmv1.UrlParamOnly
	})
}

func (m *Module) AllNestedOptions() map[string]*Option {
	nestedOptions := make(map[string]*Option)

	// Start with top-level options
	for _, option := range m.AllMmv1BodyOptions() {
		collectNestedOptions(option, nestedOptions)
	}

	return nestedOptions
}

// collectNestedOptions recursively collects all nested object options
func collectNestedOptions(option *Option, result map[string]*Option) {
	// If this option is a nested object or a list of nested objects, add it to the result
	if option.IsNestedObject() || option.IsNestedList() {
		result[option.ClassName()] = option
	}

	// Recursively check suboptions
	if option.Suboptions != nil {
		for _, suboption := range option.Suboptions {
			collectNestedOptions(suboption, result)
		}
	}
}

func sortedOptions(m map[string]*Option) []*Option {
	opts := make([]*Option, 0, len(m))
	for _, option := range m {
		opts = append(opts, option)
	}
	sort.Slice(opts, func(i, j int) bool {
		return opts[i].Name < opts[j].Name
	})
	return opts
}

type OperationConfig struct {
	UriTemplate      string `json:"uri"`
	AsyncUriTemplate string `json:"async_uri"`
	Verb             string `json:"verb"`
	TimeoutMinutes   int    `json:"timeout"`
}

func NewOperationConfigsFromMmv1(mmv1 *mmv1api.Resource) map[string]*OperationConfig {
	ops := map[string]*OperationConfig{}
	timeouts := mmv1.GetTimeouts()
	defaultVerbs := map[string]string{
		"read":   "GET",
		"create": "POST",
		"update": "PUT",
		"delete": "DELETE",
	}

	// Helper function to get verb or default
	getVerb := func(mmv1Verb, operation string) string {
		if mmv1Verb != "" {
			return mmv1Verb
		}
		return defaultVerbs[operation]
	}

	escapeCurlyBraces := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "{{", "{"), "}}", "}")
	}

	ops["read"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.SelfLinkUri()),
		Verb:             getVerb(mmv1.ReadVerb, "read"),
		AsyncUriTemplate: "",
	}
	ops["create"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.CreateUri()),
		Verb:             getVerb(mmv1.CreateVerb, "create"),
		TimeoutMinutes:   timeouts.InsertMinutes,
		AsyncUriTemplate: "",
	}
	ops["update"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.UpdateUri()),
		Verb:             getVerb(mmv1.UpdateVerb, "update"),
		TimeoutMinutes:   timeouts.UpdateMinutes,
		AsyncUriTemplate: "",
	}
	ops["delete"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.DeleteUri()),
		Verb:             getVerb(mmv1.DeleteVerb, "delete"),
		TimeoutMinutes:   timeouts.DeleteMinutes,
		AsyncUriTemplate: "",
	}

	async := mmv1.GetAsync()
	if async != nil {
		for _, action := range async.Actions {
			ops[strings.ToLower(action)].AsyncUriTemplate = escapeCurlyBraces(async.Operation.BaseUrl)
		}
	}

	log.Debug().Msgf("operation configs: %v", ops)

	return ops
}
