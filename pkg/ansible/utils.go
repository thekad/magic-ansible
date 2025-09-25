// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"bytes"
	"fmt"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"gopkg.in/yaml.v3"
)

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

// parseDescription converts API property description to Ansible format i.e. multi-line string to list of strings
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
	lines := strings.Split(description, "\n")
	var cleanLines []interface{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
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

// ToYAML converts an interface to YAML format with 2-space indentation
func ToYAML(data interface{}) string {
	if data == nil {
		return ""
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	// Set indentation to 2 spaces
	encoder.SetIndent(2)

	err := encoder.Encode(data)
	if err != nil {
		return ""
	}

	encoder.Close()
	return buf.String()
}
