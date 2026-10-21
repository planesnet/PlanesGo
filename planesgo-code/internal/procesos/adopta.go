package procesos

import (
	"bufio"
	"context"
	"errors"
	"os"
	"time"

	"planesgo-code/internal/proceso"
)

// Adopta un programa que ya está en marcha (p. ej. el que el agente dejó en segundo plano: el programa que se
// está desarrollando) para que siga como cualquier otro de pgocode, sin el agente: su salida se lee del fichero
// donde escribe y se puede parar. Así el turno del agente acaba (no hay que esperarlo) y no gasta más tokens.
func (g *Gestor) Adopta(nombre string, proyecto int, carpeta, orden string, grupo int, fichero string) (*Proceso, error) {
	if !nombreValido.MatchString(nombre) {
		return nil, ErrNombre
	}
	if grupo <= 1 || !proceso.GrupoVivo(grupo) {
		return nil, errors.New("ese programa ya no está en marcha")
	}
	g.Para(nombre)
	ctx, cancela := context.WithCancel(context.Background())
	p := &Proceso{Nombre: nombre, Proyecto: proyecto, Carpeta: carpeta, Orden: orden, Estado: "curso", Inicio: time.Now(),
		Lanzado: "agente", cancela: cancela, hecho: make(chan struct{})}
	g.mu.Lock()
	g.ps[nombre] = p
	g.mu.Unlock()
	g.avisa()
	if fichero != "" {
		go g.sigue(ctx, p, fichero)
	}
	go func() {
		parado := false
		for proceso.GrupoVivo(grupo) {
			select {
			case <-ctx.Done():
				if !parado {
					parado = true
					proceso.SenalGrupo(grupo, false) // que termine; si no, a la fuerza
					go func() { time.Sleep(5 * time.Second); proceso.SenalGrupo(grupo, true) }()
				}
				time.Sleep(200 * time.Millisecond)
			case <-time.After(time.Second):
			}
		}
		g.mu.Lock()
		p.Fin = time.Now()
		if parado {
			p.Estado, p.Codigo = "parado", -1
		} else {
			p.Estado = "terminado"
		}
		g.mu.Unlock()
		cancela()
		close(p.hecho)
		g.avisa()
	}()
	return p, nil
}

// sigue va leyendo lo que el programa escribe en su fichero de salida (como «tail -f»).
func (g *Gestor) sigue(ctx context.Context, p *Proceso, fichero string) {
	var f *os.File
	for f == nil {
		if x, err := os.Open(fichero); err == nil {
			f = x
		} else {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
	defer f.Close()
	r := bufio.NewReader(f)
	pendiente := ""
	for {
		l, err := r.ReadString('\n')
		pendiente += l
		if err == nil {
			g.linea(p, pendiente[:len(pendiente)-1])
			pendiente = ""
			continue
		}
		select {
		case <-ctx.Done():
			if pendiente != "" {
				g.linea(p, pendiente)
			}
			return
		case <-p.hecho:
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (g *Gestor) linea(p *Proceso, l string) {
	g.mu.Lock()
	p.lineas = append(p.lineas, l)
	p.total++
	if len(p.lineas) > maxLineas {
		p.lineas = p.lineas[len(p.lineas)-maxLineas:]
	}
	nueva := false
	if p.URL == "" {
		if u := urlDe(l); u != "" {
			p.URL, nueva = u, true
		}
	}
	g.mu.Unlock()
	if nueva {
		g.avisa()
	}
}
