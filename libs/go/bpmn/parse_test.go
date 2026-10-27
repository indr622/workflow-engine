package bpmn

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "regenerate golden files")

var expectPattern = regexp.MustCompile(`<!-- expect: ([A-Z0-9_, ]+) -->`)

func corpus(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", dir, "*.bpmn"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus files in testdata/%s: %v", dir, err)
	}
	sort.Strings(files)
	return files
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func checkGolden(t *testing.T, goldenPath string, got any) {
	t.Helper()
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	gotJSON = append(gotJSON, '\n')
	if *update {
		if err := os.WriteFile(goldenPath, gotJSON, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", goldenPath, err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), gotJSON) {
		t.Errorf("golden mismatch for %s (run with -update to regenerate)\n--- got ---\n%s", goldenPath, gotJSON)
	}
}

// TestValidCorpus: semua file valid ter-parse, bebas issue error di ProfileFull, dan cocok golden.
func TestValidCorpus(t *testing.T) {
	for _, path := range corpus(t, "valid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			d, err := Parse(bytes.NewReader(readFile(t, path)))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			issues := Validate(d, ProfileFull)
			if errs := issues.Errors(); len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			checkGolden(t, strings.TrimSuffix(path, ".bpmn")+".golden.json", map[string]any{
				"model":    dump(d),
				"warnings": issueCodes(issues.Warnings()),
			})
		})
	}
}

// TestInvalidCorpus: setiap file invalid memicu persis kode yang dideklarasikan di komentar
// "<!-- expect: ... -->" (sebagai error), dan daftar issue lengkap cocok dengan golden.
func TestInvalidCorpus(t *testing.T) {
	for _, path := range corpus(t, "invalid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data := readFile(t, path)
			m := expectPattern.FindSubmatch(data)
			if m == nil {
				t.Fatal("file has no <!-- expect: ... --> comment")
			}
			want := map[Code]bool{}
			for _, c := range strings.Split(string(m[1]), ",") {
				want[Code(strings.TrimSpace(c))] = true
			}

			_, issues, err := ParseAndValidate(bytes.NewReader(data), ProfileFull)
			var pe *ParseError
			if err != nil && !errors.As(err, &pe) {
				t.Fatalf("unexpected non-ParseError: %v", err)
			}
			got := map[Code]bool{}
			for _, i := range issues.Errors() {
				got[i.Code] = true
			}
			for c := range want {
				if !got[c] {
					t.Errorf("expected error code %s, got %v", c, issues)
				}
			}
			for c := range got {
				if !want[c] {
					t.Errorf("unexpected error code %s (declare it in the expect comment if intended): %v", c, issues)
				}
			}
			checkGolden(t, strings.TrimSuffix(path, ".bpmn")+".issues.golden.json", issuesForGolden(issues))
		})
	}
}

func issueCodes(is Issues) []string {
	out := make([]string, 0, len(is))
	for _, i := range is {
		out = append(out, string(i.Code)+"@"+i.ElementID)
	}
	return out
}

