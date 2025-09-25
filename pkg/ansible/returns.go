// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// ReturnType represents the data types returned by the module
type ReturnType string

// ReturnType as defined in the official documentation
const (
	ReturnTypeStr     ReturnType = "str"
	ReturnTypeInt     ReturnType = "int"
	ReturnTypeBool    ReturnType = "bool"
	ReturnTypeList    ReturnType = "list"
	ReturnTypeDict    ReturnType = "dict"
	ReturnTypeFloat   ReturnType = "float"
	ReturnTypeComplex ReturnType = "complex"
)

// String returns the string representation of the AnsibleType
func (t ReturnType) String() string {
	return string(t)
}

// ReturnAttribute represents the returns section of the Ansible module documentation
// Based on: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#return-block
type ReturnAttribute struct {
	// Description - detailed description of what this value represents
	// Required field - string or list of strings, capitalized with trailing dot
	Description interface{} `yaml:"description" json:"description"`

	// Returned - when this value is returned (e.g., "always", "changed", "success")
	// Required field - string with human-readable content
	Returned string `yaml:"returned" json:"returned"`

	// Type - data type of the returned value
	// Required field - one of the ReturnType constants
	Type ReturnType `yaml:"type" json:"type"`

	// Elements - if type='list', specifies the data type of the list's elements
	// Optional field
	Elements ReturnType `yaml:"elements,omitempty" json:"elements,omitempty"`

	// Contains - for nested return values (type: dict, list/elements: dict, or complex)
	// Optional field - map of nested ReturnAttribute objects
	Contains map[string]*ReturnAttribute `yaml:"contains,omitempty" json:"contains,omitempty"`
}

// mapMmv1TypeToReturnType maps magic-modules API types to Ansible module return types
// Returns ReturnType enum and error for better error handling
func mapMmv1TypeToReturnType(property *api.Type) (ReturnType, error) {
	if property == nil {
		return "", fmt.Errorf("property is nil")
	}

	if property.Type == "" {
		return ReturnTypeStr, fmt.Errorf("property type is empty, defaulting to string")
	}

	switch property.Type {
	case "String":
		return ReturnTypeStr, nil
	case "Integer":
		return ReturnTypeInt, nil
	case "Boolean":
		return ReturnTypeBool, nil
	case "NestedObject":
		return ReturnTypeComplex, nil
	case "KeyValueAnnotations":
		return ReturnTypeDict, nil
	case "Array":
		return ReturnTypeList, nil
	case "Enum":
		return ReturnTypeStr, nil
	case "Fingerprint":
		return ReturnTypeStr, nil
	default:
		return ReturnTypeStr, fmt.Errorf("unknown API type '%s' defaulting to string", property.Type)
	}
}

