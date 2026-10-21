package servidor

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"planesgo-code/internal/proceso"
	"planesgo-code/internal/proyectos"
)

func TestDentro(t *testing.T) {
	c := []string{"/home/luis/cowork/intradema"}
	if !dentro("/home/luis/cowork/intradema/web", c) || !dentro("/home/luis/cowork/intradema", c) || dentro("/home/luis/cowork/intradema2", c) {
		t.Error("dentro")
	}
	if !ordenEn("python /home/luis/cowork/intradema/app.py", c) || ordenEn("python /home/luis/cowork/intradema2/app.py", c) {
		t.Error("ordenEn")
	}
}

// Un servidor de una carpeta de trabajo sale entre los externos y se para con su grupo.
func TestExternosReal(t *testing.T) {
	casa, _ := os.UserHomeDir()
	raiz := filepath.Join(casa, "cowork")
	if _, err := os.Stat(raiz); err != nil {
		t.Skip("sin ~/cowork")
	}
	d, err := os.MkdirTemp(raiz, ".pgocode-prueba-")
	if err != nil {
		t.Skip(err)
	}
	defer os.RemoveAll(d)
	cmd := exec.Command("python3", "-m", "http.server", "8979", "--bind", "127.0.0.1")
	cmd.Dir = d
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer cmd.Process.Kill()
	s := &Servidor{Indice: &proyectos.Indice{Raices: []string{raiz}, Registro: filepath.Join(d, "registro.json")}}
	var grupo int
	for i := 0; i < 30 && grupo == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		_, otros := s.externos(1)
		for _, x := range otros {
			if x.Carpeta == d {
				grupo = x.Grupo
			}
		}
	}
	if grupo != cmd.Process.Pid {
		t.Fatalf("no aparece (grupo %d, pid %d)", grupo, cmd.Process.Pid)
	}
	proceso.SenalGrupo(grupo, false)
	cmd.Wait()
	if proceso.GrupoVivo(grupo) {
		t.Error("sigue vivo")
	}
}
