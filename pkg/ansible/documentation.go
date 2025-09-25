// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"

	"github.com/thekad/magic-ansible/pkg/api"
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
	Options map[string]*Option `yaml:"options,omitempty" json:"options,omitempty"`

	// Requirements for the module to work
	Requirements []string `yaml:"requirements,omitempty" json:"requirements,omitempty"`

	// Notes about the module
	Notes []string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// NewDocumentationFromOptions creates a new Documentation from a resource and options
func NewDocumentationFromOptions(resource *api.Resource, options map[string]*Option) *Documentation {
	urlNotes := []string{
		fmt.Sprintf("API Reference: U(%s)", resource.Mmv1.References.Api),
		fmt.Sprintf("Official Documentation: U(%s)", resource.Mmv1.References.Guides["Official Documentation"]),
	}
	resourceNotes := append(urlNotes, STANDARD_AUTH_NOTES...)
	return &Documentation{
		Module:           resource.AnsibleName(),
		ShortDescription: fmt.Sprintf("Creates a GCP %s.%s resource", resource.Parent.Mmv1.Name, resource.Mmv1.Name),
		Description:      parseDescription(resource.Mmv1.Description),
		Options:          options,
		Requirements:     STANDARD_MODULE_REQUIREMENTS,
		Notes:            resourceNotes,
	}
}

// Show the documentation as a YAML string
func (d *Documentation) ToString() string {
	return ToYAML(d)
}
