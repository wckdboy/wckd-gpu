// Package serve is the loopback JSON API over the session engine.
// Invincible supervises this process and proxies /v1/rent/* to it.
// The binary is not linked into the gateway.
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/config"
	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
	"github.com/wckdboy/wckd-gpu/cli/internal/runpod"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

const confirmMessage = "confirmation required; resend with confirm true to rent the chosen offer. Nothing was created."

// Server is one local rental API process.
type Server struct {
	Addr             string
	AllowNonLoopback bool
	Service          *session.Service
	PresetsDir       string
	Image            string
	Storage          session.StorageCreds
	Key              string
	SweepEvery       time.Duration
	Log              *log.Logger
	// Redact is extra secret material (provider keys) stripped from
	// responses and logs. The bearer key and storage credentials are always
	// included. Values shorter than 8 characters are ignored.
	Redact []string
	// Ready is closed after the listener is bound. Optional.
	Ready chan struct{}

	URL string
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type offerJSON struct {
	offer.Offer
	EstimateUSD float64 `json:"estimate_usd"`
}

type createBody struct {
	ProjectID      string    `json:"project_id"`
	PresetID       string    `json:"preset_id"`
	Hours          float64   `json:"hours"`
	OfferID        string    `json:"offer_id"`
	Confirm        bool      `json:"confirm"`
	RequireRegions *[]string `json:"require_regions"`
}

type stopBody struct {
	Force bool `json:"force"`
}

// Run listens, sweeps on a timer, and returns when ctx is cancelled or the
// listener fails. The first sweep runs immediately so a restarted process
// terminates sessions that are already past their deadline.
func (s *Server) Run(ctx context.Context) error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.New("serve key is empty")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ln, err := Listen(s.listenAddr(), s.AllowNonLoopback)
	if err != nil {
		return err
	}
	s.URL = "http://" + ln.Addr().String()
	httpSrv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          s.Log,
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.sweepLoop(ctx)
	}()
	errCh := make(chan error, 1)
	go func() {
		err := httpSrv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	s.signalReady()
	s.logf("listening %s", s.URL)

	select {
	case <-ctx.Done():
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := httpSrv.Shutdown(shutCtx)
		shutCancel()
		wg.Wait()
		serveErr := <-errCh
		if err != nil {
			return err
		}
		return serveErr
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
}

func (s *Server) listenAddr() string {
	if strings.TrimSpace(s.Addr) == "" {
		return "127.0.0.1:8787"
	}
	return s.Addr
}

func (s *Server) signalReady() {
	if s.Ready == nil {
		return
	}
	select {
	case <-s.Ready:
	default:
		close(s.Ready)
	}
}

func (s *Server) sweepLoop(ctx context.Context) {
	s.sweepOnce(ctx)
	interval := s.SweepEvery
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOnce(ctx)
		}
	}
}

func (s *Server) sweepOnce(ctx context.Context) {
	if s.Service == nil {
		return
	}
	if err := s.Service.Sweep(ctx); err != nil && ctx.Err() == nil {
		s.logf("sweep: %v", err)
	}
}

// Handler is the rental API. Every route requires the bearer key.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/rent/health", s.health)
	mux.HandleFunc("/v1/rent/offers", s.offers)
	mux.HandleFunc("/v1/rent/sessions", s.sessions)
	mux.HandleFunc("/v1/rent/sessions/{id}", s.sessionByID)
	mux.HandleFunc("/v1/rent/sessions/{id}/stop", s.stopSession)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, http.StatusNotFound, "not_found", "unknown route")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorize(r, s.Key) {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			s.logf("%s %s 401", r.Method, r.URL.Path)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		mux.ServeHTTP(rec, r)
		s.logf("%s %s %d", r.Method, r.URL.Path, rec.code)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.methodNotAllowed(w, http.MethodGet)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "engine": "wckd"})
}

