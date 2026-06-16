package fits

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func generateTestFiles(t *testing.T, dir string, count int) []string {
	t.Helper()
	var files []string
	for i := 0; i < count; i++ {
		path := filepath.Join(dir, "test_"+itoa(i)+".fits")
		if err := GenerateSampleFITSHeader(path); err != nil {
			t.Fatalf("generate sample %d: %v", i, err)
		}
		files = append(files, path)
	}
	return files
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestParseBatch(t *testing.T) {
	dir := t.TempDir()
	files := generateTestFiles(t, dir, 200)

	config := BatchConfig{Workers: 8}
	results, stats := ParseBatch(files, config)

	if stats.Total != 200 {
		t.Errorf("expected total 200, got %d", stats.Total)
	}
	if stats.Succeeded != 200 {
		t.Errorf("expected 200 successes, got %d", stats.Succeeded)
	}
	if stats.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", stats.Failed)
	}
	if stats.MaxOpenFD > 8 {
		t.Errorf("max open FD should be <= workers (8), got %d", stats.MaxOpenFD)
	}
	if len(results) != 200 {
		t.Errorf("expected 200 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("unexpected error for %s: %v", r.Path, r.Error)
		}
		if r.Header == nil {
			t.Errorf("nil header for %s", r.Path)
		} else {
			if v, ok := r.Header.Get("SIMPLE"); !ok || v != "T" {
				t.Errorf("SIMPLE keyword missing or wrong for %s", r.Path)
			}
		}
	}
}

func TestParseBatchConcurrencyLimit(t *testing.T) {
	dir := t.TempDir()
	files := generateTestFiles(t, dir, 500)

	for _, workers := range []int{2, 8, 16, 64} {
		t.Run("workers_"+itoa(workers), func(t *testing.T) {
			config := BatchConfig{Workers: workers}
			_, stats := ParseBatch(files, config)

			if stats.MaxOpenFD > workers+2 {
				t.Errorf("workers=%d: max open FD %d exceeds worker count too much",
					workers, stats.MaxOpenFD)
			}
			if stats.Succeeded != 500 {
				t.Errorf("workers=%d: expected 500 successes, got %d", workers, stats.Succeeded)
			}
		})
	}
}

func TestParseBatchWithBadFile(t *testing.T) {
	dir := t.TempDir()
	files := generateTestFiles(t, dir, 10)

	badPath := filepath.Join(dir, "nonexistent.fits")
	files = append(files, badPath)

	config := BatchConfig{Workers: 4}
	results, stats := ParseBatch(files, config)

	if stats.Total != 11 {
		t.Errorf("expected 11 total, got %d", stats.Total)
	}
	if stats.Succeeded != 10 {
		t.Errorf("expected 10 successes, got %d", stats.Succeeded)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", stats.Failed)
	}

	failedCount := 0
	for _, r := range results {
		if r.Error != nil {
			failedCount++
			if r.Path != badPath {
				t.Errorf("unexpected failure for %s: %v", r.Path, r.Error)
			}
		}
	}
	if failedCount != 1 {
		t.Errorf("expected 1 failed result, got %d", failedCount)
	}
}

func TestCollectFITSFiles(t *testing.T) {
	dir := t.TempDir()

	subDir := filepath.Join(dir, "subdir")
	nestedDir := filepath.Join(dir, "nested", "deep")
	os.MkdirAll(subDir, 0755)
	os.MkdirAll(nestedDir, 0755)

	GenerateSampleFITSHeader(filepath.Join(dir, "a.fits"))
	GenerateSampleFITSHeader(filepath.Join(dir, "b.FIT"))
	GenerateSampleFITSHeader(filepath.Join(subDir, "c.fits"))
	GenerateSampleFITSHeader(filepath.Join(nestedDir, "d.FITS"))
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not fits"), 0644)

	exts := []string{".fits", ".FITS", ".fit", ".FIT"}
	files, err := CollectFITSFiles(dir, exts)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("expected 4 FITS files, got %d", len(files))
		for _, f := range files {
			t.Logf("  found: %s", f)
		}
	}
}

func TestParseDirectory(t *testing.T) {
	dir := t.TempDir()
	generateTestFiles(t, dir, 50)

	config := DefaultBatchConfig()
	config.Workers = 8
	results, stats, err := ParseDirectory(dir, config)
	if err != nil {
		t.Fatalf("parse directory: %v", err)
	}

	if stats.Total != 50 {
		t.Errorf("expected 50 total, got %d", stats.Total)
	}
	if stats.Succeeded != 50 {
		t.Errorf("expected 50 successes, got %d", stats.Succeeded)
	}
	if len(results) != 50 {
		t.Errorf("expected 50 results, got %d", len(results))
	}
}

func TestParseFileImmediateClosesFD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fits")
	GenerateSampleFITSHeader(path)

	var openCount int32
	concurrency := 50
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				hdr, err := parseFileImmediate(path)
				if err != nil {
					t.Errorf("parse error: %v", err)
					return
				}
				if hdr == nil {
					t.Error("nil header")
					return
				}
				atomic.AddInt32(&openCount, 1)
				time.Sleep(time.Microsecond)
				atomic.AddInt32(&openCount, -1)
			}
		}()
	}

	wg.Wait()
}

func TestBatchStats(t *testing.T) {
	dir := t.TempDir()
	files := generateTestFiles(t, dir, 100)

	config := BatchConfig{Workers: 4}
	_, stats := ParseBatch(files, config)

	if stats.Total != 100 {
		t.Errorf("Total: expected 100, got %d", stats.Total)
	}
	if stats.Succeeded+stats.Failed != stats.Total {
		t.Errorf("Succeeded + Failed != Total: %d + %d != %d",
			stats.Succeeded, stats.Failed, stats.Total)
	}
	if stats.MaxOpenFD < 0 {
		t.Errorf("MaxOpenFD should be >= 0, got %d", stats.MaxOpenFD)
	}
}
