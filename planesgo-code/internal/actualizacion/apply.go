package actualizacion

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Apply puts the prepared version (<DirPreparado>/nuevo) in place of the installed app (DirApp), keeping the
// previous one in <DirApp>.anterior and going back to it if the swap fails. The server calls it once it has shut
// down, and then starts the new version (it no longer needs the desktop app for this: the server lives on its own).
func (a *Actualizador) Apply() error {
	nuevo := filepath.Join(a.DirPreparado, "nuevo")
	backend := filepath.Base(a.Backend())
	if _, err := os.Stat(filepath.Join(a.DirApp, "paquete.sha256")); err != nil {
		return errors.New("la app no está instalada desde un paquete")
	}
	if _, err := os.Stat(filepath.Join(nuevo, backend)); err != nil {
		return errors.New("no hay ninguna versión preparada")
	}
	anterior := a.DirApp + ".anterior"
	if err := os.RemoveAll(anterior); err != nil {
		return err
	}
	if err := os.Rename(a.DirApp, anterior); err != nil {
		return fmt.Errorf("no se puede apartar la versión instalada: %w", err)
	}
	if err := os.Rename(nuevo, a.DirApp); err != nil {
		os.Rename(anterior, a.DirApp) // back to the installed one
		return fmt.Errorf("no se puede poner la versión nueva: %w", err)
	}
	return nil
}

// Backend: the server program of the installed app.
func (a *Actualizador) Backend() string {
	name := "pgocode-backend"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(a.DirApp, name)
}
