// Package vendor is the GPU provider port. P1 ships a RunPod adapter; further
// vendors implement the same interface.
package vendor

import (
	"context"
	"errors"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
)

// Endpoint is a user-facing URL or command for a running pod.
type Endpoint struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Instance is a provisioned pod.
type Instance struct {
	ID          string
	Status      string
	CostPerHour float64
	DataCenter  string
	Endpoints   []Endpoint
}

// NamedPort is a preset port plus the vendor's port spec (for example "8188/http").
type NamedPort struct {
	Name      string
	Container int
	Spec      string
}

// ProvisionRequest is everything an adapter needs to create one pod.
type ProvisionRequest struct {
	Name          string
	Image         string
	Cloud         string
	GPUID         string
	MinRAMGB      int
	DiskGB        int
	Ports         []NamedPort
	Env           map[string]string
	DataCenterIDs []string
}

// Vendor lists offers and owns instance lifecycle.
type Vendor interface {
	ListOffers(ctx context.Context, q offer.Query) ([]offer.Offer, error)
	Provision(ctx context.Context, req ProvisionRequest) (Instance, error)
	Status(ctx context.Context, id string) (Instance, error)
	Terminate(ctx context.Context, id string) error
}

type httpStatus interface {
	HTTPStatus() int
}

// FatalProvision reports errors that will not succeed on a different offer.
// 400 and 403 are treated as "try the next candidate" because RunPod uses 400
// both for capacity misses and for some rule violations.
func FatalProvision(err error) bool {
	var hs httpStatus
	if !errors.As(err, &hs) {
		return false
	}
	switch hs.HTTPStatus() {
	case 401, 402, 422:
		return true
	default:
		return false
	}
}
