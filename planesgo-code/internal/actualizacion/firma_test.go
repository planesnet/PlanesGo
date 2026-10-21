package actualizacion

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// La clave pública que lleva la app es válida.
func TestClavePublicaIncluida(t *testing.T) {
	if len(ClavePublica()) != ed25519.PublicKeySize {
		t.Fatal("clave pública mal formada")
	}
}

func paqueteDePrueba(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	cuerpo := []byte("#!/bin/sh\n")
	tw.WriteHeader(&tar.Header{Name: "pgocode-backend", Mode: 0o755, Size: int64(len(cuerpo)), Typeflag: tar.TypeReg})
	tw.Write(cuerpo)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// servidor publica el paquete y un version.json al que firma le pone la firma (nil: sin firma).
func servidor(t *testing.T, firma func(Publicada) string) (*httptest.Server, Publicada) {
	t.Helper()
	paq := paqueteDePrueba(t)
	h := sha256.Sum256(paq)
	p := Publicada{Version: "9.9.9", SHA256: hex.EncodeToString(h[:]), Bytes: int64(len(paq)), Archivo: "p.tar.gz", Electron: "1.0.0"}
	if firma != nil {
		p.Firma = firma(p)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			json.NewEncoder(w).Encode(p)
		case "/p.tar.gz":
			w.Write(paq)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, p
}

// actualizador instalado (con paquete.sha256) que comprueba con pub.
func actualizador(t *testing.T, url string, pub ed25519.PublicKey) *Actualizador {
	t.Helper()
	app := t.TempDir()
	os.WriteFile(filepath.Join(app, "paquete.sha256"), []byte(strings.Repeat("0", 64)+"\n"), 0o644)
	a := Nuevo(url, app, t.TempDir(), "0.0.1")
	a.ClavePub = pub
	if e := a.Estado(); e.Estado != "al-dia" {
		t.Fatalf("estado inicial %q (%s)", e.Estado, e.Error)
	}
	return a
}

func TestFirma(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, otra, _ := ed25519.GenerateKey(rand.Reader)
	firmaCon := func(k ed25519.PrivateKey) func(Publicada) string {
		return func(p Publicada) string { return base64.StdEncoding.EncodeToString(ed25519.Sign(k, MensajeFirmado(p))) }
	}
	casos := []struct {
		nombre string
		firma  func(Publicada) string
		error  string // "" si se acepta
	}{
		{"buena", firmaCon(priv), ""},
		{"de otra clave", firmaCon(otra), "no es válida"},
		{"de otra versión", func(p Publicada) string { p.Version = "1.0.0"; return firmaCon(priv)(p) }, "no es válida"},
		{"basura", func(Publicada) string { return "no-es-base64!" }, "no es válida"},
		{"ausente", nil, "sin firma"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv, p := servidor(t, c.firma)
			a := actualizador(t, srv.URL, pub)
			a.revisa(context.Background())
			e := a.Estado()
			_, errPrep := os.Stat(filepath.Join(a.DirPreparado, "nuevo", "pgocode-backend"))
			if c.error == "" {
				if e.Estado != "pendiente" || e.Nueva != p.Version || errPrep != nil {
					t.Fatalf("firma buena rechazada: %+v", e)
				}
				return
			}
			if e.Estado == "pendiente" || errPrep == nil {
				t.Fatalf("se ha preparado un paquete con firma %s: %+v", c.nombre, e)
			}
			if !strings.Contains(e.Error, c.error) {
				t.Fatalf("error %q, se esperaba %q", e.Error, c.error)
			}
		})
	}
}

func TestURLSegura(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://planesgo.autopyme.com/install/pgocode": true,
		"http://127.0.0.1:8080":                         true,
		"http://localhost:8080/x":                       true,
		"http://[::1]:8080":                             true,
		"http://planesgo.autopyme.com/install/pgocode":  false,
		"http://127.0.0.1.example.com":                  false,
		"ftp://localhost/x":                             false,
		"planesgo.autopyme.com":                         false,
	} {
		if err := URLSegura(u); (err == nil) != ok {
			t.Errorf("URLSegura(%q) = %v", u, err)
		}
	}
}

// Con un servidor http que no es local, el actualizador no se pone en marcha.
func TestActualizadorRechazaHTTP(t *testing.T) {
	app := t.TempDir()
	os.WriteFile(filepath.Join(app, "paquete.sha256"), []byte(strings.Repeat("0", 64)+"\n"), 0o644)
	e := Nuevo("http://planesgo.autopyme.com/install/pgocode", app, t.TempDir(), "0.0.1").Estado()
	if e.Estado != "desactivada" || !strings.Contains(e.Error, "https") {
		t.Fatalf("estado %+v", e)
	}
}

// Una redirección a http (no local) se rechaza.
func TestRedireccionAHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://planesgo.example.com/version.json", http.StatusFound)
	}))
	defer srv.Close()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	a := actualizador(t, srv.URL, pub)
	if _, err := a.publicada(context.Background()); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("redirección a http aceptada: %v", err)
	}
}
