// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// findNodeByKey looks for a given field in a generic Node structure
// and returns its first occurrence as a YAML node if found (or nil)
func findNodeByKey(node *yaml.Node, key string) *yaml.Node {
	switch node.Kind {
	case yaml.DocumentNode:
		// If the node is a Document, iterate through its content.
		for _, contentNode := range node.Content {
			if found := findNodeByKey(contentNode, key); found != nil {
				return found
			}
		}
	case yaml.MappingNode:
		// If the node is a Map, iterate through its key/value pairs.
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == key {
				return valueNode
			}

			// Recursively search nested maps and sequences.
			if found := findNodeByKey(valueNode, key); found != nil {
				return found
			}
		}
	case yaml.SequenceNode:
		// If the node is a Sequence (list), iterate through its children.
		for _, childNode := range node.Content {
			if found := findNodeByKey(childNode, key); found != nil {
				return found
			}
		}
	}
	return nil
}

// overrideYAML will apply our overrides (if any) on top of the source YAML file
func overrideYAML(rootNode *yaml.Node, overridesDir string, yamlFile string) {
	pieces := strings.Split(yamlFile, "/")
	productName := pieces[len(pieces)-2]
	baseFile := pieces[len(pieces)-1]
	overrideFile := path.Join(overridesDir, productName, baseFile)
	overrideData, err := os.ReadFile(overrideFile)
	if err != nil {
		log.Debug().Msgf("no override file found for file: %s/%s", productName, baseFile)
		return
	}
	log.Info().Msgf("applying overrides for file: %s/%s", productName, baseFile)

	// unmarshal the override data
	overrideNode := yaml.Node{}
	if err := yaml.Unmarshal(overrideData, &overrideNode); err != nil {
		log.Error().Msgf("error unmarshalling override file: %v", err)
	}

	// merge the override data into the root node
	mergeYAMLNodes(rootNode, &overrideNode)
}

// mergeYAMLNodes merges the override node into the root node
func mergeYAMLNodes(root, override *yaml.Node) {
	if root == nil || override == nil {
		return
	}

	// Handle document nodes by merging their content
	if root.Kind == yaml.DocumentNode && override.Kind == yaml.DocumentNode {
		if len(root.Content) > 0 && len(override.Content) > 0 {
			mergeYAMLNodes(root.Content[0], override.Content[0])
		}
		return
	}

	// Handle mapping nodes (objects)
	if root.Kind == yaml.MappingNode && override.Kind == yaml.MappingNode {
		mergeMappingNodes(root, override)
		return
	}

	// Handle sequence nodes (arrays)
	if root.Kind == yaml.SequenceNode && override.Kind == yaml.SequenceNode {
		overrideSequenceNodes(root, override)
		return
	}

	// For scalar nodes, override replaces root
	if override.Kind == yaml.ScalarNode {
		root.Value = override.Value
		root.Tag = override.Tag
	}
}

// mergeMappingNodes merges two mapping (object) nodes
func mergeMappingNodes(node, override *yaml.Node) {
	// Iterate through override pairs (key, value, key, value, ...)
	for i := 0; i < len(override.Content); i += 2 {
		if i+1 >= len(override.Content) {
			break
		}

		overrideKey := override.Content[i]
		overrideValue := override.Content[i+1]

		// Find matching key in root
		found := false
		for j := 0; j < len(node.Content); j += 2 {
			if j+1 >= len(node.Content) {
				break
			}

			nodeKey := node.Content[j]
			nodeValue := node.Content[j+1]

			if nodeKey.Value == overrideKey.Value {
				// Key exists, merge the values
				mergeYAMLNodes(nodeValue, overrideValue)
				found = true
				break
			}
		}

		// If key doesn't exist in node, add it
		if !found {
			node.Content = append(node.Content, overrideKey, overrideValue)
		}
	}
}

// overrideSequenceNodes intelligently handles sequence node overrides
// - If the list contains dictionaries, it merges matching dictionaries by key
// - If the list contains scalars, it replaces the entire list
func overrideSequenceNodes(node, override *yaml.Node) {
	if len(override.Content) == 0 {
		// If override is empty, clear the original list
		node.Content = override.Content
		return
	}

	// Check if the override list contains dictionaries
	containsDictionaries := false
	for _, item := range override.Content {
		if item.Kind == yaml.MappingNode {
			containsDictionaries = true
			break
		}
	}

	// If the list contains scalars or mixed types, replace entirely
	if !containsDictionaries {
		node.Content = override.Content
		return
	}

	// Handle dictionary merging within the list
	mergeSequenceDictionaries(node, override)
}

