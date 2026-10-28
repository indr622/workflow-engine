// Command worker adalah demo end-to-end SDK: men-deploy order-process.bpmn, menjalankan tiga job
// worker, membuat N instance dengan CreateInstanceWithResult, lalu mencetak ringkasan. Keluar 0
// bila semua instance COMPLETED sebelum batas waktu.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/indr622/workflow-engine/apps/worker-example/internal/handlers"
	"github.com/indr622/workflow-engine/apps/worker-example/processes"
	"github.com/indr622/workflow-engine/libs/go/sdk"
)

func main() {
	engineURL := flag.String("engine-url", envOr("ENGINE_URL", "http://localhost:8080"), "engine base URL")
	instances := flag.Int("instances", 10, "number of order instances to run")
	timeout := flag.Duration("timeout", 30*time.Second, "overall deadline")
	serve := flag.Bool("serve", false, "keep workers running after the demo instances (container mode)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log, *engineURL, *instances, *timeout, *serve); err != nil {
		log.Error("demo failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(log *slog.Logger, engineURL string, instances int, timeout time.Duration, serve bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := sdk.NewClient(engineURL)
	if err != nil {
		return err
	}

	deployCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	dep, err := client.Deploy(deployCtx, sdk.Resource{Name: "order-process.bpmn", Content: processes.OrderProcess})
	cancel()
	if err != nil {
		return fmt.Errorf("deploy: %w", err)
	}
	log.Info("deployed", slog.String("process", processes.OrderProcessID), slog.Int("version", int(dep.Processes[0].Version)))

	payments := &handlers.Payments{}
	var workers []*sdk.JobWorker
	for _, spec := range []struct {
		jobType string
		handler sdk.Handler
	}{
		{"validate-order", handlers.ValidateOrder},
		{"charge-payment", payments.Charge},
		{"ship-order", handlers.ShipOrder},
	} {
		w, err := sdk.NewJobWorker(client).JobType(spec.jobType).Handler(spec.handler).Name("demo-" + spec.jobType).
			Concurrency(4).RequestTimeout(5 * time.Second).Logger(log).Build()
		if err != nil {
			return err
		}
		if err := w.Start(ctx); err != nil {
			return err
		}
		workers = append(workers, w)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, w := range workers {
			_ = w.Close(closeCtx)
		}
	}()

	start := time.Now()
	runCtx, cancelRun := context.WithTimeout(ctx, timeout)
	defer cancelRun()
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		completed int
		failures  []error
	)
	for i := 1; i <= instances; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := client.CreateInstanceWithResult(runCtx, sdk.Latest(processes.OrderProcessID),
				handlers.Order{OrderID: fmt.Sprintf("ORD-%03d", i), Amount: float64(100 * i)}, sdk.WithResultTimeout(timeout))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Errorf("order %d: %w", i, err))
				return
			}
			tracking, _ := res.Variables.String("trackingNumber")
			log.Info("order completed", slog.String("instance", res.Key), slog.String("state", res.State), slog.String("tracking", tracking))
			if res.State == "COMPLETED" {
				completed++
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("%d instances completed, %d retries, in %s\n", completed, payments.Retries.Load(), time.Since(start).Round(time.Millisecond))
	if len(failures) > 0 || completed != instances {
		return errors.Join(append(failures, fmt.Errorf("%d of %d instances completed", completed, instances))...)
	}
	if serve {
		log.Info("serving workers until interrupted")
		<-ctx.Done()
	}
	return nil
}
