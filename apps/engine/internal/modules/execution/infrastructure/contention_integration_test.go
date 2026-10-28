//go:build integration

package infrastructure_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
)

func percentile(d []time.Duration, p float64) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[int(float64(len(d)-1)*p)]
}

// timedParallel menjalankan fn untuk setiap key secara paralel dan mengembalikan latensi per panggilan.
func timedParallel(t *testing.T, keys []string, fn func(key string) error) []time.Duration {
	t.Helper()
	out := make([]time.Duration, len(keys))
	var wg sync.WaitGroup
	errs := make(chan error, len(keys))
	for i, k := range keys {
		wg.Add(1)
		go func(i int, k string) {
			defer wg.Done()
			start := time.Now()
			if err := fn(k); err != nil {
				errs <- err
			}
			out[i] = time.Since(start)
		}(i, k)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	return out
}

// TestContentionNumbers mencatat latensi CompleteJob saat 50 command bersaing pada satu instance
// (row lock) dibanding 500 command paralel pada instance berbeda (rencana 12.11, ADR-0018).
func TestContentionNumbers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	fork := f.deployXML(t, "contention-fork", forkXML(50))
	linear := f.deployXML(t, "contention-linear", linearXML)

	openJobs := func(piKey string, limit int32) []string {
		jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: piKey, OnlyOpen: true, Limit: limit})
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, len(jobs))
		for i, j := range jobs {
			keys[i] = j.Key
		}
		return keys
	}

	var sameInstance []time.Duration
	for rep := 0; rep < 5; rep++ {
		created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: fork})
		if err != nil {
			t.Fatal(err)
		}
		sameInstance = append(sameInstance, timedParallel(t, openJobs(created.ProcessInstanceKey, 100), func(k string) error {
			return completeJob(ctx, f, k, nil)
		})...)
	}

	instances := make([]string, 500)
	for i := range instances {
		created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: linear})
		if err != nil {
			t.Fatal(err)
		}
		instances[i] = openJobs(created.ProcessInstanceKey, 1)[0]
	}
	distinct := timedParallel(t, instances, func(k string) error { return completeJob(ctx, f, k, nil) })

	t.Logf("50 parallel CompleteJob on one instance (5 runs, %d samples): p50=%v p99=%v max=%v",
		len(sameInstance), percentile(sameInstance, 0.50), percentile(sameInstance, 0.99), percentile(sameInstance, 1))
	t.Logf("500 parallel CompleteJob on distinct instances: p50=%v p99=%v max=%v",
		percentile(distinct, 0.50), percentile(distinct, 0.99), percentile(distinct, 1))
}
