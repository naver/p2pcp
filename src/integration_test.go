// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/naver/p2pcp/testutil"
)

func TestMainIntegration(t *testing.T) {
	src, dst, err := createTestData()
	if err != nil {
		t.Fatalf("createTestData failed.: %s", err.Error())
	}
	defer os.RemoveAll(src)
	defer os.RemoveAll(dst)
	srcChecksum, err := computeChecksum(src)
	if err != nil {
		t.Fatalf("computeChecksum failed.: %s", err.Error())
	}

	testLock := sync.Mutex{}

	tests := []struct {
		name      string
		setupFunc func(t *testing.T) *testSetup
	}{
		{
			name: "local copy",
			setupFunc: func(t *testing.T) *testSetup {
				return &testSetup{
					peerSetups: []peerSetupFunc{
						func() string {
							clearFlag()
							*flagExitComplete = true
							*flagSrc = src
							*flagDst = dst
							return dst
						},
					},
				}
			},
		},
		{
			name: "local copy with verify",
			setupFunc: func(t *testing.T) *testSetup {
				return &testSetup{
					peerSetups: []peerSetupFunc{
						func() string {
							clearFlag()
							*flagExitComplete = true
							*flagVerifyOnComplete = true
							*flagSrc = src
							*flagDst = dst
							return dst
						},
					},
				}
			},
		},
		{
			name: "seeder 1, peer 1",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(2)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				return setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 8 * 1024 * 1024
						*flagCompressType = "zstd"
					},
					nil,
					func(ip, seederIP string) {
						*flagPeerList = seederIP
					})
			},
		},
		{
			name: "seeder 1, peer 12",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(13)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				return setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 1 * 1024 * 1024
						*flagMaxFileCountPerChunk = 3
						*flagCompressType = "zstd"
						*flagStatistics = true
					},
					nil,
					func(ip, seederIP string) {
						*flagPeerList = strings.Join(ips, ",")
					})
			},
		},
		{
			name: "seeder 1, peer 12(with local src)",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(13)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				return setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 1 * 1024 * 1024
						*flagMaxFileCountPerChunk = 3
						*flagCompressType = "zstd"
					},
					nil,
					func(ip, seederIP string) {
						*flagSrc = src
						*flagPeerList = strings.Join(ips[1:], ",")
					})
			},
		},
		{
			name: "seeder 1, peer 12(peer list url)",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(13)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				peerListServer, peerListURL := createDynamicPeerListServer(ips)

				setup := setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 1 * 1024 * 1024
						*flagMaxFileCountPerChunk = 3
						*flagCompressType = "zstd"
					},
					nil,
					func(ip, seederIP string) {
						*flagPeerListURL = peerListURL
						*flagPeerListPollInterval = 1 * time.Second
					})
				setup.cleanup = func() {
					peerListServer.Close()
				}
				return setup
			},
		},
		{
			name: "seeder 1, peer 12(peer list srv)",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(13)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				dnsServer := testutil.NewDNSMockServer("127.0.0.1:5353")

				var srvRecords []testutil.SRVRecord
				for _, peer := range ips[1:] {
					host, portStr, err := net.SplitHostPort(peer)
					if err != nil {
						continue
					}
					port, err := strconv.Atoi(portStr)
					if err != nil {
						continue
					}
					srvRecords = append(srvRecords, testutil.SRVRecord{
						Priority: 10,
						Weight:   10,
						Port:     uint16(port),
						Target:   host,
					})
				}
				dnsServer.AddDynamicSRVRecord("_p2pcp._tcp.test.p2pcp.local", srvRecords)
				dnsServer.AddRecord("127.0.0.1.local", []string{"127.0.0.1"})

				go func() {
					if err := dnsServer.Start(); err != nil {
						t.Logf("DNS server error: %v", err)
					}
				}()
				time.Sleep(50 * time.Millisecond)

				setup := setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 1 * 1024 * 1024
						*flagMaxFileCountPerChunk = 3
					},
					nil,
					func(ip, seederIP string) {
						*flagPeerList = seederIP
						*flagPeerListSRV = "_p2pcp._tcp.test.p2pcp.local"
						*flagPeerListDNSServer = "127.0.0.1:5353"
						*flagPeerListPollInterval = 1 * time.Second
					})
				setup.cleanup = func() {
					dnsServer.Stop()
				}
				return setup
			},
		},
		{
			name: "with peer registry",
			setupFunc: func(t *testing.T) *testSetup {
				ips, err := newIPs(13)
				if err != nil {
					t.Fatalf("NewIPs failed: %s", err.Error())
				}

				registryServer, registryURL := createPeerRegistryServer()

				setup := setupSeederWithPeers(t, src, ips,
					func() {
						*flagChunkSize = 1 * 1024 * 1024
						*flagMaxFileCountPerChunk = 3
						*flagCompressType = "zstd"
						*flagPeerRegistryHeartbeatURL = registryURL
						*flagPeerRegistryHeartbeatInterval = 1 * time.Second
					},
					func(seederIP string) {
						*flagPeerRegistrySelfEndpoint = seederIP
					},
					func(ip, seederIP string) {
						*flagPeerRegistrySelfEndpoint = ip
						*flagPeerListURL = registryURL
						*flagPeerListPollInterval = 1 * time.Second
					})
				setup.cleanup = func() {
					registryServer.Close()
				}
				return setup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLock.Lock()
			defer testLock.Unlock()

			setup := tt.setupFunc(t)
			runAndVerify(t, setup, srcChecksum)
		})
	}
}
