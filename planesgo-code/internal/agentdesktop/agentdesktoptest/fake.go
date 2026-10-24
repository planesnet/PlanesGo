// Package agentdesktoptest: fake programs (sh scripts) for the agent desktop's tests. Each one writes its arguments
// to <dir>/<name>.args, one line per call; FreeRDP also writes what it reads from its input to <dir>/xfreerdp.stdin
// and, with the Unicode keyboard, what reaches its pipe to <dir>/typed.
package agentdesktoptest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"planesgo-code/internal/agentdesktop"
)

// Screenshot: the PNG the fake import copies (the test writes it with the size it wants).
func Screenshot(dir string) string { return filepath.Join(dir, "screenshot.png") }

// Tools writes the fake programs in dir. clientExit: if not empty, FreeRDP prints it and exits with code 131
// (a failed login); if empty, it stays connected.
func Tools(t testing.TB, dir, clientExit string) agentdesktop.Tools {
	t.Helper()
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	log := func(name string) string { return "echo \"$*\" >> " + q(filepath.Join(dir, name+".args")) + "\n" }
	client := log("xfreerdp") + "IFS= read -r pw; printf '%s' \"$pw\" > " + q(filepath.Join(dir, "xfreerdp.stdin")) + "\n" +
		"if IFS= read -r more; then echo \"$more\" >> " + q(filepath.Join(dir, "xfreerdp.stdin")) + "; fi\n"
	if clientExit != "" {
		client += "echo " + q(clientExit) + "\nexit 131\n"
	} else {
		client += "for a; do case \"$a\" in /kbd:*pipe:*) p=${a#*pipe:}; mkfifo \"$p\"; cat \"$p\" > " + q(filepath.Join(dir, "typed")) + " 2>/dev/null </dev/null & ;; esac; done\n" +
			"exec sleep 60\n"
	}
	scripts := map[string]string{
		"Xvfb":     log("Xvfb") + "echo 99 >&3\nexec sleep 60\n",
		"xfreerdp": client,
		"xdotool":  log("xdotool") + "[ \"$1\" = search ] && echo 4242\nexit 0\n",
		"import":   log("import") + "for a; do out=$a; done\ncp " + q(Screenshot(dir)) + " \"${out#png:}\"\n",
		"ffmpeg":   log("ffmpeg") + "for a; do out=$a; done\nread q\necho video > \"$out\"\n",
	}
	paths := map[string]string{}
	for name, body := range scripts {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
		paths[name] = p
	}
	return agentdesktop.Tools{Xvfb: paths["Xvfb"], Xdotool: paths["xdotool"], XFreeRDP: paths["xfreerdp"], Import: paths["import"], FFmpeg: paths["ffmpeg"]}
}

// Args: the calls of a fake program (one per line).
func Args(dir, name string) []string {
	b, _ := os.ReadFile(filepath.Join(dir, name+".args"))
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
