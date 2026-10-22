package actualizacion

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Con algo en marcha no se actualiza ni aunque el usuario lo pida (pulsando el aviso); al acabar, sí.
func TestNoSeActualizaTrabajando(t *testing.T) {
	a := Nuevo("https://127.0.0.1:1/pgocode", t.TempDir(), t.TempDir(), "0.0.1")
	var trabajando, aplicada atomic.Bool
	trabajando.Store(true)
	a.Trabajando = trabajando.Load
	a.AlAplicar = func() { aplicada.Store(true) }
	a.pon(func(e *Estado) { e.Estado, e.Nueva = "pendiente", "0.0.2" })
	ctx, cancela := context.WithCancel(context.Background())
	defer cancela()
	go a.Funciona(ctx)
	if !a.AplicarYa() {
		t.Fatal("AplicarYa con una versión pendiente")
	}
	time.Sleep(200 * time.Millisecond)
	if aplicada.Load() {
		t.Fatal("se ha actualizado con un agente trabajando")
	}
	trabajando.Store(false)
	a.AplicarYa()
	for i := 0; i < 100 && !aplicada.Load(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !aplicada.Load() {
		t.Fatal("sin nada en marcha, pedirla la aplica")
	}
}
