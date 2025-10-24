package voxelworld

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSubmoduleTestSuites(t *testing.T) {
	modules := []struct {
		name string
		dir  string
	}{
		{name: "central", dir: "central"},
		{name: "chunk-server", dir: "chunk-server"},
	}

	for _, module := range modules {
		module := module
		t.Run(module.name, func(t *testing.T) {
			cmd := exec.Command("go", "test", "./...")
			cmd.Dir = filepath.Join(".", module.dir)
			cmd.Env = append(os.Environ(),
				"GOWORK=off",
				"GOPROXY=off",
				"GOSUMDB=off",
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go test ./... in %s failed: %v\n%s", module.dir, err, output)
			}
		})
	}
}
