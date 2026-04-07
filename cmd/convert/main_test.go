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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ionos-cloud/cluster-api-provider-proxmox/pkg/convert"
)

const configMapYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`

const proxmoxClusterV1Alpha1YAML = `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: ProxmoxCluster
metadata:
  name: test
spec:
  controlPlaneEndpoint:
    host: 10.0.0.1
    port: 6443
`

const testfile = "test.yaml"

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	return path
}

func TestStderrWarn(_ *testing.T) {
	w := convert.Warning{
		File:    testfile,
		Kind:    "passthrough",
		Message: "test warning",
	}
	stderrWarn(w)
}

func TestExecute_Success(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, configMapYAML)

	oldArgs := os.Args
	os.Args = []string{"convert", "-f", path}
	defer func() { os.Args = oldArgs }()

	code := execute()
	if code != 0 {
		t.Errorf("execute() = %d, want 0", code)
	}
}

func TestExecute_Error(t *testing.T) {
	r, w, _ := os.Pipe()
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	oldArgs := os.Args
	os.Args = []string{"convert", "-i", ".bak"}
	defer func() { os.Args = oldArgs }()

	code := execute()
	if code != 1 {
		t.Errorf("execute() = %d, want 1", code)
	}
}

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "convert" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short should not be empty")
	}
	if cmd.Flags().Lookup("filename") == nil {
		t.Error("missing --filename flag")
	}
	if cmd.Flags().Lookup("in-place") == nil {
		t.Error("missing --in-place flag")
	}
}

func TestNewCommand_FileMode(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, proxmoxClusterV1Alpha1YAML)

	cmd := newCommand()
	cmd.SetArgs([]string{"-f", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestNewCommand_InPlace(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, proxmoxClusterV1Alpha1YAML)

	cmd := newCommand()
	cmd.SetArgs([]string{"-f", path, "-i", ".bak"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v1alpha2") {
		t.Error("file should be converted")
	}

	backup, err := os.ReadFile(path + ".bak") //nolint:gosec // test //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(backup), "v1alpha1") {
		t.Error("backup should contain original")
	}
}

func TestNewCommand_InPlaceNoSuffix(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, proxmoxClusterV1Alpha1YAML)

	cmd := newCommand()
	cmd.SetArgs([]string{"-f", path, "-i", ""})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v1alpha2") {
		t.Error("file should be converted in-place")
	}
	if _, err := os.Stat(path + ".bak"); err == nil {
		t.Error("no backup file should be created with empty suffix")
	}
}

func TestNewCommand_InPlaceWithoutFile(t *testing.T) {
	cmd := newCommand()
	cmd.SetArgs([]string{"-i", ".bak"})

	r, w, _ := os.Pipe()
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for --in-place without --filename")
	}
}

func TestNewCommand_Stdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.Write([]byte(proxmoxClusterV1Alpha1YAML))
		w.Close()
	}()

	cmd := newCommand()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute stdin: %v", err)
	}
}

func TestRun_InPlaceRequiresFilename(t *testing.T) {
	err := run(nil, "", true)
	if err == nil {
		t.Fatal("expected error for --in-place without --filename")
	}
	if !strings.Contains(err.Error(), "--in-place requires --filename") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_FileMode(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, configMapYAML)

	if err := run([]string{path}, "", false); err != nil {
		t.Fatalf("run file mode: %v", err)
	}
}

func TestRun_FileMode_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	path1 := writeFixture(t, dir, "a.yaml", configMapYAML)
	path2 := writeFixture(t, dir, "b.yaml", configMapYAML)

	if err := run([]string{path1, path2}, "", false); err != nil {
		t.Fatalf("run multiple files: %v", err)
	}
}

func TestRun_InPlace(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, proxmoxClusterV1Alpha1YAML)

	if err := run([]string{path}, "", true); err != nil {
		t.Fatalf("run in-place: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "v1alpha1") {
		t.Error("in-place file should be converted to v1alpha2")
	}
	if !strings.Contains(string(data), "v1alpha2") {
		t.Error("in-place file should contain v1alpha2")
	}
}

func TestRun_InPlaceWithBackup(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, proxmoxClusterV1Alpha1YAML)

	if err := run([]string{path}, ".bak", true); err != nil {
		t.Fatalf("run in-place with backup: %v", err)
	}

	backup, err := os.ReadFile(path + ".bak") //nolint:gosec // test
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	if string(backup) != proxmoxClusterV1Alpha1YAML {
		t.Error("backup should contain original content")
	}

	data, err := os.ReadFile(path) //nolint:gosec // test
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v1alpha2") {
		t.Error("converted file should contain v1alpha2")
	}
}

func TestRun_FileNotFound(t *testing.T) {
	if err := run([]string{"/nonexistent/file.yaml"}, "", false); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRun_Stdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.Write([]byte(configMapYAML))
		w.Close()
	}()

	if err = run(nil, "", false); err != nil {
		t.Fatalf("run stdin: %v", err)
	}
}

func TestRun_StdinConversion(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.Write([]byte(proxmoxClusterV1Alpha1YAML))
		w.Close()
	}()

	if err = run(nil, "", false); err != nil {
		t.Fatalf("run stdin conversion: %v", err)
	}
}

func TestRun_BackupWriteError(t *testing.T) {
	path := writeFixture(t, t.TempDir(), testfile, configMapYAML)

	err := run([]string{path}, "/nonexistent/dir/.bak", true)
	if err == nil {
		t.Fatal("expected error writing backup to nonexistent dir")
	}
	if !strings.Contains(err.Error(), "writing backup") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_InPlaceWriteError(t *testing.T) {
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "subdir", "nested", testfile)
	if err := run([]string{fakePath}, "", true); err == nil {
		t.Fatal("expected error for file in nonexistent directory")
	}
}
