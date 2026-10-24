package actualizacion

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// failedFile: in DirPreparado, the package (its sha256) that could not be put in place and why. revisa does not
// prepare it again, so a failed swap does not become a loop of restarts (the server restarts after each attempt).
const failedFile = "fallida"

// shaFile: in an installed or prepared version, the sha256 of the package it came from.
const shaFile = "paquete.sha256"

// readSHA: the package sha256 of the version in dir ("" if unknown).
func readSHA(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, shaFile))
	return strings.TrimSpace(string(b))
}

// Apply puts the prepared version (<DirPreparado>/nuevo) in place of the installed app (DirApp), keeping the
// previous one in <DirApp>.anterior and going back to it if the swap fails. The server calls it once it has shut
// down, and then starts the new version (it no longer needs the desktop app for this: the server lives on its own).
// If the swap fails, the prepared version is discarded and remembered as failed: it is not tried again.
func (a *Actualizador) Apply() error {
	nuevo := filepath.Join(a.DirPreparado, "nuevo")
	backend := filepath.Base(a.Backend())
	if _, err := os.Stat(filepath.Join(a.DirApp, shaFile)); err != nil {
		return errors.New("la app no está instalada desde un paquete")
	}
	if _, err := os.Stat(filepath.Join(nuevo, backend)); err != nil {
		return errors.New("no hay ninguna versión preparada")
	}
	err := a.swap(nuevo)
	if err != nil {
		os.WriteFile(filepath.Join(a.DirPreparado, failedFile), []byte(readSHA(nuevo)+"\n"+err.Error()+"\n"), 0o600)
		os.RemoveAll(nuevo)
	}
	return err
}

func (a *Actualizador) swap(nuevo string) error {
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

// failed: why the package with that sha256 could not be installed before ("" if it was not tried, or it worked).
func (a *Actualizador) failed(sha string) string {
	b, err := os.ReadFile(filepath.Join(a.DirPreparado, failedFile))
	if err != nil {
		return ""
	}
	got, why, _ := strings.Cut(string(b), "\n")
	if got != sha {
		return ""
	}
	return strings.TrimSpace(why)
}

// Backend: the server program of the installed app.
func (a *Actualizador) Backend() string {
	name := "pgocode-backend"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(a.DirApp, name)
}
