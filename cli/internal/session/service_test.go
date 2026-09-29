package session

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

func TestStartFailoverAndStopDrainGate(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	v := &fakeVendor{
		offers: []offer.Offer{
			testOffer("runpod:community:GPU-A", "GPU-A", 0.40),
			testOffer("runpod:secure:GPU-B", "GPU-B", 0.90),
		},
		failN:      1,
		failStatus: 400,
		status:     "RUNNING",
	}
	obj := newMem()
	svc := testService(t, v, obj, now)

	sess, err := svc.Start(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	if sess.Phase != PhaseHydrating {
		t.Fatalf("phase %s", sess.Phase)
	}
	if sess.Offer.ID != "runpod:secure:GPU-B" {
		t.Fatalf("failover landed on %s", sess.Offer.ID)
	}
	if sess.InstanceID == "" || len(v.created) != 2 {
		t.Fatalf("created %d id %s", len(v.created), sess.InstanceID)
	}
	if v.created[1].Env["WCKD_S3_SECRET_ACCESS_KEY"] != "sek" {
		t.Fatal("pod env missing storage secret")
	}
	if v.created[1].Env["WCKD_HYDRATE_MAP"] != "projects/hailuo-tests/workspace|/workspace" {
		t.Fatalf("hydrate %s", v.created[1].Env["WCKD_HYDRATE_MAP"])
	}
	if v.created[1].Env["WCKD_WORKLOAD"] != strconv.Quote("/usr/local/bin/start-comfy.sh") {
		t.Fatalf("workload %s", v.created[1].Env["WCKD_WORKLOAD"])
	}
	if _, ok := obj.m[projectKey("hailuo-tests")]; !ok {
		t.Fatal("missing project.json")
	}
	if sess.CostEstimateUSD != 1.8 {
		t.Fatalf("estimate %v", sess.CostEstimateUSD)
	}

	_, err = svc.Stop(context.Background(), sess.ID, false)
	if !errors.Is(err, ErrDrainFailed) {
		t.Fatalf("expected drain failure, got %v", err)
	}
	if len(v.terminated) != 0 {
		t.Fatalf("terminated early: %v", v.terminated)
	}
	got, err := svc.Sessions.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseDraining {
		t.Fatalf("phase after failed drain %s", got.Phase)
	}
	if _, ok := obj.m[controlKey(sess.ID)]; !ok {
		t.Fatal("missing control.json")
	}

	obj.put(drainKey(sess.ID), []byte(`{"ok":true}`))
	done, err := svc.Stop(context.Background(), sess.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if done.Phase != PhaseTerminated || !done.DrainOK || done.Forced {
		t.Fatalf("%+v", done)
	}
	if len(v.terminated) != 1 || v.terminated[0] != done.InstanceID {
		t.Fatalf("terminated %v", v.terminated)
	}
	if _, ok := obj.m[receiptKey(sess.ID)]; !ok {
		t.Fatal("missing receipt")
	}
}

func TestStopForceWithoutDrain(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	v := &fakeVendor{offers: []offer.Offer{testOffer("runpod:community:GPU-A", "GPU-A", 0.5)}, status: "RUNNING"}
	obj := newMem()
	svc := testService(t, v, obj, now)
	sess, err := svc.Start(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	done, err := svc.Stop(context.Background(), sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Forced || done.DrainOK || done.Phase != PhaseTerminated {
		t.Fatalf("%+v", done)
	}
	if len(v.terminated) != 1 {
		t.Fatalf("terminated %v", v.terminated)
	}
}

func TestOfferLockSkipsFailover(t *testing.T) {
	v := &fakeVendor{
		offers: []offer.Offer{
			testOffer("runpod:community:GPU-A", "GPU-A", 0.4),
			testOffer("runpod:secure:GPU-B", "GPU-B", 0.9),
		},
		failN:      1,
		failStatus: 400,
	}
	svc := testService(t, v, newMem(), time.Now().UTC())
	req := testReq()
	req.OfferID = "runpod:community:GPU-A"
	_, err := svc.Start(context.Background(), req)
	if err == nil {
		t.Fatal("expected provision error")
	}
	if len(v.created) != 1 {
		t.Fatalf("tried %d offers", len(v.created))
	}
}

func TestFatalProvisionStopsFailover(t *testing.T) {
	v := &fakeVendor{
		offers: []offer.Offer{
			testOffer("runpod:community:GPU-A", "GPU-A", 0.4),
			testOffer("runpod:secure:GPU-B", "GPU-B", 0.9),
		},
		failN:      5,
		failStatus: 402,
	}
	svc := testService(t, v, newMem(), time.Now().UTC())
	_, err := svc.Start(context.Background(), testReq())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(v.created) != 1 {
		t.Fatalf("tried %d", len(v.created))
	}
}

func TestRefreshAndSweep(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	v := &fakeVendor{offers: []offer.Offer{testOffer("runpod:community:GPU-A", "GPU-A", 1)}, status: "RUNNING"}
	obj := newMem()
	svc := testService(t, v, obj, now)
	sess, err := svc.Start(context.Background(), testReq())
	if err != nil {
		t.Fatal(err)
	}
	obj.put(hydrateKey(sess.ID), []byte(`{"ok":true}`))
	got, err := svc.Refresh(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseReady {
		t.Fatalf("phase %s", got.Phase)
	}

	svc.Clock = func() time.Time { return now.Add(2 * time.Hour) }
	obj.put(drainKey(sess.ID), []byte(`{"ok":true}`))
	if err := svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	done, err := svc.Sessions.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Phase != PhaseTerminated {
		t.Fatalf("phase %s err %s", done.Phase, done.Error)
	}
}

func TestSyncMapAndCost(t *testing.T) {
	p := testPreset()
	p.Spec.Sync.Drain[0].Exclude = []string{"**/.cache/**", "**/__pycache__/**"}
	h, d, ex := SyncMaps(p, "hailuo-tests")
	if h != "projects/hailuo-tests/workspace|/workspace" {
		t.Fatal(h)
	}
	if d != "/workspace|projects/hailuo-tests/workspace" {
		t.Fatal(d)
	}
	if ex != "**/.cache/**,**/__pycache__/**" {
		t.Fatal(ex)
	}
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	if got := Actual(2, start, end); got != 1 {
		t.Fatalf("actual %v", got)
	}
}

func testService(t *testing.T, v *fakeVendor, obj *memObj, now time.Time) *Service {
	t.Helper()
	return &Service{
		Vendor:    v,
		Objects:   obj,
		Sessions:  NewStore(t.TempDir()),
		Clock:     func() time.Time { return now },
		DrainWait: 40 * time.Millisecond,
		DrainPoll: 5 * time.Millisecond,
		Failover:  3,
	}
}

func testReq() StartRequest {
	return StartRequest{
		ProjectID: "hailuo-tests",
		Preset:    testPreset(),
		Hours:     2,
		Image:     "runpod/pytorch:test",
		Storage: StorageCreds{
			Endpoint:  "https://example.r2.cloudflarestorage.com",
			Bucket:    "wckd-gpu",
			AccessKey: "ak",
			SecretKey: "sek",
			Region:    "auto",
		},
	}
}

func testPreset() preset.File {
	return preset.File{
		APIVersion: "wckd.gpu/v1",
		Kind:       "Preset",
		Metadata:   preset.Metadata{ID: "comfyui-minimax-h3", Name: "H3"},
		Spec: preset.Spec{
			WorkloadClass: "video_dit",
			Constraints:   preset.Constraints{MinVRAMGB: 24, MinRAMGB: 64, MinDiskGB: 100},
			Runtime: preset.Runtime{
				Image:      "runpod/pytorch:test",
				Entrypoint: []string{"/usr/local/bin/start-comfy.sh"},
			},
			Sync: preset.Sync{
				Hydrate: []preset.Path{{From: "workspace/", To: "/workspace"}},
				Drain:   []preset.Path{{From: "/workspace", To: "workspace/"}},
			},
		},
	}
}

func testOffer(id, sku string, price float64) offer.Offer {
	return offer.Offer{
		ID:         id,
		Vendor:     "runpod",
		SKU:        sku,
		Name:       sku,
		VRAMGB:     24,
		USDPerHour: price,
		Cloud:      "community",
		Score:      1,
	}
}

type statusErr struct {
	code int
	msg  string
}

func (e *statusErr) Error() string   { return e.msg }
func (e *statusErr) HTTPStatus() int { return e.code }

type fakeVendor struct {
	offers     []offer.Offer
	failN      int
	failStatus int
	status     string
	created    []vendor.ProvisionRequest
	terminated []string
}

func (f *fakeVendor) ListOffers(context.Context, offer.Query) ([]offer.Offer, error) {
	return f.offers, nil
}

func (f *fakeVendor) Provision(_ context.Context, req vendor.ProvisionRequest) (vendor.Instance, error) {
	f.created = append(f.created, req)
	if f.failN > 0 {
		f.failN--
		code := f.failStatus
		if code == 0 {
			code = 400
		}
		return vendor.Instance{}, &statusErr{code: code, msg: "no capacity"}
	}
	return vendor.Instance{ID: "pod-1", Status: "PROVISIONING", DataCenter: "EU-RO-1"}, nil
}

func (f *fakeVendor) Status(context.Context, string) (vendor.Instance, error) {
	st := f.status
	if st == "" {
		st = "RUNNING"
	}
	return vendor.Instance{ID: "pod-1", Status: st, DataCenter: "EU-RO-1"}, nil
}

func (f *fakeVendor) Terminate(_ context.Context, id string) error {
	f.terminated = append(f.terminated, id)
	return nil
}

type memObj struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMem() *memObj { return &memObj{m: map[string][]byte{}} }

func (m *memObj) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = append([]byte(nil), body...)
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
