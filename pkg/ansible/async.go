// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
)

type AsyncOps struct {
	LinkTemplate string         `json:"base_url"`
	Timeouts     map[string]int `json:"timeouts"`
	Actions      []string       `json:"actions"`
}

// NewAsyncOps creates a custom AsyncOps struct from a link template and timeouts
// which is easier to just serialize to JSON in the template
func NewAsyncOps(linkTemplate string, actions []string, timeouts *mmv1api.Timeouts) *AsyncOps {
	r := &AsyncOps{
		LinkTemplate: strings.ReplaceAll(strings.ReplaceAll(linkTemplate, "{{", "{"), "}}", "}"),
		Actions:      actions,
		Timeouts: map[string]int{
			"create": timeouts.InsertMinutes * 60,
			"delete": timeouts.DeleteMinutes * 60,
			"update": timeouts.UpdateMinutes * 60,
		},
	}

	return r
}
