package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

// ErrDrainFailed means the pod is still running because S3 drain did not finish.
var ErrDrainFailed = errors.New("drain failed")

// ObjectStore is the subset of S3 the session service uses.
type ObjectStore interface {
	Put(ctx context.Context, key string, body []byte) error
	Exists(ctx context.Context, key string) (bool, error)
}

// Service owns start, refresh, stop, and the deadline sweep.
type Service struct {
	Vendor    vendor.Vendor
	Objects   ObjectStore
	Sessions  *Store
	Clock     func() time.Time
	DrainWait time.Duration
	DrainPoll time.Duration
	Failover  int
}

// StartRequest is one `wckd start`.
type StartRequest struct {
	ProjectID string
	Preset    preset.File
	Hours     float64
	OfferID   string
	Image     string
	Ports     []vendor.NamedPort
	Storage   StorageCreds
}

// Ranked returns every offer that matches the preset, best first.
func (s *Service) Ranked(ctx context.Context, p preset.File) ([]offer.Offer, error) {
	return s.Vendor.ListOffers(ctx, offer.Query{
		WorkloadClass: p.Spec.WorkloadClass,
		Constraints: offer.Constraints{
			MinVRAMGB:     p.Spec.Constraints.MinVRAMGB,
			GPUFamilies:   p.Spec.Constraints.GPUFamilies,
			Reliability:   p.Spec.Constraints.Reliability,
			PreferRegions: p.Spec.Constraints.PreferRegions,
		},
	})
}

// Candidates returns the offers start would try, best first.
// A locked offer id yields that offer only. Otherwise the result is capped
// at the failover window (default 3).
func (s *Service) Candidates(ctx context.Context, p preset.File, offerID string) ([]offer.Offer, error) {
	offers, err := s.Ranked(ctx, p)
	if err != nil {
		return nil, err
	}
	return selectOffers(offers, offerID, s.failover())
}

// Start provisions a pod and records the session locally and in S3.
func (s *Service) Start(ctx context.Context, req StartRequest) (Session, error) {
	candidates, err := s.Candidates(ctx, req.Preset, req.OfferID)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	id, err := NewID()
	if err != nil {
		return Session{}, err
	}
	sess := Session{
		ID:         id,
		ProjectID:  req.ProjectID,
		PresetID:   req.Preset.Metadata.ID,
		Hours:      req.Hours,
		CreatedAt:  now,
		DeadlineAt: now.Add(time.Duration(req.Hours * float64(time.Hour))),
		Phase:      PhasePending,
		Offer:      candidates[0],
	}
	sess.CostEstimateUSD = Estimate(candidates[0].USDPerHour, req.Hours)
	sess.addEvent(now, "session created")
	if err := s.Sessions.Create(sess); err != nil {
		return Session{}, err
	}
	if err := s.Sessions.SetCurrent(sess.ID); err != nil {
		return Session{}, err
	}

	sess, err = s.Sessions.Mutate(sess.ID, func(sess *Session) error {
		if err := s.writeProject(ctx, sess, req); err != nil {
			return s.fail(ctx, sess, err)
		}
		if err := s.sync(ctx, sess); err != nil {
			return s.fail(ctx, sess, err)
		}
		return nil
	})
	if err != nil {
		return sess, err
	}

	return s.Sessions.Mutate(sess.ID, func(sess *Session) error {
		env := PodEnv(*sess, req.Preset, req.Storage)
		var last error
		for i, candidate := range candidates {
			inst, err := s.Vendor.Provision(ctx, vendor.ProvisionRequest{
				Name:          "wckd-" + sess.ID,
				Image:         req.Image,
				Cloud:         candidate.Cloud,
				GPUID:         candidate.SKU,
				MinRAMGB:      req.Preset.Spec.Constraints.MinRAMGB,
				DiskGB:        diskGB(req.Preset),
				Ports:         req.Ports,
				Env:           env,
				DataCenterIDs: candidate.DataCenterIDs,
			})
			if err != nil {
				last = fmt.Errorf("provision %s: %w", candidate.ID, err)
				sess.addEvent(s.now(), last.Error())
				if vendor.FatalProvision(err) || i == len(candidates)-1 {
					break
				}
				continue
			}
			sess.Offer = candidate
			sess.CostEstimateUSD = Estimate(candidate.USDPerHour, sess.Hours)
			sess.InstanceID = inst.ID
			sess.PodStatus = inst.Status
			sess.DataCenter = inst.DataCenter
			sess.Endpoints = mergeEndpoints(sess.Endpoints, inst.Endpoints)
			if err := Transition(sess.Phase, PhaseHydrating); err != nil {
				return err
			}
			sess.Phase = PhaseHydrating
			sess.Error = ""
			sess.addEvent(s.now(), "pod "+inst.ID+" created")
			_ = s.sync(ctx, sess)
			return nil
		}
		if last == nil {
			last = errors.New("provision failed")
		}
		return s.fail(ctx, sess, last)
	})
}

