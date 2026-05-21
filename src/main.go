// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/naver/p2pcp/constants"
	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/peers"
	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/transfer"
	"github.com/naver/p2pcp/utils"
	"github.com/naver/p2pcp/version"
)

var (
	// Group: Version/Logging/Statistics
	flagVersion    = flag.Bool("version", false, "Show version information.")
	flagVerbose    = flag.Bool("verbose", false, "Enable verbose logging.")
	flagStatistics = flag.Bool("statistics", false, "Print transfer statistics on completion.")
	flagShowHelp   = flag.Bool("help", false, "Show usage.")

	// Group: Wait/Exit
	flagWaitHost        = flag.String("wait-host", "", "Wait for the specified host to complete the transfer. Set the timeout with -wait-host-timeout.")
	flagWaitHostTimeout = flag.Duration("wait-host-timeout", 10*time.Minute, "Timeout for -wait-host.")
	flagExitComplete    = flag.Bool("exit-complete", false, "Exit immediately after the transfer completes.")

	// Group: Concurrency/Compression/Chunk/Verification/Command
	flagPeerNumConcurrent         = flag.Int("peer-num-concurrent", 2, fmt.Sprintf("Number of concurrent transfer jobs per peer (1-%d).", constants.MaxPeerNumConcurrent))
	flagLocalNumConcurrent        = flag.Int("local-num-concurrent", 1, fmt.Sprintf("Number of concurrent local transfer jobs (1-%d).", constants.MaxLocalNumConcurrent))
	flagCompressType              = flag.String("compress-type", p2pcpHttp.EncodingTypeNone, "Compression algorithm for network transfers. Options: none, zstd.")
	flagMaxFileCountPerChunk      = flag.Int("files-per-chunk", 5, "Maximum count of files per chunk.")
	flagChunkSize                 = flag.Int64("chunk-size", 16777216, "Size of each transfer chunk in bytes.")
	flagVerifyOnComplete          = flag.Bool("verify-on-complete", false, "Verify file integrity by comparing hash values after all files are transferred.")
	flagExecuteOnComplete         = flag.String("execute-on-complete", "", "Run command after all files complete.")
	flagExecuteOnEachFileComplete = flag.String("execute-on-each-file-complete", "", "Run command after each file completes.")
	flagCommandTimeout            = flag.Duration("command-timeout", 10*time.Second, "Timeout for command execution. Commands that exceed this timeout are terminated with SIGKILL.")

	// Group: Networking
	flagListenAddr = flag.String("listen-addr", fmt.Sprintf("0.0.0.0:%d", constants.DefaultPort), "TCP listen address.")

	// Group: Discovery
	flagPeerList             = flag.String("peer-list", "", "Static peers (host:port,host:port); merges with others.")
	flagPeerListURL          = flag.String("peer-list-url", "", "HTTP URL returning comma-separated peers; exclusive with SRV/A.")
	flagPeerListSRV          = flag.String("peer-list-srv", "", "SRV FQDN for discovery; exclusive with URL/A.")
	flagPeerListA            = flag.String("peer-list-a", "", "A-record FQDN for discovery; exclusive with URL/SRV.")
	flagPeerListDNSServer    = flag.String("peer-list-dns-server", "", "Custom DNS server for A/SRV record lookups.")
	flagPeerListPollInterval = flag.Duration("peer-list-poll-interval", 3*time.Second, "Polling interval for dynamic peer providers.")
	flagPeerWaitTimeout      = flag.Duration("peer-wait-timeout", 20*time.Second, "Timeout for waiting until at least one peer becomes available.")

	// Group: External Registry
	flagPeerRegistryHeartbeatURL      = flag.String("peer-registry-heartbeat-url", "", "Registry endpoint to publish this peer; discover via -peer-list-url.")
	flagPeerRegistrySelfEndpoint      = flag.String("peer-registry-self-endpoint", "", "This peer's connect address (host:port) to publish.")
	flagPeerRegistryHeartbeatInterval = flag.Duration("peer-registry-heartbeat-interval", 5*time.Second, "Interval between registry heartbeats.")
	flagPeerRegistryTimeout           = flag.Duration("peer-registry-timeout", 3*time.Second, "Timeout for registry requests.")

	// Group: Paths
	flagSrc = flag.String("src", "", "Source path (local:/path or /path).")
	flagDst = flag.String("dst", "", "Destination path (local path). Omit for seed-only mode.")

	// Group: Timeouts/Synchronization
	flagTransferIdleTimeout       = flag.Duration("transfer-idle-timeout", 30*time.Second, "Timeout for idle transfers before treating them as failed. Set to 0 to wait indefinitely.")
	flagSeedIdleTimeout           = flag.Duration("seed-idle-timeout", 0*time.Minute, "Timeout before closing the HTTP server when no requests are received. Set to 0 to disable.")
	flagAvailableListSyncInterval = flag.Duration("sync-interval", 2000*time.Millisecond, "Interval to synchronize available file chunks between peers.")
)

