// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var STANDARD_MODULE_REQUIREMENTS = []string{
	"python >= 3.8",
	"requests >= 2.18.4",
	"google-auth >= 1.3.0",
}
var STANDARD_AUTH_NOTES = []string{
	"For authentication, you can set service_account_file using the C(GCP_SERVICE_ACCOUNT_FILE) env variable.",
	"For authentication, you can set service_account_contents using the C(GCP_SERVICE_ACCOUNT_CONTENTS) env variable.",
	"For authentication, you can set service_account_email using the C(GCP_SERVICE_ACCOUNT_EMAIL) env variable.",
	"For authentication, you can set access_token using the C(GCP_ACCESS_TOKEN) env variable.",
	"For authentication, you can set auth_kind using the C(GCP_AUTH_KIND) env variable.",
	"For authentication, you can set scopes using the C(GCP_SCOPES) env variable.",
	"Environment variables values will only be used if the playbook values are not set.",
	"The I(service_account_email) and I(service_account_file) options are mutually exclusive.",
}

// DocType represents the data types supported by Ansible modules
type DocType string

// Ansible module data types as defined in the official documentation
const (
	DocTypeStr     DocType = "str"
	DocTypeInt     DocType = "int"
	DocTypeBool    DocType = "bool"
	DocTypeList    DocType = "list"
	DocTypeDict    DocType = "dict"
	DocTypePath    DocType = "path"
	DocTypeRaw     DocType = "raw"
	DocTypeJsonarg DocType = "jsonarg"
	DocTypeBytes   DocType = "bytes"
	DocTypeBits    DocType = "bits"
	DocTypeFloat   DocType = "float"
)

// String returns the string representation of the AnsibleType
func (t DocType) String() string {
	return string(t)
}

