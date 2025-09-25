// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	mmv1resource "github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"github.com/thekad/magic-ansible/pkg/api"
)

type Module struct {
	Name          string
	Resource      *api.Resource
	MinVersion    string
	Options       map[string]*Option
	Documentation *Documentation
	Returns       *ReturnBlock
	Examples      *ExampleBlock
	ArgumentSpec  *ArgumentSpec
}

func NewFromResource(resource *api.Resource) *Module {
	m := &Module{
		Name:     resource.AnsibleName(),
		Resource: resource,
		Options:  NewOptionsFromMmv1(resource.Mmv1),
		Examples: NewExampleBlockFromMmv1(resource.Mmv1),
	}
	log.Info().Msgf("creating return block for %s", resource.AnsibleName())
	m.Returns = NewReturnBlockFromMmv1(resource.Mmv1)
	log.Info().Msgf("creating documentation for %s", resource.AnsibleName())
	m.Documentation = NewDocumentationFromOptions(resource, m.Options)
	log.Info().Msgf("creating argument spec for %s", resource.AnsibleName())
	m.ArgumentSpec = NewArgSpecFromOptions(m.Options)
	return m
}

// Option represents a single option in the Ansible module documentation
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#documentation-block
type Option struct {
	// Description is required - explanation of what this option does
	// Can be a string or list of strings (each string is one paragraph)
	Description []string `yaml:"description" json:"description"`

	// Type is optional - data type of the option
	// Uses AnsibleType enum for type safety
	Type Type `yaml:"type,omitempty" json:"type,omitempty"`

	// Default is optional - default value for the option
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`

	// Required is optional - whether this option is required
	// Defaults to false if not specified
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`

	// Choices is optional - list of valid values for this option
	Choices []string `yaml:"choices,omitempty" json:"choices,omitempty"`

	// Elements is optional - if type='list', specifies the data type of list elements
	Elements Type `yaml:"elements,omitempty" json:"elements,omitempty"`

	// Suboptions is optional - for complex types (dict), defines nested options
	Suboptions map[string]*Option `yaml:"suboptions,omitempty" json:"suboptions,omitempty"`

	// MutuallyExclusive is optional - list of options that cannot be used together
	MutuallyExclusive [][]string `yaml:"mutually_exclusive,omitempty" json:"mutually_exclusive,omitempty"`

	// RequiredTogether is optional - list of options that must be used together
	RequiredTogether [][]string `yaml:"required_together,omitempty" json:"required_together,omitempty"`

	// RequiredOneOf is optional - list where at least one option must be specified
	RequiredOneOf [][]string `yaml:"required_one_of,omitempty" json:"required_one_of,omitempty"`

	// RequiredIf is optional - conditional requirements
	RequiredIf [][]interface{} `yaml:"required_if,omitempty" json:"required_if,omitempty"`

	// RequiredBy is optional - options that are required when this option is specified
	RequiredBy map[string][]string `yaml:"required_by,omitempty" json:"required_by,omitempty"`
}

// NewOptionsFromMmv1 creates a map of Ansible options from a magic-modules API Resource
// This constructor extracts user properties from the API Resource and converts them
// to Ansible module options following the documentation format
func NewOptionsFromMmv1(resource *mmv1api.Resource) map[string]*Option {
	if resource == nil {
		return nil
	}

	options := make(map[string]*Option)

	// Always add the standard 'state' option for GCP resources
	options["state"] = &Option{
		Description: []string{
			"Whether the resource should exist in GCP.",
		},
		Type:    TypeStr,
		Default: "present",
		Choices: []string{"present", "absent"},
	}

	// Process all user properties from the API Resource
	convertedOptions := convertPropertiesToOptions(resource.AllUserProperties())

	// Merge the converted options with the state option
	for name, option := range convertedOptions {
		options[name] = option
	}

	return options
}

// convertPropertiesToOptions converts MMv1 properties to Ansible options
func convertPropertiesToOptions(properties []*mmv1api.Type) map[string]*Option {
	if properties == nil {
		return nil
	}

	options := make(map[string]*Option)

	for _, property := range properties {
		// Skip output-only properties
		if property.Output {
			continue
		}

		// Convert property name to Ansible-style underscore format
		optionName := google.Underscore(property.Name)

		// Create the option
		ansibleType, err := mapMmv1ToAnsible(property)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for property %s", property.Name)
		}

		option := &Option{
			Description: parsePropertyDescription(property),
			Type:        ansibleType,
			Required:    property.Required,
			Default:     property.DefaultValue,
			Choices:     property.EnumValues,
		}

		// Handle list element types
		if option.Type == TypeList && property.ItemType != nil {
			elementType, err := mapMmv1ToAnsible(property.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping element type for property %s", property.Name)
			}
			option.Elements = elementType

			// If the list contains nested objects, create suboptions for the element type
			if property.ItemType.Type == "NestedObject" && property.ItemType.Properties != nil {
				option.Suboptions = convertPropertiesToOptions(property.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (direct suboptions)
		if option.Type == TypeDict && property.Properties != nil {
			option.Suboptions = convertPropertiesToOptions(property.Properties)
		}

		options[optionName] = option
	}

	return options
}

// CustomCode returns the custom code (if any) defined in the API Resource YAML file
func (m *Module) CustomCode() mmv1resource.CustomCode {
	return m.Resource.Mmv1.CustomCode
}

func (m *Module) GettableProperties() []*mmv1api.Type {
	return sortProperties(m.Resource.Mmv1.GettableProperties())
}

func (m *Module) SettableProperties() []*mmv1api.Type {
	return sortProperties(m.Resource.Mmv1.SettableProperties())
}

func (m *Module) UrlParamOnlyProperties() []*mmv1api.Type {
	return sortProperties(google.Select(m.Resource.Mmv1.AllUserProperties(), func(p *mmv1api.Type) bool {
		return p.UrlParamOnly
	}))
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

func sortProperties(props []*mmv1api.Type) []*mmv1api.Type {
	slices.SortFunc(props, func(a, b *mmv1api.Type) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return props
}

func (m *Module) AllUserProperties() []*mmv1api.Type {
	return sortProperties(m.Resource.Mmv1.AllUserProperties())
}

func (m *Module) SelfLinkTpl() string {
	tpl := strings.ReplaceAll(strings.ReplaceAll(m.Resource.Mmv1.SelfLink, "{{", "{"), "}}", "}")
	return fmt.Sprintf("%s%s", m.BaseUrl(), tpl)
}