// Custom grouped usage matching README groups and order
func printGroupedUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])

	type group struct {
		title string
		flags []string
	}

	groups := []group{
		{title: "Version/Logging/Statistics", flags: []string{"version", "verbose", "statistics", "help"}},
		{title: "Wait/Exit", flags: []string{"wait-host", "wait-host-timeout", "exit-complete"}},
		{title: "Concurrency/Compression/Chunk/Verification/Command", flags: []string{"peer-num-concurrent", "local-num-concurrent", "compress-type", "chunk-size", "files-per-chunk", "verify-on-complete", "execute-on-complete", "execute-on-each-file-complete", "command-timeout"}},
		{title: "Networking", flags: []string{"listen-addr"}},
		{title: "Discovery", flags: []string{"peer-list", "peer-list-url", "peer-list-srv", "peer-list-a", "peer-list-dns-server", "peer-list-poll-interval", "peer-wait-timeout"}},
		{title: "External Registry", flags: []string{"peer-registry-heartbeat-url", "peer-registry-self-endpoint", "peer-registry-heartbeat-interval", "peer-registry-timeout"}},
		{title: "Paths", flags: []string{"src", "dst"}},
		{title: "Timeouts/Synchronization", flags: []string{"transfer-idle-timeout", "seed-idle-timeout", "sync-interval"}},
	}

	for idx, g := range groups {
		fmt.Fprintf(os.Stderr, "%s\n", g.title)
		for _, name := range g.flags {
			if f := flag.Lookup(name); f != nil {
				// Print in a style similar to flag.PrintDefaults
				if f.DefValue != "" {
					fmt.Fprintf(os.Stderr, "  -%s\t%s (default %s)\n", f.Name, f.Usage, f.DefValue)
				} else {
					fmt.Fprintf(os.Stderr, "  -%s\t%s\n", f.Name, f.Usage)
				}
			}
		}
		if idx != len(groups)-1 {
			fmt.Fprintln(os.Stderr)
		}
	}
}

func init() {
	flag.Usage = printGroupedUsage
}

type MainContext struct {
	// === Application Control ===
	ExitCode atomic.Int32
	UUID     string

	// === Network & Peers ===
	PeerListProvider peers.Provider
	PeerRegistry     *peers.Registry

	// === Transfer Management ===
	ChunkManager   *transfer.ChunkManager
	DownloadCtx    context.Context
	DownloadCancel context.CancelFunc

	// === State Management ===
	Completed     atomic.Bool
	ServeOnly     bool
	LastSentAt    utils.AtomicTime
	LastUpdatedAt utils.AtomicTime

	// === Storage ===
	Src storage.Storage
	Dst storage.Storage

	// === Utilities ===
	Command    *utils.Command
	Statistics *Statistics
}

func (mainContext *MainContext) Init(ctx context.Context) error {
	mainContext.UUID = uuid.New().String()
	mainContext.Statistics = NewStatistics(*flagStatistics)
	mainContext.DownloadCtx, mainContext.DownloadCancel = context.WithCancel(ctx)

	if !p2pcpHttp.IsAvailableEncoding(flagCompressType) {
		return fmt.Errorf("unsupported compress type (%s)", *flagCompressType)
	}

	if (*flagChunkSize) <= 0 || (*flagChunkSize) > constants.MaxChunkSize {
		return fmt.Errorf("invalid chunk size parameter (%d)", (*flagChunkSize))
	}

	srcFactory, err := storage.NewFactory(*flagSrc)
	if err != nil {
		return fmt.Errorf("failed to create source factory: %s", err.Error())
	}
	mainContext.Src, err = srcFactory.Create()
	if err != nil {
		return fmt.Errorf("failed to create source storage: %s", err.Error())
	}

	dstFactory, err := storage.NewFactory(*flagDst)
	if err != nil {
		return fmt.Errorf("failed to create destination factory: %s", err.Error())
	}
	mainContext.Dst, err = dstFactory.Create()
	if err != nil {
		return fmt.Errorf("failed to create destination storage: %s", err.Error())
	}

	if mainContext.Src != nil && mainContext.Dst == nil {
		// Using only the Src for distribution is possible,
		// but if the Src is located on a network drive, performance may degrade due to heavy access.
		// Distributing files copied to the Dst ensures better performance.
		mainContext.Completed.Store(true)
		mainContext.ServeOnly = true
	}

	peerListFactory, err := peers.NewFactory(*flagPeerList, constants.DefaultPort, *flagPeerWaitTimeout, constants.DNSLookupTimeout)
	if err != nil {
		utils.ErrorPrintf("Failed to create peer list factory: %s", err.Error())
		return err
	}
	mainContext.PeerListProvider, err = peerListFactory.Create(
		peers.WithARecord(*flagPeerListA),
		peers.WithSRVRecord(*flagPeerListSRV),
		peers.WithURL(*flagPeerListURL),
		peers.WithDNSServer(*flagPeerListDNSServer),
	)
	if err != nil {
		utils.ErrorPrintf("Failed to create peer list provider: %s", err.Error())
		return err
	}

	mainContext.ChunkManager, err = transfer.NewChunkManager(mainContext.Src, mainContext.PeerListProvider, *flagPeerWaitTimeout, *flagChunkSize, *flagMaxFileCountPerChunk)
	if err != nil {
		return err
	}

	if *flagPeerNumConcurrent < 1 || *flagPeerNumConcurrent > constants.MaxPeerNumConcurrent {
		return fmt.Errorf("invalid concurrent download jobs parameter (%d)", *flagPeerNumConcurrent)
	}

	if *flagLocalNumConcurrent < 1 || *flagLocalNumConcurrent > constants.MaxLocalNumConcurrent {
		return fmt.Errorf("invalid concurrent download jobs parameter (%d)", *flagLocalNumConcurrent)
	}

	mainContext.Command = utils.NewCommand(*flagExecuteOnComplete, *flagExecuteOnEachFileComplete, *flagCommandTimeout)
	mainContext.Command.Run()

	if *flagPeerRegistryHeartbeatURL != "" {
		peerRegistry, err := peers.NewRegistry(*flagPeerRegistryHeartbeatURL, *flagPeerRegistrySelfEndpoint, mainContext.UUID, *flagPeerRegistryHeartbeatInterval, *flagPeerRegistryTimeout)
		if err != nil {
			utils.ErrorPrintf("Failed to create peer registry: %s", err.Error())
			return err
		}
		err = peerRegistry.Start()
		if err != nil {
			utils.ErrorPrintf("Failed to start peer registry: %s", err.Error())
			return err
		}
		mainContext.PeerRegistry = peerRegistry
	}

	utils.DebugPrintf("Initialization done.")
	return nil
}

