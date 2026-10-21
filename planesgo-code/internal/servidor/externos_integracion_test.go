//go:build integracion

// Pruebas con procesos reales: necesitan ~/cowork y python3 (make integracion).
package servidor

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"planesgo-code/internal/proceso"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
)

// puertoLibre pide al sistema un puerto libre (en vez de uno fijo que puede estar ocupado).
func puertoLibre(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	return p
}

// espera repite cond hasta que se cumple o pasa max.
func espera(max time.Duration, cond func() bool) bool {
	for fin := time.Now().Add(max); ; time.Sleep(100 * time.Millisecond) {
		if cond() {
			return true
		}
		if time.Now().After(fin) {
			return false
		}
	}
}

func cowork(t *testing.T) string {
	casa, _ := os.UserHomeDir()
	raiz := filepath.Join(casa, "cowork")
	if _, err := os.Stat(raiz); err != nil {
		t.Skip("sin ~/cowork")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("sin python3")
	}
	return raiz
}

// Un servidor de una carpeta de trabajo sale entre los externos y se para con su grupo.
func TestExternosReal(t *testing.T) {
	raiz := cowork(t)
	d, err := os.MkdirTemp(raiz, ".pgocode-prueba-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(d)
	cmd := exec.Command("python3", "-m", "http.server", puertoLibre(t), "--bind", "127.0.0.1")
	cmd.Dir = d
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer cmd.Process.Kill()
	s := &Servidor{Indice: &proyectos.Indice{Raices: []string{raiz}, Registro: filepath.Join(d, "registro.json")}}
	var grupo int
	espera(5*time.Second, func() bool {
		_, otros := s.externos(1)
		for _, x := range otros {
			if x.Carpeta == d {
				grupo = x.Grupo
			}
		}
		return grupo != 0
	})
	if grupo != cmd.Process.Pid {
		t.Fatalf("no aparece (grupo %d, pid %d)", grupo, cmd.Process.Pid)
	}
	proceso.SenalGrupo(grupo, false)
	cmd.Wait()
	if proceso.GrupoVivo(grupo) {
		t.Error("sigue vivo")
	}
}

// Un programa de pgocode que arranca un servidor en otro grupo (como «odt run» con Odoo): el servidor no sale
// como externo, su puerto pasa a ser la dirección del programa, y parar el programa lo para también.
func TestProgramaConHijoEnOtroGrupo(t *testing.T) {
	raiz := cowork(t)
	d, err := os.MkdirTemp(raiz, ".pgocode-prueba-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(d)
	puerto := puertoLibre(t)
	s := &Servidor{Indice: &proyectos.Indice{Raices: []string{raiz}, Registro: filepath.Join(d, "registro.json")}, Procesos: procesos.Nuevo()}
	if _, err := s.Procesos.Lanza("web", 1, d, "setsid python3 -m http.server "+puerto+" --bind 127.0.0.1 & sleep 60", "usuario", nil); err != nil {
		t.Fatal(err)
	}
	var url string
	espera(5*time.Second, func() bool {
		_, otros := s.externos(1)
		for _, x := range otros {
			if x.Carpeta == d {
				t.Fatalf("sale como externo: %+v", x)
			}
		}
		p, _ := s.Procesos.Busca("web")
		url = p.URL
		return url != ""
	})
	if want := "http://localhost:" + puerto + "/"; url != want {
		t.Fatalf("dirección: %q, quiero %q", url, want)
	}
	s.Procesos.Para("web")
	// El servidor del otro grupo tiene que dejar de escuchar (con un máximo, sin un tiempo fijo).
	sigue := func() bool {
		for _, x := range proceso.Escuchando() {
			if x.Carpeta == d {
				return true
			}
		}
		return false
	}
	if !espera(15*time.Second, func() bool { return !sigue() }) {
		for _, x := range proceso.Escuchando() {
			if x.Carpeta == d {
				t.Errorf("el servidor sigue vivo: %+v (puerto %s)", x, puerto)
			}
		}
	}
}
