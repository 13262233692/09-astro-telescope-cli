package fits

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type BatchResult struct {
	Path     string
	Header   *Header
	Error    error
}

type BatchStats struct {
	Total      int
	Succeeded  int
	Failed     int
	MaxOpenFD  int
}

type BatchConfig struct {
	Workers    int
	Extensions []string
}

func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		Workers:    32,
		Extensions: []string{".fits", ".FITS", ".fit", ".FIT"},
	}
}

func CollectFITSFiles(root string, exts []string) ([]string, error) {
	var files []string
	extSet := make(map[string]bool)
	for _, e := range exts {
		extSet[e] = true
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if extSet[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

func ParseBatch(files []string, config BatchConfig) ([]BatchResult, BatchStats) {
	if config.Workers <= 0 {
		config.Workers = 32
	}

	jobs := make(chan string, config.Workers*2)
	results := make(chan BatchResult, config.Workers*2)

	var wg sync.WaitGroup
	var openCount int32
	var maxOpen int32

	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				current := atomic.AddInt32(&openCount, 1)
				for {
					m := atomic.LoadInt32(&maxOpen)
					if current <= m || atomic.CompareAndSwapInt32(&maxOpen, m, current) {
						break
					}
				}

				hdr, err := parseFileImmediate(path)

				atomic.AddInt32(&openCount, -1)

				results <- BatchResult{
					Path:   path,
					Header: hdr,
					Error:  err,
				}
			}
		}()
	}

	go func() {
		for _, f := range files {
			jobs <- f
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var allResults []BatchResult
	stats := BatchStats{Total: len(files)}

	for r := range results {
		allResults = append(allResults, r)
		if r.Error == nil {
			stats.Succeeded++
		} else {
			stats.Failed++
		}
	}

	stats.MaxOpenFD = int(maxOpen)
	return allResults, stats
}

func parseFileImmediate(path string) (*Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	hdr, err := ParseHeader(f)

	f.Close()

	if err != nil {
		return nil, err
	}
	return hdr, nil
}

func ParseDirectory(root string, config BatchConfig) ([]BatchResult, BatchStats, error) {
	files, err := CollectFITSFiles(root, config.Extensions)
	if err != nil {
		return nil, BatchStats{}, err
	}
	results, stats := ParseBatch(files, config)
	return results, stats, nil
}
