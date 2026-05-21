// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// Provider abstracts a source that can return a list of peers.
// Returned entries can be host, host:port, or FQDN; the caller may normalize them.
type Provider interface {
	AddStaticPeers(peers []string)
	GetPeers(ctx context.Context) ([]string, error)
}

type Factory struct {
	staticProvider   Provider
	defaultPort      int
	peerWaitTimeout  time.Duration
	dnsLookupTimeout time.Duration
}

func NewFactory(staticPeers string, defaultPort int, peerWaitTimeout time.Duration, dnsLookupTimeout time.Duration) (*Factory, error) {
	factory := Factory{
		staticProvider:   nil,
		defaultPort:      defaultPort,
		peerWaitTimeout:  peerWaitTimeout,
		dnsLookupTimeout: dnsLookupTimeout,
	}

	if staticPeers == "" {
		return &factory, nil
	}

	staticProvider, err := NewStaticProvider(staticPeers, defaultPort)
	if err != nil {
		return nil, err
	}
	factory.staticProvider = staticProvider

	return &factory, nil
}

func (f *Factory) Create(options ...Option) (Provider, error) {
	config := Config{}
	for _, option := range options {
		option(&config)
	}

	optionCount := 0
	if config.aRecordFQDN != "" {
		optionCount++
	}
	if config.srvRecordFQDN != "" {
		optionCount++
	}
	if config.url != "" {
		optionCount++
	}
	if optionCount > 1 {
		return nil, fmt.Errorf("cannot specify multiple remote peer list options: please specify only one of -peer-list-url, -peer-list-srv, or -peer-list-a")
	}

	var provider Provider
	var err error
	if config.aRecordFQDN != "" {
		provider, err = NewARecordProvider(config.aRecordFQDN, f.dnsLookupTimeout, f.defaultPort, config.dnsServer)
	} else if config.srvRecordFQDN != "" {
		provider, err = NewSRVRecordProvider(config.srvRecordFQDN, f.dnsLookupTimeout, config.dnsServer)
	} else if config.url != "" {
		provider, err = NewURLProvider(config.url, f.defaultPort)
	}
	if err != nil {
		return nil, err
	}

	if provider == nil {
		return f.staticProvider, nil
	}

	if f.staticProvider != nil {
		peers, err := f.staticProvider.GetPeers(context.Background())
		if err != nil {
			return nil, err
		}
		provider.AddStaticPeers(peers)
	}

	return provider, nil
}

// normalizePeerAddress normalizes a peer address to a host:port string.
// It handles IPv6 addresses, hostnames, and port numbers.
// It returns an error if the address is invalid.
func normalizePeerAddress(raw string, defaultPort int) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("empty peer address")
	}

	// Remove scheme using url.Parse (handles mixed case like HTTP://, Https://)
	// Note: url.Parse may fail for inputs like "127.0.0.1:8080" (colon in first path segment),
	// so we only process when parsing succeeds AND scheme is http/https.
	parsed, err := url.Parse(s)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		if parsed.Host == "" {
			return "", errors.New("empty host in URL")
		}
		// Reconstruct without scheme: Host + Path
		s = parsed.Host
		if parsed.Path != "" {
			s += parsed.Path
		}
	}

	// Verify the ability to create a valid URL.
	u, err := url.Parse("http://" + s)
	if err != nil {
		return "", fmt.Errorf("invalid peer address: %s", err.Error())
	}

	if u.Path != "" {
		return strings.TrimSuffix(s, "/"), nil
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		// No port specified, add default port
		host = u.Host
		// Remove brackets from IPv6 if present (JoinHostPort will add them)
		if len(host) > 2 && host[0] == '[' && host[len(host)-1] == ']' {
			host = host[1 : len(host)-1]
		}
		port = fmt.Sprintf("%d", defaultPort)
	}
	if len(host) == 0 {
		return "", errors.New("empty host")
	}
	return net.JoinHostPort(host, port), nil
}

// deduplicatePeers removes duplicate peers from a list.
// It returns a new list with duplicates removed.
func deduplicatePeers(peers []string) []string {
	uniqueMap := make(map[string]bool)

	uniqueArray := []string{}
	for _, str := range peers {
		if _, ok := uniqueMap[str]; !ok {
			uniqueMap[str] = true
			uniqueArray = append(uniqueArray, str)
		}
	}

	return uniqueArray
}