func (s *Server) offers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.methodNotAllowed(w, http.MethodGet)
		return
	}
	presetID := strings.TrimSpace(r.URL.Query().Get("preset"))
	if err := config.ValidatePresetID(presetID); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	hours, err := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("hours")), 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "hours is required")
		return
	}
	if err := config.ValidateHours(hours); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	p, err := preset.LoadID(s.PresetsDir, presetID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	override, err := requireFromQuery(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	policy, err := offer.ResolveRequire(p.Spec.Constraints.RequireRegions, override)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	offers, err := s.Service.RankedOverride(r.Context(), p, override)
	if err != nil {
		s.writeVendorError(w, err)
		return
	}
	if limit > 0 && limit < len(offers) {
		offers = offers[:limit]
	}
	rows := make([]offerJSON, 0, len(offers))
	for _, o := range offers {
		rows = append(rows, priceOffer(o, hours))
	}
	regions := policy.Regions
	if regions == nil {
		regions = []string{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"preset_id":       presetID,
		"hours":           hours,
		"require_regions": regions,
		"require_secure":  policy.Secure,
		"cost_basis":      session.CostBasisEstimate,
		"offers":          rows,
	})
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSessions(w, r)
	case http.MethodPost:
		s.createSession(w, r)
	default:
		s.methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.Service.Sessions.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal", s.scrub(err.Error()))
		return
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		}
		return list[i].ID > list[j].ID
	})
	if list == nil {
		list = []session.Session{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if err := decodeJSON(r.Body, &body); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if err := config.ValidateProjectID(body.ProjectID); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := config.ValidatePresetID(body.PresetID); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := config.ValidateHours(body.Hours); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	p, err := preset.LoadID(s.PresetsDir, body.PresetID)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.RequireRegions != nil {
		if _, err := offer.ParseRequire(*body.RequireRegions); err != nil {
			s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}
	ports, err := namedPorts(p)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	candidates, err := s.Service.CandidatesOverride(r.Context(), p, strings.TrimSpace(body.OfferID), body.RequireRegions)
	if err != nil {
		s.writeVendorError(w, err)
		return
	}
	policy, err := offer.ResolveRequire(p.Spec.Constraints.RequireRegions, body.RequireRegions)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	chosen := candidates[0]
	// Query-string confirm is ignored on purpose. Approval is a JSON field or header.
	if !body.Confirm && !headerConfirm(r) {
		regions := policy.Regions
		if regions == nil {
			regions = []string{}
		}
		failover := make([]offerJSON, 0, len(candidates))
		for _, o := range candidates {
			failover = append(failover, priceOffer(o, body.Hours))
		}
		s.writeJSON(w, http.StatusConflict, map[string]any{
			"error":             apiError{Code: "confirmation_required", Message: confirmMessage},
			"project_id":        body.ProjectID,
			"preset_id":         body.PresetID,
			"hours":             body.Hours,
			"cost_basis":        session.CostBasisEstimate,
			"cost_estimate_usd": session.Estimate(chosen.USDPerHour, body.Hours),
			"require_regions":   regions,
			"require_secure":    policy.Secure,
			"offer":             priceOffer(chosen, body.Hours),
			"failover":          failover,
		})
		return
	}
	sess, err := s.Service.Start(r.Context(), session.StartRequest{
		ProjectID:      body.ProjectID,
		Preset:         p,
		Hours:          body.Hours,
		OfferID:        strings.TrimSpace(body.OfferID),
		Image:          p.Image(s.Image),
		Ports:          ports,
		Storage:        s.Storage,
		RequireRegions: body.RequireRegions,
	})
	if err != nil {
		if sess.ID != "" {
			s.logf("provision %s: %v", sess.ID, err)
			s.writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":   apiError{Code: "provision_failed", Message: s.scrub(err.Error())},
				"session": sess,
			})
			return
		}
		s.writeVendorError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) sessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.methodNotAllowed(w, http.MethodGet)
		return
	}
	id := r.PathValue("id")
	if !session.ValidID(id) {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid session id")
		return
	}
	sess, err := s.Service.Refresh(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}
		local, loadErr := s.Service.Sessions.Load(id)
		if loadErr != nil {
			if os.IsNotExist(loadErr) {
				s.writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			s.writeError(w, http.StatusBadGateway, "vendor_error", s.scrub(err.Error()))
			return
		}
		local.Warning = s.scrub(err.Error())
		s.writeJSON(w, http.StatusOK, local)
		return
	}
	s.writeJSON(w, http.StatusOK, sess)
}