// Refresh polls the vendor and promotes hydrating → ready when the sidecar
// writes the hydrate marker.
func (s *Service) Refresh(ctx context.Context, id string) (Session, error) {
	return s.Sessions.Mutate(id, func(sess *Session) error {
		if !sess.Phase.Active() {
			return nil
		}
		if sess.InstanceID != "" && s.Vendor != nil {
			inst, err := s.Vendor.Status(ctx, sess.InstanceID)
			if err != nil {
				return err
			}
			sess.PodStatus = inst.Status
			if inst.DataCenter != "" {
				sess.DataCenter = inst.DataCenter
			}
			sess.Endpoints = mergeEndpoints(sess.Endpoints, inst.Endpoints)
		}
		if sess.Phase == PhaseHydrating && s.Objects != nil {
			ok, err := s.Objects.Exists(ctx, hydrateKey(sess.ID))
			if err != nil {
				return err
			}
			if ok {
				if err := Transition(sess.Phase, PhaseReady); err != nil {
					return err
				}
				sess.Phase = PhaseReady
				sess.addEvent(s.now(), "hydrate marker found")
				_ = s.sync(ctx, sess)
			}
		}
		return nil
	})
}

// Stop drains project data to S3 and then terminates the pod.
// Without force, a missing drain marker leaves the pod running and returns ErrDrainFailed.
func (s *Service) Stop(ctx context.Context, id string, force bool) (Session, error) {
	return s.Sessions.Mutate(id, func(sess *Session) error {
		if sess.Phase == PhaseTerminated {
			return nil
		}
		if sess.Phase == PhaseFailed && sess.InstanceID == "" {
			if sess.Error != "" {
				return fmt.Errorf("session %s failed: %s", sess.ID, sess.Error)
			}
			return fmt.Errorf("session %s failed", sess.ID)
		}
		if sess.Phase != PhaseDraining {
			if err := Transition(sess.Phase, PhaseDraining); err != nil {
				return err
			}
			sess.Phase = PhaseDraining
			sess.addEvent(s.now(), "drain requested")
		}
		if sess.InstanceID == "" {
			return s.finish(ctx, sess, true, false)
		}
		if s.Objects == nil {
			sess.Error = "object store is not configured"
			return errors.New(sess.Error)
		}
		if err := s.putControl(ctx, sess.ID); err != nil {
			sess.Error = err.Error()
			return err
		}
		ok, err := s.waitDrain(ctx, sess.ID)
		if err != nil {
			sess.Error = err.Error()
			return err
		}
		if !ok && !force {
			sess.Error = "drain did not finish; pod left running"
			sess.addEvent(s.now(), sess.Error)
			return ErrDrainFailed
		}
		if err := s.Vendor.Terminate(ctx, sess.InstanceID); err != nil {
			sess.Error = err.Error()
			return err
		}
		return s.finish(ctx, sess, ok, force && !ok)
	})
}

