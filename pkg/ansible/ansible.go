// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
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
	Api          *api.Product
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
		Api:          &api.Product{},
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
	yamlValidator.Parse(patchedData, p.Api, p.File)

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
	Name          string
	File          string
	Api           *api.Resource
	Parent        *Product
	TemplateDir   string
	OverridesDir  string
	Documentation *Documentation
}

// NewResource is a constructor that returns an initialized Resource type
func NewResource(yamlPath string, parent *Product, templateDir string, overridesDir string) *Resource {
	name := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml")

	return &Resource{
		Name:          name,
		File:          yamlPath,
		Api:           &api.Resource{},
		Parent:        parent,
		TemplateDir:   templateDir,
		OverridesDir:  overridesDir,
		Documentation: &Documentation{},
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
	yamlValidator.Parse(patchedData, r.Api, r.File)

	// once it has successfully unmarshaled, we can generate more pieces of the struct
	// generate the documentation
	r.Documentation = NewDocumentationFromAPI(r)

	return nil
}

func (r *Resource) ToLower() string {
	return strings.ToLower(r.Name)
}

func (r *Resource) AnsibleName() string {
	return fmt.Sprintf("%s_%s", r.Parent.AnsibleName(), google.Underscore(r.Name))
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
