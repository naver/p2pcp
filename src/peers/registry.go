// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/utils"
)

// HeartbeatRequest represents the request structure for heartbeat
type HeartbeatRequest struct {
	UUID             string `json:"uuid" example:"123e4567-e89b-12d3-a456-426614174000"`
	Address          string `json:"address" example:"192.168.1.100:8080"`
	ExpiresInSeconds int    `json:"expires_in_seconds" example:"30"`
}

// Registry manages peer registration and heartbeat functionality
type Registry struct {
	// Configuration
	heartbeatURL      string
	address           string
	uuid              string
	heartbeatInterval int
	timeout           time.Duration

	// Control
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	isRunning atomic.Bool
}

// NewRegistry creates a new PeerRegistry instance
func NewRegistry(heartbeatURL string, address string, uuid string, heartbeatInterval time.Duration, timeout time.Duration) (*Registry, error) {
	if heartbeatURL == "" {
		return nil, errors.New("-peer-registry-heartbeat-url is required")
	}
	if address == "" {
		return nil, errors.New("-peer-registry-self-endpoint is required")
	}
	if uuid == "" {
		return nil, errors.New("uuid is required")
	}
	if heartbeatInterval <= 0 {
		return nil, errors.New("-peer-registry-heartbeat-interval must be greater than 0")
	}
	if timeout <= 0 {
		return nil, errors.New("-peer-registry-timeout must be greater than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		heartbeatURL:      heartbeatURL,
		address:           address,
		heartbeatInterval: int(heartbeatInterval.Seconds()),
		timeout:           timeout,
		uuid:              uuid,
		ctx:               ctx,
		cancel:            cancel,
	}, nil
}

// Start begins the peer registry service (registration + periodic heartbeats)
func (pr *Registry) Start() error {
	if pr.isRunning.Load() {
		return fmt.Errorf("peer registry is already running")
	}

	// Perform initial heartbeat
	if err := pr.heartbeat(pr.ctx); err != nil {
		utils.ErrorPrintf("Failed to send heartbeat: %s", err.Error())
		return fmt.Errorf("initial heartbeat failed: %w", err)
	}

	// Start heartbeat goroutine
	pr.isRunning.Store(true)
	pr.wg.Add(1)
	go func() {
		defer pr.wg.Done()

		ticker := time.NewTicker(time.Duration(pr.heartbeatInterval) * time.Second)
		defer ticker.Stop()

		utils.DebugPrintf("Starting heartbeat loop with interval: %v", pr.heartbeatInterval)

		for {
			select {
			case <-pr.ctx.Done():
				utils.DebugPrintf("Heartbeat loop stopped due to context cancellation")
				return

			case <-ticker.C:
				if err := pr.heartbeat(pr.ctx); err != nil {
					utils.ErrorPrintf("Heartbeat failed: %s", err.Error())
					// Continue trying even if heartbeat fails
				}
			}
		}
	}()

	utils.DebugPrintf("Peer registry started successfully")
	return nil
}

// Stop stops the peer registry service
func (pr *Registry) Stop() {
	if !pr.isRunning.Load() {
		return
	}

	pr.isRunning.Store(false)
	pr.cancel()

	// Wait for heartbeat goroutine to finish
	pr.wg.Wait()

	utils.DebugPrintf("Peer registry stopped")
}

// Heartbeat sends a heartbeat signal to maintain registration
func (pr *Registry) heartbeat(ctx context.Context) error {
	utils.DebugPrintf("Sending heartbeat to registry: %s", pr.heartbeatURL)

	request := HeartbeatRequest{
		UUID:    pr.uuid,
		Address: pr.address,
		// Expires in twice the heartbeat interval to ensure the peer is still registered
		// even if the heartbeat interval is longer than the expected heartbeat interval.
		ExpiresInSeconds: pr.heartbeatInterval * 2,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, pr.timeout)
	defer cancel()

	_, err = p2pcpHttp.RequestWithContext(ctx, pr.heartbeatURL,
		p2pcpHttp.WithMethod("POST"),
		p2pcpHttp.WithBody(bytes.NewBuffer(jsonData)),
		p2pcpHttp.WithContentType("application/json"),
	)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat request: %w", err)
	}

	utils.DebugPrintf("Successfully sent heartbeat for UUID: %s", pr.uuid)
	return nil
}
