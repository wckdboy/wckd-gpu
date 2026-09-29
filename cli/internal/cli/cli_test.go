package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("WCKD_DISABLE_DOTENV", "1")
	_ = os.Setenv("WCKD_NO_SWEEPER", "1")
	os.Exit(m.Run())
}

func TestHelpListsCommands(t *testing.T) {
	app := &App{}
	buf := &bytes.Buffer{}
	app.Stdout = buf
	app.Stderr = buf
	cmd := app.Command()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config", "offers", "presets", "projects", "start", "status", "stop", "sweeper"} {
		if !strings.Contains(buf.String(), name) {
			t.Fatalf("help missing %s\n%s", name, buf.String())
		}
	}
	for _, args := range [][]string{
		{"config", "check", "--help"},
		{"offers", "--help"},
		{"presets", "--help"},
		{"projects", "--help"},
		{"projects", "add", "--help"},
		{"start", "--help"},
		{"status", "--help"},
		{"stop", "--help"},
	} {
		buf.Reset()
		cmd = app.Command()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(buf.String(), args[0]) {
			t.Fatalf("help %v:\n%s", args, buf.String())
		}
	}
}

func TestConfigCheckOffersStartStop(t *testing.T) {
	secret := "super-secret-value"
	var mu sync.Mutex
	var createBody map[string]any
	deleted := []string{}
	pods := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			writeProblem(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"pods":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/catalog/gpus":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"gpus":[{
				"id":"NVIDIA GeForce RTX 4090",
				"name":"RTX 4090",
				"memory":24,
				"secure":true,
				"community":true,
				"availability":"HIGH",
				"price":{"secure":0.69,"community":0.34},
				"dataCenters":[{"id":"EU-RO-1","name":"EU Romania","availability":"HIGH"}]
			}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(body, &createBody)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"pod123","status":"PROVISIONING","cost":0.34,"dataCenterId":"EU-RO-1","ssh":{"proxy":{"command":"ssh pod123@ssh.runpod.io"},"direct":null}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods/pod123":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"pod123","status":"RUNNING","cost":0.34,"dataCenterId":"EU-RO-1","runtime":{"ports":[{"private":8188,"type":"http"}]}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/pods/pod123":
			mu.Lock()
			deleted = append(deleted, "pod123")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			writeProblem(w, http.StatusNotFound, r.Method+" "+r.URL.Path)
		}
	}))
	defer pods.Close()

	bucket := map[string][]byte{}
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/wckd/")
		if r.URL.Path == "/wckd" || r.URL.Path == "/wckd/" {
			key = ""
		}
		switch r.Method {
		case http.MethodHead:
			if key == "" {
				w.WriteHeader(http.StatusOK)
				return
			}
			mu.Lock()
			_, ok := bucket[key]
			mu.Unlock()
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NotFound</Code><Message>Not Found</Message></Error>`))
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bucket[key] = body
			mu.Unlock()
			w.Header().Set("ETag", `"etag"`)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer s3.Close()

	dir := t.TempDir()
	presets := filepath.Join(dir, "presets")
	if err := os.Mkdir(presets, 0o755); err != nil {
		t.Fatal(err)
	}
	preset := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: comfyui-minimax-h3
  name: MiniMax H3 (ComfyUI)
  version: 1
spec:
  workload_class: video_dit
  constraints:
    min_vram_gb: 24
    min_ram_gb: 64
    min_disk_gb: 100
    gpu_families: ["4090", "5090"]
    reliability: any
    prefer_regions: ["EU"]
  runtime:
    image: "runpod/pytorch:test"
    entrypoint: ["/usr/local/bin/start-comfy.sh"]
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
	if err := os.WriteFile(filepath.Join(presets, "comfyui-minimax-h3.yaml"), preset, 0o644); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	t.Setenv("HOME", dir)
	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "test-key")
	t.Setenv("WCKD_RUNPOD_BASE_URL", pods.URL)
	t.Setenv("WCKD_S3_ENDPOINT", s3.URL)
	t.Setenv("WCKD_S3_BUCKET", "wckd")
	t.Setenv("WCKD_S3_ACCESS_KEY", "ak")
	t.Setenv("WCKD_S3_SECRET_KEY", secret)
	t.Setenv("WCKD_S3_REGION", "auto")
	t.Setenv("WCKD_S3_PATH_STYLE", "true")
	t.Setenv("WCKD_SESSION_HOURS", "1")
	t.Setenv("WCKD_PROJECT_ID", "hailuo-tests")
	t.Setenv("WCKD_PRESET_ID", "comfyui-minimax-h3")
	t.Setenv("WCKD_STATE_DIR", state)
	t.Setenv("WCKD_PRESETS_DIR", presets)
	t.Setenv("WCKD_IMAGE", "")
	t.Setenv("WCKD_DRAIN_WAIT", "200ms")
	t.Setenv("WCKD_DRAIN_POLL", "20ms")

	out := &bytes.Buffer{}
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		out.Reset()
		app := &App{Stdout: out, Stderr: out}
		cmd := app.Command()
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs(args)
		cmd.SetContext(context.Background())
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	check := run(t, "config", "check")
	if !strings.Contains(check, "runpod: ok") || !strings.Contains(check, "s3: ok") {
		t.Fatalf("check:\n%s", check)
	}
	if strings.Contains(check, secret) {
		t.Fatal("config check printed the secret")
	}
	checkJSON := run(t, "config", "check", "--json")
	var checkReport map[string]any
	if err := json.Unmarshal([]byte(checkJSON), &checkReport); err != nil {
		t.Fatalf("config json: %v\n%s", err, checkJSON)
	}
	if checkReport["ok"] != true || strings.Contains(checkJSON, secret) || strings.Contains(checkJSON, "test-key") {
		t.Fatalf("config json leaked or failed:\n%s", checkJSON)
	}

	offers := run(t, "offers", "--preset", "comfyui-minimax-h3", "--hours", "1")
	if !strings.Contains(offers, "runpod:community:NVIDIA GeForce RTX 4090") {
		t.Fatalf("offers:\n%s", offers)
	}
	if !strings.Contains(offers, "score =") {
		t.Fatalf("offers missing formula:\n%s", offers)
	}
	offersJSON := run(t, "offers", "--preset", "comfyui-minimax-h3", "--hours", "1", "--json")
	var offerRows []map[string]any
	if err := json.Unmarshal([]byte(offersJSON), &offerRows); err != nil {
		t.Fatalf("offers json: %v\n%s", err, offersJSON)
	}
	if len(offerRows) == 0 || offerRows[0]["id"] != "runpod:community:NVIDIA GeForce RTX 4090" {
		t.Fatalf("offers json:\n%s", offersJSON)
	}
	if strings.Contains(offersJSON, secret) {
		t.Fatal("offers json printed the secret")
	}

	dry := run(t, "start", "--dry-run", "--json", "--hours", "1", "--project", "hailuo-tests", "--preset", "comfyui-minimax-h3")
	if !strings.Contains(dry, `"dry_run": true`) || strings.Contains(dry, secret) {
		t.Fatalf("dry-run:\n%s", dry)
	}

	presetsOut := run(t, "presets", "--json")
	if !strings.Contains(presetsOut, `"id": "comfyui-minimax-h3"`) || strings.Contains(presetsOut, secret) {
		t.Fatalf("presets:\n%s", presetsOut)
	}
	added := run(t, "projects", "add", "hailuo-tests", "--json")
	if !strings.Contains(added, `"id": "hailuo-tests"`) || !strings.Contains(added, `"saved": true`) {
		t.Fatalf("projects add:\n%s", added)
	}

	started := run(t, "start", "--hours", "1", "--project", "hailuo-tests", "--preset", "comfyui-minimax-h3", "--no-sweeper")
	if strings.Contains(started, secret) {
		t.Fatal("start printed the secret")
	}
	if !strings.Contains(started, "phase: hydrating") || !strings.Contains(started, "session_id: sess_") {
		t.Fatalf("start:\n%s", started)
	}
	mu.Lock()
	body := createBody
	mu.Unlock()
	gpu, _ := body["gpu"].(map[string]any)
	if gpu["id"] != "NVIDIA GeForce RTX 4090" {
		t.Fatalf("gpu %#v", body["gpu"])
	}
	if gpu["minRamPerGpu"] != float64(64) {
		t.Fatalf("ram %#v", gpu["minRamPerGpu"])
	}
	env, _ := body["env"].(map[string]any)
	if env["WCKD_S3_SECRET_ACCESS_KEY"] != secret {
		t.Fatalf("env not injected: %#v", env["WCKD_S3_SECRET_ACCESS_KEY"])
	}
	if !strings.Contains(env["WCKD_HYDRATE_MAP"].(string), "projects/hailuo-tests/workspace") {
		t.Fatalf("hydrate %#v", env["WCKD_HYDRATE_MAP"])
	}
	entries, err := os.ReadDir(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(state, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), secret) {
			t.Fatalf("local session file %s contains the S3 secret", ent.Name())
		}
	}

	status := run(t, "status")
	if !strings.Contains(status, "pod: pod123") || !strings.Contains(status, "pod_status: RUNNING") {
		t.Fatalf("status:\n%s", status)
	}
	if !strings.Contains(status, "endpoint_ui: https://pod123-8188.proxy.runpod.net") {
		t.Fatalf("status endpoints:\n%s", status)
	}

	// Drain has not happened; stop must refuse to delete.
	out.Reset()
	app := &App{Stdout: out, Stderr: out}
	cmd := app.Command()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"stop"})
	cmd.SetContext(context.Background())
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "drain did not finish") {
		t.Fatalf("stop err %v\n%s", err, out.String())
	}
	mu.Lock()
	nDeleted := len(deleted)
	mu.Unlock()
	if nDeleted != 0 {
		t.Fatal("pod was deleted after a failed drain")
	}

	mu.Lock()
	var sessionID string
	for key := range bucket {
		if strings.HasPrefix(key, "sessions/sess_") && strings.HasSuffix(key, "/manifest.json") {
			sessionID = strings.TrimSuffix(strings.TrimPrefix(key, "sessions/"), "/manifest.json")
		}
	}
	if sessionID == "" {
		t.Fatal("manifest was not written")
	}
	bucket["sessions/"+sessionID+"/drain-ok.json"] = []byte(`{"ok":true}`)
	mu.Unlock()

	stopped := run(t, "stop", sessionID)
	if !strings.Contains(stopped, "phase: terminated") || !strings.Contains(stopped, "drain_ok: true") {
		t.Fatalf("stop:\n%s", stopped)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 1 {
		t.Fatalf("deleted %#v", deleted)
	}
	if _, ok := bucket["sessions/"+sessionID+"/receipt.json"]; !ok {
		t.Fatal("missing receipt")
	}
	if _, ok := bucket["projects/hailuo-tests/.wckd/project.json"]; !ok {
		t.Fatal("missing project.json")
	}
	for key, body := range bucket {
		if strings.Contains(string(body), secret) {
			t.Fatalf("s3 object %s contains the S3 secret", key)
		}
	}
}

func TestConfigCheckAuthFailureDoesNotPrintKey(t *testing.T) {
	pods := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, http.StatusUnauthorized, "missing bearer token")
	}))
	defer pods.Close()
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("WCKD_CONFIG", "")
	t.Setenv("RUNPOD_API_KEY", "")
	t.Setenv("WCKD_RUNPOD_API_KEY", "visible-key-should-not-print")
	t.Setenv("WCKD_RUNPOD_BASE_URL", pods.URL)
	t.Setenv("WCKD_S3_ENDPOINT", s3.URL)
	t.Setenv("WCKD_S3_BUCKET", "wckd")
	t.Setenv("WCKD_S3_ACCESS_KEY", "ak")
	t.Setenv("WCKD_S3_SECRET_KEY", "sek")
	t.Setenv("WCKD_S3_REGION", "auto")
	t.Setenv("WCKD_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("WCKD_PRESETS_DIR", dir)
	t.Setenv("WCKD_PROJECT_ID", "")
	t.Setenv("WCKD_PRESET_ID", "comfyui-minimax-h3")
	t.Setenv("WCKD_SESSION_HOURS", "")
	t.Setenv("WCKD_IMAGE", "")
	t.Setenv("WCKD_S3_PATH_STYLE", "true")

	buf := &bytes.Buffer{}
	app := &App{Stdout: buf, Stderr: buf}
	cmd := app.Command()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "check"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if strings.Contains(buf.String(), "visible-key-should-not-print") || strings.Contains(err.Error(), "visible-key-should-not-print") {
		t.Fatalf("leaked key: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "runpod: fail") {
		t.Fatalf("output:\n%s", buf.String())
	}

	buf.Reset()
	cmd = app.Command()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"config", "check", "--json"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected json failure")
	}
	if strings.Contains(buf.String(), "visible-key-should-not-print") || strings.Contains(err.Error(), "visible-key-should-not-print") {
		t.Fatalf("leaked key: %v\n%s", err, buf.String())
	}
	var report map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &report); jerr != nil {
		t.Fatalf("json: %v\n%s", jerr, buf.String())
	}
	if report["ok"] != false {
		t.Fatalf("report %#v", report)
	}
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"title":"error","status":` + strconv.Itoa(status) + `,"detail":"` + detail + `"}`))
}
