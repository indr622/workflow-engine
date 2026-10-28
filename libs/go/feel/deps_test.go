package feel

import (
	"os/exec"
	"strings"
	"testing"
)

// TestOnlyStdlibDependencies menjaga paket ini tetap kernel murni: tanpa dependency pihak ketiga.
func TestOnlyStdlibDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "github.com/indr622/workflow-engine/libs/go/feel") {
			continue
		}
		if strings.Contains(strings.SplitN(dep, "/", 2)[0], ".") {
			t.Errorf("non-stdlib dependency: %s", dep)
		}
	}
}
