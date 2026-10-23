package gbrowser

import (
	"os"
	"path/filepath"
	"strings"
)

// RemoveOAuthLeftovers borra de la carpeta de datos de pgocode lo que guardaba el acceso a Google por OAuth (ya no
// existe): el fichero del cliente (google-oauth-client.json) y las conexiones de cada usuario (google/<usuario>.json,
// con su token de renovación). De la carpeta google/ solo borra ficheros .json normales, y la carpeta si queda vacía:
// nada más. Devuelve lo que ha borrado.
func RemoveOAuthLeftovers(dataDir string) []string {
	if dataDir == "" {
		return nil
	}
	var removed []string
	client := filepath.Join(dataDir, "google-oauth-client.json")
	if fi, err := os.Lstat(client); err == nil && fi.Mode().IsRegular() && os.Remove(client) == nil {
		removed = append(removed, client)
	}
	dir := filepath.Join(dataDir, "google")
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return removed
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return removed
	}
	for _, e := range es {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f := filepath.Join(dir, e.Name())
		if os.Remove(f) == nil {
			removed = append(removed, f)
		}
	}
	if os.Remove(dir) == nil { // solo si ha quedado vacía
		removed = append(removed, dir)
	}
	return removed
}
