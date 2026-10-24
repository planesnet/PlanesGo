//go:build windows

package proceso

import "os"

// DetachedEnv: see inherited_unix.go. On Windows a child only inherits the handles marked inheritable.
const DetachedEnv = "PGO_DESACOPLADO"

// StartClean only clears DetachedEnv on Windows.
func StartClean() { os.Unsetenv(DetachedEnv) }
