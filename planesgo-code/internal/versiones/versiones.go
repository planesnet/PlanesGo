// Package versiones: how pgocode's versions are compared. In «0.7.0» the major version is «0.7» and the minor one is
// the number after it («0»). The client (the desktop app and its helper) and the server must have the same major
// version; their minor versions may differ (they are updated apart). A development build («dev», or no version) is
// compatible with any.
package versiones

import (
	"fmt"
	"strings"
)

// Header: the client's version, in its requests to the server.
const Header = "X-PGO-Version"

// Major: the major version of v («0.7» of «0.7.0»); "" for a development build or one that is not x.y.z.
func Major(v string) string {
	p := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(p) != 3 {
		return ""
	}
	for _, x := range p {
		if x == "" || strings.Trim(x, "0123456789") != "" {
			return ""
		}
	}
	return p[0] + "." + p[1]
}

// Compatible: whether a client and a server of these versions can work together.
func Compatible(client, server string) bool {
	a, b := Major(client), Major(server)
	return a == "" || b == "" || a == b
}

// Mismatch: what the person is told when they are not compatible.
func Mismatch(client, server string) string {
	return fmt.Sprintf("la app (versión %s) no es compatible con el servidor de pgocode (versión %s): tienen que tener la misma versión mayor (%s); actualiza la que vaya por detrás",
		client, server, Major(server))
}