// mergeSequenceDictionaries merges dictionaries within sequence nodes
// It attempts to match dictionaries by common identifying keys
func mergeSequenceDictionaries(node, override *yaml.Node) {
	// Common keys used to identify matching dictionary items in lists
	identifyingKeys := []string{"name", "id"}

	for _, overrideItem := range override.Content {
		if overrideItem.Kind != yaml.MappingNode {
			// Skip non-dictionary items in override
			continue
		}

		// Find the identifying key/value for this override item
		overrideIdentifier := findIdentifyingKeyValue(overrideItem, identifyingKeys)

		if overrideIdentifier == nil {
			// No identifying key found, append to the list
			node.Content = append(node.Content, overrideItem)
			continue
		}

		// Look for a matching item in the original list
		matchFound := false
		for _, originalItem := range node.Content {
			if originalItem.Kind != yaml.MappingNode {
				continue
			}

			originalIdentifier := findIdentifyingKeyValue(originalItem, identifyingKeys)
			if originalIdentifier != nil &&
				originalIdentifier.key == overrideIdentifier.key &&
				originalIdentifier.value == overrideIdentifier.value {
				// Found a match, check if we should drop this item
				if shouldDropItem(overrideItem) {
					// Remove the item from the original list
					removeItemFromSequence(node, originalItem)
				} else {
					// Merge the dictionaries
					mergeMappingNodes(originalItem, overrideItem)
				}
				matchFound = true
				break
			}
		}

		// If no match found, append the new item
		if !matchFound {
			node.Content = append(node.Content, overrideItem)
		}
	}
}

// keyValuePair represents a key-value pair for identifying dictionary items
type keyValuePair struct {
	key   string
	value string
}

// findIdentifyingKeyValue finds the first identifying key-value pair in a mapping node
func findIdentifyingKeyValue(mappingNode *yaml.Node, identifyingKeys []string) *keyValuePair {
	if mappingNode.Kind != yaml.MappingNode {
		return nil
	}

	// Iterate through the mapping node's key-value pairs
	for i := 0; i < len(mappingNode.Content); i += 2 {
		if i+1 >= len(mappingNode.Content) {
			break
		}

		keyNode := mappingNode.Content[i]
		valueNode := mappingNode.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode || valueNode.Kind != yaml.ScalarNode {
			continue
		}

		// Check if this key is one of our identifying keys
		for _, identifyingKey := range identifyingKeys {
			if keyNode.Value == identifyingKey {
				return &keyValuePair{
					key:   keyNode.Value,
					value: valueNode.Value,
				}
			}
		}
	}

	return nil
}

// shouldDropItem checks if a dictionary item should be dropped based on the _drop key
func shouldDropItem(item *yaml.Node) bool {
	if item.Kind != yaml.MappingNode {
		return false
	}

	// Look for the _drop key in the mapping
	for i := 0; i < len(item.Content); i += 2 {
		if i+1 >= len(item.Content) {
			break
		}

		keyNode := item.Content[i]
		valueNode := item.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "_drop" {
			if valueNode.Kind == yaml.ScalarNode {
				// Check if the value is "true" (string) or boolean true
				return valueNode.Value == "true" || valueNode.Tag == "!!bool" && valueNode.Value == "true"
			}
		}
	}

	return false
}

// removeItemFromSequence removes a specific item from a sequence node
func removeItemFromSequence(sequenceNode *yaml.Node, itemToRemove *yaml.Node) {
	if sequenceNode.Kind != yaml.SequenceNode {
		return
	}

	// Find the index of the item to remove
	indexToRemove := -1
	for i, item := range sequenceNode.Content {
		if item == itemToRemove {
			indexToRemove = i
			break
		}
	}

	// Remove the item if found
	if indexToRemove >= 0 {
		sequenceNode.Content = append(
			sequenceNode.Content[:indexToRemove],
			sequenceNode.Content[indexToRemove+1:]...,
		)
	}
}