type goldenIssue struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	ProcessID string `json:"processId,omitempty"`
	ElementID string `json:"elementId,omitempty"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Message   string `json:"message"`
}

func issuesForGolden(is Issues) []goldenIssue {
	out := make([]goldenIssue, 0, len(is))
	for _, i := range is {
		out = append(out, goldenIssue{
			Code: string(i.Code), Severity: i.Severity.String(), ProcessID: i.ProcessID,
			ElementID: i.ElementID, Line: i.Position.Line, Column: i.Position.Column, Message: i.Message,
		})
	}
	return out
}

// TestExecutableProfile memastikan ProfileExecutable menerima tepat elemen Fase 2 dan menolak sisanya.
func TestExecutableProfile(t *testing.T) {
	accepted := map[string]bool{
		"v01_minimal.bpmn": true, "v02_service_task.bpmn": true, "v03_exclusive_gateway.bpmn": true,
		"v04_parallel_gateway.bpmn": true, "v05_inclusive_gateway.bpmn": true, "v06_undefined_manual_task.bpmn": true,
		"v22_bpmn2_prefix.bpmn": true, "v23_large_process.bpmn": true, "v24_implicit_end.bpmn": true,
		"v25_non_executable_pool.bpmn": true, "v26_camunda7_extension.bpmn": true, "v27_unreachable_element.bpmn": true,
	}
	for _, path := range corpus(t, "valid") {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			d, err := Parse(bytes.NewReader(readFile(t, path)))
			if err != nil {
				t.Fatal(err)
			}
			issues := Validate(d, ProfileExecutable)
			if got := !issues.HasErrors(); got != accepted[name] {
				t.Fatalf("executable profile accepted=%v, want %v: %v", got, accepted[name], issues.Errors())
			}
			for _, i := range issues.Errors() {
				if i.Code != CodeUnsupportedElement {
					t.Fatalf("executable profile must only add UNSUPPORTED_ELEMENT, got %s", i.Code)
				}
			}
		})
	}
}

// TestModelInvariants memeriksa indeks dan referensi silang konsisten di semua file valid.
func TestModelInvariants(t *testing.T) {
	for _, path := range corpus(t, "valid") {
		d, err := Parse(bytes.NewReader(readFile(t, path)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range d.Processes() {
			assertInvariants(t, p)
		}
	}
}

func assertInvariants(t *testing.T, p *Process) {
	t.Helper()
	var walk func(s *Scope)
	walk = func(s *Scope) {
		for _, el := range s.Elements() {
			if got, ok := p.Element(el.ID()); !ok || got != el {
				t.Errorf("%s: element %q not indexed to itself", p.ID(), el.ID())
			}
			if el.Scope() != s {
				t.Errorf("%s: element %q has wrong scope", p.ID(), el.ID())
			}
			switch x := el.(type) {
			case *SequenceFlow:
				if x.Source() != nil && !containsFlow(x.Source().Outgoing(), x) {
					t.Errorf("%s: flow %q missing from source outgoing", p.ID(), x.ID())
				}
				if x.Target() != nil && !containsFlow(x.Target().Incoming(), x) {
					t.Errorf("%s: flow %q missing from target incoming", p.ID(), x.ID())
				}
			case *Activity:
				for _, b := range x.BoundaryEvents() {
					if host, _ := b.AttachedTo(); host != x {
						t.Errorf("%s: boundary %q not attached back to %q", p.ID(), b.ID(), x.ID())
					}
				}
				if x.Inner() != nil {
					if owner, _ := x.Inner().Owner(); owner != x {
						t.Errorf("%s: inner scope of %q has wrong owner", p.ID(), x.ID())
					}
					walk(x.Inner())
				}
			}
		}
	}
	walk(&p.Scope)
}

func containsFlow(fs []*SequenceFlow, f *SequenceFlow) bool {
	for _, x := range fs {
		if x == f {
			return true
		}
	}
	return false
}

// TestConcurrentReads: model aman dibaca paralel (jalankan dengan -race).
func TestConcurrentReads(t *testing.T) {
	d, err := Parse(bytes.NewReader(readFile(t, filepath.Join("testdata", "valid", "v23_large_process.bpmn"))))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, p := range d.Processes() {
				start, _ := p.NoneStartEvent()
				var n FlowNode = start
				for n != nil && len(n.Outgoing()) > 0 {
					n = n.Outgoing()[0].Target()
				}
				_ = Validate(d, ProfileExecutable)
			}
		}()
	}
	wg.Wait()
}

func TestParseOptionsAndFatalErrors(t *testing.T) {
	big := readFile(t, filepath.Join("testdata", "valid", "v23_large_process.bpmn"))
	tests := []struct {
		name string
		data []byte
		opts []ParseOption
		code Code
	}{
		{"too large", big, []ParseOption{WithMaxBytes(1024)}, CodeResourceTooLarge},
		{"too many elements", big, []ParseOption{WithMaxElements(10)}, CodeTooManyElements},
		{"too deep", big, []ParseOption{WithMaxDepth(2)}, CodeNestingTooDeep},
		{"not bpmn", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), nil, CodeNotBPMN},
		{"malformed", []byte(`<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL">`), nil, CodeXMLMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(bytes.NewReader(tt.data), tt.opts...)
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want *ParseError", err)
			}
			if pe.Code != tt.code {
				t.Fatalf("code = %s, want %s (%v)", pe.Code, tt.code, err)
			}
		})
	}
}
