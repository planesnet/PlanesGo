package selector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"planesgo-code/internal/skills"
)

// Con un embebedor de mentira: las skills se ordenan por parecido y los vectores se guardan en disco.
func TestEligeYGuarda(t *testing.T) {
	var llamadas atomic.Int32 // el selector embebe las skills en paralelo
	falso := func(_ context.Context, texto, _ string) ([]float32, error) {
		llamadas.Add(1)
		v := make([]float32, 3)
		if strings.Contains(texto, "saludo") {
			v[0] = 1
		}
		if strings.Contains(texto, "odoo") {
			v[1] = 1
		}
		v[2] = 0.1
		return v, nil
	}
	f := filepath.Join(t.TempDir(), "v.json")
	l := []skills.Skill{{Nombre: "saludo", Descripcion: "saludo formal", Ruta: "/a"}, {Nombre: "odoo", Descripcion: "odoo dev", Ruta: "/b"}}
	s := &Selector{Fichero: f, Embebe: falso}
	c, _, err := s.Elige(context.Background(), "un saludo", l)
	if err != nil || c[0].Nombre != "saludo" || llamadas.Load() != 3 {
		t.Fatalf("%v %v %d", c, err, llamadas.Load())
	}
	s2 := &Selector{Fichero: f, Embebe: falso}
	s2.Elige(context.Background(), "odoo", l)
	if llamadas.Load() != 4 { // solo la petición: los de las skills ya estaban guardados
		t.Errorf("llamadas %d", llamadas.Load())
	}
}

// Con Vertex AI de verdad: PGO_PRUEBA_SKILLS=<carpeta> go test -v -run Real ./internal/selector
func TestRankingReal(t *testing.T) {
	c := os.Getenv("PGO_PRUEBA_SKILLS")
	if c == "" {
		t.Skip()
	}
	var l []skills.Skill
	for _, k := range skills.Inventario(c) {
		if !k.Rota {
			l = append(l, k)
		}
	}
	s := &Selector{Fichero: filepath.Join(t.TempDir(), "v.json"), SoloVertex: os.Getenv("PGO_PRUEBA_VERTEX") != ""}
	t.Logf("motor: %s", s.MotorActual(context.Background()).Nombre)
	ini := time.Now()
	if err := s.Prepara(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d skills preparadas en %s", len(l), time.Since(ini).Round(time.Millisecond))
	for _, q := range []string{
		"lanza la prueba de la madera",
		"crea un módulo de Odoo 18 con un modelo y su vista",
		"imputa las horas de hoy en PlanesGo",
		"revisa la concurrencia de este código Go",
		"hazme un documento de Word con el informe",
		"¿qué hora es?",
		"arregla el CSS del botón de la página",
		"redacta un saludo formal para un cliente",
		"configura el plan contable de la empresa en Odoo",
		"cuéntame un chiste",
		"sube los cambios a git",
	} {
		ini := time.Now()
		r, _, err := s.Elige(context.Background(), q, l)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-55q %4dms → %s %.3f | %s %.3f | %s %.3f", q, time.Since(ini).Milliseconds(), r[0].Nombre, r[0].Puntuacion, r[1].Nombre, r[1].Puntuacion, r[2].Nombre, r[2].Puntuacion)
	}
}