// Sweep refreshes live sessions and stops anything past its deadline.
// A session already draining is retried so a crashed stop can finish.
func (s *Service) Sweep(ctx context.Context) error {
	list, err := s.Sessions.List()
	if err != nil {
		return err
	}
	now := s.now()
	var errs []error
	for _, sess := range list {
		if sess.Phase == PhaseTerminated || sess.Phase == PhaseFailed {
			continue
		}
		due := !sess.DeadlineAt.IsZero() && !now.Before(sess.DeadlineAt)
		if due || sess.Phase == PhaseDraining {
			if _, err := s.Stop(ctx, sess.ID, false); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", sess.ID, err))
			}
			continue
		}
		if _, err := s.Refresh(ctx, sess.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sess.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) finish(ctx context.Context, sess *Session, drainOK, forced bool) error {
	if err := Transition(sess.Phase, PhaseTerminated); err != nil {
		return err
	}
	now := s.now()
	sess.Phase = PhaseTerminated
	sess.DrainOK = drainOK
	sess.Forced = forced
	sess.EndedAt = &now
	sess.CostActualUSD = Actual(sess.Offer.USDPerHour, sess.CreatedAt, now)
	if forced {
		sess.Error = "terminated with --force before drain completed"
	} else {
		sess.Error = ""
	}
	sess.addEvent(now, "terminated")
	if s.Objects != nil {
		body, err := json.MarshalIndent(receiptFrom(*sess), "", "  ")
		if err != nil {
			return err
		}
		if err := s.Objects.Put(ctx, receiptKey(sess.ID), append(body, '\n')); err != nil {
			sess.Warning = "pod terminated but receipt upload failed: " + err.Error()
		}
		_ = s.sync(ctx, sess)
	}
	return nil
}

func (s *Service) fail(ctx context.Context, sess *Session, cause error) error {
	if sess.Phase != PhaseFailed {
		if err := Transition(sess.Phase, PhaseFailed); err != nil {
			return err
		}
		sess.Phase = PhaseFailed
	}
	sess.Error = cause.Error()
	sess.addEvent(s.now(), sess.Error)
	_ = s.sync(ctx, sess)
	return cause
}

func (s *Service) writeProject(ctx context.Context, sess *Session, req StartRequest) error {
	if s.Objects == nil {
		return errors.New("object store is not configured")
	}
	doc := map[string]any{
		"id":                sess.ProjectID,
		"default_preset_id": sess.PresetID,
		"s3": map[string]string{
			"bucket": req.Storage.Bucket,
			"prefix": "projects/" + sess.ProjectID + "/",
		},
	}
	if err := s.putJSON(ctx, projectKey(sess.ProjectID), doc); err != nil {
		return err
	}
	return s.putJSON(ctx, lastSessionKey(sess.ProjectID), map[string]string{"session_id": sess.ID})
}

func (s *Service) sync(ctx context.Context, sess *Session) error {
	if s.Objects == nil {
		return nil
	}
	if err := s.putJSON(ctx, manifestKey(sess.ID), sess); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, ev := range sess.Events {
		line, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return s.Objects.Put(ctx, eventsKey(sess.ID), buf.Bytes())
}

func (s *Service) putControl(ctx context.Context, id string) error {
	return s.putJSON(ctx, controlKey(id), map[string]any{
		"action":       "drain",
		"requested_at": s.now().UTC().Format(time.RFC3339),
	})
}

func (s *Service) putJSON(ctx context.Context, key string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return s.Objects.Put(ctx, key, body)
}

func (s *Service) waitDrain(ctx context.Context, id string) (bool, error) {
	deadline := time.Now().Add(s.drainWait())
	for {
		ok, err := s.Objects.Exists(ctx, drainKey(id))
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		timer := time.NewTimer(s.drainPoll())
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func (s *Service) drainWait() time.Duration {
	if s.DrainWait > 0 {
		return s.DrainWait
	}
	return 2 * time.Minute
}

func (s *Service) drainPoll() time.Duration {
	if s.DrainPoll > 0 {
		return s.DrainPoll
	}
	return 5 * time.Second
}

func (s *Service) failover() int {
	if s.Failover > 0 {
		return s.Failover
	}
	return 3
}

func selectOffers(offers []offer.Offer, offerID string, failover int) ([]offer.Offer, error) {
	if len(offers) == 0 {
		return nil, errors.New("no RunPod offers matched the preset constraints")
	}
	if offerID != "" {
		for _, o := range offers {
			if o.ID == offerID {
				return []offer.Offer{o}, nil
			}
		}
		return nil, fmt.Errorf("offer %s is not in the ranked list for this preset", offerID)
	}
	if failover <= 0 {
		failover = 3
	}
	if failover > len(offers) {
		failover = len(offers)
	}
	return offers[:failover], nil
}

func diskGB(p preset.File) int {
	if p.Spec.Constraints.MinDiskGB > 0 {
		return p.Spec.Constraints.MinDiskGB
	}
	return 50
}

func mergeEndpoints(old, neu []vendor.Endpoint) []vendor.Endpoint {
	if len(neu) == 0 {
		return old
	}
	by := map[string]vendor.Endpoint{}
	var order []string
	add := func(e vendor.Endpoint) {
		if _, ok := by[e.Name]; !ok {
			order = append(order, e.Name)
		}
		by[e.Name] = e
	}
	for _, e := range old {
		add(e)
	}
	for _, e := range neu {
		add(e)
	}
	out := make([]vendor.Endpoint, 0, len(order))
	for _, name := range order {
		out = append(out, by[name])
	}
	return out
}

func projectKey(projectID string) string {
	return "projects/" + projectID + "/.wckd/project.json"
}

func lastSessionKey(projectID string) string {
	return "projects/" + projectID + "/.wckd/last_session.json"
}

func manifestKey(id string) string { return "sessions/" + id + "/manifest.json" }
func eventsKey(id string) string   { return "sessions/" + id + "/events.jsonl" }
func receiptKey(id string) string  { return "sessions/" + id + "/receipt.json" }
func controlKey(id string) string  { return "sessions/" + id + "/control.json" }
func hydrateKey(id string) string  { return "sessions/" + id + "/hydrate-ok.json" }
func drainKey(id string) string    { return "sessions/" + id + "/drain-ok.json" }
