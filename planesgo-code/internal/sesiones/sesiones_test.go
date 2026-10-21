package sesiones

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// almacenConReloj: un almacén con la hora controlada por la prueba.
func almacenConReloj(ahora *time.Time) *Almacen {
	a := NuevoAlmacen()
	a.ahora = func() time.Time { return *ahora }
	return a
}

// Una sesión sin uso caduca a las Duracion; usarla renueva el plazo.
func TestCaducidad(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	a := almacenConReloj(&ahora)
	s, err := a.Crea(2, "Prueba", "prueba", "odoo")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Token) != 64 {
		t.Errorf("token de %d caracteres", len(s.Token))
	}
	ahora = ahora.Add(Duracion - time.Minute)
	if _, ok := a.Busca(s.Token); !ok {
		t.Fatal("caducada antes de tiempo")
	}
	ahora = ahora.Add(Duracion - time.Minute) // renovada en la búsqueda anterior: sigue viva
	if _, ok := a.Busca(s.Token); !ok {
		t.Fatal("la búsqueda no renovó la sesión")
	}
	ahora = ahora.Add(Duracion + time.Second)
	if _, ok := a.Busca(s.Token); ok {
		t.Fatal("debería haber caducado")
	}
	if _, ok := a.Busca(s.Token); ok {
		t.Fatal("una sesión caducada no vuelve")
	}
	if _, ok := a.Busca("no-existe"); ok {
		t.Fatal("token inventado aceptado")
	}
}

// Al crear una sesión se purgan las caducadas; Borra cierra la sesión.
func TestPurgaYBorra(t *testing.T) {
	ahora := time.Now()
	a := almacenConReloj(&ahora)
	vieja, _ := a.Crea(1, "A", "a", "")
	ahora = ahora.Add(Duracion + time.Second)
	nueva, _ := a.Crea(2, "B", "b", "")
	a.mu.Lock()
	_, sigue := a.sesiones[vieja.Token]
	a.mu.Unlock()
	if sigue {
		t.Error("la sesión caducada no se purgó")
	}
	a.Borra(nueva.Token)
	if _, ok := a.Busca(nueva.Token); ok {
		t.Error("sesión borrada sigue viva")
	}
}

// Comprobación de proyectos con vigencia y conexiones de Studio TV.
func TestProyectosYStudio(t *testing.T) {
	a := NuevoAlmacen()
	s, _ := a.Crea(2, "P", "p", "")
	ahora := time.Now()
	s.MarcaProyecto(7, ahora.Add(time.Minute))
	if !s.ProyectoComprobado(7, ahora) || s.ProyectoComprobado(7, ahora.Add(2*time.Minute)) || s.ProyectoComprobado(8, ahora) {
		t.Error("vigencia de la comprobación de proyectos")
	}
	s.PonStudio(7, ConexionStudio{URL: "http://127.0.0.1:7802", Token: "ver"})
	if c, ok := s.Studio(7); !ok || c.Token != "ver" {
		t.Errorf("studio: %+v %v", c, ok)
	}
	s.PonStudio(7, ConexionStudio{})
	if _, ok := s.Studio(7); ok {
		t.Error("la URL vacía debería borrar la conexión")
	}
}

// Guarda escribe un fichero 0600 y Carga recupera las sesiones (con su Studio TV) y borra el fichero.
func TestGuardaYCarga(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sesiones.json")
	a := NuevoAlmacen()
	s, _ := a.Crea(2, "Prueba", "prueba", "odoo-123")
	s.PonStudio(5, ConexionStudio{URL: "http://127.0.0.1:7802", Token: "secreto", Local: true})
	if err := a.Guarda(f); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if p := st.Mode().Perm(); p != 0o600 {
		t.Errorf("permisos %o, quiero 600", p)
	}
	if _, err := os.Stat(f + ".tmp"); err == nil {
		t.Error("queda el temporal")
	}
	b := NuevoAlmacen()
	n, err := b.Carga(f, time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("carga: %d %v", n, err)
	}
	r, ok := b.Busca(s.Token)
	if !ok || r.UID != 2 || r.Login != "prueba" || r.OdooSesion != "odoo-123" {
		t.Fatalf("sesión recuperada: %+v %v", r, ok)
	}
	if c, ok := r.Studio(5); !ok || c.Token != "secreto" || !c.Local {
		t.Errorf("studio recuperado: %+v", c)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("Carga debe borrar el fichero")
	}
}

// Un fichero viejo no se carga (pero se borra); sin fichero no hay error.
func TestCargaVieja(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sesiones.json")
	a := NuevoAlmacen()
	a.Crea(2, "P", "p", "")
	if err := a.Guarda(f); err != nil {
		t.Fatal(err)
	}
	viejo := time.Now().Add(-time.Hour)
	os.Chtimes(f, viejo, viejo)
	b := NuevoAlmacen()
	if n, err := b.Carga(f, 5*time.Minute); n != 0 || err != nil {
		t.Errorf("fichero viejo: %d %v", n, err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("el fichero viejo debe borrarse")
	}
	if n, err := b.Carga(f, time.Minute); n != 0 || err != nil {
		t.Errorf("sin fichero: %d %v", n, err)
	}
	os.WriteFile(f, []byte("no es json"), 0o600)
	if _, err := b.Carga(f, time.Minute); err == nil {
		t.Error("fichero corrupto sin error")
	}
}
