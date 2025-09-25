// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Configurable is generic interface for both Product and Resource
type Configurable interface {
	Unmarshal() error
	ToLower() string
	AnsibleName() string
}

// Product is a representation of a directory in the mmv1/products directory
// from the magic-modules clone e.g. mmv1/products/<product>/product.yaml
type Product struct {
	Name         string
	File         string
	ApiPtr       *mmv1api.Product
	Resources    []*Resource
	TemplateDir  string
	OverridesDir string
}

// NewProduct is a constructor that returns an initialized Product type
func NewProduct(yamlPath string, templateDir string, overridesDir string) *Product {
	name := filepath.Base(filepath.Dir(yamlPath))
	return &Product{
		Name:         strings.ToLower(name),
		File:         yamlPath,
		ApiPtr:       &mmv1api.Product{},
		TemplateDir:  templateDir,
		OverridesDir: overridesDir,
	}
}

func (p *Product) Unmarshal() error {
	yamlData, err := os.ReadFile(p.File)
	if err != nil {
		return fmt.Errorf("cannot open product file: %v", p.File)
	}

	rootNode := yaml.Node{}
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return fmt.Errorf("cannot unmarshal product file: %v", p.File)
	}
	p.ApplyOverrides(&rootNode)

	// marshal the patched data back into a string
	patchedData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return fmt.Errorf("error marshaling patched data: %v", err)
	}

	// load main product file
	yamlValidator := google.YamlValidator{}
	yamlValidator.Parse(patchedData, p.ApiPtr, p.File)

	return nil
}

// AnsibleName will return a properly formatted Ansible name for the given product
func (p *Product) AnsibleName() string {
	return fmt.Sprintf("gcp_%s", google.Underscore(p.Name))
}

// ApplyOverrides will apply our overrides for the given product
func (p *Product) ApplyOverrides(rootNode *yaml.Node) {
	overrideYAML(rootNode, p.OverridesDir, p.File)
}

// Resource is a representation of a file found in the products directory
// from magic-modules clone e.g. mmv1/products/<product>/<resource>.yaml
type Resource struct {
	Name         string
	File         string
	ApiPtr       *mmv1api.Resource
	Parent       *Product
	TemplateDir  string
	OverridesDir string
}

// NewResource is a constructor that returns an initialized Resource type
func NewResource(yamlPath string, parent *Product, templateDir string, overridesDir string) *Resource {
	name := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml")

	return &Resource{
		Name:         name,
		File:         yamlPath,
		ApiPtr:       &mmv1api.Resource{},
		Parent:       parent,
		TemplateDir:  templateDir,
		OverridesDir: overridesDir,
	}
}

func (r *Resource) Unmarshal() error {
	yamlData, err := os.ReadFile(r.File)
	if err != nil {
		return fmt.Errorf("cannot open resource file: %v", r.File)
	}

	// TODO: Patch the generic YAML struct so all examples point to *our* templateDir
	// then marshal it back again so it can be unmarshaled *yet again* with the
	// right template paths. This is because of the custom UnmarshalYAML defined
	// upstream for product/resource/example which renders the templates when
	// the YAML is unmarshaled into a struct. Sigh - @thekad
	rootNode := yaml.Node{}
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return fmt.Errorf("cannot unmarshal file: %v", r.File)
	}
	r.ApplyOverrides(&rootNode)
	r.patchExamples(&rootNode)

	// marshal the patched data back into a string
	patchedData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return fmt.Errorf("error marshaling patched data: %v", err)
	}

	// finally load into the actual resource struct and unmarshal
	yamlValidator := google.YamlValidator{}
	yamlValidator.Parse(patchedData, r.ApiPtr, r.File)

	return nil
}

func (r *Resource) ToLower() string {
	return strings.ToLower(r.Name)
}

func (r *Resource) AnsibleName() string {
	return fmt.Sprintf("%s_%s", r.Parent.AnsibleName(), google.Underscore(r.Name))
}

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

// ApplyOverrides will apply our overrides for the given resource
func (r *Resource) ApplyOverrides(rootNode *yaml.Node) {
	overrideYAML(rootNode, r.OverridesDir, r.File)
}

// patchExamples will update the config_path for each item in the examples list
func (r *Resource) patchExamples(rootNode *yaml.Node) {
	// patch examples' config_path
	examplesNode := findNodeByKey(rootNode, "examples")
	if examplesNode == nil || examplesNode.Kind != yaml.SequenceNode {
		// Return immediately if the node is not found or is not a list
		return
	}
	log.Debug().Msgf("patching examples for resource: %s.%s", r.Parent.Name, r.Name)

	pathPrefix := path.Join(r.TemplateDir, "examples")
	keyToUpdate := "config_path"

	// Iterate over each item in the examples list.
	for _, exampleMapNode := range examplesNode.Content {
		var name string
		var valueToSet string

		if exampleMapNode.Kind == yaml.MappingNode {
			found := false

			// Find the example name, gotta loop twice because we first have to find the name :(
			for i := 0; i < len(exampleMapNode.Content); i += 2 {
				keyNode := exampleMapNode.Content[i]
				valueNode := exampleMapNode.Content[i+1]

				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" {
					name = valueNode.Value
					break
				}
			}

			// load the same template as the example name
			valueToSet = path.Join(pathPrefix, fmt.Sprintf("%s.tmpl", name))

			// Now find the key to update
			for i := 0; i < len(exampleMapNode.Content); i += 2 {
				keyNode := exampleMapNode.Content[i]
				valueNode := exampleMapNode.Content[i+1]

				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == keyToUpdate {
					valueNode.Value = valueToSet
					found = true
					break
				}
			}

			// If the key wasn't found, append it to the map.
			if !found {
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: keyToUpdate}
				valueNode := &yaml.Node{Kind: yaml.ScalarNode, Value: valueToSet}
				exampleMapNode.Content = append(exampleMapNode.Content, keyNode, valueNode)
			}
		}
	}
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
