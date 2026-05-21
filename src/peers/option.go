// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

type Option func(*Config)

type Config struct {
	aRecordFQDN   string
	srvRecordFQDN string
	url           string
	dnsServer     string
}

func WithARecord(fqdn string) Option {
	return func(c *Config) {
		c.aRecordFQDN = fqdn
	}
}

func WithSRVRecord(fqdn string) Option {
	return func(c *Config) {
		c.srvRecordFQDN = fqdn
	}
}

func WithURL(url string) Option {
	return func(c *Config) {
		c.url = url
	}
}

func WithDNSServer(dnsServer string) Option {
	return func(c *Config) {
		c.dnsServer = dnsServer
	}
}
