// Package config loads wckd settings from an optional YAML file and the
// environment. Environment variables win. Secrets are never written by this
// package.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MaxSessionHours caps a single P1 session so a bad flag cannot schedule
	// a multi-day GPU rental. The deadline sweeper is still the real stop.
	MaxSessionHours = 24

	defaultRunPodBase = "https://api.runpod.io"
	defaultPresetID   = "comfyui-minimax-h3"
	defaultRegion     = "auto"
)

// Config is the resolved runtime configuration.
type Config struct {
	RunPod     RunPod   `yaml:"runpod"`
	S3         S3       `yaml:"s3"`
	Defaults   Defaults `yaml:"defaults"`
	StateDir   string   `yaml:"state_dir"`
	PresetsDir string   `yaml:"presets_dir"`
	// Image overrides the preset container image when set (sidecar builds).
	Image string `yaml:"image"`
}

// RunPod holds the user's own RunPod API credentials.
type RunPod struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// S3 is any S3-compatible endpoint (Cloudflare R2 is the recommended one).
type S3 struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	PathStyle *bool  `yaml:"path_style"`
}

// UsePathStyle reports whether the S3 client should use path-style URLs.
// Path-style is the default because R2, MinIO, and the test server all accept it.
func (s S3) UsePathStyle() bool {
	if s.PathStyle == nil {
		return true
	}
	return *s.PathStyle
}

// Defaults are used when a command flag is omitted.
type Defaults struct {
	Hours     float64 `yaml:"hours"`
	ProjectID string  `yaml:"project_id"`
	PresetID  string  `yaml:"preset_id"`
}

