// Package preset loads wckd.gpu/v1 preset YAML files.
package preset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const apiVersion = "wckd.gpu/v1"

// File is one preset document.
type File struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata identifies the preset.
type Metadata struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Version int    `yaml:"version"`
}

// Spec is the runnable body. See docs/PRESETS.md.
type Spec struct {
	WorkloadClass string      `yaml:"workload_class"`
	Constraints   Constraints `yaml:"constraints"`
	Runtime       Runtime     `yaml:"runtime"`
	Sync          Sync        `yaml:"sync"`
	Notes         string      `yaml:"notes"`
}

// Constraints are the offer filter.
type Constraints struct {
	MinVRAMGB     int      `yaml:"min_vram_gb"`
	MinRAMGB      int      `yaml:"min_ram_gb"`
	MinDiskGB     int      `yaml:"min_disk_gb"`
	GPUFamilies   []string `yaml:"gpu_families"`
	Reliability   string   `yaml:"reliability"`
	PreferRegions []string `yaml:"prefer_regions"`
}

// Runtime is the container contract.
type Runtime struct {
	Image       string            `yaml:"image"`
	Entrypoint  []string          `yaml:"entrypoint"`
	Env         map[string]string `yaml:"env"`
	Ports       []Port            `yaml:"ports"`
	Healthcheck Healthcheck       `yaml:"healthcheck"`
}

// Port is published from the pod.
type Port struct {
	Name      string `yaml:"name"`
	Container int    `yaml:"container"`
	Expose    string `yaml:"expose"`
}

// Healthcheck is recorded for the sidecar. P1 does not probe it from the CLI.
type Healthcheck struct {
	HTTPGet        string `yaml:"http_get"`
	IntervalSec    int    `yaml:"interval_sec"`
	StartPeriodSec int    `yaml:"start_period_sec"`
}

// Sync lists hydrate and drain paths relative to the project prefix.
type Sync struct {
	Hydrate []Path `yaml:"hydrate"`
	Drain   []Path `yaml:"drain"`
}

// Path is a relative object prefix and a container path.
type Path struct {
	From    string   `yaml:"from"`
	To      string   `yaml:"to"`
	Exclude []string `yaml:"exclude"`
}

// Parse decodes and validates a preset document.
func Parse(data []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parse preset: %w", err)
	}
	if err := f.Validate(); err != nil {
		return File{}, err
	}
	return f, nil
}

// LoadFile reads a preset from disk.
func LoadFile(path string) (File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	f, err := Parse(body)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// LoadID loads presetsDir/<id>.yaml and checks metadata.id.
func LoadID(presetsDir, id string) (File, error) {
	if err := validateID(id); err != nil {
		return File{}, err
	}
	path := filepath.Join(presetsDir, id+".yaml")
	f, err := LoadFile(path)
	if err != nil {
		return File{}, err
	}
	if f.Metadata.ID != id {
		return File{}, fmt.Errorf("preset file %s has metadata.id %q", path, f.Metadata.ID)
	}
	return f, nil
}

// Image returns the override when set, otherwise the preset image.
func (f File) Image(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return f.Spec.Runtime.Image
}

// Validate checks the schema fields P1 relies on.
func (f File) Validate() error {
	if f.APIVersion != apiVersion {
		return fmt.Errorf("unsupported apiVersion %q (want %s)", f.APIVersion, apiVersion)
	}
	if f.Kind != "Preset" {
		return fmt.Errorf("unsupported kind %q", f.Kind)
	}
	if err := validateID(f.Metadata.ID); err != nil {
		return err
	}
	if f.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	switch f.Spec.WorkloadClass {
	case "video_dit", "llm", "generic":
	default:
		return fmt.Errorf("unsupported workload_class %q", f.Spec.WorkloadClass)
	}
	switch strings.ToLower(strings.TrimSpace(f.Spec.Constraints.Reliability)) {
	case "", "any", "community", "secure":
	default:
		return fmt.Errorf("unsupported reliability %q", f.Spec.Constraints.Reliability)
	}
	if strings.TrimSpace(f.Spec.Runtime.Image) == "" {
		return fmt.Errorf("runtime.image is required")
	}
	if f.Spec.Constraints.MinVRAMGB < 0 || f.Spec.Constraints.MinRAMGB < 0 || f.Spec.Constraints.MinDiskGB < 0 {
		return fmt.Errorf("constraints must be >= 0")
	}
	for _, p := range f.Spec.Runtime.Ports {
		if p.Name == "" {
			return fmt.Errorf("port name is required")
		}
		if p.Container <= 0 || p.Container > 65535 {
			return fmt.Errorf("port %s container %d is invalid", p.Name, p.Container)
		}
		switch strings.ToLower(p.Expose) {
		case "", "tunnel", "http", "https", "tcp":
		default:
			return fmt.Errorf("port %s expose %q is unsupported", p.Name, p.Expose)
		}
	}
	return nil
}

func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("metadata.id is required")
	}
	if len(id) > 64 {
		return fmt.Errorf("metadata.id is too long")
	}
	for i, c := range id {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if i == 0 && (c == '-' || c == '_') {
			ok = false
		}
		if !ok {
			return fmt.Errorf("metadata.id %q must match [A-Za-z0-9][A-Za-z0-9_-]*", id)
		}
	}
	return nil
}
