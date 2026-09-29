package runpod

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

func TestListOffersAndProvision(t *testing.T) {
	var mu sync.Mutex
	var sawAuth bool
	var createBody createPodBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			writeProblem(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		mu.Lock()
		sawAuth = true
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/catalog/gpus":
			if r.URL.Query().Get("include") != "AVAILABILITY" || r.URL.Query().Get("product") != "POD" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
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
			},{
				"id":"NVIDIA GeForce RTX 3090",
				"name":"RTX 3090",
				"memory":24,
				"secure":false,
				"community":true,
				"availability":"HIGH",
				"price":{"secure":0,"community":0.2},
				"dataCenters":[{"id":"EU-RO-1","name":"EU Romania","availability":"HIGH"}]
			}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/pods":
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"pod_abc","status":"PROVISIONING","cost":0.34,"dataCenterId":"EU-RO-1","ssh":{"proxy":{"command":"ssh pod_abc@ssh.runpod.io"},"direct":null}}`))
		default:
			writeProblem(w, http.StatusNotFound, "nope")
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	c.MaxAttempts = 1
	offers, err := c.ListOffers(context.Background(), offer.Query{
		WorkloadClass: "video_dit",
		Constraints: offer.Constraints{
			MinVRAMGB:     24,
			GPUFamilies:   []string{"4090", "5090"},
			Reliability:   "any",
			PreferRegions: []string{"EU"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 2 {
		t.Fatalf("offers %#v", offers)
	}
	if offers[0].ID != "runpod:community:NVIDIA GeForce RTX 4090" {
		t.Fatalf("top %s", offers[0].ID)
	}
	if !sawAuth {
		t.Fatal("auth header was not sent")
	}

	port, err := FormatPorts("ui", 8188, "tunnel")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := c.Provision(context.Background(), vendor.ProvisionRequest{
		Name:          "wckd-sess_abc",
		Image:         "runpod/pytorch:test",
		Cloud:         "community",
		GPUID:         offers[0].SKU,
		MinRAMGB:      64,
		DiskGB:        100,
		Ports:         []vendor.NamedPort{port},
		Env:           map[string]string{"WCKD_S3_SECRET_ACCESS_KEY": "super-secret"},
		DataCenterIDs: offers[0].DataCenterIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inst.ID != "pod_abc" || inst.Endpoints[0].URL != "https://pod_abc-8188.proxy.runpod.net" {
		t.Fatalf("%+v", inst)
	}
	if createBody.GPU.MinRAMPerGPU == nil || *createBody.GPU.MinRAMPerGPU != 64 {
		t.Fatalf("ram %+v", createBody.GPU)
	}
	if createBody.Cloud != "COMMUNITY" || createBody.Ports[0] != "8188/http" {
		t.Fatalf("body %+v", createBody)
	}
	if createBody.Env["WCKD_S3_SECRET_ACCESS_KEY"] != "super-secret" {
		t.Fatal("env not forwarded to the pod request")
	}
}

func TestTerminateIdempotentAndPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"pods":[]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/pods/gone":
			writeProblem(w, http.StatusNotFound, "pod not found")
		case r.Method == http.MethodGet && r.URL.Path == "/v2/pods/pod_abc":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"pod_abc","status":"RUNNING","cost":0.34,"dataCenterId":"EU-RO-1","runtime":{"ports":[{"private":8188,"type":"http"}]}}`)
		default:
			writeProblem(w, http.StatusBadRequest, "unexpected")
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	c.MaxAttempts = 1
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Terminate(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	inst, err := c.Status(context.Background(), "pod_abc")
	if err != nil {
		t.Fatal(err)
	}
	if inst.Status != "RUNNING" || len(inst.Endpoints) != 1 {
		t.Fatalf("%+v", inst)
	}
}

func TestErrorDoesNotEchoRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		writeProblem(w, http.StatusBadRequest, "no capacity")
	}))
	defer srv.Close()
	c := New(srv.URL, "secret-api-key")
	c.MaxAttempts = 1
	_, err := c.Provision(context.Background(), vendor.ProvisionRequest{
		Name:   "n",
		Image:  "img",
		Cloud:  "secure",
		GPUID:  "NVIDIA GeForce RTX 4090",
		DiskGB: 20,
		Env:    map[string]string{"WCKD_S3_SECRET_ACCESS_KEY": "super-secret-value"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-value") || strings.Contains(err.Error(), "secret-api-key") {
		t.Fatalf("error leaked secrets: %v", err)
	}
	var api *APIError
	if !errors.As(err, &api) || api.Status != 400 {
		t.Fatalf("%v", err)
	}
}

func TestFormatPorts(t *testing.T) {
	p, err := FormatPorts("ui", 22, "tcp")
	if err != nil || p.Spec != "22/tcp" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := FormatPorts("ui", 1, "udp"); err == nil {
		t.Fatal("expected error")
	}
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"title": "error", "status": status, "detail": detail})
}