// DocOption represents a single option in the Ansible module documentation
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#documentation-block
type DocOption struct {
	// Description is required - explanation of what this option does
	// Can be a string or list of strings (each string is one paragraph)
	Description []interface{} `yaml:"description" json:"description"`

	// Type is optional - data type of the option
	// Uses AnsibleType enum for type safety
	Type DocType `yaml:"type,omitempty" json:"type,omitempty"`

	// Default is optional - default value for the option
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty"`

	// Required is optional - whether this option is required
	// Defaults to false if not specified
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`

	// Choices is optional - list of valid values for this option
	Choices []interface{} `yaml:"choices,omitempty" json:"choices,omitempty"`

	// Elements is optional - if type='list', specifies the data type of list elements
	Elements DocType `yaml:"elements,omitempty" json:"elements,omitempty"`

	// Suboptions is optional - for complex types (dict), defines nested options
	Suboptions map[string]*DocOption `yaml:"suboptions,omitempty" json:"suboptions,omitempty"`

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

// BuildDocOptionsFromMmv1 creates a map of Ansible options from a magic-modules API Resource
// This constructor extracts user properties from the API Resource and converts them
// to Ansible module options following the documentation format
func BuildDocOptionsFromMmv1(resource *api.Resource) map[string]*DocOption {
	if resource == nil {
		return nil
	}

	options := make(map[string]*DocOption)

	// Always add the standard 'state' option for GCP resources
	options["state"] = &DocOption{
		Description: []interface{}{
			"Whether the resource should exist in GCP.",
		},
		Type:     DocTypeStr,
		Default:  "present",
		Required: true,
		Choices:  []interface{}{"present", "absent"},
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
		ansibleType, err := mapMmv1TypeToDocType(property)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for property %s", property.Name)
		}

		option := &DocOption{
			Description: parsePropertyDocDesc(property),
			Type:        ansibleType,
			Required:    property.Required,
			Default:     property.DefaultValue,
		}

		// Handle list element types
		if option.Type == DocTypeList {
			log.Debug().Msgf("%v is a list", property.Name)
			elementType, err := mapMmv1TypeToDocType(property.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping element type for property %s", property.Name)
			}
			option.Elements = elementType

			// If the list contains nested objects, create suboptions for the element type
			if property.ItemType.Type == "NestedObject" && property.ItemType.Properties != nil {
				option.Suboptions = createDocSuboptions(property.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (direct suboptions)
		if option.Type == DocTypeDict && property.Properties != nil {
			option.Suboptions = createDocSuboptions(property.Properties)
		}

		// Note: Choices and default values would be handled here if available in the API
		// The magic-modules API structure may not expose these fields directly

		options[optionName] = option
	}

	return options
}

// createDocSuboptions recursively creates suboptions from API properties
func createDocSuboptions(properties []*api.Type) map[string]*DocOption {
	if properties == nil {
		return nil
	}

	suboptions := make(map[string]*DocOption)

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
		subAnsibleType, err := mapMmv1TypeToDocType(subProp)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping type for suboption %s", subProp.Name)
		}

		suboption := &DocOption{
			Description: parsePropertyDocDesc(subProp),
			Type:        subAnsibleType,
			Required:    subProp.Required,
			Default:     subProp.DefaultValue,
		}

		// Handle list element types for suboptions
		if subProp.ItemType != nil {
			subElementType, err := mapMmv1TypeToDocType(subProp.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping element type for suboption %s", subProp.Name)
			}
			suboption.Elements = subElementType

			// If the list contains nested objects, recursively create suboptions
			if subProp.ItemType.Type == "NestedObject" && subProp.ItemType.Properties != nil {
				suboption.Suboptions = createDocSuboptions(subProp.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (recursive suboptions)
		if suboption.Type == DocTypeDict && subProp.Properties != nil {
			suboption.Suboptions = createDocSuboptions(subProp.Properties)
		}

		// Note: Choices and default values for suboptions would be handled here
		// if available in the API structure

		suboptions[subOptionName] = suboption
	}

	return suboptions
}

// mapMmv1TypeToDocType maps magic-modules API types to Ansible module types
// Returns AnsibleType enum and error for better error handling
func mapMmv1TypeToDocType(property *api.Type) (DocType, error) {
	if property == nil {
		return "", fmt.Errorf("property is nil")
	}

	if property.Type == "" {
		return DocTypeStr, fmt.Errorf("property type is empty, defaulting to string")
	}

	switch property.Type {
	case "String":
		return DocTypeStr, nil
	case "Integer":
		return DocTypeInt, nil
	case "Boolean":
		return DocTypeBool, nil
	case "NestedObject":
		return DocTypeDict, nil
	case "KeyValueAnnotations":
		return DocTypeDict, nil
	case "Array":
		return DocTypeList, nil
	case "Enum":
		return DocTypeStr, nil
	case "ResourceRef":
		return DocTypeDict, nil
	default:
		return DocTypeStr, fmt.Errorf("unknown API type '%s' defaulting to string", property.Type)
	}
}

// Documentation represents the complete module specification
type Documentation struct {
	// Module name - must match the filename without .py extension
	Module string `yaml:"module" json:"module"`

	// Short description displayed in ansible-doc -l
	ShortDescription string `yaml:"short_description" json:"short_description"`

	// Detailed description - string or list of strings
	Description interface{} `yaml:"description" json:"description"`

	// Author information - string or list of strings
	Author interface{} `yaml:"author,omitempty" json:"author,omitempty"`

	// Module options
	Options map[string]*DocOption `yaml:"options,omitempty" json:"options,omitempty"`

	// Requirements for the module to work
	Requirements []string `yaml:"requirements,omitempty" json:"requirements,omitempty"`

	// Notes about the module
	Notes []string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

func NewDocumentationFromMmv1(resource *Resource) *Documentation {
	urlNotes := []string{
		fmt.Sprintf("API Reference: U(%s)", resource.Mmv1.References.Api),
		fmt.Sprintf("Official Documentation: U(%s)", resource.Mmv1.References.Guides["Official Documentation"]),
	}
	resourceNotes := append(urlNotes, STANDARD_AUTH_NOTES...)
	return &Documentation{
		Module:           resource.AnsibleName(),
		ShortDescription: fmt.Sprintf("Creates a GCP %s.%s resource", resource.Parent.Mmv1.Name, resource.Mmv1.Name),
		Description:      resource.Mmv1.Description,
		Options:          BuildDocOptionsFromMmv1(resource.Mmv1),
		Requirements:     STANDARD_MODULE_REQUIREMENTS,
		Notes:            resourceNotes,
	}
}

// ToYAML converts the Documentation struct to YAML string
func (d *Documentation) ToYAML() string {
	yaml, err := yaml.Marshal(d)
	if err != nil {
		return ""
	}

	return string(yaml)
}

// parsePropertyDocDesc converts API property description to Ansible format
func parsePropertyDocDesc(property *api.Type) []interface{} {
	if property == nil {
		return []interface{}{"No description available."}
	}

	// Get the description from the property
	description := property.GetDescription()
	if description == "" {
		description = "No description available."
	}

	// cleanup description from magic-modules
	description = strings.TrimPrefix(description, "Required. ")  // there's a specific "required" field
	description = strings.TrimPrefix(description, "Optional. ")  // the absence of "required" field means optional
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

	if property.Immutable {
		cleanLines = append(cleanLines, "This property is immutable, to change it, you must delete and recreate the resource.")
	}

	return cleanLines
}
