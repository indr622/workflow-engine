// Package archtest menegakkan aturan Clean Architecture dan batas modul engine
// dengan membaca graf impor dari `go list`. Test ini gagal jika ada impor yang melanggar.
package archtest

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const modulePrefix = "github.com/indr622/workflow-engine/apps/engine/internal/modules/"

var layerPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(modulePrefix) + `([^/]+)/(api|domain|application|infrastructure|interfaces)(/|$)`)

// allowedSameModule: lapisan mana yang boleh diimpor oleh lapisan lain di modul yang sama.
var allowedSameModule = map[string][]string{
	"api":    {},
	"domain": {},
	// application boleh memakai api modulnya sendiri: tipe event yang dipublikasikan dan
	// interface publik yang diimplementasikannya tinggal di sana.
	"application":    {"domain", "api"},
	"infrastructure": {"domain", "application", "api"},
	"interfaces":     {"domain", "application", "api"},
}

type pkgLayer struct {
	module string
	layer  string
}

func parseLayer(importPath string) (pkgLayer, bool) {
	m := layerPattern.FindStringSubmatch(importPath)
	if m == nil {
		return pkgLayer{}, false
	}
	return pkgLayer{module: m[1], layer: m[2]}, true
}

func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

// checkImport mengembalikan pesan pelanggaran, atau string kosong jika impor diizinkan.
func checkImport(from pkgLayer, importPath string) string {
	to, isModulePkg := parseLayer(importPath)

	if from.layer == "domain" && !isStdlib(importPath) && (!isModulePkg || to != from) {
		return "domain may only import the standard library"
	}
	if !isModulePkg {
		return ""
	}
	if to.module != from.module {
		if to.layer != "api" {
			return fmt.Sprintf("module %q may only import the api package of module %q", from.module, to.module)
		}
		return ""
	}
	if to.layer == from.layer {
		return ""
	}
	for _, allowed := range allowedSameModule[from.layer] {
		if allowed == to.layer {
			return ""
		}
	}
	return fmt.Sprintf("layer %q must not import layer %q", from.layer, to.layer)
}

// touchSources membaca semua file .go modul agar cache `go test` batal saat kode modul berubah.
// Tanpa ini, hasil `go list` di bawah tidak dilacak cache dan test bisa lolos dari cache lama.
func touchSources(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir("../modules", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		_, err = os.ReadFile(path)
		return err
	})
	if err != nil {
		t.Fatalf("read module sources: %v", err)
	}
}

func TestModuleBoundaries(t *testing.T) {
	touchSources(t)
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Imports " "}}`, modulePrefix+"...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v: %s", err, stderr.String())
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	checked := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		from, ok := parseLayer(fields[0])
		if !ok {
			continue
		}
		checked++
		for _, imp := range fields[1:] {
			if msg := checkImport(from, imp); msg != "" {
				t.Errorf("%s imports %s: %s", fields[0], imp, msg)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no module packages found; the layer pattern is probably wrong")
	}
}

func TestCheckImportRules(t *testing.T) {
	p := modulePrefix
	tests := []struct {
		name    string
		from    pkgLayer
		import_ string
		ok      bool
	}{
		{"domain uses stdlib", pkgLayer{"job", "domain"}, "time", true},
		{"domain uses driver", pkgLayer{"job", "domain"}, "github.com/jackc/pgx/v5", false},
		{"application uses own domain", pkgLayer{"job", "application"}, p + "job/domain", true},
		{"application uses own api", pkgLayer{"job", "application"}, p + "job/api", true},
		{"domain uses own api", pkgLayer{"job", "domain"}, p + "job/api", false},
		{"application uses own infrastructure", pkgLayer{"job", "application"}, p + "job/infrastructure", false},
		{"interfaces uses own application", pkgLayer{"job", "interfaces"}, p + "job/application", true},
		{"cross module api", pkgLayer{"job", "application"}, p + "health/api", true},
		{"cross module domain", pkgLayer{"job", "application"}, p + "health/domain", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkImport(tt.from, tt.import_) == ""
			if got != tt.ok {
				t.Fatalf("checkImport(%v, %q) allowed=%v, want %v", tt.from, tt.import_, got, tt.ok)
			}
		})
	}
}