// Load resolves configuration. explicitPath forces a file. An empty path
// searches WCKD_CONFIG, ./wckd.yaml, then ~/.wckd/config.yaml. A missing
// optional file is not an error.
func Load(explicitPath string) (Config, error) {
	cfg := Config{
		RunPod:   RunPod{BaseURL: defaultRunPodBase},
		S3:       S3{Region: defaultRegion},
		Defaults: Defaults{Hours: 1, PresetID: defaultPresetID},
		StateDir: "~/.wckd",
	}

	path, required, err := resolveConfigPath(explicitPath)
	if err != nil {
		return Config{}, err
	}
	if path != "" {
		if err := applyFile(path, &cfg); err != nil {
			if required || !os.IsNotExist(err) {
				return Config{}, err
			}
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	cfg.StateDir = expandHome(cfg.StateDir)
	cfg.PresetsDir = expandHome(cfg.PresetsDir)
	if cfg.PresetsDir == "" {
		cfg.PresetsDir = findPresetsDir()
	}
	if cfg.RunPod.BaseURL == "" {
		cfg.RunPod.BaseURL = defaultRunPodBase
	}
	cfg.RunPod.BaseURL = strings.TrimRight(cfg.RunPod.BaseURL, "/")
	if cfg.S3.Region == "" {
		cfg.S3.Region = defaultRegion
	}
	if cfg.Defaults.PresetID == "" {
		cfg.Defaults.PresetID = defaultPresetID
	}
	if cfg.Defaults.Hours == 0 {
		cfg.Defaults.Hours = 1
	}
	return cfg, nil
}

// MissingCredentials lists credential fields that config check and start need.
// It does not include default project or preset; those have their own checks.
func (c Config) MissingCredentials() []string {
	var missing []string
	if strings.TrimSpace(c.RunPod.APIKey) == "" {
		missing = append(missing, "RunPod API key (WCKD_RUNPOD_API_KEY or RUNPOD_API_KEY)")
	}
	if strings.TrimSpace(c.S3.Endpoint) == "" {
		missing = append(missing, "S3 endpoint (WCKD_S3_ENDPOINT)")
	}
	if strings.TrimSpace(c.S3.Bucket) == "" {
		missing = append(missing, "S3 bucket (WCKD_S3_BUCKET)")
	}
	if strings.TrimSpace(c.S3.AccessKey) == "" {
		missing = append(missing, "S3 access key (WCKD_S3_ACCESS_KEY)")
	}
	if strings.TrimSpace(c.S3.SecretKey) == "" {
		missing = append(missing, "S3 secret key (WCKD_S3_SECRET_KEY)")
	}
	return missing
}

// ValidateHours enforces the P1 duration window.
func ValidateHours(hours float64) error {
	if hours <= 0 || hours > MaxSessionHours {
		return fmt.Errorf("hours must be in (0, %g], got %v", float64(MaxSessionHours), hours)
	}
	return nil
}

// ValidateProjectID restricts ids so they are safe as a single S3 prefix segment.
func ValidateProjectID(id string) error {
	if err := validateSlug(id, "project id"); err != nil {
		return err
	}
	return nil
}

// ValidatePresetID restricts preset ids to a single path segment.
func ValidatePresetID(id string) error {
	return validateSlug(id, "preset id")
}

func validateSlug(id, what string) error {
	if id == "" {
		return fmt.Errorf("%s is required", what)
	}
	if len(id) > 64 {
		return fmt.Errorf("%s is too long", what)
	}
	for i, c := range id {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if i == 0 && (c == '-' || c == '_') {
			ok = false
		}
		if !ok {
			return fmt.Errorf("%s %q must match [A-Za-z0-9][A-Za-z0-9_-]*", what, id)
		}
	}
	return nil
}

func resolveConfigPath(explicit string) (path string, required bool, err error) {
	if explicit != "" {
		return expandHome(explicit), true, nil
	}
	if env := strings.TrimSpace(os.Getenv("WCKD_CONFIG")); env != "" {
		return expandHome(env), true, nil
	}
	for _, candidate := range []string{"wckd.yaml", filepath.Join(expandHome("~/.wckd"), "config.yaml")} {
		if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
			return candidate, false, nil
		}
	}
	return "", false, nil
}

func applyFile(path string, cfg *Config) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(body, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if v := firstEnv("WCKD_RUNPOD_API_KEY", "RUNPOD_API_KEY"); v != "" {
		cfg.RunPod.APIKey = v
	}
	if v := firstEnv("WCKD_RUNPOD_BASE_URL", "RUNPOD_BASE_URL"); v != "" {
		cfg.RunPod.BaseURL = v
	}
	if v := os.Getenv("WCKD_S3_ENDPOINT"); v != "" {
		cfg.S3.Endpoint = v
	}
	if v := os.Getenv("WCKD_S3_BUCKET"); v != "" {
		cfg.S3.Bucket = v
	}
	if v := firstEnv("WCKD_S3_ACCESS_KEY", "AWS_ACCESS_KEY_ID"); v != "" {
		cfg.S3.AccessKey = v
	}
	if v := firstEnv("WCKD_S3_SECRET_KEY", "AWS_SECRET_ACCESS_KEY"); v != "" {
		cfg.S3.SecretKey = v
	}
	if v := firstEnv("WCKD_S3_REGION", "AWS_REGION"); v != "" {
		cfg.S3.Region = v
	}
	if v := os.Getenv("WCKD_S3_PATH_STYLE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("WCKD_S3_PATH_STYLE: %w", err)
		}
		cfg.S3.PathStyle = &b
	}
	if v := os.Getenv("WCKD_SESSION_HOURS"); v != "" {
		h, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("WCKD_SESSION_HOURS: %w", err)
		}
		cfg.Defaults.Hours = h
	}
	if v := os.Getenv("WCKD_PROJECT_ID"); v != "" {
		cfg.Defaults.ProjectID = v
	}
	if v := os.Getenv("WCKD_PRESET_ID"); v != "" {
		cfg.Defaults.PresetID = v
	}
	if v := os.Getenv("WCKD_STATE_DIR"); v != "" {
		cfg.StateDir = v
	}
	if v := os.Getenv("WCKD_PRESETS_DIR"); v != "" {
		cfg.PresetsDir = v
	}
	if v := os.Getenv("WCKD_IMAGE"); v != "" {
		cfg.Image = v
	}
	return nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func findPresetsDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "presets"
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, "presets")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "presets"
}
