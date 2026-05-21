// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"strings"

	"github.com/naver/p2pcp/utils"
)

type StaticProvider struct {
	peers []string
}

func NewStaticProvider(sources string, defaultPort int) (*StaticProvider, error) {
	raw := strings.Split(sources, ",")

	provider := &StaticProvider{}
	for _, peer := range raw {
		normalized, err := normalizePeerAddress(peer, defaultPort)
		if err != nil {
			utils.ErrorPrintf("Invalid peer address: %s", peer)
			return nil, err
		}
		provider.peers = append(provider.peers, normalized)
	}

	provider.peers = deduplicatePeers(provider.peers)
	return provider, nil
}

func (p *StaticProvider) AddStaticPeers(peers []string) {
	p.peers = append(p.peers, peers...)
	p.peers = deduplicatePeers(p.peers)
}

func (p *StaticProvider) GetPeers(ctx context.Context) ([]string, error) {
	return p.peers, nil
}