func (mainContext *MainContext) Fin() {
	if mainContext.PeerRegistry != nil {
		mainContext.PeerRegistry.Stop()
	}
	mainContext.Command.Stop()
	mainContext.Statistics.PrintAll()
}

func main() {
	// Testing with simultaneous transmission to 30 peers showed that memory usage significantly decreases at GCPercent.
	// Reducing GCPercent further decreases memory usage but affects transmission speed.
	debug.SetGCPercent(35)

	var mainContext MainContext
	defer func() {
		os.Exit(int(mainContext.ExitCode.Load()))
	}()

	flag.Parse()
	if *flagShowHelp {
		flag.Usage()
		return
	}

	if *flagVersion {
		fmt.Fprintln(os.Stderr, version.GetVersion())
		return
	}

	utils.GetLogger().SetVerbose(*flagVerbose)
	defer utils.GetLogger().Close()

	if *flagWaitHost != "" {
		err := func() error {
			// Retry until /completed returns 200
			t0 := time.Now()
			ticker := time.NewTicker(constants.PeerWaitDuration)
			defer ticker.Stop()
			for ; time.Since(t0) < *flagWaitHostTimeout; <-ticker.C {
				_, err := p2pcpHttp.Request(fmt.Sprintf("http://%s/completed", *flagWaitHost))
				if err == nil {
					return nil
				}
			}

			return fmt.Errorf("wait timeout %v exceeded", *flagWaitHostTimeout)
		}()
		if err != nil {
			utils.ErrorPrintf("Failed to wait for the host to complete the transfer.: (%s) %s", *flagWaitHost, err.Error())
			mainContext.ExitCode.Store(1)
			return
		}
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	err := mainContext.Init(ctx)
	if err != nil {
		utils.ErrorPrintf("Initialize failed.: %s", err.Error())
		mainContext.ExitCode.Store(1)
		return
	}
	defer mainContext.Fin()

	utils.DebugPrintf("p2pcp server starting.")

	server := CreateHttpServer(&mainContext)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		utils.ErrorPrintf("p2pcp server Listen failed.: %s", err.Error())
		mainContext.ExitCode.Store(1)
		return
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}
		if err != nil {
			utils.ErrorPrintf("p2pcp server has failed.: %s", err.Error())
			ctxCancel()
			mainContext.ExitCode.Store(1)
			return
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if mainContext.ServeOnly {
			return
		}

		err := DownloadFiles(&mainContext)
		if err != nil {
			utils.ErrorPrintf("Download failed.: %s", err.Error())
			ctxCancel()
			mainContext.ExitCode.Store(1)
			return
		}

		if *flagExitComplete {
			ctxCancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		defer func() {
			signal.Stop(signalChan)
			close(signalChan)
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		mainContext.LastSentAt.Store(time.Now())

	ForLoop:
		for {
			select {
			case <-ticker.C:
				if *flagSeedIdleTimeout > 0 && time.Since(mainContext.LastSentAt.Load()) >= *flagSeedIdleTimeout {
					utils.Printf("server idle timeout %v exceeded.", *flagSeedIdleTimeout)
					ctxCancel()
					break ForLoop
				}

			case sig := <-signalChan:
				utils.Printf("received signal %v", sig)
				ctxCancel()
				break ForLoop

			case <-ctx.Done():
				break ForLoop
			}
		}

		err := CloseServer(server)
		if err != nil {
			utils.ErrorPrintf("Shutting down p2pcp server failed.: %s", err.Error())
		}
	}()

	wg.Wait()

	utils.DebugPrintf("p2pcp server stopped.")
}
