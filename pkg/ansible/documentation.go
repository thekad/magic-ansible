// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	"github.com/thekad/magic-ansible/pkg/api"
)

var STANDARD_MODULE_REQUIREMENTS = []string{
	"python >= 3.8",
	"requests >= 2.18.4",
	"google-auth >= 2.25.1",
}

// Documentation represents the complete module specification
type Documentation struct {
	// Module name - must match the filename without .py extension
	Module string `yaml:"module"`

	// Short description displayed in ansible-doc -l
	ShortDescription string `yaml:"short_description"`

	// Detailed description - string or list of strings
	Description []string `yaml:"description"`

	// Author information - string or list of strings
	Author []string `yaml:"author,omitempty"`

	// Module options
	Options map[string]*Option `yaml:"options,omitempty"`

	// Requirements for the module to work
	Requirements []string `yaml:"requirements,omitempty"`

	// Notes about the module
	Notes []string `yaml:"notes,omitempty"`

	// DocFragments are fragments of shared documentation that will be included in the documentation
	DocFragments []string `yaml:"extends_documentation_fragment,omitempty"`
}

// NewDocumentationFromOptions creates a new Documentation from a resource and options
func NewDocumentationFromOptions(resource *api.Resource, options map[string]*Option) *Documentation {
	resourceNotes := []string{
		fmt.Sprintf("API Reference: U(%s)", resource.Mmv1.References.Api),
		fmt.Sprintf("Official Documentation: U(%s)", resource.Mmv1.References.Guides["Official Documentation"]),
	}
	docFragments := []string{
		"google.cloud.gcp",
	}
	return &Documentation{
		Module:           resource.AnsibleName(),
		Author:           []string{"Google Inc. (@googlecloudplatform)"},
		ShortDescription: fmt.Sprintf("Creates a GCP %s.%s resource", resource.Parent.Mmv1.Name, resource.Mmv1.Name),
		Description:      cleanModuleDescription(resource.Mmv1.Description),
		Options:          options,
		Requirements:     STANDARD_MODULE_REQUIREMENTS,
		Notes:            resourceNotes,
		DocFragments:     docFragments,
	}
}

func cleanModuleDescription(description string) []string {
	var cleanLines []string
	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	return cleanLines
}

// Show the documentation as a YAML string
func (d *Documentation) ToString() string {
	return ToYAML(d)
}
