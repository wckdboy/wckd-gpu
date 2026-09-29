package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wckd.yaml")
	body := []byte(`
runpod:
  api_key: file-key
  base_url: https://example.test
s3:
  endpoint: https://file.example
  bucket: file-bucket
  access_key: file-ak
  secret_key: file-sk
  region: eu
defaults:
  hours: 2
  project_id: from-file
  preset_id: comfyui-minimax-h3
state_dir: ~/from-file
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "env-key")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_S3_BUCKET", "env-bucket")
	t.Setenv("WCKD_SESSION_HOURS", "4")
	t.Setenv("WCKD_PROJECT_ID", "env-project")
	t.Setenv("WCKD_STATE_DIR", dir)
	t.Setenv("WCKD_PRESETS_DIR", filepath.Join(dir, "presets"))
	t.Setenv("WCKD_S3_PATH_STYLE", "false")
	t.Setenv("WCKD_IMAGE", "example.com/sidecar:dev")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunPod.APIKey != "env-key" {
		t.Fatalf("api key: got %q", cfg.RunPod.APIKey)
	}
	if cfg.RunPod.BaseURL != "https://example.test" {
		t.Fatalf("base url: got %q", cfg.RunPod.BaseURL)
	}
	if cfg.S3.Bucket != "env-bucket" || cfg.S3.AccessKey != "file-ak" || cfg.S3.SecretKey != "file-sk" {
		t.Fatalf("s3 merge: %+v", cfg.S3)
	}
	if cfg.S3.UsePathStyle() {
		t.Fatal("expected path style false")
	}
	if cfg.Defaults.Hours != 4 || cfg.Defaults.ProjectID != "env-project" {
		t.Fatalf("defaults: %+v", cfg.Defaults)
	}
	if cfg.StateDir != dir {
		t.Fatalf("state dir: %s", cfg.StateDir)
	}
	if cfg.Image != "example.com/sidecar:dev" {
		t.Fatalf("image: %s", cfg.Image)
	}
	if got := cfg.MissingCredentials(); len(got) != 0 {
		t.Fatalf("missing: %v", got)
	}
}

func TestLoadMissingOptionalFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_S3_ENDPOINT", "")
	t.Setenv("WCKD_S3_BUCKET", "")
	t.Setenv("WCKD_S3_ACCESS_KEY", "")
	t.Setenv("WCKD_S3_SECRET_KEY", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("WCKD_SESSION_HOURS", "")
	t.Setenv("WCKD_PROJECT_ID", "")
	t.Setenv("WCKD_PRESET_ID", "")
	t.Setenv("WCKD_STATE_DIR", dir)
	t.Setenv("WCKD_PRESETS_DIR", dir)
	t.Setenv("WCKD_IMAGE", "")
	t.Setenv("WCKD_S3_PATH_STYLE", "")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunPod.BaseURL != defaultRunPodBase {
		t.Fatalf("base: %s", cfg.RunPod.BaseURL)
	}
	if cfg.Defaults.Hours != 1 || cfg.Defaults.PresetID != defaultPresetID {
		t.Fatalf("defaults: %+v", cfg.Defaults)
	}
	missing := cfg.MissingCredentials()
	if len(missing) != 5 {
		t.Fatalf("missing %d: %v", len(missing), missing)
	}
}

func TestLoadRequiredFileMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateHoursAndIDs(t *testing.T) {
	if err := ValidateHours(1); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHours(24); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHours(0); err == nil {
		t.Fatal("expected 0 to fail")
	}
	if err := ValidateHours(25); err == nil {
		t.Fatal("expected 25 to fail")
	}
	if err := ValidateProjectID("hailuo-tests"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectID("../x"); err == nil {
		t.Fatal("expected traversal to fail")
	}
	if err := ValidateProjectID("-bad"); err == nil {
		t.Fatal("expected leading dash to fail")
	}
	if err := ValidatePresetID("comfyui-minimax-h3"); err != nil {
		t.Fatal(err)
	}
}

func TestDotEnvDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nexport WCKD_PROJECT_ID=\"from-dotenv\"\nWCKD_PRESET_ID='preset-a'\nBROKEN LINE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WCKD_PROJECT_ID", "already")
	t.Setenv("WCKD_PRESET_ID", "")
	os.Unsetenv("WCKD_PRESET_ID")

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("WCKD_PROJECT_ID"); got != "already" {
		t.Fatalf("overrode existing: %q", got)
	}
	if got := os.Getenv("WCKD_PRESET_ID"); got != "preset-a" {
		t.Fatalf("preset: %q", got)
	}
	if err := LoadDotEnv(filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidHoursEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WCKD_SESSION_HOURS", "nope")
	t.Setenv("WCKD_STATE_DIR", dir)
	t.Setenv("WCKD_PRESETS_DIR", dir)
	if _, err := Load(""); err == nil {
		t.Fatal("expected parse error")
	}
}