func (s *Server) stopSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.methodNotAllowed(w, http.MethodPost)
		return
	}
	id := r.PathValue("id")
	if !session.ValidID(id) {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid session id")
		return
	}
	var body stopBody
	if err := decodeOptionalJSON(r.Body, &body); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	sess, err := s.Service.Stop(r.Context(), id, body.Force)
	if errors.Is(err, session.ErrDrainFailed) {
		s.writeJSON(w, http.StatusConflict, map[string]any{
			"error":   apiError{Code: "drain_failed", Message: "drain did not finish; pod left running"},
			"session": sess,
		})
		return
	}
	if err != nil {
		if os.IsNotExist(err) {
			s.writeError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}
		if sess.ID != "" {
			s.writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":   apiError{Code: "stop_failed", Message: s.scrub(err.Error())},
				"session": sess,
			})
			return
		}
		s.writeError(w, http.StatusBadGateway, "stop_failed", s.scrub(err.Error()))
		return
	}
	s.writeJSON(w, http.StatusOK, sess)
}

func (s *Server) writeVendorError(w http.ResponseWriter, err error) {
	msg := s.scrub(err.Error())
	code := "vendor_error"
	status := http.StatusBadGateway
	switch {
	case strings.Contains(msg, "no RunPod offers"), strings.Contains(msg, "not in the ranked list"):
		code = "no_offers"
		status = http.StatusUnprocessableEntity
	case strings.Contains(msg, "require_regions"):
		code = "bad_request"
		status = http.StatusBadRequest
	}
	s.writeError(w, status, code, msg)
}

func (s *Server) methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", allow+" is required")
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, msg string) {
	if status >= 500 {
		s.logf("%s %s", code, msg)
	}
	s.writeJSON(w, status, map[string]any{
		"error": apiError{Code: code, Message: s.scrub(msg)},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":{"code":"internal","message":"encode failed"}}`+"\n", http.StatusInternalServerError)
		return
	}
	body = []byte(s.scrub(string(body)))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

func (s *Server) logf(format string, args ...any) {
	if s.Log == nil {
		return
	}
	s.Log.Print(s.scrub(fmt.Sprintf(format, args...)))
}

func (s *Server) scrub(msg string) string {
	secs := s.secretValues()
	sort.Slice(secs, func(i, j int) bool { return len(secs[i]) > len(secs[j]) })
	for _, sec := range secs {
		msg = strings.ReplaceAll(msg, sec, "[redacted]")
	}
	return msg
}

func (s *Server) secretValues() []string {
	raw := []string{s.Key, s.Storage.AccessKey, s.Storage.SecretKey}
	raw = append(raw, s.Redact...)
	var out []string
	seen := map[string]bool{}
	for _, sec := range raw {
		sec = strings.TrimSpace(sec)
		if len(sec) < 8 || seen[sec] {
			continue
		}
		seen[sec] = true
		out = append(out, sec)
	}
	return out
}

func priceOffer(o offer.Offer, hours float64) offerJSON {
	return offerJSON{Offer: o, EstimateUSD: session.Estimate(o.USDPerHour, hours)}
}

func requireFromQuery(r *http.Request) (*[]string, error) {
	if !r.URL.Query().Has("require_regions") {
		return nil, nil
	}
	parts := offer.SplitList(r.URL.Query().Get("require_regions"))
	if _, err := offer.ParseRequire(parts); err != nil {
		return nil, err
	}
	return &parts, nil
}

func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 20, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, errors.New("limit must be a non-negative integer")
	}
	if n == 0 {
		return 20, nil
	}
	if n > 100 {
		return 100, nil
	}
	return n, nil
}

func namedPorts(p preset.File) ([]vendor.NamedPort, error) {
	var ports []vendor.NamedPort
	for _, port := range p.Spec.Runtime.Ports {
		np, err := runpod.FormatPorts(port.Name, port.Container, port.Expose)
		if err != nil {
			return nil, err
		}
		ports = append(ports, np)
	}
	return ports, nil
}

func decodeJSON(body io.Reader, dst any) error {
	if body == nil {
		return errors.New("invalid JSON body")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, io.NopCloser(body), 1<<20))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func decodeOptionalJSON(body io.Reader, dst any) error {
	if body == nil {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, io.NopCloser(body), 1<<20))
	err := dec.Decode(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type statusRecorder struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wrote {
		return
	}
	s.wrote = true
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}
