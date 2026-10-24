package anydesk

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Client: the installed AnyDesk client (PGO_ANYDESK, the one in the PATH or the usual place); "" if there is none.
func Client() string {
	if p := os.Getenv("PGO_ANYDESK"); p != "" {
		return p
	}
	names := []string{"anydesk"}
	var places []string
	switch runtime.GOOS {
	case "windows":
		names = []string{"AnyDesk.exe", "anydesk.exe"}
		for _, v := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if d := os.Getenv(v); d != "" {
				places = append(places, filepath.Join(d, "AnyDesk", "AnyDesk.exe"))
			}
		}
	case "darwin":
		places = []string{"/Applications/AnyDesk.app/Contents/MacOS/AnyDesk"}
	default:
		places = []string{"/usr/bin/anydesk", "/usr/local/bin/anydesk", "/opt/anydesk/anydesk"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	for _, p := range places {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// TraceFiles: the traces where AnyDesk writes its sessions on this computer (the user's and the service's). Not all
// exist, and the service's may not be readable: they are followed if they are.
func TraceFiles() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		var l []string
		if d := os.Getenv("AppData"); d != "" {
			l = append(l, filepath.Join(d, "AnyDesk", "ad.trace"))
		}
		if d := os.Getenv("ProgramData"); d != "" {
			l = append(l, filepath.Join(d, "AnyDesk", "ad_svc.trace"))
		}
		return l
	case "darwin":
		return []string{filepath.Join(home, ".anydesk", "anydesk.trace"), filepath.Join(home, ".anydesk", "ad.trace"),
			filepath.Join(home, "Library", "Application Support", "AnyDesk", "ad.trace")}
	}
	return []string{filepath.Join(home, ".anydesk", "anydesk.trace"), filepath.Join(home, ".anydesk", "ad.trace"), "/var/log/anydesk.trace"}
}
