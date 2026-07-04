package server

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

func TestRun_GracefulShutdownOnSIGTERM(t *testing.T) {
	// Grab a free port so Run can bind it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	s := NewWithBrokers("127.0.0.1", port, nil, map[string]broker.Broker{})

	runErr := make(chan error, 1)
	go func() {
		runErr <- s.Run()
	}()

	// Wait for the server to accept requests.
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never became ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Run's signal handler catches this instead of killing the test process.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned error on graceful shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after SIGTERM")
	}
}
