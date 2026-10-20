// Package web incluye en el binario la interfaz compilada (npm run build deja el resultado en web/dist).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Archivos devuelve la interfaz compilada, o nil si no se ha compilado todavía.
func Archivos() fs.FS {
	f, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(f, "index.html"); err != nil {
		return nil
	}
	return f
}
