package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Nama service dan network di infra/docker/docker-compose.chaos.yml.
const (
	servicePostgres = "postgres"
	serviceEngine   = "engine"
	serviceEngine2  = "engine-2"
	chaosNetwork    = "workflow-engine-chaos"
	replicasProfile = "replicas"
)

// compose membungkus Docker CLI untuk project chaos. Semua perintah memakai file compose yang
// sama; profil `replicas` selalu diaktifkan pada perintah yang menyentuh seluruh project (down,
// logs) agar engine-2 ikut dibersihkan.
type compose struct {
	file  string
	image string
}

// cmdResult adalah keluaran gabungan dan exit code satu perintah.
type cmdResult struct {
	Output   string
	ExitCode int
}

// docker menjalankan `docker args...` dan mengembalikan output gabungan stdout+stderr. Exit code
// bukan nol dikembalikan lewat cmdResult tanpa error; error hanya bila proses gagal dijalankan.
func (c *compose) docker(ctx context.Context, args ...string) (cmdResult, error) {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // argumen dibangun runner sendiri, bukan input pengguna
	cmd.Env = os.Environ()
	if c.image != "" {
		cmd.Env = append(cmd.Env, "CHAOS_ENGINE_IMAGE="+c.image)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	res := cmdResult{Output: out.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return res, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return res, nil
}

// must menjalankan perintah docker dan mengubah exit code bukan nol menjadi error.
func (c *compose) must(ctx context.Context, args ...string) (string, error) {
	res, err := c.docker(ctx, args...)
	if err != nil {
		return res.Output, err
	}
	if res.ExitCode != 0 {
		return res.Output, fmt.Errorf("docker %s: exit %d: %s", strings.Join(args, " "), res.ExitCode, strings.TrimSpace(tail(res.Output, 2000)))
	}
	return res.Output, nil
}

func (c *compose) composeArgs(profile bool, args ...string) []string {
	base := []string{"compose", "-f", c.file}
	if profile {
		base = append(base, "--profile", replicasProfile)
	}
	return append(base, args...)
}

// down menghapus seluruh project chaos beserta volume-nya.
func (c *compose) down(ctx context.Context) error {
	_, err := c.must(ctx, c.composeArgs(true, "down", "-v", "--remove-orphans", "-t", "5")...)
	return err
}

// up menyalakan service dan menunggu healthcheck-nya.
func (c *compose) up(ctx context.Context, services ...string) error {
	_, err := c.must(ctx, c.composeArgs(false, append([]string{"up", "-d", "--wait", "--wait-timeout", "180"}, services...)...)...)
	return err
}

// upNoWait menyalakan service tanpa menunggu healthcheck (start serentak).
func (c *compose) upNoWait(ctx context.Context, services ...string) error {
	_, err := c.must(ctx, c.composeArgs(false, append([]string{"up", "-d", "--no-deps"}, services...)...)...)
	return err
}

func (c *compose) start(ctx context.Context, service string) error {
	_, err := c.must(ctx, c.composeArgs(false, "start", service)...)
	return err
}

func (c *compose) restart(ctx context.Context, service string) error {
	_, err := c.must(ctx, c.composeArgs(false, "restart", "-t", "10", service)...)
	return err
}

// containerID mengembalikan ID container service (termasuk yang sudah berhenti).
func (c *compose) containerID(ctx context.Context, service string) (string, error) {
	out, err := c.must(ctx, c.composeArgs(true, "ps", "-a", "-q", service)...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" || strings.Contains(id, "\n") {
		return "", fmt.Errorf("expected one container for %s, got %q", service, id)
	}
	return id, nil
}

// kill mengirim SIGKILL ke container service.
func (c *compose) kill(ctx context.Context, service string) error {
	id, err := c.containerID(ctx, service)
	if err != nil {
		return err
	}
	_, err = c.must(ctx, "kill", "-s", "KILL", id)
	return err
}

// containerState mengembalikan status, waktu start, dan status health container service.
func (c *compose) containerState(ctx context.Context, service string) (status, startedAt, health string, err error) {
	id, err := c.containerID(ctx, service)
	if err != nil {
		return "", "", "", err
	}
	out, err := c.must(ctx, "inspect", "-f", "{{.State.Status}}|{{.State.StartedAt}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", id)
	if err != nil {
		return "", "", "", err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("unexpected inspect output %q", out)
	}
	return parts[0], parts[1], parts[2], nil
}

// waitHealthy menunggu healthcheck container service menjadi healthy.
func (c *compose) waitHealthy(ctx context.Context, service string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		status, _, health, err := c.containerState(ctx, service)
		if err == nil && status == "running" && health == "healthy" {
			return nil
		}
		last = fmt.Sprintf("status=%s health=%s err=%v", status, health, err)
		if err := sleepCtx(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("%s not healthy after %s (%s)", service, timeout, last)
}

// disconnect memutus container service dari network chaos.
func (c *compose) disconnect(ctx context.Context, service string) error {
	id, err := c.containerID(ctx, service)
	if err != nil {
		return err
	}
	_, err = c.must(ctx, "network", "disconnect", chaosNetwork, id)
	return err
}

// connect menyambungkan kembali container dengan alias nama service agar DNS `postgres` pulih.
func (c *compose) connect(ctx context.Context, service string) error {
	id, err := c.containerID(ctx, service)
	if err != nil {
		return err
	}
	_, err = c.must(ctx, "network", "connect", "--alias", service, chaosNetwork, id)
	return err
}

// engineCLI menjalankan subperintah admin engine (`verify`, `rebuild`) di container sekali pakai
// dengan image dan environment yang sama dengan service engine, tanpa migrasi (perintah operator).
func (c *compose) engineCLI(ctx context.Context, args ...string) (cmdResult, error) {
	return c.docker(ctx, c.composeArgs(false, append([]string{"run", "--rm", "--no-deps", "-T", "-e", "ENGINE_MIGRATE=false", serviceEngine}, args...)...)...)
}

// logs mengembalikan log service tanpa prefix (kosong bila container tidak ada).
func (c *compose) logs(ctx context.Context, services ...string) string {
	args := append([]string{"logs", "--no-color", "--timestamps"}, services...)
	if len(services) == 1 {
		args = append([]string{"logs", "--no-color", "--no-log-prefix"}, services...)
	}
	res, err := c.docker(ctx, c.composeArgs(true, args...)...)
	if err != nil {
		return err.Error()
	}
	return res.Output
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
