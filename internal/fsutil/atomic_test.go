package fsutil

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileAtomic_ConcurrentWritersLeaveOneWholeFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	payloads := [][]byte{
		[]byte(`{"writer":"a","padding":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		[]byte(`{"writer":"b"}`),
	}

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			if err := WriteFileAtomic(path, payloads[i%2], 0o600); err != nil {
				t.Errorf("WriteFileAtomic: %v", err)
			}
		})
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(payloads[0]) && string(got) != string(payloads[1]) {
		t.Fatalf("file holds interleaved content: %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}
