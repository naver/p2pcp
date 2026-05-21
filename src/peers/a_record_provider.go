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

type ARecordProvider struct {
	peers       []string
	fqdn        string
	defaultPort int
	dialer      *net.Dialer
	resolver    *net.Resolver
}

func NewARecordProvider(fqdn string, dnsLookupTimeout time.Duration, defaultPort int, dnsServer string) (*ARecordProvider, error) {
	fqdn = strings.TrimSpace(fqdn)
	if fqdn == "" {
		return nil, fmt.Errorf("empty FQDN for A record lookup")
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

	return &ARecordProvider{
		fqdn:        fqdn,
		defaultPort: defaultPort,
		dialer:      dialer,
		resolver:    resolver,
	}, nil
}

func (p *ARecordProvider) AddStaticPeers(peers []string) {
	p.peers = append(p.peers, peers...)
	p.peers = deduplicatePeers(p.peers)
}

func (p *ARecordProvider) GetPeers(ctx context.Context) ([]string, error) {
	ips, err := p.resolver.LookupIP(ctx, "ip", p.fqdn)
	if err != nil {
		return nil, err
	}

	peers := make([]string, 0, len(ips)+len(p.peers))
	for _, ip := range ips {
		peers = append(peers, net.JoinHostPort(ip.String(), fmt.Sprintf("%d", p.defaultPort)))
	}
	peers = append(peers, p.peers...)

	return deduplicatePeers(peers), nil
}
