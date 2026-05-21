// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/utils"
)

type URLProvider struct {
	peers       []string
	url         string
	defaultPort int
}

func NewURLProvider(rawURL string, defaultPort int) (*URLProvider, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("-peer-list-url must start with http:// or https://")
	}

	return &URLProvider{
		url:         rawURL,
		defaultPort: defaultPort,
	}, nil
}

func (p *URLProvider) AddStaticPeers(peers []string) {
	p.peers = append(p.peers, peers...)
	p.peers = deduplicatePeers(p.peers)
}

func (p *URLProvider) GetPeers(ctx context.Context) ([]string, error) {
	content, err := p2pcpHttp.RequestWithContext(ctx, p.url)
	if err != nil {
		return nil, fmt.Errorf("failed to get peers: %w", err)
	}

	if len(content) == 0 {
		return p.peers, nil
	}

	raw := strings.Split(string(content), ",")
	peers := make([]string, 0, len(raw)+len(p.peers))
	for _, peer := range raw {
		normalized, err := normalizePeerAddress(peer, p.defaultPort)
		if err != nil {
			utils.ErrorPrintf("Invalid peer address: %s", peer)
			return nil, err
		}
		peers = append(peers, normalized)
	}
	peers = append(peers, p.peers...)

	return deduplicatePeers(peers), nil
}
