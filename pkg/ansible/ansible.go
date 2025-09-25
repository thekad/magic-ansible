// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"github.com/thekad/magic-ansible/pkg/api"
)

type Module struct {
	Resource      *api.Resource
	Options       map[string]*Option
	Documentation *Documentation
	Returns       *ReturnBlock
	Examples      *ExampleBlock
}

func NewFromResource(resource *api.Resource) *Module {
	m := &Module{
		Resource: resource,
		Options:  NewOptionsFromMmv1(resource.Mmv1),
		Returns:  NewReturnBlockFromMmv1(resource.Mmv1),
		Examples: NewExampleBlockFromMmv1(resource.Mmv1),
	}
	m.Documentation = BuildDocumentationWithOptions(resource, m.Options)
	return m
}

func (m *Module) String() string {
	return m.Resource.AnsibleName()
}

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

// Option represents a single option in the Ansible module documentation
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#documentation-block
type Option struct {
	// Description is required - explanation of what this option does
	// Can be a string or list of strings (each string is one paragraph)
	Description []interface{} `yaml:"description" json:"description"`

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
		Description: []interface{}{
			"Whether the resource should exist in GCP.",
		},
		Type:     TypeStr,
		Default:  "present",
		Required: true,
		Choices:  []string{"present", "absent"},
	}

	// Process all user properties from the API Resource
	allUserProperties := resource.AllUserProperties()
	for _, property := range allUserProperties {
		// Skip output-only
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
			Description: parseDescription(property.GetDescription()),
			Type:        ansibleType,
			Required:    property.Required,
			Default:     property.DefaultValue,
			Choices:     property.EnumValues,
		}

		// Handle list element types
		if option.Type == TypeList {
			log.Debug().Msgf("%v is a list", property.Name)
			elementType, err := mapMmv1ToAnsible(property.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping element type for property %s", property.Name)
			}
			option.Elements = elementType

			// If the list contains nested objects, create suboptions for the element type
			if property.ItemType.Type == "NestedObject" && property.ItemType.Properties != nil {
				option.Suboptions = createSuboptions(property.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (direct suboptions)
		if option.Type == TypeDict && property.Properties != nil {
			option.Suboptions = createSuboptions(property.Properties)
		}

		options[optionName] = option
	}

	return options
}

// createSuboptions recursively creates suboptions from API properties
func createSuboptions(properties []*mmv1api.Type) map[string]*Option {
	if properties == nil {
		return nil
	}

	suboptions := make(map[string]*Option)

	for _, subProp := range properties {
		// Skip output-only properties
		if subProp.Output {
			continue
		}

		// Convert property name to Ansible-style underscore format
		subOptionName := google.Underscore(subProp.Name)

		// Create the suboption
		subAnsibleType, err := mapMmv1ToAnsible(subProp)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for suboption %s", subProp.Name)
		}

		suboption := &Option{
			Description: parseDescription(subProp.GetDescription()),
			Type:        subAnsibleType,
			Required:    subProp.Required,
			Default:     subProp.DefaultValue,
		}

		// Handle list element types for suboptions
		if subProp.ItemType != nil {
			subElementType, err := mapMmv1ToAnsible(subProp.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping element type for suboption %s", subProp.Name)
			}
			suboption.Elements = subElementType

			// If the list contains nested objects, recursively create suboptions
			if subProp.ItemType.Type == "NestedObject" && subProp.ItemType.Properties != nil {
				suboption.Suboptions = createSuboptions(subProp.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (recursive suboptions)
		if suboption.Type == TypeDict && subProp.Properties != nil {
			suboption.Suboptions = createSuboptions(subProp.Properties)
		}

		// Note: Choices and default values for suboptions would be handled here
		// if available in the API structure

		suboptions[subOptionName] = suboption
	}

	return suboptions
}

// mapMmv1ToAnsible maps magic-modules API types to Ansible module types
// Returns AnsibleType enum and error for better error handling
func mapMmv1ToAnsible(property *mmv1api.Type) (Type, error) {
	if property == nil {
		return "", fmt.Errorf("property is nil")
	}

	if property.Type == "" {
		return TypeStr, fmt.Errorf("property type is empty, defaulting to string")
	}

	switch property.Type {
	case "String":
		return TypeStr, nil
	case "Integer":
		return TypeInt, nil
	case "Boolean":
		return TypeBool, nil
	case "NestedObject":
		return TypeDict, nil
	case "KeyValueAnnotations":
		return TypeDict, nil
	case "Array":
		return TypeList, nil
	case "Enum":
		return TypeStr, nil
	case "ResourceRef":
		return TypeDict, nil
	default:
		return TypeStr, fmt.Errorf("unknown API type '%s' defaulting to string", property.Type)
	}
}

// parseDescription converts API property description to Ansible format
func parseDescription(description string) []interface{} {
	if description == "" {
		description = "No description available."
	}

	// cleanup description from magic-modules
	description = strings.TrimPrefix(description, "Required. ") // there's a specific "required" field
	description = strings.TrimPrefix(description, "Optional. ") // the absence of "required" field means optional
	immutable := strings.HasPrefix(description, "Immutable.")
	description = strings.TrimPrefix(description, "Immutable. ") // a note is added to the description if the property is immutable

	// Split description into lines and clean them up
	lines := strings.Split(description, ". ")
	var cleanLines []interface{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = fmt.Sprintf("%s.", strings.TrimSuffix(trimmed, "."))
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	if len(cleanLines) == 0 {
		cleanLines = []interface{}{"No description available."}
	}

	if immutable {
		cleanLines = append(cleanLines, "This property is immutable, to change it, you must delete and recreate the resource.")
	}

	return cleanLines
}
