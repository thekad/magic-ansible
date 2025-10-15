// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package ansible

import (
	"strings"

	mmv1api "github.com/GoogleCloudPlatform/magic-modules/mmv1/api"
	"github.com/rs/zerolog/log"
)

type OperationConfig struct {
	UriTemplate      string `json:"uri"`
	AsyncUriTemplate string `json:"async_uri"`
	Verb             string `json:"verb"`
	TimeoutMinutes   int    `json:"timeout_minutes"`
}

func NewOperationConfigsFromMmv1(mmv1 *mmv1api.Resource) map[string]*OperationConfig {
	ops := map[string]*OperationConfig{}
	timeouts := mmv1.GetTimeouts()
	defaultVerbs := map[string]string{
		"read":   "GET",
		"create": "POST",
		"update": "PUT",
		"delete": "DELETE",
	}

	// Helper function to get verb or default
	getVerb := func(mmv1Verb, operation string) string {
		if mmv1Verb != "" {
			return mmv1Verb
		}
		return defaultVerbs[operation]
	}

	escapeCurlyBraces := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "{{", "{"), "}}", "}")
	}

	ops["read"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.SelfLinkUri()),
		Verb:             getVerb(mmv1.ReadVerb, "read"),
		AsyncUriTemplate: "",
	}
	ops["create"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.CreateUri()),
		Verb:             getVerb(mmv1.CreateVerb, "create"),
		TimeoutMinutes:   timeouts.InsertMinutes,
		AsyncUriTemplate: "",
	}
	ops["update"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.UpdateUri()),
		Verb:             getVerb(mmv1.UpdateVerb, "update"),
		TimeoutMinutes:   timeouts.UpdateMinutes,
		AsyncUriTemplate: "",
	}
	ops["delete"] = &OperationConfig{
		UriTemplate:      escapeCurlyBraces(mmv1.DeleteUri()),
		Verb:             getVerb(mmv1.DeleteVerb, "delete"),
		TimeoutMinutes:   timeouts.DeleteMinutes,
		AsyncUriTemplate: "",
	}

	async := mmv1.GetAsync()
	if async != nil {
		for _, action := range async.Actions {
			ops[strings.ToLower(action)].AsyncUriTemplate = escapeCurlyBraces(async.Operation.BaseUrl)
		}
	}

	log.Debug().Msgf("operation configs: %v", ops)

	return ops
}
