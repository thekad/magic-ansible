// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

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

// overrideSequenceNodes overrides the content of the node with the content of the override node
// this is different from the other merge functions because there's no easy way to remove items from a sequence
func overrideSequenceNodes(node, override *yaml.Node) {
	node.Content = override.Content
}
