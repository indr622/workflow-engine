package bpmn

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// FuzzParse: parser tidak boleh panic atau hang untuk input apa pun, dan bila sukses, model harus
// memenuhi invarian serta Validate tidak panic.
func FuzzParse(f *testing.F) {
	for _, dir := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*.bpmn"))
		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(data)
		}
	}
	f.Add([]byte(`<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"><bpmn:process id="p" isExecutable="true"><bpmn:startEvent id="s"/></bpmn:process></bpmn:definitions>`))

	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Parse(bytes.NewReader(data), WithMaxBytes(1<<20), WithMaxElements(5000), WithMaxDepth(32))
		if err != nil {
			return
		}
		issues := Validate(d, ProfileFull)
		_ = Validate(d, ProfileExecutable)
		// Invarian indeks hanya berlaku bila id unik dan tidak kosong.
		for _, i := range issues {
			if i.Code == CodeDuplicateID || i.Code == CodeMissingID {
				return
			}
		}
		for _, p := range d.Processes() {
			assertInvariants(t, p)
		}
	})
}
