// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"sync"
	"time"

	"github.com/naver/p2pcp/utils"
)

type Controller struct {
	ctx             context.Context
	provider        Provider
	peerWaitTimeout time.Duration
	pollInterval    time.Duration
	peers           map[string]bool

	onPeerAdded   func(host string)
	onPeerRemoved func(host string)
}

func NewController(ctx context.Context, provider Provider, peerWaitTimeout time.Duration, pollInterval time.Duration, onPeerAdded func(host string), onPeerRemoved func(host string)) *Controller {
	return &Controller{
		ctx:             ctx,
		provider:        provider,
		peerWaitTimeout: peerWaitTimeout,
		pollInterval:    pollInterval,
		peers:           map[string]bool{},

		onPeerAdded:   onPeerAdded,
		onPeerRemoved: onPeerRemoved,
	}
}

func (c *Controller) Run() {
	peerListTicker := time.NewTicker(c.pollInterval)
	defer peerListTicker.Stop()

	for {
		peers, err := c.getPeers()
		if err != nil {
			utils.DebugPrintf("Failed to get peer list: %s", err.Error())
		} else {
			c.reconcile(peers)
		}

		select {
		case <-c.ctx.Done():
			c.stopAll()
			return
		case <-peerListTicker.C:
			// next iteration
		}
	}
}

func (c *Controller) getPeers() ([]string, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.peerWaitTimeout)
	defer cancel()

	return c.provider.GetPeers(ctx)
}

func (c *Controller) reconcile(desired []string) {
	var wg sync.WaitGroup

	want := map[string]bool{}
	for _, h := range desired {
		want[h] = true
	}

	// start new workers
	for host := range want {
		if _, ok := c.peers[host]; ok {
			continue
		}

		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			c.onPeerAdded(h)
		}(host)

		c.peers[host] = true
	}

	// stop workers not desired
	var toStop []string
	for host := range c.peers {
		if !want[host] {
			toStop = append(toStop, host)
		}
	}

	for _, host := range toStop {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			c.onPeerRemoved(h)
		}(host)

		delete(c.peers, host)
	}

	wg.Wait()
}

func (c *Controller) stopAll() {
	c.reconcile([]string{})
}
