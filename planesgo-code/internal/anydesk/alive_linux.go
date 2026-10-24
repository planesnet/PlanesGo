package anydesk

import (
	"os"
	"strconv"
	"strings"
)

// Alive: whether the process exists and is AnyDesk's (a PID from an old trace line may belong to another program
// by now).
func Alive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "anydesk")
}
