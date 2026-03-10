package pipeline

import (
	"context"
	"errors"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pbnjay/memory"

	"vargasparse/internal/progress"
)

// ComputeWorkers returns the optimal number of goroutines.
func ComputeWorkers(override int) int {
	if override > 0 {
		return override
	}

	cpuCount := runtime.NumCPU()
	totalRAMGB := float64(memory.TotalMemory()) / (1024 * 1024 * 1024)

	ramBudget := int(math.Floor(totalRAMGB * 10))
	workers := cpuCount
	if ramBudget < workers {
		workers = ramBudget
	}
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}
	return workers
}

func validateConfig(cfg *Config) {
	if cfg.EngineMode == "" {
		cfg.EngineMode = EngineDeterministic
	}
	if cfg.QualityDecider == nil {
		cfg.QualityDecider = defaultQualityDecider{}
	}
}

// Run launches the worker pool and returns sorted results.
func Run(cfg *Config, numPages int, progressCh chan<- progress.Event) []PageResult {
	validateConfig(cfg)

	jobs := make(chan int, numPages)
	results := make(chan PageResult, numPages)

	ctx := context.Background()

	var wg sync.WaitGroup
	for w := 0; w < cfg.NumWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageIndex := range jobs {
				var pageCtx context.Context
				var cancel context.CancelFunc
				if cfg.PageTimeoutSec > 0 {
					pageCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.PageTimeoutSec)*time.Second)
				} else {
					pageCtx, cancel = context.WithCancel(ctx)
				}

				r := processPage(pageCtx, cfg, pageIndex)
				if errors.Is(pageCtx.Err(), context.DeadlineExceeded) {
					r.ErrorCode = ErrorCodeTimeout
					if strings.TrimSpace(r.Text) == "" {
						r.Method = progress.MethodOCRFail
					}
				}

				results <- r
				progressCh <- progress.Event{
					PageNum:  r.PageNum,
					Method:   r.Method,
					Score:    r.Confidence,
					Duration: r.Duration,
					Warning:  strings.Join(r.Warnings, "; "),
				}
				cancel()
			}
		}()
	}

	for i := 0; i < numPages; i++ {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]PageResult, 0, numPages)
	for r := range results {
		all = append(all, r)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].PageNum < all[j].PageNum
	})
	return all
}
