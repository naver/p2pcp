// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/naver/p2pcp/utils"
	"github.com/zeebo/xxh3"
)

// newIPs creates a list of available TCP addresses for testing
func newIPs(count int) ([]string, error) {
	listener := []net.Listener{}
	ip := []string{}

	for i := 0; i < count; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}

		listener = append(listener, l)
		ip = append(ip, l.Addr().String())
	}

	for _, l := range listener {
		l.Close()
	}

	return ip, nil
}

// clearFlag resets all flags to their default values
func clearFlag() {
	flag.VisitAll(func(f *flag.Flag) {
		if strings.Index(f.Name, "test.") != 0 {
			f.Value.Set(f.DefValue)
		}
	})
}

type testInstance struct {
	ctx    context.Context
	cancel context.CancelFunc
	main   *MainContext
	server *http.Server
	dst    string

	// Flag value snapshots captured at instance creation to prevent data races
	exitComplete    bool
	seedIdleTimeout time.Duration
}

type peerSetupFunc func() string // returns dst path

type testSetup struct {
	seederSetup func()
	peerSetups  []peerSetupFunc
	cleanup     func()
}

func newTestInstance(t *testing.T, dst string) *testInstance {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	var mainCtx MainContext
	if err := mainCtx.Init(ctx); err != nil {
		cancel()
		t.Fatalf("MainContext.Init failed: %s", err.Error())
	}

	return &testInstance{
		ctx:             ctx,
		cancel:          cancel,
		main:            &mainCtx,
		server:          CreateHttpServer(&mainCtx),
		dst:             dst,
		exitComplete:    *flagExitComplete,
		seedIdleTimeout: *flagSeedIdleTimeout,
	}
}

func waitForServer(t *testing.T, addr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func setupSeederWithPeers(t *testing.T, src string, ips []string, commonSetup func(), seederSetup func(seederIP string), peerSetup func(ip, seederIP string)) *testSetup {
	t.Helper()

	setup := &testSetup{
		seederSetup: func() {
			clearFlag()

			if commonSetup != nil {
				commonSetup()
			}

			*flagSrc = src
			*flagListenAddr = ips[0]

			if seederSetup != nil {
				seederSetup(ips[0])
			}
		},
	}

	for i := 1; i < len(ips); i++ {
		idx := i
		setup.peerSetups = append(setup.peerSetups, func() string {
			clearFlag()

			if commonSetup != nil {
				commonSetup()
			}

			dstDir, err := os.MkdirTemp("", "p2pcp.dst.peer.*")
			if err != nil {
				t.Fatalf("MkdirTemp failed: %s", err.Error())
			}

			*flagExitComplete = true
			*flagVerifyOnComplete = true
			*flagDst = dstDir
			*flagListenAddr = ips[idx]

			peerSetup(ips[idx], ips[0])
			return dstDir
		})
	}

	return setup
}

func runAndVerify(t *testing.T, setup *testSetup, srcChecksum uint64) {
	t.Helper()

	if setup.cleanup != nil {
		defer setup.cleanup()
	}

	var seeder *testInstance
	var peers []*testInstance

	// Create and start seeder
	if setup.seederSetup != nil {
		setup.seederSetup()
		seeder = newTestInstance(t, "")
		go func() {
			runTestInstance(t, seeder)
		}()
		defer seeder.cancel()

		// Wait until seeder server is ready
		if !waitForServer(t, seeder.server.Addr, 10*time.Second) {
			t.Fatalf("seeder server not ready: %s", seeder.server.Addr)
		}
	}

	// Create all peers first
	for _, peerSetup := range setup.peerSetups {
		dst := peerSetup()
		peer := newTestInstance(t, dst)
		peers = append(peers, peer)
	}

	// Cleanup peers dst directories
	defer func() {
		for _, p := range peers {
			if p.dst != "" {
				os.RemoveAll(p.dst)
			}
		}
	}()

	// Run peers in parallel
	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(p *testInstance) {
			defer wg.Done()
			if runTestInstance(t, p) != 0 {
				t.Errorf("Run failed.")
			}
		}(p)
	}
	wg.Wait()

	// Verify
	for _, p := range peers {
		if err := diffWith(p.dst, srcChecksum); err != nil {
			t.Errorf("DiffWith failed.: %s", err.Error())
		}
	}
}

func runTestInstance(t *testing.T, inst *testInstance) int {
	if inst.main == nil {
		t.Error("MainContext is nil")
		return 1
	}

	ctx := inst.ctx
	ctxCancel := inst.cancel
	mainContext := inst.main
	server := inst.server

	defer mainContext.Fin()

	exitComplete := inst.exitComplete
	seedIdleTimeout := inst.seedIdleTimeout

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Errorf("Listen failed.: %s", err.Error())
		mainContext.ExitCode.Store(1)
		return int(mainContext.ExitCode.Load())
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
			t.Errorf("Serve failed.: %s", err.Error())
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

		err := DownloadFiles(mainContext)
		if err != nil {
			t.Errorf("DownloadFiles failed.: %s", err.Error())
			ctxCancel()
			mainContext.ExitCode.Store(1)
			return
		}

		if exitComplete {
			ctxCancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		signalChan := make(chan os.Signal, 1)
		defer close(signalChan)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		mainContext.LastSentAt.Store(time.Now())

	ForLoop:
		for {
			select {
			case <-ticker.C:
				if seedIdleTimeout > 0 && time.Since(mainContext.LastSentAt.Load()) >= seedIdleTimeout {
					utils.ErrorPrintf("server idle timeout %v exceeded.", seedIdleTimeout)
					ctxCancel()
					break ForLoop
				}

			case sig := <-signalChan:
				t.Errorf("received signal %v", sig)
				ctxCancel()
				break ForLoop

			case <-ctx.Done():
				break ForLoop
			}
		}

		err := CloseServer(server)
		if err != nil {
			t.Errorf("CloseServer failed.: %s", err.Error())
		}
	}()

	wg.Wait()

	return int(mainContext.ExitCode.Load())
}

