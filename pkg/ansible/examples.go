// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"fmt"
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	mmv1resource "github.com/GoogleCloudPlatform/magic-modules/mmv1/api/resource"
)

type Examples struct {
	DocExamples  []mmv1resource.Examples
	TestExamples []mmv1resource.Examples
}

func NewExamplesFromMmv1(mmv1 *mmv1api.Resource) *Examples {
	docExamples := []mmv1resource.Examples{}
	testExamples := []mmv1resource.Examples{}
	for _, example := range mmv1.Examples {
		if !example.ExcludeDocs {
			docExamples = append(docExamples, example)
		}
		if !example.ExcludeTest {
			testExamples = append(testExamples, example)
		}
	}
	return &Examples{
		DocExamples:  docExamples,
		TestExamples: testExamples,
	}
}

func (e *Examples) ToString(which string) string {
	separator := fmt.Sprintf("\n%s\n\n", strings.Repeat("#", 80))
	exampleStrings := []string{}
	examples := []mmv1resource.Examples{}
	switch which {
	case "doc":
		examples = e.DocExamples
	case "test":
		examples = e.TestExamples
	}
	for _, example := range examples {
		exampleStrings = append(exampleStrings, example.TestHCLText)
	}
	return strings.Join(exampleStrings, separator)
}
