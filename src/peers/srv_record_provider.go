// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type SRVRecordProvider struct {
	peers    []string
	fqdn     string
	dialer   *net.Dialer
	resolver *net.Resolver
}

func NewSRVRecordProvider(fqdn string, dnsLookupTimeout time.Duration, dnsServer string) (*SRVRecordProvider, error) {
	fqdn = strings.TrimSpace(fqdn)
	if fqdn == "" {
		return nil, fmt.Errorf("empty FQDN for SRV record lookup")
	}

	dialer := &net.Dialer{
		Timeout: dnsLookupTimeout,
	}

	var resolver *net.Resolver
	if dnsServer != "" {
		resolver = &net.Resolver{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, dnsServer)
			},
		}
	} else {
		resolver = &net.Resolver{Dial: dialer.DialContext}
	}

	return &SRVRecordProvider{
		fqdn:     fqdn,
		dialer:   dialer,
		resolver: resolver,
	}, nil
}

func (p *SRVRecordProvider) AddStaticPeers(peers []string) {
	p.peers = append(p.peers, peers...)
	p.peers = deduplicatePeers(p.peers)
}

func (p *SRVRecordProvider) GetPeers(ctx context.Context) ([]string, error) {
	_, srvs, err := p.resolver.LookupSRV(ctx, "", "", p.fqdn)
	if err != nil {
		return nil, fmt.Errorf("SRV lookup failed: %w", err)
	}

	peers := make([]string, 0, len(srvs)+len(p.peers))
	for _, srv := range srvs {
		ips, err := p.resolver.LookupIP(ctx, "ip", srv.Target)
		if err != nil {
			return nil, fmt.Errorf("host lookup failed for %s: %w", srv.Target, err)
		}

		for _, ip := range ips {
			peers = append(peers, net.JoinHostPort(ip.String(), fmt.Sprintf("%d", srv.Port)))
		}
	}
	peers = append(peers, p.peers...)

	return deduplicatePeers(peers), nil
}
