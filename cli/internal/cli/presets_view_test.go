package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
)

func TestPresetViewDistinguishesOmittedAndEmptyRequireRegions(t *testing.T) {
	base := preset.File{
		Metadata: preset.Metadata{ID: "comfyui-minimax-h3", Name: "H3"},
		Spec:     preset.Spec{WorkloadClass: "video_dit"},
	}
	omitted, err := json.Marshal(presetToView(base))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(omitted), "require_regions") {
		t.Fatalf("omitted field was serialized: %s", omitted)
	}

	empty := base
	empty.Spec.Constraints.RequireRegions = []string{}
	body, err := json.Marshal(presetToView(empty))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"require_regions":[]`) {
		t.Fatalf("explicit empty list: %s", body)
	}
}
