/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package diff

import (
	"bytes"
	"fmt"
	"os"

	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
	"sigs.k8s.io/yaml"
)

// YAMLDiffer provides semantic YAML diffing using the dyff library.
type YAMLDiffer struct {
	// IgnoreOrderChanges ignores changes in array order.
	IgnoreOrderChanges bool
}

// NewYAMLDiffer creates a new YAMLDiffer.
func NewYAMLDiffer() *YAMLDiffer {
	return &YAMLDiffer{
		IgnoreOrderChanges: false,
	}
}

// DiffYAML compares two YAML byte slices and returns a diff report.
func (d *YAMLDiffer) DiffYAML(from, to []byte) (*dyff.Report, error) {
	// Create input files using ytbx
	fromInput, err := ytbx.LoadDocuments(from)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source YAML: %w", err)
	}

	toInput, err := ytbx.LoadDocuments(to)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target YAML: %w", err)
	}

	fromFile := ytbx.InputFile{
		Location:  "source",
		Documents: fromInput,
	}
	toFile := ytbx.InputFile{
		Location:  "target",
		Documents: toInput,
	}

	// Compare
	report, err := dyff.CompareInputFiles(fromFile, toFile)
	if err != nil {
		return nil, fmt.Errorf("failed to compare YAML: %w", err)
	}

	return &report, nil
}

// DiffYAMLFiles compares two YAML files and returns a diff report.
func (d *YAMLDiffer) DiffYAMLFiles(fromPath, toPath string) (*dyff.Report, error) {
	fromBytes, err := os.ReadFile(fromPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read source file %s: %w", fromPath, err)
	}

	toBytes, err := os.ReadFile(toPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read target file %s: %w", toPath, err)
	}

	return d.DiffYAML(fromBytes, toBytes)
}

// FormatReport formats a dyff report as a human-readable string.
func (d *YAMLDiffer) FormatReport(report *dyff.Report) string {
	if report == nil || len(report.Diffs) == 0 {
		return "No differences found"
	}

	var buf bytes.Buffer
	writer := &dyff.HumanReport{
		Report:            *report,
		DoNotInspectCerts: true,
		NoTableStyle:      false,
		OmitHeader:        true,
	}

	if err := writer.WriteReport(&buf); err != nil {
		return fmt.Sprintf("Error formatting report: %v", err)
	}

	return buf.String()
}

// HasDiffs returns true if the report contains differences.
func (d *YAMLDiffer) HasDiffs(report *dyff.Report) bool {
	return report != nil && len(report.Diffs) > 0
}

// ConvertToDiffResult converts a dyff report to PropertyDiffs.
func (d *YAMLDiffer) ConvertToDiffResult(report *dyff.Report) []PropertyDiff {
	var diffs []PropertyDiff

	if report == nil {
		return diffs
	}

	for _, diff := range report.Diffs {
		path := diff.Path.String()

		for _, detail := range diff.Details {
			propDiff := PropertyDiff{
				Path: path,
			}

			switch detail.Kind {
			case dyff.ADDITION:
				propDiff.Change = ChangeAdded
				propDiff.NewValue = detail.To
			case dyff.REMOVAL:
				propDiff.Change = ChangeRemoved
				propDiff.OldValue = detail.From
			case dyff.MODIFICATION:
				propDiff.Change = ChangeModified
				propDiff.OldValue = detail.From
				propDiff.NewValue = detail.To
			}

			diffs = append(diffs, propDiff)
		}
	}

	return diffs
}

// DiffMaps compares two maps and returns the differences.
func DiffMaps(from, to map[string]any) ([]PropertyDiff, error) {
	fromYAML, err := yaml.Marshal(from)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal source: %w", err)
	}

	toYAML, err := yaml.Marshal(to)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal target: %w", err)
	}

	differ := NewYAMLDiffer()
	report, err := differ.DiffYAML(fromYAML, toYAML)
	if err != nil {
		return nil, err
	}

	return differ.ConvertToDiffResult(report), nil
}

// DiffStrings compares two YAML strings and returns the differences.
func DiffStrings(from, to string) ([]PropertyDiff, string, error) {
	differ := NewYAMLDiffer()
	report, err := differ.DiffYAML([]byte(from), []byte(to))
	if err != nil {
		return nil, "", err
	}

	diffs := differ.ConvertToDiffResult(report)
	output := differ.FormatReport(report)

	return diffs, output, nil
}