// BuildReturnBlockFromMmv1 creates a map of Ansible return attributes from a magic-modules API Resource
// This function extracts properties from the API Resource and converts them to Ansible module return format
// following the specification at: https://docs.ansible.com/ansible/latest/dev_guide/developing_modules_documenting.html#return-block
func BuildReturnBlockFromMmv1(resource *api.Resource) map[string]*ReturnAttribute {
	if resource == nil {
		return nil
	}

	returns := make(map[string]*ReturnAttribute)

	// Add standard return values that all GCP modules should have
	returns["changed"] = &ReturnAttribute{
		Description: "Whether the resource was changed.",
		Returned:    "always",
		Type:        ReturnTypeBool,
	}

	returns["state"] = &ReturnAttribute{
		Description: "The current state of the resource.",
		Returned:    "always",
		Type:        ReturnTypeStr,
	}

	// Process properties from the API Resource
	outputProperties := resource.GettableProperties()
	for _, property := range outputProperties {
		returnName := property.Name

		// Create the return attribute
		returnType, err := mapMmv1TypeToReturnType(property)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping return type for property %s", property.Name)
		}

		returnAttr := &ReturnAttribute{
			Description: parseReturnDescription(property),
			Returned:    determineReturnedCondition(property),
			Type:        returnType,
		}

		// Handle list element types
		if returnAttr.Type == ReturnTypeList && property.ItemType != nil {
			log.Debug().Msgf("%v is a list return", property.Name)
			elementType, err := mapMmv1TypeToReturnType(property.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping return element type for property %s", property.Name)
			}
			returnAttr.Elements = elementType

			// If the list contains nested objects, create contains for the element type
			if property.ItemType.Type == "NestedObject" && property.ItemType.Properties != nil {
				returnAttr.Contains = createReturnContains(property.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (direct contains)
		if (returnAttr.Type == ReturnTypeDict || returnAttr.Type == ReturnTypeComplex) && property.Properties != nil {
			returnAttr.Contains = createReturnContains(property.Properties)
		}

		returns[returnName] = returnAttr
	}

	return returns
}

// createReturnContains recursively creates contains from API properties for nested return values
func createReturnContains(properties []*api.Type) map[string]*ReturnAttribute {
	if properties == nil {
		return nil
	}

	contains := make(map[string]*ReturnAttribute)

	for _, subProp := range properties {
		containsName := subProp.Name

		// Create the nested return attribute
		subReturnType, err := mapMmv1TypeToReturnType(subProp)
		if err != nil {
			log.Warn().Err(err).Msgf("error mapping return type for nested property %s", subProp.Name)
		}

		containsAttr := &ReturnAttribute{
			Description: parseReturnDescription(subProp),
			Returned:    determineReturnedCondition(subProp),
			Type:        subReturnType,
		}

		// Handle list element types for nested properties
		if subProp.ItemType != nil {
			subElementType, err := mapMmv1TypeToReturnType(subProp.ItemType)
			if err != nil {
				log.Warn().Err(err).Msgf("error mapping return element type for nested property %s", subProp.Name)
			}
			containsAttr.Elements = subElementType

			// If the list contains nested objects, recursively create contains
			if subProp.ItemType.Type == "NestedObject" && subProp.ItemType.Properties != nil {
				containsAttr.Contains = createReturnContains(subProp.ItemType.Properties)
			}
		}

		// Handle nested dictionary objects (recursive contains)
		if containsAttr.Type == ReturnTypeDict && subProp.Properties != nil {
			containsAttr.Contains = createReturnContains(subProp.Properties)
		}

		contains[containsName] = containsAttr
	}

	return contains
}

// parseReturnDescription converts API property description to Ansible return description format
// Returns a string (single paragraph) with proper capitalization and trailing dot
func parseReturnDescription(property *api.Type) interface{} {
	if property == nil || property.Description == "" {
		return fmt.Sprintf("The %s field.", strings.ToLower(property.Name))
	}

	desc := strings.TrimSpace(property.Description)

	// cleanup description from magic-modules
	desc = strings.TrimPrefix(desc, "Required. ")
	desc = strings.TrimPrefix(desc, "Optional. ")
	desc = strings.TrimPrefix(desc, "Immutable. ")

	// Ensure description starts with a capital letter
	if len(desc) > 0 {
		desc = strings.ToUpper(desc[:1]) + desc[1:]
	}

	// Ensure description ends with a period
	if !strings.HasSuffix(desc, ".") {
		desc += "."
	}

	return desc
}

// determineReturnedCondition determines when a return value is returned based on property characteristics
func determineReturnedCondition(property *api.Type) string {
	if property == nil {
		return "success"
	}

	// Output-only properties are always returned when the resource exists
	if property.Output {
		return "success"
	}

	// Required properties are always returned
	if property.Required {
		return "always"
	}

	// Optional properties are returned when set
	return "when set"
}

// ReturnsToYAML converts a map of ReturnAttribute to YAML string
// This is used in templates to generate the RETURN block
func ReturnsToYAML(returns map[string]*ReturnAttribute) string {
	if returns == nil {
		return ""
	}

	yamlData, err := yaml.Marshal(returns)
	if err != nil {
		log.Warn().Err(err).Msg("error marshaling returns to YAML")
		return ""
	}

	return string(yamlData)
}
