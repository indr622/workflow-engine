//go:build parity

package domain_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestExecutionParityLive menjalankan setiap skenario di Camunda 8.8 referensi (REST v2) dan
// menulis ulang testdata/parity/expectations.json. Deviasi yang sudah tercatat dipertahankan.
//
//	ZEEBE_URL=http://localhost:8089 go test -tags=parity -run TestExecutionParityLive ./apps/engine/internal/modules/execution/domain
func TestExecutionParityLive(t *testing.T) {
	base := os.Getenv("ZEEBE_URL")
	if base == "" {
		base = "http://localhost:8089"
	}
	z := &zeebeClient{base: base, http: &http.Client{Timeout: 30 * time.Second}}
	if err := z.ping(); err != nil {
		t.Fatalf("camunda reference at %s is not reachable (make up PROFILES=camunda): %v", base, err)
	}
	previous := map[string]parityEntry{}
	if data, err := os.ReadFile(filepath.FromSlash(parityExpectationsPath)); err == nil {
		var old parityFile
		if json.Unmarshal(data, &old) == nil {
			for _, e := range old.Entries {
				previous[e.Scenario] = e
			}
		}
	}
	run := fmt.Sprintf("run-%d", time.Now().UnixNano())
	file := parityFile{Reference: "camunda/zeebe:8.8.44 (REST v2, no secondary storage)", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, sc := range parityScenarios() {
		camunda, err := z.runScenario(sc, run)
		if err != nil {
			t.Fatalf("%s on camunda: %v", sc.Name, err)
		}
		ours := runOurs(t, sc, run)
		entry := parityEntry{Scenario: sc.Name, Camunda: camunda, Ours: ours, Deviation: previous[sc.Name].Deviation}
		status := "same"
		if !sameOutcome(ours, camunda) {
			status = "DIFFERENT"
			if entry.Deviation == "" {
				t.Errorf("%s differs from Camunda:\nours:    %+v\ncamunda: %+v", sc.Name, ours, camunda)
			}
		}
		t.Logf("%-40s %s", sc.Name, status)
		file.Entries = append(file.Entries, entry)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.FromSlash(parityExpectationsPath)), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.FromSlash(parityExpectationsPath), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

type zeebeClient struct {
	base string
	http *http.Client
}

func (z *zeebeClient) ping() error {
	resp, err := z.http.Get(z.base + "/v2/topology")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("topology: %s", resp.Status)
	}
	return nil
}

func (z *zeebeClient) post(ctx context.Context, path string, body any, out any) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, z.base+path, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := z.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s", path, resp.Status, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

func (z *zeebeClient) deploy(name, xml string) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("resources", name+".bpmn")
	if err != nil {
		return err
	}
	if _, err := part.Write([]byte(xml)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	resp, err := z.http.Post(z.base+"/v2/deployments", w.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deploy %s: %s: %s", name, resp.Status, data)
	}
	return nil
}

type activatedJob struct {
	JobKey    string         `json:"jobKey"`
	Type      string         `json:"type"`
	Variables map[string]any `json:"variables"`
}

// activate mengaktifkan job bertipe tertentu milik run ini; job run lain diabaikan (timeout singkat
// membuatnya kembali tersedia).
func (z *zeebeClient) activate(jobType, run string, wait time.Duration) ([]activatedJob, error) {
	var res struct {
		Jobs []activatedJob `json:"jobs"`
	}
	_, err := z.post(context.Background(), "/v2/jobs/activation", map[string]any{
		"type": jobType, "maxJobsToActivate": 32, "timeout": 5000, "worker": "parity",
		"requestTimeout": wait.Milliseconds(),
	}, &res)
	if err != nil {
		return nil, err
	}
	var mine []activatedJob
	for _, j := range res.Jobs {
		if j.Variables[parityRunVar] == run {
			mine = append(mine, j)
		}
	}
	return mine, nil
}

func (z *zeebeClient) complete(jobKey, vars string) error {
	var m map[string]any
	if err := json.Unmarshal([]byte(vars), &m); err != nil {
		return err
	}
	_, err := z.post(context.Background(), "/v2/jobs/"+jobKey+"/completion", map[string]any{"variables": m}, nil)
	return err
}

func (z *zeebeClient) runScenario(sc parityScenario, run string) (parityOutcome, error) {
	if err := z.deploy(sc.Name, sc.Model.xml()); err != nil {
		return parityOutcome{}, err
	}
	var start map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sc.Variables), &start); err != nil {
		return parityOutcome{}, err
	}
	start[parityRunVar] = json.RawMessage(`"` + run + `"`)

	type created struct {
		Variables map[string]json.RawMessage `json:"variables"`
	}
	done := make(chan struct {
		res created
		err error
	}, 1)
	go func() {
		var res created
		_, err := z.post(context.Background(), "/v2/process-instances", map[string]any{
			"processDefinitionId": sc.Model.id, "variables": start, "awaitCompletion": true, "requestTimeout": 8000,
		}, &res)
		done <- struct {
			res created
			err error
		}{res, err}
	}()

	out := parityOutcome{Jobs: []string{}, OpenJobs: []string{}}
	for _, step := range sc.Steps {
		var job *activatedJob
		deadline := time.Now().Add(6 * time.Second)
		for job == nil && time.Now().Before(deadline) {
			jobs, err := z.activate(step.JobType, run, 1500*time.Millisecond)
			if err != nil {
				return out, err
			}
			if len(jobs) > 0 {
				job = &jobs[0]
				// Job lain yang ikut teraktivasi dilepas lewat timeout dan diaktifkan ulang nanti.
			}
		}
		if job == nil {
			// Job tidak muncul (mis. incident sebelum service task): langkah berikutnya dilewati.
			break
		}
		out.Jobs = append(out.Jobs, job.Type)
		if err := z.complete(job.JobKey, step.Variables); err != nil {
			return out, err
		}
	}

	result := <-done
	if result.err == nil {
		out.Completed = true
		out.Variables = map[string]json.RawMessage{}
		for k, v := range result.res.Variables {
			if k != parityRunVar {
				out.Variables[k] = v
			}
		}
	} else if !isTimeout(result.err) {
		return out, result.err
	}

	// Sapu job yang masih terbuka (tidak diselesaikan langkah mana pun). Tunggu timeout aktivasi
	// sebelumnya habis agar job yang sempat teraktivasi kembali tersedia.
	time.Sleep(5500 * time.Millisecond)
	for _, jt := range sc.JobTypes {
		jobs, err := z.activate(jt, run, 500*time.Millisecond)
		if err != nil {
			return out, err
		}
		for _, j := range jobs {
			out.OpenJobs = append(out.OpenJobs, j.Type)
			out.Jobs = append(out.Jobs, j.Type)
		}
	}
	sort.Strings(out.Jobs)
	sort.Strings(out.OpenJobs)
	return out, nil
}

func isTimeout(err error) bool {
	var msg string
	if err != nil {
		msg = err.Error()
	}
	return errors.Is(err, context.DeadlineExceeded) || bytes.Contains([]byte(msg), []byte("504")) ||
		bytes.Contains([]byte(msg), []byte("DEADLINE_EXCEEDED")) || bytes.Contains([]byte(msg), []byte("timed out"))
}
