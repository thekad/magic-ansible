// Copyright 2025 Red Hat Inc.
// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"github.com/rs/zerolog/log"
	"github.com/thekad/magic-ansible/pkg/api"
)

type TemplateData struct {
	TemplateDirectory        string
	OutputFolder             string
	ModuleDirectory          string
	IntegrationTestDirectory string
	OverWrite                bool
}

func NewTemplateData(templateDirectory, outputFolder string, overWrite bool) *TemplateData {
	absTemplateDirectory, err := filepath.Abs(templateDirectory)
	if err != nil {
		log.Panic().Err(err)
	}
	absOutputFolder, err := filepath.Abs(outputFolder)
	if err != nil {
		log.Panic().Err(err)
	}
	return &TemplateData{
		TemplateDirectory:        absTemplateDirectory,
		OutputFolder:             absOutputFolder,
		ModuleDirectory:          path.Join(absOutputFolder, "plugins", "modules"),
		IntegrationTestDirectory: path.Join(absOutputFolder, "tests", "integration", "targets"),
		OverWrite:                overWrite,
	}
}

func (td *TemplateData) executeTemplate(templateName string, input any) (bytes.Buffer, error) {
	contents := bytes.Buffer{}

	templatePath := path.Join(td.TemplateDirectory, templateName)
	tpls := []string{
		path.Join(td.TemplateDirectory, "base", "fragments.tmpl"),
		templatePath,
	}

	tmpl, err := template.New(filepath.Base(templateName)).Funcs(funcMap()).ParseFiles(tpls...)
	if err != nil {
		return contents, err
	}

	if err = tmpl.ExecuteTemplate(&contents, filepath.Base(templateName), input); err != nil {
		return contents, err
	}

	return contents, nil
}

func (td *TemplateData) writeFile(filePath, templateName string, input any) error {
	log.Debug().Msgf("writing file: %s", filePath)
	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	if fileExists(filePath) {
		if !td.OverWrite {
			return fmt.Errorf("file already exists: %s", filePath)
		} else {
			log.Warn().Msgf("file already exists: %s", filePath)
		}
	}

	contents, err := td.executeTemplate(templateName, input)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, contents.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

func (td *TemplateData) GenerateCode(resource *api.Resource) error {
	log.Info().Msgf("generating code for resource: %s", resource.AnsibleName())

	if err := os.MkdirAll(td.ModuleDirectory, 0755); err != nil {
		return fmt.Errorf("error creating module directory: %v", err)
	}

	moduleFile := path.Join(td.ModuleDirectory, fmt.Sprintf("%s.py", resource.AnsibleName()))

	if err := td.writeFile(moduleFile, "plugins/module.tmpl", resource); err != nil {
		return fmt.Errorf("error generating module file: %v", err)
	}

	return nil
}

func (td *TemplateData) GenerateTests(resource *api.Resource) error {
	log.Info().Msgf("generating tests for resource: %s", resource.AnsibleName())

	if err := os.MkdirAll(td.IntegrationTestDirectory, 0755); err != nil {
		return fmt.Errorf("error creating integration test directory: %v", err)
	}

	return nil
}

// fileExists returns true if the given path exists, false otherwise
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
