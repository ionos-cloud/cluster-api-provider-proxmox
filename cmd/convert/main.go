/*
Copyright 2026 IONOS Cloud.

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

// Package main implements the convert CLI tool.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/convert"
)

func main() {
	os.Exit(execute())
}

func execute() int {
	if err := newCommand().Execute(); err != nil {
		return 1
	}
	return 0
}

func newCommand() *cobra.Command {
	var filenames []string
	var inPlace string
	var inPlaceSet bool

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert CAPMOX v1alpha1 and CAPI v1beta1 templates to v1alpha2/v1beta2",
		Long: `convert converts Cluster API Provider Proxmox (CAPMOX) YAML manifests
and templates from v1alpha1 to v1alpha2, and CAPI resources from v1beta1 to v1beta2.

It preserves envsubst variables (${VAR}) and YAML comments where possible.

Resources that are not recognized are passed through unchanged with a warning.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inPlaceSet = cmd.Flags().Changed("in-place")
			return run(filenames, inPlace, inPlaceSet)
		},
	}

	cmd.Flags().StringArrayVarP(&filenames, "filename", "f", nil, "input file(s) to convert (reads stdin if omitted)")
	cmd.Flags().StringVarP(&inPlace, "in-place", "i", "", "edit file(s) in-place; optional suffix for backup (e.g. -i.bak)")

	return cmd
}

// stderrWarn prints a warning to stderr immediately.
func stderrWarn(w convert.Warning) {
	fmt.Fprintln(os.Stderr, w.String())
}

func run(filenames []string, inPlaceSuffix string, inPlaceSet bool) error {
	if inPlaceSet && len(filenames) == 0 {
		return fmt.Errorf("--in-place requires --filename")
	}

	if len(filenames) == 0 {
		return runStdin()
	}

	multiFile := len(filenames) > 1
	for _, filename := range filenames {
		if err := processFile(filename, inPlaceSuffix, inPlaceSet, multiFile); err != nil {
			return err
		}
	}

	return nil
}

func runStdin() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	out, err := convert.Convert(data, convert.Options{
		Filename: "<stdin>",
		Warn:     stderrWarn,
	})
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

func processFile(filename, inPlaceSuffix string, inPlaceSet, multiFile bool) error {
	data, err := os.ReadFile(filename) //nolint:gosec // CLI tool reads user-specified files
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}

	out, err := convert.Convert(data, convert.Options{
		Filename: filename,
		Warn:     stderrWarn,
	})
	if err != nil {
		return fmt.Errorf("converting %s: %w", filename, err)
	}

	if inPlaceSet {
		return writeInPlace(filename, data, out, inPlaceSuffix)
	}

	if multiFile {
		fmt.Fprintf(os.Stderr, "--- %s ---\n", filename)
	}
	_, err = os.Stdout.Write(out)
	return err
}

func writeInPlace(filename string, original, converted []byte, backupSuffix string) error {
	if backupSuffix != "" {
		backupName := filename + backupSuffix
		if err := os.WriteFile(backupName, original, 0o644); err != nil { //nolint:gosec // match source file permissions
			return fmt.Errorf("writing backup %s: %w", backupName, err)
		}
	}
	if err := os.WriteFile(filename, converted, 0o644); err != nil { //nolint:gosec // match source file permissions
		return fmt.Errorf("writing %s: %w", filename, err)
	}
	return nil
}
