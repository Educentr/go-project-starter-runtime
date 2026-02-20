package rest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// mockServerConfigProvider implements ServerConfigProvider for testing
type mockServerConfigProvider struct {
	mu            sync.Mutex
	readTimeout   time.Duration
	writeTimeout  time.Duration
	headerTimeout time.Duration
	ip            string
	port          string
	callback      func()
}

func newMockConfigProvider(ip, port string, read, write, header time.Duration) *mockServerConfigProvider {
	return &mockServerConfigProvider{
		ip:            ip,
		port:          port,
		readTimeout:   read,
		writeTimeout:  write,
		headerTimeout: header,
	}
}

func (m *mockServerConfigProvider) GetServerConfig(_ context.Context, _, _ string) (ServerConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return ServerConfig{
		IP:                m.ip,
		Port:              m.port,
		ReadTimeout:       m.readTimeout,
		WriteTimeout:      m.writeTimeout,
		ReadHeaderTimeout: m.headerTimeout,
	}, nil
}

func (m *mockServerConfigProvider) SubscribeTimeoutChanges(_ context.Context, _, _ string, callback func()) error {
	m.callback = callback
	return nil
}

func (m *mockServerConfigProvider) setTimeouts(read, write, header time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.readTimeout = read
	m.writeTimeout = write
	m.headerTimeout = header
}

// TestServer_UpdateTimeouts_Race verifies that concurrent calls to updateTimeouts()
// don't cause data races when the HTTP server is actively serving requests.
func TestServer_UpdateTimeouts_Race(t *testing.T) {
	cp := newMockConfigProvider("127.0.0.1", "0", 30*time.Second, 60*time.Second, 10*time.Second)

	s := &Server{
		name:           "test",
		serviceName:    "test-svc",
		appName:        "test-app",
		configProvider: cp,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_ = s.GetWriteTimeout()
		_ = s.GetReadTimeout()
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s.httpSrv = &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.readTimeout.Store(int64(30 * time.Second))
	s.writeTimeout.Store(int64(60 * time.Second))
	s.idleTimeout.Store(int64(30 * time.Second))
	s.readHeaderTimeout.Store(int64(10 * time.Second))

	// Don't set errGr — updateTimeouts() won't trigger restartServer
	// (we only test atomic field updates here, not the full restart path)
	go func() { _ = s.httpSrv.Serve(ln) }()
	defer s.httpSrv.Close()

	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	const iterations = 100
	var wg sync.WaitGroup

	// Concurrent updateTimeouts: if it writes to http.Server fields,
	// the race detector flags it vs net/http's unsynchronized reads.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			s.updateTimeouts(ctx)
		}
	}()

	// Concurrent timeout reads via atomic getters (simulates middleware).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = s.GetWriteTimeout()
			_ = s.GetReadTimeout()
		}
	}()

	// Concurrent HTTP requests (triggers net/http to read server fields).
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := &http.Client{Timeout: 2 * time.Second}
		for i := 0; i < iterations; i++ {
			resp, err := client.Get("http://" + ln.Addr().String() + "/")
			if err == nil {
				resp.Body.Close()
			}
		}
	}()

	wg.Wait()
}

