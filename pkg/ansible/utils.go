// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"gopkg.in/yaml.v3"
)

const MAX_DESCRIPTION_LENGTH = 140

// parsePropertyDescription converts API property description to Ansible format i.e. multi-line string to list of strings
func parsePropertyDescription(property *mmv1api.Type) []string {
	description := property.Description
	if property.Description == "" {
		description = "No description available."
	}

	// cleanup description from magic-modules
	description = strings.TrimPrefix(description, "Required. ") // there's a specific "required" field
	description = strings.TrimPrefix(description, "Optional. ") // the absence of "required" field means optional
	immutable := strings.HasPrefix(description, "Immutable.")
	description = strings.TrimPrefix(description, "Immutable. ") // a note is added to the description if the property is immutable
	description = strings.Join(strings.Split(description, "\n"), " ")

	// Split description by sentences
	sentences := strings.Split(description, ". ")
	var cleanLines []string

	for _, line := range sentences {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, fmt.Sprintf("%s.", strings.TrimSuffix(trimmed, ".")))
		}
	}

	if len(cleanLines) == 0 {
		cleanLines = []string{"No description available."}
	}

	if property.Type == "ResourceRef" {
		sourceRefDesc := []string{
			fmt.Sprintf("This field is a reference to a %s resource in GCP.", property.Resource),
			fmt.Sprintf("It can be specified in two ways: First, you can place a dictionary with key '%s' matching your resource.", string(property.Imports)),
			fmt.Sprintf("Alternatively, you can add `register: name-of-resource` to a %s task and then set this field to `{{ name-of-resource }}`.", property.Resource),
		}
		cleanLines = append(cleanLines, sourceRefDesc...)
	}

	if immutable {
		cleanLines = append(cleanLines, "This property is immutable, to change it, you must delete and recreate the resource.")
	}

	return cleanLines
}

// ToYAML converts an interface to YAML format with 2-space indentation
// Uses folded style for description fields
func ToYAML(data interface{}) string {
	if data == nil {
		return ""
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	// Set indentation to 2 spaces
	encoder.SetIndent(2)

	// Create a node tree and set folded style for description fields
	node := &yaml.Node{}
	err := node.Encode(data)
	if err != nil {
		return ""
	}

	// Walk the node tree and set folded style for description fields
	setFoldedStyleForDescriptions(node)

	// Sort all map keys for consistent output
	sortYAMLMapKeys(node)

	err = encoder.Encode(node)
	if err != nil {
		return ""
	}

	encoder.Close()
	return buf.String()
}

// setFoldedStyleForDescriptions recursively sets folded style for description fields
func setFoldedStyleForDescriptions(node *yaml.Node) {
	if node == nil {
		return
	}

	// If this is a mapping node, look for description keys
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Check if the key is "description"
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "description" {
				// Set folded style for the description value
				if valueNode.Kind == yaml.SequenceNode {
					// For arrays of strings, set each string to use folded style
					for _, item := range valueNode.Content {
						if item.Kind == yaml.ScalarNode && item.Tag == "!!str" {
							if len(item.Value) > MAX_DESCRIPTION_LENGTH {
								item.Style = yaml.FoldedStyle
								item.Value = strings.Join(breakLineByLength(item.Value), "\n")
							}
						}
					}
				} else if valueNode.Kind == yaml.ScalarNode && valueNode.Tag == "!!str" {
					// For single strings, use folded style
					if len(valueNode.Value) > MAX_DESCRIPTION_LENGTH {
						valueNode.Style = yaml.FoldedStyle
						valueNode.Value = strings.Join(breakLineByLength(valueNode.Value), "\n")
					}
				}
			}
		}
	}

	// Recursively process child nodes
	for _, child := range node.Content {
		setFoldedStyleForDescriptions(child)
	}
}

// breakLineByLength breaks a line into chunks of maxLength characters or less, breaking on word boundaries
func breakLineByLength(line string) []string {
	if len(line) <= MAX_DESCRIPTION_LENGTH {
		return []string{line}
	}

	var chunks []string
	words := strings.Fields(line) // Split by whitespace
	currentChunk := ""

	for _, word := range words {
		// Check if adding this word would exceed the limit
		testChunk := currentChunk
		if testChunk != "" {
			testChunk += " "
		}
		testChunk += word

		if len(testChunk) <= MAX_DESCRIPTION_LENGTH {
			// Word fits, add it to current chunk
			currentChunk = testChunk
		} else {
			// Word doesn't fit, start a new chunk
			if currentChunk != "" {
				chunks = append(chunks, currentChunk)
			}
			currentChunk = word
		}
	}

	// Add the last chunk if it's not empty
	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// sortYAMLMapKeys recursively sorts all map keys in a YAML node tree for consistent output
func sortYAMLMapKeys(node *yaml.Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		// For mapping nodes, we need to sort the key-value pairs by key
		if len(node.Content)%2 == 0 { // Ensure we have pairs
			// Create a slice of key-value pairs
			type kvPair struct {
				key   *yaml.Node
				value *yaml.Node
			}

			var pairs []kvPair
			for i := 0; i < len(node.Content); i += 2 {
				pairs = append(pairs, kvPair{
					key:   node.Content[i],
					value: node.Content[i+1],
				})
			}

			// Sort pairs by key value
			sort.Slice(pairs, func(i, j int) bool {
				return pairs[i].key.Value < pairs[j].key.Value
			})

			// Rebuild the content array with sorted pairs
			node.Content = make([]*yaml.Node, 0, len(pairs)*2)
			for _, pair := range pairs {
				node.Content = append(node.Content, pair.key, pair.value)
			}
		}

		// Recursively sort child nodes
		for _, child := range node.Content {
			sortYAMLMapKeys(child)
		}

	case yaml.SequenceNode:
		// For sequence nodes, just recursively sort child nodes
		for _, child := range node.Content {
			sortYAMLMapKeys(child)
		}
	}
}
