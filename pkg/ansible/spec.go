// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
)

// Type represents the data types supported by Ansible modules
type Type string

// Ansible module data types as defined in the official documentation
const (
	AnsibleTypeStr     Type = "str"
	AnsibleTypeInt     Type = "int"
	AnsibleTypeBool    Type = "bool"
	AnsibleTypeList    Type = "list"
	AnsibleTypeDict    Type = "dict"
	AnsibleTypePath    Type = "path"
	AnsibleTypeRaw     Type = "raw"
	AnsibleTypeJsonarg Type = "jsonarg"
	AnsibleTypeBytes   Type = "bytes"
	AnsibleTypeBits    Type = "bits"
	AnsibleTypeFloat   Type = "float"
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
	Choices []interface{} `yaml:"choices,omitempty" json:"choices,omitempty"`

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
func NewOptionsFromMmv1(resource *api.Resource) map[string]*Option {
	if resource == nil {
		return nil
	}

	options := make(map[string]*Option)

	// Always add the standard 'state' option for GCP resources
	options["state"] = &Option{
		Description: []interface{}{
			"Whether the resource should exist in GCP.",
		},
		Type:     AnsibleTypeStr,
		Default:  "present",
		Required: false,
		Choices:  []interface{}{"present", "absent"},
	}

	// Process all user properties from the API Resource
	allUserProperties := resource.AllUserProperties()
	for _, property := range allUserProperties {
		log.Debug().Msgf("processing property: %s", property.Name)
		// Skip output-only
		if property.Output {
			continue
		}

		// Convert property name to Ansible-style underscore format
		optionName := google.Underscore(property.Name)

		// Create the option
		ansibleType, err := mapAPITypeToAnsible(property)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for property %s", property.Name)
		}

		option := &Option{
			Description: parsePropertyDescription(property),
			Type:        ansibleType,
			Required:    property.Required,
		}

		// Handle list element types
		if option.Type == AnsibleTypeList {
			log.Debug().Msgf("%v is a list", property.Name)
			elementType, err := mapAPITypeToAnsible(property.ItemType)
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
		if option.Type == AnsibleTypeDict && property.Properties != nil {
			log.Debug().Msgf("%v is a dict", property.Name)
			option.Suboptions = createSuboptions(property.Properties)
		}

		// Note: Choices and default values would be handled here if available in the API
		// The magic-modules API structure may not expose these fields directly

		options[optionName] = option
	}

	return options
}

// createSuboptions recursively creates suboptions from API properties
func createSuboptions(properties []*api.Type) map[string]*Option {
	if properties == nil {
		return nil
	}

	suboptions := make(map[string]*Option)

	for _, subProp := range properties {
		// Skip output-only properties
		if subProp.Output {
			continue
		}

		// Skip immutable properties
		if subProp.Immutable {
			continue
		}

		// Skip url-param-only properties
		if subProp.UrlParamOnly {
			continue
		}

		// Convert property name to Ansible-style underscore format
		subOptionName := google.Underscore(subProp.Name)

		// Create the suboption
		subAnsibleType, err := mapAPITypeToAnsible(subProp)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for suboption %s", subProp.Name)
		}

		suboption := &Option{
			Description: parsePropertyDescription(subProp),
			Type:        subAnsibleType,
			Required:    false, // Default to optional
		}

		// Handle list element types for suboptions
		if subProp.ItemType != nil {
			subElementType, err := mapAPITypeToAnsible(subProp.ItemType)
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
		if suboption.Type == AnsibleTypeDict && subProp.Properties != nil {
			suboption.Suboptions = createSuboptions(subProp.Properties)
		}

		// Note: Choices and default values for suboptions would be handled here
		// if available in the API structure

		suboptions[subOptionName] = suboption
	}

	return suboptions
}

// parsePropertyDescription converts API property description to Ansible format
func parsePropertyDescription(property *api.Type) []interface{} {
	if property == nil {
		return []interface{}{"No description available."}
	}

	// Get the description from the property
	description := property.GetDescription()
	if description == "" {
		description = "No description available."
	}

	// Split description into lines and clean them up
	lines := strings.Split(description, "\n")
	var cleanLines []interface{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	if len(cleanLines) == 0 {
		return []interface{}{"No description available."}
	}

	return cleanLines
}

// mapAPITypeToAnsible maps magic-modules API types to Ansible module types
// Returns AnsibleType enum and error for better error handling
func mapAPITypeToAnsible(property *api.Type) (Type, error) {
	if property == nil {
		return "", fmt.Errorf("property is nil")
	}

	if property.Type == "" {
		return AnsibleTypeStr, fmt.Errorf("property type is empty, defaulting to string")
	}

	switch property.Type {
	case "String":
		return AnsibleTypeStr, nil
	case "Integer":
		return AnsibleTypeInt, nil
	case "Boolean":
		return AnsibleTypeBool, nil
	case "NestedObject":
		return AnsibleTypeDict, nil
	case "KeyValueAnnotations":
		return AnsibleTypeDict, nil
	case "Array":
		return AnsibleTypeList, nil
	case "Enum":
		return AnsibleTypeStr, nil
	case "ResourceRef":
		return AnsibleTypeDict, nil
	default:
		return AnsibleTypeStr, fmt.Errorf("unknown API type '%s' defaulting to string", property.Type)
	}
}
