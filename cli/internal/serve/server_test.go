package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

const (
	testBearer = "bearer-secret-do-not-leak"
	testRunPod = "runpod-secret-do-not-leak"
	testS3Key  = "s3access-secret-do-not-leak"
	testS3Sec  = "s3secret-do-not-leak-value"
)

func TestMissingAndWrongKey(t *testing.T) {
	h := newHarness(t, false)
	base, stop := h.start(t)
	defer stop()

	res, body := h.request(t, base, http.MethodGet, "/v1/rent/health", "", map[string]string{
		"Authorization": "",
	})
	if res != http.StatusUnauthorized {
		t.Fatalf("missing key: %d %s", res, body)
	}
	res, body = h.request(t, base, http.MethodGet, "/health", "", map[string]string{
		"Authorization": "Bearer wrong-token-value",
	})
	if res != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d %s", res, body)
	}
	if strings.Contains(body, testBearer) || strings.Contains(h.logs.String(), testBearer) {
		t.Fatalf("auth failure leaked the key\nbody %s\nlog %s", body, h.logs.String())
	}
	res, body = h.request(t, base, http.MethodGet, "/v1/rent/health", "", nil)
	if res != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("health %d %s", res, body)
	}
	if h.vendor.provisionCount() != 0 {
		t.Fatal("health provisioned")
	}
}

func TestNoResponseOrLogContainsSecrets(t *testing.T) {
	h := newHarness(t, false)
	base, stop := h.start(t)
	defer stop()

	code, body := h.request(t, base, http.MethodPost, "/v1/rent/sessions?confirm=true", h.createJSON(false), nil)
	if code != http.StatusConflict {
		t.Fatalf("quote %d %s", code, body)
	}
	h.assertClean(t, body)

	h.vendor.setFail(1, "provision failed "+testRunPod+" "+testS3Sec)
	code, body = h.request(t, base, http.MethodPost, "/v1/rent/sessions", h.createJSON(true), nil)
	if code != http.StatusBadGateway {
		t.Fatalf("provision %d %s", code, body)
	}
	h.assertClean(t, body)
	if !strings.Contains(body, "[redacted]") {
		t.Fatalf("expected redaction marker in %s", body)
	}
	h.assertClean(t, h.logs.String())
	if h.vendor.provisionCount() != 1 {
		t.Fatalf("provision calls %d", h.vendor.provisionCount())
	}
	if got := h.vendor.createdEnv(); got != testS3Sec {
		t.Fatalf("pod env secret = %q", got)
	}
}

func TestCreateWithoutConfirmDoesNotProvision(t *testing.T) {
	h := newHarness(t, false)
	base, stop := h.start(t)
	defer stop()

	code, body := h.request(t, base, http.MethodPost, "/v1/rent/sessions?confirm=true", h.createJSON(false), map[string]string{
		"X-Wckd-Confirm": "yes",
	})
	if code != http.StatusConflict {
		t.Fatalf("status %d %s", code, body)
	}
	var quote map[string]any
	if err := json.Unmarshal([]byte(body), &quote); err != nil {
		t.Fatal(err)
	}
	errObj, _ := quote["error"].(map[string]any)
	if errObj["code"] != "confirmation_required" {
		t.Fatalf("error %#v", quote["error"])
	}
	if quote["cost_estimate_usd"] != float64(1) {
		t.Fatalf("estimate %#v", quote["cost_estimate_usd"])
	}
	offerObj, _ := quote["offer"].(map[string]any)
	if offerObj["id"] != "runpod:community:GPU-A" || offerObj["usd_per_hr"] != 0.5 {
		t.Fatalf("offer %#v", offerObj)
	}
	if quote["cost_basis"] != session.CostBasisEstimate {
		t.Fatalf("basis %#v", quote["cost_basis"])
	}
	if h.vendor.provisionCount() != 0 {
		t.Fatalf("provisioned %d", h.vendor.provisionCount())
	}
	if n, err := h.countSessions(); err != nil || n != 0 {
		t.Fatalf("sessions %d %v", n, err)
	}
	if h.objects.puts() != 0 {
		t.Fatalf("object puts %d", h.objects.puts())
	}

	code, body = h.request(t, base, http.MethodPost, "/v1/rent/sessions", h.createJSON(false), map[string]string{
		"X-Wckd-Confirm": "true",
	})
	if code != http.StatusCreated {
		t.Fatalf("header confirm %d %s", code, body)
	}
	if h.vendor.provisionCount() != 1 {
		t.Fatalf("provisioned %d", h.vendor.provisionCount())
	}
}

