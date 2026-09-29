// Package runpod is the RunPod REST API v2 adapter.
package runpod

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

const userAgent = "wckd-gpu/0.1"

// APIError is a RunPod problem+json failure. The message never includes the request body.
type APIError struct {
	Status int
	Detail string
}

func (e *APIError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("runpod api status %d", e.Status)
	}
	return fmt.Sprintf("runpod api status %d: %s", e.Status, e.Detail)
}

func (e *APIError) HTTPStatus() int { return e.Status }

// Client talks to the RunPod REST API.
type Client struct {
	BaseURL     string
	APIKey      string
	HTTP        *http.Client
	MaxAttempts int
	// Backoff, when set, replaces the default sleep between retries.
	Backoff func(attempt int)
}

// New returns a client. baseURL defaults to the public API host when empty.
func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://api.runpod.io"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Ping checks that the API key can list pods. It does not create a GPU.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/v2/pods", nil, nil, nil)
}

// ListOffers loads the pod catalog for the reliability tiers the preset allows
// and ranks the result.
func (c *Client) ListOffers(ctx context.Context, q offer.Query) ([]offer.Offer, error) {
	var quotes []offer.Quote
	for _, cloud := range cloudsFor(q.Constraints.Reliability) {
		gpus, err := c.listGPUs(ctx, cloud)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quotesFrom(gpus, cloud)...)
	}
	return offer.Rank(quotes, q.Constraints, q.WorkloadClass), nil
}

// Provision creates one pod for an already chosen offer.
func (c *Client) Provision(ctx context.Context, req vendor.ProvisionRequest) (vendor.Instance, error) {
	body := createPodBody{
		Name:          req.Name,
		Image:         req.Image,
		Cloud:         strings.ToUpper(req.Cloud),
		Disk:          req.DiskGB,
		Env:           req.Env,
		DataCenterIDs: req.DataCenterIDs,
		GPU: gpuBody{
			ID:    req.GPUID,
			Count: 1,
		},
	}
	if req.MinRAMGB > 0 {
		ram := req.MinRAMGB
		body.GPU.MinRAMPerGPU = &ram
	}
	for _, p := range req.Ports {
		body.Ports = append(body.Ports, p.Spec)
	}
	var pod podDTO
	if err := c.do(ctx, http.MethodPost, "/v2/pods", nil, body, &pod); err != nil {
		return vendor.Instance{}, err
	}
	if pod.ID == "" {
		return vendor.Instance{}, fmt.Errorf("runpod create pod returned an empty id")
	}
	return pod.instance(req.Ports), nil
}

// Status fetches one pod.
func (c *Client) Status(ctx context.Context, id string) (vendor.Instance, error) {
	var pod podDTO
	path := "/v2/pods/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &pod); err != nil {
		return vendor.Instance{}, err
	}
	return pod.instance(nil), nil
}

// Terminate deletes a pod. A 404 is success so retries are safe.
func (c *Client) Terminate(ctx context.Context, id string) error {
	path := "/v2/pods/" + url.PathEscape(id)
	err := c.do(ctx, http.MethodDelete, path, nil, nil, nil)
	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func cloudsFor(reliability string) []string {
	switch strings.ToLower(strings.TrimSpace(reliability)) {
	case "community":
		return []string{"COMMUNITY"}
	case "secure":
		return []string{"SECURE"}
	default:
		return []string{"COMMUNITY", "SECURE"}
	}
}

func (c *Client) listGPUs(ctx context.Context, cloud string) ([]gpuDTO, error) {
	q := url.Values{}
	q.Set("include", "AVAILABILITY")
	q.Set("product", "POD")
	q.Set("count", "1")
	q.Set("cloud", cloud)
	var resp gpuListDTO
	if err := c.do(ctx, http.MethodGet, "/v2/catalog/gpus", q, nil, &resp); err != nil {
		return nil, err
	}
	return resp.GPUs, nil
}

func quotesFrom(gpus []gpuDTO, cloud string) []offer.Quote {
	wantCommunity := strings.EqualFold(cloud, "COMMUNITY")
	var out []offer.Quote
	for _, g := range gpus {
		price := g.Price.Secure
		enabled := g.Secure
		nameCloud := "secure"
		if wantCommunity {
			price = g.Price.Community
			enabled = g.Community
			nameCloud = "community"
		}
		if !enabled {
			continue
		}
		q := offer.Quote{
			GPUID:        g.ID,
			Name:         g.Name,
			VRAMGB:       g.Memory,
			Cloud:        nameCloud,
			USDPerHour:   price,
			Availability: g.Availability,
		}
		if q.Name == "" {
			q.Name = g.ID
		}
		for _, dc := range g.DataCenters {
			q.DataCenters = append(q.DataCenters, offer.DataCenter{
				ID:           dc.ID,
				Name:         dc.Name,
				Availability: dc.Availability,
			})
		}
		out = append(out, q)
	}
	return out
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in any, out any) error {
	attempts := c.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		err := c.doOnce(ctx, method, path, query, in, out)
		if err == nil {
			return nil
		}
		last = err
		var api *APIError
		if errors.As(err, &api) && !retryable(api.Status) {
			return err
		}
		if attempt == attempts {
			break
		}
		c.sleep(attempt)
	}
	return last
}

