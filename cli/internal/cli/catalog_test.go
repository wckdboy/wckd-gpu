package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectsAndPresetsWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	presets := filepath.Join(dir, "presets")
	if err := os.Mkdir(presets, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: generic-cuda
  name: Generic CUDA
  version: 1
spec:
  workload_class: generic
  constraints:
    min_vram_gb: 8
    reliability: any
  runtime:
    image: "runpod/pytorch:test"
    ports:
      - name: ui
        container: 22
        expose: tcp
`)
	if err := os.WriteFile(filepath.Join(presets, "generic-cuda.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	t.Setenv("HOME", dir)
	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "")
	t.Setenv("WCKD_S3_ENDPOINT", "")
	t.Setenv("WCKD_S3_BUCKET", "")
	t.Setenv("WCKD_S3_ACCESS_KEY", "")
	t.Setenv("WCKD_S3_SECRET_KEY", "")
	t.Setenv("WCKD_STATE_DIR", state)
	t.Setenv("WCKD_PRESETS_DIR", presets)
	t.Setenv("WCKD_PROJECT_ID", "")
	t.Setenv("WCKD_PRESET_ID", "generic-cuda")

	out := &bytes.Buffer{}
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		out.Reset()
		app := &App{Stdout: out, Stderr: out}
		cmd := app.Command()
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	listed := run(t, "presets", "--json")
	var presetsJSON []map[string]any
	if err := json.Unmarshal([]byte(listed), &presetsJSON); err != nil {
		t.Fatal(err)
	}
	if len(presetsJSON) != 1 || presetsJSON[0]["id"] != "generic-cuda" {
		t.Fatalf("presets %#v", presetsJSON)
	}

	if _, err := os.ReadFile(filepath.Join(state, "projects.json")); err == nil {
		t.Fatal("projects file exists before add")
	}
	added := run(t, "projects", "add", "hailuo-tests", "--json")
	var one map[string]any
	if err := json.Unmarshal([]byte(added), &one); err != nil {
		t.Fatal(err)
	}
	if one["id"] != "hailuo-tests" || one["saved"] != true || one["sessions"] != float64(0) {
		t.Fatalf("add %#v", one)
	}
	again := run(t, "projects", "--json")
	if !strings.Contains(again, "hailuo-tests") {
		t.Fatalf("list:\n%s", again)
	}

	out.Reset()
	app := &App{Stdout: out, Stderr: out}
	cmd := app.Command()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"projects", "add", "../etc"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("invalid id was accepted")
	}
}

func TestConfigCheckJSONMissingCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "")
	t.Setenv("WCKD_S3_ENDPOINT", "")
	t.Setenv("WCKD_S3_BUCKET", "")
	t.Setenv("WCKD_S3_ACCESS_KEY", "")
	t.Setenv("WCKD_S3_SECRET_KEY", "")
	t.Setenv("WCKD_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("WCKD_PRESETS_DIR", dir)

	buf := &bytes.Buffer{}
	app := &App{Stdout: buf, Stderr: buf}
	cmd := app.Command()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "check", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing credentials")
	}
	var report struct {
		OK      bool     `json:"ok"`
		Missing []string `json:"missing"`
		RunPod  struct {
			Skipped bool `json:"skipped"`
		} `json:"runpod"`
	}
	if jerr := json.Unmarshal(buf.Bytes(), &report); jerr != nil {
		t.Fatalf("json: %v\n%s", jerr, buf.String())
	}
	if report.OK || !report.RunPod.Skipped || len(report.Missing) < 2 {
		t.Fatalf("report %#v\n%s", report, buf.String())
	}
}
