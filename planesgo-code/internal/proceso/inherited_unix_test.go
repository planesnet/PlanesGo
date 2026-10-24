//go:build linux

package proceso

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The helper process: StartClean, then it says whether it still has descriptor 3.
func TestMain(m *testing.M) {
	if os.Getenv("PGO_TEST_FD3") == "1" {
		StartClean()
		_, err := os.Stat("/proc/self/fd/3")
		fmt.Printf("fd3=%v\n", err == nil)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStartCleanDropsInherited(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	run := func(detached bool) string {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "PGO_TEST_FD3=1")
		if detached {
			cmd.Env = append(cmd.Env, DetachedEnv+"=1")
		}
		cmd.ExtraFiles = []*os.File{w} // like a descriptor the parent left open (fd 3 in the child)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := run(false); got != "fd3=true" {
		t.Fatalf("without %s the inherited descriptor stays: %s", DetachedEnv, got)
	}
	if got := run(true); got != "fd3=false" {
		t.Errorf("StartClean must drop it: %s", got)
	}
}