func (c *Client) sleep(attempt int) {
	if c.Backoff != nil {
		c.Backoff(attempt)
		return
	}
	time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func (c *Client) doOnce(ctx context.Context, method, path string, query url.Values, in any, out any) error {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ht := c.HTTP
	if ht == nil {
		ht = http.DefaultClient
	}
	resp, err := ht.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Detail: problemDetail(respBody)}
	}
	if out == nil || len(bytes.TrimSpace(respBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode runpod response: %w", err)
	}
	return nil
}

func problemDetail(body []byte) string {
	var problem struct {
		Detail string `json:"detail"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(body, &problem); err == nil {
		if problem.Detail != "" {
			return problem.Detail
		}
		if problem.Title != "" {
			return problem.Title
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

type gpuListDTO struct {
	GPUs []gpuDTO `json:"gpus"`
}

type gpuDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Memory       int    `json:"memory"`
	Secure       bool   `json:"secure"`
	Community    bool   `json:"community"`
	Availability string `json:"availability"`
	Price        struct {
		Secure    float64 `json:"secure"`
		Community float64 `json:"community"`
	} `json:"price"`
	DataCenters []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Availability string `json:"availability"`
	} `json:"dataCenters"`
}

type createPodBody struct {
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Cloud         string            `json:"cloud,omitempty"`
	GPU           gpuBody           `json:"gpu"`
	Disk          int               `json:"disk"`
	Ports         []string          `json:"ports,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	DataCenterIDs []string          `json:"dataCenterIds,omitempty"`
}

type gpuBody struct {
	ID           string `json:"id"`
	Count        int    `json:"count"`
	MinRAMPerGPU *int   `json:"minRamPerGpu,omitempty"`
}

type podDTO struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Cost         float64 `json:"cost"`
	DataCenterID string  `json:"dataCenterId"`
	SSH          struct {
		Proxy *struct {
			Command string `json:"command"`
		} `json:"proxy"`
	} `json:"ssh"`
	Runtime *struct {
		Ports []struct {
			Private int    `json:"private"`
			Type    string `json:"type"`
		} `json:"ports"`
	} `json:"runtime"`
}

func (p podDTO) instance(ports []vendor.NamedPort) vendor.Instance {
	inst := vendor.Instance{
		ID:          p.ID,
		Status:      p.Status,
		CostPerHour: p.Cost,
		DataCenter:  p.DataCenterID,
	}
	for _, port := range ports {
		inst.Endpoints = append(inst.Endpoints, vendor.Endpoint{
			Name: port.Name,
			URL:  ProxyURL(p.ID, port.Container),
		})
	}
	if p.Runtime != nil {
		for _, port := range p.Runtime.Ports {
			if port.Private <= 0 {
				continue
			}
			name := fmt.Sprintf("port-%d", port.Private)
			inst.Endpoints = append(inst.Endpoints, vendor.Endpoint{
				Name: name,
				URL:  ProxyURL(p.ID, port.Private),
			})
		}
	}
	if p.SSH.Proxy != nil && p.SSH.Proxy.Command != "" {
		inst.Endpoints = append(inst.Endpoints, vendor.Endpoint{Name: "ssh", URL: p.SSH.Proxy.Command})
	}
	return inst
}

// ProxyURL is the RunPod HTTPS proxy for an HTTP port.
func ProxyURL(podID string, port int) string {
	return fmt.Sprintf("https://%s-%d.proxy.runpod.net", podID, port)
}

// FormatPorts converts preset ports into RunPod "port/protocol" specs.
func FormatPorts(name string, container int, expose string) (vendor.NamedPort, error) {
	if container <= 0 || container > 65535 {
		return vendor.NamedPort{}, fmt.Errorf("invalid container port %d", container)
	}
	var spec string
	switch strings.ToLower(expose) {
	case "", "tunnel", "http", "https":
		spec = fmt.Sprintf("%d/http", container)
	case "tcp":
		spec = fmt.Sprintf("%d/tcp", container)
	default:
		return vendor.NamedPort{}, fmt.Errorf("unsupported expose %q", expose)
	}
	if name == "" {
		name = fmt.Sprintf("port-%d", container)
	}
	return vendor.NamedPort{Name: name, Container: container, Spec: spec}, nil
}
