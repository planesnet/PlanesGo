package studiotv

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	jpg "image/jpeg"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func jpegReal(t *testing.T, c uint8) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for x := 0; x < 64; x++ {
		for y := 0; y < 48; y++ {
			im.Set(x, y, color.RGBA{c, 100, 150, 255})
		}
	}
	var b bytes.Buffer
	if err := jpg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Por defecto no se graba: sin GrabarEn, pedir grabar da 501.
func TestSinGrabacionPorDefecto(t *testing.T) {
	_, srv := nuevoServidor(t)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{}`))
	if r := peticion(t, "POST", srv.URL+"/api/grabar?canal=a", tokVer, "", []byte(`{"grabar":true}`)); r.StatusCode != 501 {
		t.Errorf("grabar sin directorio: %d, quiero 501", r.StatusCode)
	}
}

// Grabar, emitir con pasos, parar: queda un MP4 con sus marcadores, se lista y se descarga.
func TestGrabacion(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("sin ffmpeg")
	}
	s, srv := nuevoServidor(t)
	dir := t.TempDir()
	if err := s.GrabarEn(dir); err != nil {
		t.Fatal(err)
	}
	if r := peticion(t, "POST", srv.URL+"/api/grabar?canal=nadie", tokVer, "", []byte(`{"grabar":true}`)); r.StatusCode != 404 {
		t.Errorf("grabar un canal inexistente: %d", r.StatusCode)
	}
	if r := peticion(t, "POST", srv.URL+"/api/grabar?canal=a", tokEmitir, "", []byte(`{"grabar":true}`)); r.StatusCode != 401 {
		t.Errorf("grabar con el token de emitir: %d, quiero 401", r.StatusCode)
	}
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{"paso":"uno"}`))
	peticion(t, "POST", srv.URL+"/api/frame?canal=a", tokEmitir, "", jpegReal(t, 10))
	if r := peticion(t, "POST", srv.URL+"/api/grabar?canal=a", tokVer, "", []byte(`{"grabar":true}`)); r.StatusCode != 200 {
		t.Fatalf("empezar a grabar: %d", r.StatusCode)
	}
	if l := s.Canales(); len(l) != 1 || !l[0].Grabando {
		t.Errorf("el canal debería salir como grabando: %+v", l)
	}
	time.Sleep(300 * time.Millisecond)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{"paso":"dos"}`))
	peticion(t, "POST", srv.URL+"/api/frame?canal=a", tokEmitir, "", jpegReal(t, 200))
	time.Sleep(300 * time.Millisecond)
	r := peticion(t, "POST", srv.URL+"/api/grabar?canal=a", tokVer, "", []byte(`{"grabar":false}`))
	var res struct {
		Grabacion Grabacion `json:"grabacion"`
	}
	if r.StatusCode != 200 || json.NewDecoder(r.Body).Decode(&res) != nil {
		t.Fatalf("parar: %d", r.StatusCode)
	}
	g := res.Grabacion
	if g.Bytes == 0 || g.Dur < 0.4 || len(g.Marcadores) != 2 || g.Marcadores[0].Txt != "uno" || g.Marcadores[1].Txt != "dos" {
		t.Fatalf("grabación: %+v", g)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "a", g.ID+".mp4")); err != nil {
		t.Fatal(err)
	}
	r = peticion(t, "GET", srv.URL+"/api/grabaciones?canal=a", tokVer, "", nil)
	var l []Grabacion
	if json.NewDecoder(r.Body).Decode(&l) != nil || len(l) != 1 || l[0].ID != g.ID {
		t.Fatalf("lista: %+v", l)
	}
	r = peticion(t, "GET", srv.URL+"/grabacion?canal=a&id="+g.ID, tokVer, "", nil)
	if b, _ := io.ReadAll(r.Body); r.StatusCode != 200 || r.Header.Get("Content-Type") != "video/mp4" || int64(len(b)) != g.Bytes {
		t.Errorf("descarga: %d %q %d bytes", r.StatusCode, r.Header.Get("Content-Type"), len(b))
	}
	if r := peticion(t, "GET", srv.URL+"/grabacion?canal=a&id=../../etc", tokVer, "", nil); r.StatusCode != 404 {
		t.Errorf("id con ruta: %d", r.StatusCode)
	}
	if r := peticion(t, "POST", srv.URL+"/api/grabar?canal=a", tokVer, "", []byte(`{"grabar":false}`)); r.StatusCode != 409 {
		t.Errorf("parar sin grabar: %d", r.StatusCode)
	}
}
