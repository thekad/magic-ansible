// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	mmv1resource "github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
	"github.com/GoogleCloudPlatform/magic-modules/mmv1/google"
	"github.com/rs/zerolog/log"
	"github.com/thekad/magic-ansible/pkg/api"
)

type Module struct {
	Name             string
	Resource         *api.Resource
	MinVersion       string
	Options          map[string]*Option
	Documentation    *Documentation
	Returns          *ReturnBlock
	Examples         *Examples
	ArgumentSpec     *ArgumentSpec
	OperationConfigs map[string]*OperationConfig
	Dependencies     *Dependencies
}

// NewFromResource creates a new Module from an API Resource
// The rule of thumb for this constructor is to build the options, examples,
// returns, and operation configs from the Mmv1 API Resource object, and then
// build the rest of the members based off the options.
func NewFromResource(resource *api.Resource) *Module {
	m := &Module{
		Name:             resource.AnsibleName(),
		Resource:         resource,
		Options:          NewOptionsFromMmv1(resource.Mmv1),
		Examples:         NewExamplesFromMmv1(resource.Mmv1),
		Returns:          NewReturnBlockFromMmv1(resource.Mmv1),
		OperationConfigs: NewOperationConfigsFromMmv1(resource.Mmv1),
	}
	m.Dependencies = getDependency(m.Options)

	// filter the options to only include input options
	inputOptions := make(map[string]*Option, 0)
	for _, option := range m.Options {
		if option.OutputOnly() {
			continue
		}
		inputOptions[option.AnsibleName()] = option
	}

	log.Info().Msgf("creating documentation for %s", resource.AnsibleName())
	m.Documentation = NewDocumentationFromOptions(resource, inputOptions)

	log.Info().Msgf("creating argument spec for %s", resource.AnsibleName())
	m.ArgumentSpec = NewArgSpecFromOptions(inputOptions, m.Dependencies)

	return m
}

func (m *Module) String() string {
	return m.Resource.AnsibleName()
}

// CustomCode returns the custom code (if any) defined in the API Resource YAML file
func (m *Module) CustomCode() mmv1resource.CustomCode {
	return m.Resource.Mmv1.CustomCode
}

func (m *Module) UrlParamOnlyProperties() []*mmv1api.Type {
	return google.Select(m.Resource.Mmv1.AllUserProperties(), func(p *mmv1api.Type) bool {
		return p.UrlParamOnly
	})
}

func (m *Module) BaseUrl() string {
	productVersions := m.Resource.Parent.Mmv1.Versions
	for _, version := range productVersions {
		if version.Name == m.Resource.MinVersion() {
			return version.BaseUrl
		}
	}

	return ""
}

func (m *Module) ModuleClass() string {
	return google.Camelize(m.Resource.Parent.Mmv1.Name, "upper")
}

func (m *Module) Kind() string {
	return fmt.Sprintf("%s#%s", google.Camelize(m.Resource.Parent.Name, "lower"), google.Camelize(m.Resource.Mmv1.Name, "lower"))
}

func (m *Module) Scopes() []string {
	return m.Resource.Parent.Mmv1.Scopes
}

func (m *Module) GetAsync() *mmv1api.Async {
	return m.Resource.Mmv1.GetAsync()
}

func (m *Module) ProductName() string {
	return m.Resource.Parent.Mmv1.Name
}

func (m *Module) AllMmv1BodyOptions() []*Option {
	opts := make([]*Option, 0)
	for _, option := range sortedOptions(m.Options) {
		// we only care about options that have an mmv1 attached to them
		if option.Mmv1 == nil || option.Mmv1.UrlParamOnly {
			continue
		}
		opts = append(opts, option)
	}
	return opts
}

func (m *Module) OutputOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return o.OutputOnly()
	})
}

func (m *Module) InputOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return !o.OutputOnly()
	})
}

func (m *Module) UrlParamOnlyOptions() []*Option {
	return google.Select(m.AllMmv1BodyOptions(), func(o *Option) bool {
		return o.Mmv1.UrlParamOnly
	})
}

func (m *Module) AllNestedOptions() map[string]*Option {
	nestedOptions := make(map[string]*Option)

	// Start with top-level options
	for _, option := range m.AllMmv1BodyOptions() {
		collectNestedOptions(option, nestedOptions)
	}

	return nestedOptions
}

// collectNestedOptions recursively collects all nested object options
func collectNestedOptions(option *Option, result map[string]*Option) {
	// If this option is a nested object or a list of nested objects, add it to the result
	if option.IsNestedObject() || option.IsNestedList() {
		result[option.ClassName()] = option
	}

	// Recursively check suboptions
	if option.Suboptions != nil {
		for _, suboption := range option.Suboptions {
			collectNestedOptions(suboption, result)
		}
	}
}
