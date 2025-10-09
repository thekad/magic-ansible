// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"sort"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
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

	// Conflicts is optional - list of options that cannot be used together
	Conflicts []string `yaml:"conflicts,omitempty"`
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
			Conflicts:   property.Conflicts,
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