// --- test data ---

type randbytes struct {
	rand.Source
}

func (r *randbytes) Read(p []byte) (n int, err error) {
	todo := len(p)
	offset := 0
	for {
		val := int64(r.Int63())
		for i := 0; i < 8; i++ {
			p[offset] = byte(val)
			todo--
			if todo == 0 {
				return len(p), nil
			}
			offset++
			val >>= 8
		}
	}
}

func newRandBytes() io.Reader {
	return &randbytes{rand.NewSource(time.Now().UnixNano())}
}

func generateRandReader(length int) io.Reader {
	return io.LimitReader(newRandBytes(), int64(length))
}

func writeToFile(path string, reader io.Reader, permissions os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, permissions)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	return err
}

func createTestData() (string, string, error) {
	srcDir, err := os.MkdirTemp("", "p2pcp.src.*")
	if err != nil {
		return "", "", err
	}

	dstDir, err := os.MkdirTemp("", "p2pcp.dst.*")
	if err != nil {
		return "", "", err
	}

	if err := writeToFile(filepath.Join(srcDir, "32"), generateRandReader(32), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "32_with_perm"), generateRandReader(32), 0500); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "1024"), generateRandReader(1024), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "1024_with_perm"), generateRandReader(1024), 0611); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "32_1024"), generateRandReader(32*1024), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "32_1024_with_perm"), generateRandReader(32*1024), 0722); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "32_1024_1024"), generateRandReader(32*1024*1024), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(srcDir, "256_1024_1024"), generateRandReader(256*1024*1024), os.ModePerm); err != nil {
		return "", "", err
	}

	sub1Dir := filepath.Join(srcDir, "sub_with_perm")
	if err := os.Mkdir(sub1Dir, os.ModePerm); err != nil && !os.IsExist(err) {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(sub1Dir, "32_1024"), generateRandReader(32*1024), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := os.Symlink(filepath.Join("..", "32_1024"), filepath.Join(sub1Dir, "symlink")); err != nil {
		return "", "", err
	}
	if err := os.Chmod(sub1Dir, 0750); err != nil {
		return "", "", err
	}

	sub2Dir := filepath.Join(srcDir, "sub2")
	if err := os.Mkdir(sub2Dir, os.ModePerm); err != nil && !os.IsExist(err) {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(sub2Dir, "32_1024"), generateRandReader(32*1024), os.ModePerm); err != nil {
		return "", "", err
	}
	if err := writeToFile(filepath.Join(sub2Dir, "empty"), generateRandReader(0), os.ModePerm); err != nil {
		return "", "", err
	}

	return srcDir, dstDir, nil
}

// --- checksum ---

func computeChecksum(root string) (uint64, error) {
	var metadata []string
	var fileHashes []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		permStr := info.Mode().String()
		var line string
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			line = fmt.Sprintf("%s %s -> %s", permStr, rel, target)
		} else if info.Mode().IsRegular() {
			line = fmt.Sprintf("%s %d %s", permStr, info.Size(), rel)
		} else {
			// Directory: size varies by filesystem
			line = fmt.Sprintf("%s %s", permStr, rel)
		}
		metadata = append(metadata, line)
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			h := xxh3.New()
			if _, err := io.Copy(h, f); err != nil {
				return err
			}
			fileHashes = append(fileHashes, fmt.Sprintf("%x %s", h.Sum64(), rel))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	sort.Strings(metadata)
	sort.Strings(fileHashes)

	h := xxh3.New()
	for _, m := range metadata {
		h.WriteString(m)
	}
	for _, fh := range fileHashes {
		h.WriteString(fh)
	}

	return h.Sum64(), nil
}

func diffWith(path string, expectedChecksum uint64) error {
	actualChecksum, err := computeChecksum(path)
	if err != nil {
		return err
	}

	if expectedChecksum != actualChecksum {
		return errors.New("checksum error")
	}

	return nil
}

// --- mock servers ---

func createDynamicPeerListServer(allPeers []string) (*http.Server, string) {
	var requestCount int
	var mu sync.Mutex

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		count := requestCount
		mu.Unlock()

		var peers []string
		if count <= 5 {
			peers = allPeers[:3]
		} else if count <= 15 {
			peers = allPeers
		} else {
			peers = allPeers[:3]
		}

		response := strings.Join(peers, ",")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	go server.Serve(listener)

	return server, fmt.Sprintf("http://%s/", listener.Addr().String())
}

func createPeerRegistryServer() (*http.Server, string) {
	var peers []string
	var mu sync.RWMutex

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodPost:
			var heartbeatReq struct {
				UUID             string `json:"uuid"`
				Address          string `json:"address"`
				ExpiresInSeconds int    `json:"expires_in_seconds"`
			}

			if err := json.NewDecoder(r.Body).Decode(&heartbeatReq); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("invalid JSON body"))
				return
			}

			if heartbeatReq.Address == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("address field required"))
				return
			}

			mu.Lock()
			found := false
			for _, p := range peers {
				if p == heartbeatReq.Address {
					found = true
					break
				}
			}
			if !found {
				peers = append(peers, heartbeatReq.Address)
			}
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))

		case http.MethodGet:
			mu.RLock()
			peerList := make([]string, len(peers))
			copy(peerList, peers)
			mu.RUnlock()

			response := strings.Join(peerList, ",")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	go server.Serve(listener)

	return server, fmt.Sprintf("http://%s/", listener.Addr().String())
}
