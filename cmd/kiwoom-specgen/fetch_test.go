package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchAPIListRetriesTransientResponses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/json" || r.FormValue("apiId") != "" {
					t.Errorf("unexpected API list request: %s %v", r.Method, r.Header)
				}
				if calls == 1 {
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(status)
					_, _ = fmt.Fprint(w, "<html>Maintenance</html>")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"resp_code":"0","resp_data":[{"apiInfo":{"apiId":"ka10001"}}]}`)
			}))
			defer srv.Close()
			got, err := fetchAPIList(srv.Client(), srv.URL, 0)
			if err != nil || len(got.RespData) != 1 || calls != 2 {
				t.Fatalf("calls=%d payload=%+v err=%v", calls, got, err)
			}
		})
	}
}

func TestFetchAPIListStopsOnPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		calls  int
		want   string
	}{
		{"forbidden", http.StatusForbidden, "<html>Forbidden</html>", 1, "HTTP 403"},
		{"application error", http.StatusOK, `{"resp_code":"-1","resp_msg":"unavailable"}`, 1, "code=-1"},
		{"persistent HTML", http.StatusOK, "<html>private-session-marker</html>", 3, `HTTP 200, Content-Type "text/html"`},
		{"truncated JSON", http.StatusOK, `{"resp_code":`, 3, "unexpected end of JSON input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			_, err := fetchAPIList(srv.Client(), srv.URL, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) || calls != tc.calls {
				t.Fatalf("calls=%d err=%v, want calls=%d error containing %q", calls, err, tc.calls, tc.want)
			}
			if strings.Contains(err.Error(), "private-session-marker") {
				t.Fatal("response body leaked into error")
			}
		})
	}
}

func TestRefreshPreservesFilesOnCollectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "specs.go"), filepath.Join(dir, "types.go")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("existing content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := runRefresh([]string{"--list-url", srv.URL, "--snapshot", paths[0], "--spec-out", paths[1], "--types-out", paths[2]}); err == nil {
		t.Fatal("refresh succeeded despite failed collection")
	}
	for _, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "existing content\n" {
			t.Fatalf("refresh changed %s: content=%q err=%v", path, got, err)
		}
	}
}