func TestConfirmedCreateSurvivesClientCancel(t *testing.T) {
	h := newHarness(t, false)
	started := make(chan struct{})
	release := make(chan struct{})
	h.vendor.started = started
	h.vendor.release = release
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unlock()

	base, stop := h.start(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/rent/sessions", strings.NewReader(h.createJSON(true)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.key)
	req.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		_, _ = client.Do(req)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provision did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled client did not return")
	}
	unlock()

	deadline := time.Now().Add(3 * time.Second)
	var last session.Session
	for time.Now().Before(deadline) {
		list, err := session.NewStore(h.dir).List()
		if err == nil && len(list) == 1 {
			last = list[0]
			if last.Phase == session.PhaseHydrating && last.InstanceID != "" {
				return
			}
			if last.Phase == session.PhaseFailed {
				t.Fatalf("create followed the canceled request: %+v", last)
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("session after cancel: %+v", last)
}

func TestStopPreservesDrainGate(t *testing.T) {
	h := newHarness(t, false)
	base, stop := h.start(t)
	defer stop()
	id := h.mustCreate(t, base)
	code, body := h.request(t, base, http.MethodGet, "/v1/rent/sessions", "", nil)
	if code != http.StatusOK || !strings.Contains(body, id) {
		t.Fatalf("list %d %s", code, body)
	}
	code, body = h.request(t, base, http.MethodGet, "/v1/rent/sessions/"+id, "", nil)
	if code != http.StatusOK || !strings.Contains(body, `"phase":"hydrating"`) {
		t.Fatalf("get %d %s", code, body)
	}

	code, body = h.request(t, base, http.MethodPost, "/v1/rent/sessions/"+id+"/stop", `{}`, nil)
	if code != http.StatusConflict || !strings.Contains(body, `"drain_failed"`) {
		t.Fatalf("stop %d %s", code, body)
	}
	if h.vendor.terminateCount() != 0 {
		t.Fatalf("terminated early %d", h.vendor.terminateCount())
	}
	got := h.load(t, id)
	if got.Phase != session.PhaseDraining {
		t.Fatalf("phase %s", got.Phase)
	}

	h.objects.put("sessions/"+id+"/drain-ok.json", []byte(`{"ok":true}`))
	code, body = h.request(t, base, http.MethodPost, "/v1/rent/sessions/"+id+"/stop", `{"force":false}`, nil)
	if code != http.StatusOK {
		t.Fatalf("second stop %d %s", code, body)
	}
	got = h.load(t, id)
	if got.Phase != session.PhaseTerminated || !got.DrainOK || got.Forced {
		t.Fatalf("%+v", got)
	}
	if h.vendor.terminateCount() != 1 {
		t.Fatalf("terminated %d", h.vendor.terminateCount())
	}
}

func TestSweeperLeavesPodWhenDrainIsMissing(t *testing.T) {
	h := newHarness(t, false)
	base, stop := h.start(t)
	defer stop()
	id := h.mustCreate(t, base)
	h.clock.Add(2 * time.Hour)
	got := waitPhase(t, h.dir, id, session.PhaseDraining)
	if got.Phase != session.PhaseDraining {
		t.Fatalf("phase %s", got.Phase)
	}
	time.Sleep(80 * time.Millisecond)
	if h.vendor.terminateCount() != 0 {
		t.Fatalf("sweeper force-deleted: %d", h.vendor.terminateCount())
	}
}

func TestSweeperTerminatesPastDeadline(t *testing.T) {
	h := newHarness(t, true)
	base, stop := h.start(t)
	defer stop()
	id := h.mustCreate(t, base)
	if h.load(t, id).Phase == session.PhaseTerminated {
		t.Fatal("terminated before the deadline")
	}
	h.clock.Add(2 * time.Hour)
	got := waitPhase(t, h.dir, id, session.PhaseTerminated)
	if !got.DrainOK || got.Forced {
		t.Fatalf("%+v", got)
	}
	if h.vendor.terminateCount() != 1 || h.vendor.provisionCount() != 1 {
		t.Fatalf("provision %d terminate %d", h.vendor.provisionCount(), h.vendor.terminateCount())
	}
}

func TestRestartResumesAndTerminates(t *testing.T) {
	h := newHarness(t, true)
	base, stop := h.start(t)
	id := h.mustCreate(t, base)
	stop()
	stored := h.load(t, id)
	if stored.Phase == session.PhaseTerminated || stored.InstanceID == "" {
		t.Fatalf("before restart %+v", stored)
	}
	h.clock.Add(2 * time.Hour)
	base, stop = h.start(t)
	defer stop()
	_ = base
	got := waitPhase(t, h.dir, id, session.PhaseTerminated)
	if got.InstanceID != stored.InstanceID || !got.DrainOK {
		t.Fatalf("%+v", got)
	}
	if h.vendor.provisionCount() != 1 || h.vendor.terminateCount() != 1 {
		t.Fatalf("provision %d terminate %d", h.vendor.provisionCount(), h.vendor.terminateCount())
	}
	h.assertClean(t, h.logs.String())
}

func TestRequireRegionsHardFilterOnAPI(t *testing.T) {
	h := newHarness(t, false)
	h.vendor.offers = nil
	h.vendor.quotes = []offer.Quote{
		quoteDC("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.10, "US-KS-2"),
		quoteDC("NVIDIA GeForce RTX 4090 EU", "RTX 4090 EU", 24, "community", 0.40, "EU-RO-1"),
		quoteDC("NVIDIA GeForce RTX 4090 SEC", "RTX 4090 SEC", 24, "secure", 0.90, "EU-RO-1"),
	}
	mixed := quoteDC("NVIDIA GeForce RTX 5090", "RTX 5090", 32, "community", 0.80, "EU-NL-1")
	mixed.DataCenters = []offer.DataCenter{
		{ID: "US-KS-2", Name: "US Kansas", Availability: "HIGH"},
		{ID: "EU-NL-1", Name: "EU Netherlands", Availability: "HIGH"},
	}
	h.vendor.quotes = append(h.vendor.quotes, mixed)

	base, stop := h.start(t)
	defer stop()
	code, body := h.request(t, base, http.MethodGet, "/v1/rent/offers?preset=comfyui-minimax-h3&hours=1", "", nil)
	if code != http.StatusOK {
		t.Fatalf("offers %d %s", code, body)
	}
	if strings.Contains(body, "US-KS-2") {
		t.Fatalf("US data center leaked through the hard filter:\n%s", body)
	}
	if !strings.Contains(body, "EU-RO-1") || !strings.Contains(body, `"require_regions":["EU"]`) {
		t.Fatalf("body %s", body)
	}
	if !strings.Contains(body, `"require_secure":false`) {
		t.Fatalf("secure flag %s", body)
	}

	code, body = h.request(t, base, http.MethodGet, "/v1/rent/offers?preset=comfyui-minimax-h3&hours=1&require_regions=EU-secure", "", nil)
	if code != http.StatusOK {
		t.Fatalf("eu-secure %d %s", code, body)
	}
	if strings.Contains(body, "US-KS-2") || strings.Contains(body, `"cloud":"community"`) {
		t.Fatalf("eu-secure leaked community or US:\n%s", body)
	}
	if !strings.Contains(body, `"require_secure":true`) || !strings.Contains(body, "RTX 4090 SEC") {
		t.Fatalf("eu-secure body %s", body)
	}

	code, body = h.request(t, base, http.MethodGet, "/v1/rent/offers?preset=comfyui-minimax-h3&hours=1&require_regions=any", "", nil)
	if code != http.StatusOK || !strings.Contains(body, "US-KS-2") {
		t.Fatalf("opt-out %d %s", code, body)
	}
}

type harness struct {
	t       *testing.T
	dir     string
	presets string
	svc     *session.Service
	vendor  *fakeVendor
	objects *memObj
	clock   *lockedClock
	logs    *lockedBuf
	storage session.StorageCreds
	key     string
}

func newHarness(t *testing.T, autoDrain bool) *harness {
	t.Helper()
	dir := t.TempDir()
	presets := filepath.Join(dir, "presets")
	if err := os.Mkdir(presets, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: comfyui-minimax-h3
  name: H3
  version: 1
spec:
  workload_class: video_dit
  constraints:
    min_vram_gb: 24
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
`)
	if err := os.WriteFile(filepath.Join(presets, "comfyui-minimax-h3.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	clock := &lockedClock{t: now}
	vendor := &fakeVendor{
		offers: []offer.Offer{{
			ID:            "runpod:community:GPU-A",
			Vendor:        "runpod",
			SKU:           "GPU-A",
			Name:          "GPU-A",
			VRAMGB:        24,
			USDPerHour:    0.5,
			Cloud:         "community",
			Region:        "EU-RO-1",
			DataCenterIDs: []string{"EU-RO-1"},
		}},
	}
	objects := &memObj{m: map[string][]byte{}, autoDrain: autoDrain}
	h := &harness{
		t:       t,
		dir:     dir,
		presets: presets,
		vendor:  vendor,
		objects: objects,
		clock:   clock,
		logs:    &lockedBuf{},
		key:     testBearer,
		storage: session.StorageCreds{
			Endpoint:  "https://example.r2.cloudflarestorage.com",
			Bucket:    "wckd-gpu",
			AccessKey: testS3Key,
			SecretKey: testS3Sec,
			Region:    "auto",
		},
	}
	h.svc = &session.Service{
		Vendor:    vendor,
		Objects:   objects,
		Sessions:  session.NewStore(dir),
		Clock:     clock.Now,
		DrainWait: 40 * time.Millisecond,
		DrainPoll: 5 * time.Millisecond,
		Failover:  3,
	}
	return h
}

func (h *harness) start(t *testing.T) (string, func()) {
	t.Helper()
	srv := &Server{
		Addr:       "127.0.0.1:0",
		Service:    h.svc,
		PresetsDir: h.presets,
		Storage:    h.storage,
		Key:        h.key,
		SweepEvery: 20 * time.Millisecond,
		Log:        log.New(h.logs, "", 0),
		Redact:     []string{testRunPod},
		Ready:      make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()
	select {
	case <-srv.Ready:
	case err := <-errCh:
		t.Fatalf("serve: %v", err)
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("server did not bind")
	}
	stop := func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("serve stop: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	}
	return srv.URL, stop
}

func (h *harness) createJSON(confirm bool) string {
	flag := "false"
	if confirm {
		flag = "true"
	}
	return `{"project_id":"hailuo-tests","preset_id":"comfyui-minimax-h3","hours":2,"confirm":` + flag + `}`
}

func (h *harness) mustCreate(t *testing.T, base string) string {
	t.Helper()
	code, body := h.request(t, base, http.MethodPost, "/v1/rent/sessions", h.createJSON(true), nil)
	if code != http.StatusCreated {
		t.Fatalf("create %d %s", code, body)
	}
	var sess session.Session
	if err := json.Unmarshal([]byte(body), &sess); err != nil {
		t.Fatal(err)
	}
	if !session.ValidID(sess.ID) {
		t.Fatalf("id %s", sess.ID)
	}
	h.assertClean(t, body)
	return sess.ID
}

func (h *harness) request(t *testing.T, base, method, path, payload string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if headers == nil || !hasHeader(headers, "Authorization") {
		req.Header.Set("Authorization", "Bearer "+h.key)
	} else if headers["Authorization"] != "" {
		req.Header.Set("Authorization", headers["Authorization"])
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(buf)
	h.assertClean(t, body)
	return res.StatusCode, body
}

func hasHeader(h map[string]string, name string) bool {
	for k := range h {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

func (h *harness) assertClean(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{testBearer, testRunPod, testS3Key, testS3Sec} {
		if strings.Contains(body, secret) {
			t.Fatalf("secret %q appeared in %s", secret, body)
		}
	}
}

func (h *harness) load(t *testing.T, id string) session.Session {
	t.Helper()
	sess, err := session.NewStore(h.dir).Load(id)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func (h *harness) countSessions() (int, error) {
	list, err := session.NewStore(h.dir).List()
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

func waitPhase(t *testing.T, dir, id string, want session.Phase) session.Session {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last session.Session
	for time.Now().Before(deadline) {
		sess, err := session.NewStore(dir).Load(id)
		if err == nil {
			last = sess
			if sess.Phase == want {
				return sess
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("phase %s want %s (error %s)", last.Phase, want, last.Error)
	return last
}

func quoteDC(id, name string, vram int, cloud string, price float64, dc string) offer.Quote {
	return offer.Quote{
		GPUID:        id,
		Name:         name,
		VRAMGB:       vram,
		Cloud:        cloud,
		USDPerHour:   price,
		Availability: "HIGH",
		DataCenters:  []offer.DataCenter{{ID: dc, Name: dc, Availability: "HIGH"}},
	}
}

type lockedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *lockedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *lockedClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type fakeVendor struct {
	mu         sync.Mutex
	offers     []offer.Offer
	quotes     []offer.Quote
	failN      int
	failMsg    string
	created    []vendor.ProvisionRequest
	terminated []string
	started    chan struct{}
	release    chan struct{}
	startOnce  sync.Once
}

func (f *fakeVendor) ListOffers(_ context.Context, q offer.Query) ([]offer.Offer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.quotes != nil {
		return offer.Rank(f.quotes, q.Constraints, q.WorkloadClass), nil
	}
	out := append([]offer.Offer(nil), f.offers...)
	return out, nil
}

func (f *fakeVendor) Provision(ctx context.Context, req vendor.ProvisionRequest) (vendor.Instance, error) {
	if f.started != nil {
		f.startOnce.Do(func() { close(f.started) })
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return vendor.Instance{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, req)
	if f.failN > 0 {
		f.failN--
		msg := f.failMsg
		if msg == "" {
			msg = "no capacity"
		}
		return vendor.Instance{}, errors.New(msg)
	}
	return vendor.Instance{ID: "pod-1", Status: "RUNNING", DataCenter: "EU-RO-1"}, nil
}

func (f *fakeVendor) Status(context.Context, string) (vendor.Instance, error) {
	return vendor.Instance{ID: "pod-1", Status: "RUNNING", DataCenter: "EU-RO-1"}, nil
}

func (f *fakeVendor) Terminate(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, id)
	return nil
}

func (f *fakeVendor) setFail(n int, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failN = n
	f.failMsg = msg
}

func (f *fakeVendor) provisionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func (f *fakeVendor) terminateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.terminated)
}

func (f *fakeVendor) createdEnv() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.created) == 0 {
		return ""
	}
	return f.created[0].Env["WCKD_S3_SECRET_ACCESS_KEY"]
}

type memObj struct {
	mu        sync.Mutex
	m         map[string][]byte
	autoDrain bool
	n         int
}

func (m *memObj) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n++
	m.m[key] = append([]byte(nil), body...)
	if m.autoDrain && strings.HasSuffix(key, "/control.json") {
		drain := strings.TrimSuffix(key, "/control.json") + "/drain-ok.json"
		m.m[drain] = []byte("{}\n")
	}
	return nil
}

func (m *memObj) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.m[key]
	return ok, nil
}

func (m *memObj) put(key string, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = body
}

func (m *memObj) puts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.n
}
