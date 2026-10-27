// Command parity membandingkan keputusan terima/tolak engine kita terhadap Camunda 8.8 referensi
// untuk seluruh korpus BPMN. Mode offline membandingkan dengan expectations.json; mode -live
// juga men-deploy ke Camunda dan memperbarui expectations.json. Keputusan kita dihitung
// in-process lewat libs/go/bpmn, atau lewat engine yang berjalan bila -engine diisi.
//
//	go run ./tools/scripts/parity            # offline, exit 1 bila ada perbedaan tak terdokumentasi
//	go run ./tools/scripts/parity -live      # deploy ke Camunda (ZEEBE_URL) dan tulis ulang ekspektasi
//
// Pemeriksaan offline yang sama juga berjalan sebagai TestParityWithCamundaReference di libs/go/bpmn.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

type verdict struct {
	Accepted bool     `json:"accepted"`
	Codes    []string `json:"codes,omitempty"`
	Message  string   `json:"message,omitempty"`
}

type expectation struct {
	File      string  `json:"file"`
	Camunda   verdict `json:"camunda"`
	Ours      verdict `json:"ours"`
	Deviation string  `json:"deviation,omitempty"`
}

type expectationsFile struct {
	Reference string        `json:"reference"`
	UpdatedAt string        `json:"updatedAt"`
	Entries   []expectation `json:"entries"`
}

func main() {
	live := flag.Bool("live", false, "deploy to the Camunda reference and rewrite expectations")
	engineURL := flag.String("engine", os.Getenv("ENGINE_URL"), "validate through a running engine instead of in-process (optional)")
	zeebeURL := flag.String("zeebe", envOr("ZEEBE_URL", "http://localhost:8089"), "Camunda 8 REST base URL")
	corpus := flag.String("corpus", "libs/go/bpmn/testdata", "corpus directory")
	flag.Parse()

	expPath := filepath.Join(*corpus, "parity", "expectations.json")
	files := listCorpus(*corpus)

	previous := map[string]expectation{}
	if data, err := os.ReadFile(expPath); err == nil {
		var ef expectationsFile
		if err := json.Unmarshal(data, &ef); err != nil {
			fail("parse %s: %v", expPath, err)
		}
		for _, e := range ef.Entries {
			previous[e.File] = e
		}
	} else if !*live {
		fail("no expectations at %s; run with -live first", expPath)
	}

	var entries []expectation
	mismatches := 0
	for _, path := range files {
		rel := filepath.ToSlash(strings.TrimPrefix(path, filepath.Clean(*corpus)+string(filepath.Separator)))
		content, err := os.ReadFile(path)
		if err != nil {
			fail("read %s: %v", path, err)
		}
		e := expectation{File: rel, Ours: validateOurs(*engineURL, filepath.Base(path), content), Deviation: previous[rel].Deviation}
		if *live {
			e.Camunda = deployCamunda(*zeebeURL, filepath.Base(path), content)
		} else {
			prev, ok := previous[rel]
			if !ok {
				fmt.Printf("MISSING   %s (no Camunda expectation; run -live)\n", rel)
				mismatches++
				continue
			}
			e.Camunda = prev.Camunda
		}
		entries = append(entries, e)

		switch {
		case e.Ours.Accepted == e.Camunda.Accepted:
			fmt.Printf("OK        %s accepted=%v %v\n", rel, e.Ours.Accepted, e.Ours.Codes)
		case e.Deviation != "":
			fmt.Printf("DEVIATION %s ours=%v camunda=%v: %s\n", rel, e.Ours.Accepted, e.Camunda.Accepted, e.Deviation)
		default:
			fmt.Printf("MISMATCH  %s ours=%v %v camunda=%v %q\n", rel, e.Ours.Accepted, e.Ours.Codes, e.Camunda.Accepted, e.Camunda.Message)
			mismatches++
		}
	}

	if *live {
		out := expectationsFile{Reference: "camunda/zeebe:8.8.44", UpdatedAt: time.Now().UTC().Format(time.RFC3339), Entries: entries}
		data, _ := json.MarshalIndent(out, "", "  ")
		if err := os.MkdirAll(filepath.Dir(expPath), 0o755); err != nil {
			fail("mkdir: %v", err)
		}
		if err := os.WriteFile(expPath, append(data, '\n'), 0o644); err != nil {
			fail("write expectations: %v", err)
		}
		fmt.Printf("\nwrote %s (%d entries)\n", expPath, len(entries))
	}
	fmt.Printf("\n%d files, %d mismatch(es)\n", len(files), mismatches)
	if mismatches > 0 {
		os.Exit(1)
	}
}

func listCorpus(dir string) []string {
	var files []string
	for _, sub := range []string{"valid", "invalid"} {
		matches, _ := filepath.Glob(filepath.Join(dir, sub, "*.bpmn"))
		files = append(files, matches...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fail("no corpus files under %s", dir)
	}
	return files
}

// validateOurs menghitung keputusan kita di ProfileFull: in-process, atau lewat engine bila base diisi.
func validateOurs(base, name string, content []byte) verdict {
	if base == "" {
		_, issues, _ := bpmn.ParseAndValidate(bytes.NewReader(content), bpmn.ProfileFull)
		v := verdict{Accepted: !issues.HasErrors()}
		seen := map[string]bool{}
		for _, i := range issues.Errors() {
			if !seen[string(i.Code)] {
				seen[string(i.Code)] = true
				v.Codes = append(v.Codes, string(i.Code))
			}
		}
		sort.Strings(v.Codes)
		return v
	}
	body, _ := json.Marshal(map[string]any{
		"resources": []map[string]string{{"name": name, "content": base64.StdEncoding.EncodeToString(content)}},
		"profile":   "PROFILE_FULL",
	})
	resp, err := http.Post(base+"/workflow.deployment.v1.DeploymentService/ValidateResource", "application/json", bytes.NewReader(body))
	if err != nil {
		fail("engine request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail("engine returned %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		Valid  bool `json:"valid"`
		Issues []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		fail("engine response: %v", err)
	}
	v := verdict{Accepted: out.Valid}
	seen := map[string]bool{}
	for _, i := range out.Issues {
		if i.Severity == "SEVERITY_ERROR" && !seen[i.Code] {
			seen[i.Code] = true
			v.Codes = append(v.Codes, i.Code)
		}
	}
	sort.Strings(v.Codes)
	return v
}

// deployCamunda mengirim resource ke POST /v2/deployments (multipart) dan membaca keputusannya.
func deployCamunda(base, name string, content []byte) verdict {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("resources", name)
	_, _ = part.Write(content)
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, base+"/v2/deployments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("camunda request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return verdict{Accepted: true}
	}
	var problem struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(raw, &problem)
	msg := strings.TrimSpace(problem.Detail)
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return verdict{Accepted: false, Message: truncate(msg, 600)}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func envOr(key, def string) string { // dipakai untuk ZEEBE_URL
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "parity: "+format+"\n", args...)
	os.Exit(2)
}
