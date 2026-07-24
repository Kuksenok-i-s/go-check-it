package coverage

import (
	"bytes"
	"sync"
	"testing"
)

func TestSyncWriter_SerializesConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := SyncWriter(&buf)

	const goroutines = 32
	const writesPer = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPer; j++ {
				if _, err := w.Write([]byte("x")); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if got := buf.Len(); got != goroutines*writesPer {
		t.Fatalf("expected %d bytes, got %d", goroutines*writesPer, got)
	}
}

func TestSyncWriter_IdempotentWrap(t *testing.T) {
	var buf bytes.Buffer
	w1 := SyncWriter(&buf)
	w2 := SyncWriter(w1)
	if w1 != w2 {
		t.Fatal("expected SyncWriter to return the same wrapper when already synchronized")
	}
}