// TestServer_WriteTimeout_UpdateEnforced verifies that when timeouts change,
// the server is restarted and the new timeout is enforced on subsequent requests.
func TestServer_WriteTimeout_UpdateEnforced(t *testing.T) {
	cp := newMockConfigProvider("127.0.0.1", "0", 5*time.Second, 2*time.Second, 5*time.Second)

	s := &Server{
		name:           "test",
		serviceName:    "test-svc",
		appName:        "test-app",
		configProvider: cp,
	}

	ctx := context.Background()

	// Read initial timeouts (errGr not set yet, no restart triggered)
	s.updateTimeouts(ctx)

	if got := s.GetWriteTimeout(); got != 2*time.Second {
		t.Fatalf("initial GetWriteTimeout() = %v, want 2s", got)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.Write([]byte("ok"))
	})

	s.httpSrv = &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           mux,
		WriteTimeout:      time.Duration(s.writeTimeout.Load()),      // 2s
		ReadTimeout:       time.Duration(s.readTimeout.Load()),       // 5s
		IdleTimeout:       time.Duration(s.idleTimeout.Load()),       // 5s
		ReadHeaderTimeout: time.Duration(s.readHeaderTimeout.Load()), // 5s
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.httpSrv.Addr = ln.Addr().String()

	errGr, _ := errgroup.WithContext(ctx)
	s.errGr = errGr

	go func() { _ = s.httpSrv.Serve(ln) }()
	defer func() {
		s.shuttingDown.Store(true)
		s.httpSrv.Close()
	}()

	time.Sleep(50 * time.Millisecond)

	addr := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}

	// BEFORE update: handler sleeps 1s, WriteTimeout=2s → should succeed
	resp, err := client.Get(addr + "/slow")
	if err != nil {
		t.Fatalf("before update: expected success, got error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("before update: expected body 'ok', got %q", body)
	}

	// Update config: server/timeout/write → 200ms
	cp.setTimeouts(5*time.Second, 200*time.Millisecond, 5*time.Second)

	// Simulate config change callback (triggers restartServer in background)
	s.updateTimeouts(ctx)

	if got := s.GetWriteTimeout(); got != 200*time.Millisecond {
		t.Fatalf("after update: GetWriteTimeout() = %v, want 200ms", got)
	}

	// Wait for new server to come up using a fast /health endpoint
	var newAddr string
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		s.mu.Lock()
		newAddr = "http://" + s.httpSrv.Addr
		s.mu.Unlock()

		resp, err := client.Get(newAddr + "/health")
		if err == nil {
			resp.Body.Close()
			break
		}

		// Server not ready yet or connection refused — retry
		if i == 49 {
			t.Fatalf("new server did not come up after restart: %v", err)
		}
	}

	// AFTER update: handler sleeps 1s, new WriteTimeout=200ms → should fail
	resp, err = client.Get(newAddr + "/slow")
	if err != nil {
		// Connection error — timeout was enforced by net/http
		t.Logf("after update: request correctly failed: %v", err)
		return
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	// If we got here, net/http didn't enforce the new timeout
	t.Fatalf("after update: handler should have been cut off by 200ms WriteTimeout, "+
		"but request succeeded with body %q (http.Server.WriteTimeout not updated)", body)
}

// TestServer_RestartGracefulConnections verifies that in-flight requests
// complete successfully when the server is restarted due to timeout changes.
func TestServer_RestartGracefulConnections(t *testing.T) {
	cp := newMockConfigProvider("127.0.0.1", "0", 10*time.Second, 10*time.Second, 10*time.Second)

	s := &Server{
		name:           "test",
		serviceName:    "test-svc",
		appName:        "test-app",
		configProvider: cp,
	}

	ctx := context.Background()
	s.updateTimeouts(ctx)

	// handlerStarted is closed when the slow handler begins processing
	handlerStarted := make(chan struct{})
	// handlerResult receives the error (if any) from writing the response
	handlerResult := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/in-flight", func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		// Simulate a slow request that takes 2 seconds
		time.Sleep(2 * time.Second)
		_, writeErr := fmt.Fprintln(w, "completed")
		handlerResult <- writeErr
	})

	s.httpSrv = &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           mux,
		WriteTimeout:      time.Duration(s.writeTimeout.Load()),
		ReadTimeout:       time.Duration(s.readTimeout.Load()),
		IdleTimeout:       time.Duration(s.idleTimeout.Load()),
		ReadHeaderTimeout: time.Duration(s.readHeaderTimeout.Load()),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.httpSrv.Addr = ln.Addr().String()

	errGr, _ := errgroup.WithContext(ctx)
	s.errGr = errGr

	go func() { _ = s.httpSrv.Serve(ln) }()
	defer func() {
		s.shuttingDown.Store(true)
		s.httpSrv.Close()
	}()

	time.Sleep(50 * time.Millisecond)

	addr := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 10 * time.Second}

	// Start a slow request in background
	type httpResult struct {
		body string
		err  error
	}
	resultCh := make(chan httpResult, 1)
	go func() {
		resp, err := client.Get(addr + "/in-flight")
		if err != nil {
			resultCh <- httpResult{err: err}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resultCh <- httpResult{body: string(body)}
	}()

	// Wait until the handler has started processing
	<-handlerStarted

	// Now trigger a timeout change → restartServer()
	// The old server should wait for the in-flight request to complete
	cp.setTimeouts(10*time.Second, 5*time.Second, 10*time.Second)
	s.updateTimeouts(ctx)

	// Wait for the in-flight request to finish
	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("in-flight request failed during restart: %v", res.err)
		}
		if res.body != "completed\n" {
			t.Fatalf("in-flight request returned unexpected body: %q", res.body)
		}
		t.Logf("in-flight request completed successfully during server restart")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for in-flight request to complete")
	}

	// Verify the handler didn't get a write error
	select {
	case writeErr := <-handlerResult:
		if writeErr != nil {
			t.Fatalf("handler got write error during restart: %v", writeErr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for handler result")
	}
}
