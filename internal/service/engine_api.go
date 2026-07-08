package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var engineLog = logger.WithScope("engine-api")

// EngineAPIClient talks to the telemt engine's loopback control-plane API
// ([server.api]). It is used to reset a user's quota counter in the running
// engine without recreating the container.
type EngineAPIClient struct {
	http *http.Client
}

// NewEngineAPIClient creates a client with a short timeout suitable for the
// local loopback API.
func NewEngineAPIClient() *EngineAPIClient {
	return &EngineAPIClient{http: &http.Client{Timeout: 2 * time.Second}}
}

// ResetUserQuota calls POST /v1/users/{label}/reset-quota on the instance's
// loopback API. A 200 (reset) and a 404 (user not present on that instance) are
// both treated as success; only connectivity or 5xx/other failures return an
// error. A non-positive apiPort means the instance has no API and is skipped.
func (e *EngineAPIClient) ResetUserQuota(ctx context.Context, apiPort int, label string) error {
	if e == nil || e.http == nil {
		return nil
	}
	if apiPort <= 0 {
		return nil
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v1/users/%s/reset-quota", apiPort, url.PathEscape(label))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build reset-quota request: %w", err)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return fmt.Errorf("reset-quota %q on :%d: %w", label, apiPort, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotFound:
		return nil
	default:
		return fmt.Errorf("reset-quota %q on :%d: unexpected status %d", label, apiPort, resp.StatusCode)
	}
}

// ResetLabel resets a single user's engine quota across every enabled instance
// that exposes an API port. It does not tag-match: instances where the user is
// absent answer 404 (treated as success), so this stays correct with a tiny
// instance count. Best-effort — failures are logged, never returned.
func (e *EngineAPIClient) ResetLabel(ctx context.Context, instances []model.Instance, label string) {
	if e == nil {
		return
	}
	for i := range instances {
		inst := &instances[i]
		if !inst.Enabled || inst.APIPort <= 0 {
			continue
		}
		if err := e.ResetUserQuota(ctx, inst.APIPort, label); err != nil {
			engineLog.Warnf("engine quota reset skipped: %v", err)
		}
	}
}

// ResetAll resets every secret's engine quota across the instances it is served
// by. It tag-matches secrets to instances to avoid a full N×M fan-out. The SQLite
// ledger reset is expected to have already happened; this only propagates it to
// the running engines. Best-effort.
func (e *EngineAPIClient) ResetAll(ctx context.Context, instances []model.Instance, secrets []model.Secret) {
	if e == nil {
		return
	}
	for i := range instances {
		inst := &instances[i]
		if !inst.Enabled || inst.APIPort <= 0 {
			continue
		}
		instTags := inst.GetTags()
		for j := range secrets {
			sec := &secrets[j]
			if !model.TagsMatch(instTags, sec.GetTags()) {
				continue
			}
			if err := e.ResetUserQuota(ctx, inst.APIPort, sec.Label); err != nil {
				engineLog.Warnf("engine quota reset skipped: %v", err)
			}
		}
	}
}
