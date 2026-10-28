package sdk

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDependencies menjaga SDK siap dipisah menjadi module sendiri: hanya libs/go/api,
// connectrpc.com/connect, google.golang.org/protobuf, dan stdlib. Engine tidak boleh terseret.
func TestDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	allowed := []string{
		"github.com/indr622/workflow-engine/libs/go/sdk",
		"github.com/indr622/workflow-engine/libs/go/api/",
		"connectrpc.com/connect",
		"google.golang.org/protobuf/",
		// Stub proto meng-import anotasi buf.validate tanpa memakai runtime protovalidate.
		"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate",
	}
	for _, dep := range strings.Fields(string(out)) {
		if !strings.Contains(strings.SplitN(dep, "/", 2)[0], ".") {
			continue // stdlib
		}
		ok := false
		for _, prefix := range allowed {
			if strings.HasPrefix(dep, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("sdk depends on %s", dep)
		}
	}
}
