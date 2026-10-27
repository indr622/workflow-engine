package bpmn

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// parityExpectations adalah hasil deploy korpus ke Camunda 8 referensi,
// ditulis oleh `go run ./tools/scripts/parity -live`.
type parityExpectations struct {
	Reference string `json:"reference"`
	Entries   []struct {
		File      string `json:"file"`
		Camunda   struct{ Accepted bool }
		Deviation string `json:"deviation"`
	} `json:"entries"`
}

// TestParityWithCamundaReference: keputusan terima/tolak di ProfileFull harus sama dengan Camunda
// untuk setiap file korpus, kecuali perbedaan yang sengaja dan terdokumentasi di expectations.json.
func TestParityWithCamundaReference(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "parity", "expectations.json"))
	if err != nil {
		t.Skipf("no parity expectations: %v", err)
	}
	var exp parityExpectations
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatal(err)
	}
	if len(exp.Entries) == 0 {
		t.Fatal("expectations file has no entries")
	}
	covered := map[string]bool{}
	for _, e := range exp.Entries {
		covered[e.File] = true
		t.Run(e.File, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(e.File)))
			if err != nil {
				t.Fatal(err)
			}
			_, issues, _ := ParseAndValidate(bytes.NewReader(content), ProfileFull)
			accepted := !issues.HasErrors()
			switch {
			case accepted == e.Camunda.Accepted:
			case e.Deviation != "":
				t.Logf("documented deviation (ours=%v, %s=%v): %s", accepted, exp.Reference, e.Camunda.Accepted, e.Deviation)
			default:
				t.Errorf("ours accepted=%v but %s accepted=%v; issues: %v", accepted, exp.Reference, e.Camunda.Accepted, issues.Errors())
			}
		})
	}
	for _, dir := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*.bpmn"))
		for _, f := range files {
			rel := dir + "/" + filepath.Base(f)
			if !covered[rel] {
				t.Errorf("%s has no parity expectation; run `go run ./tools/scripts/parity -live`", rel)
			}
		}
	}
}
