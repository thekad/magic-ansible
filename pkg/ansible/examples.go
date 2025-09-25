// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	mmv1resource "github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
)

type ExampleBlock struct {
	Examples []mmv1resource.Examples `yaml:"examples" json:"examples"`
}

func NewExampleBlockFromMmv1(resource *mmv1api.Resource) *ExampleBlock {
	return &ExampleBlock{
		Examples: resource.Examples,
	}
}

func (e *ExampleBlock) ToString() string {
	separator := fmt.Sprintf("%s\n", strings.Repeat("#", 80))
	examples := []string{}
	for _, example := range e.Examples {
		examples = append(examples, example.TestHCLText)
	}
	return strings.Join(examples, separator)
}
