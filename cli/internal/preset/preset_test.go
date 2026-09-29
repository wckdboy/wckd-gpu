package preset

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseHappyPath(t *testing.T) {
	raw := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: hello-gpu
  name: Hello
  version: 1
spec:
  workload_class: generic
  constraints:
    min_vram_gb: 8
    reliability: community
    gpu_families: ["4090"]
    prefer_regions: ["EU"]
  runtime:
    image: "runpod/pytorch:test"
    entrypoint: ["/bin/sleep", "infinity"]
    env:
      FOO: "1"
    ports:
      - name: ui
        container: 8188
        expose: tunnel
  sync:
    hydrate:
      - from: workspace/
        to: /workspace
    drain:
      - from: /workspace
        to: workspace/
        exclude: ["**/.cache/**"]
`)
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Metadata.ID != "hello-gpu" || f.Spec.Constraints.MinVRAMGB != 8 {
		t.Fatalf("%+v", f)
	}
	if f.Image("override") != "override" || f.Image("") != "runpod/pytorch:test" {
		t.Fatalf("image: %s", f.Image(""))
	}
}

func TestParseRejectsUnknownClass(t *testing.T) {
	raw := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: hello-gpu
  name: Hello
spec:
  workload_class: training-cluster
  runtime:
    image: img
`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello-gpu.yaml")
	body := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: other
  name: Hello
spec:
  workload_class: generic
  runtime:
    image: img
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadID(dir, "hello-gpu"); err == nil {
		t.Fatal("expected id mismatch")
	}
}

func TestBundledPreset(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Dir(file)
	var path string
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "presets", "comfyui-minimax-h3.yaml")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
			break
		}
		dir = filepath.Dir(dir)
	}
	if path == "" {
		t.Fatal("bundled preset not found")
	}
	f, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Metadata.ID != "comfyui-minimax-h3" {
		t.Fatalf("id %s", f.Metadata.ID)
	}
	if f.Spec.WorkloadClass != "video_dit" {
		t.Fatalf("class %s", f.Spec.WorkloadClass)
	}
	if f.Spec.Constraints.MinVRAMGB < 24 {
		t.Fatalf("vram %d", f.Spec.Constraints.MinVRAMGB)
	}
}
