# Preset schema

Presets are versioned YAML files. Clients and control plane share the same schema.

```yaml
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: comfyui-minimax-h3
  name: MiniMax H3 (ComfyUI)
  version: 1
spec:
  workload_class: video_dit   # video_dit | llm | generic
  constraints:
    min_vram_gb: 24
    min_ram_gb: 64
    min_disk_gb: 100
    gpu_families: ["5090", "4090", "L40S", "H100", "PRO6000"]
    reliability: any          # community | secure | any
    prefer_regions: ["EU"]
  runtime:
    image: "ghcr.io/wckd/comfyui-h3:latest"   # placeholder until built
    entrypoint: ["/usr/local/bin/start-comfy.sh"]
    env:
      COMFY_PORT: "8188"
    ports:
      - name: ui
        container: 8188
        expose: tunnel
    healthcheck:
      http_get: "http://127.0.0.1:8188/"
      interval_sec: 15
      start_period_sec: 120
  sync:
    hydrate:
      - from: "workspace/"
        to: "/workspace"
    drain:
      - from: "/workspace"
        to: "workspace/"
        # exclude caches if needed
        exclude: ["**/.cache/**", "**/__pycache__/**"]
  notes: |
    Peak VRAM for pruned H3 measured ~31.8GB — 32GB cards are tight.
    Prefer host RAM ≥80GB on rented pods.
```

## Gallery (planned)

| id | Class | Notes |
|----|-------|-------|
| `comfyui-minimax-h3` | video | Open weights; RunPod-friendly |
| `comfyui-wan22` | video | Last open Wan line (2.2); not Wan 3.0 |
| `vllm-openweights` | llm | OpenAI-compatible local serve |
| `generic-cuda` | generic | Ubuntu + CUDA + rclone only |
