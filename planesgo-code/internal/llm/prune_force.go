package llm

import (
	"context"
	"errors"
	"time"

	"planesgo-code/internal/depuracion"
)

// «Barrer ahora» (la ventanita del hilo de limpieza): el usuario pide retirar ya lo que está fuera de vigencia (3 o
// más), sin el mínimo de tokens ni la cuenta de la caché. En el hilo del recolector de la sesión, solo con cálculo
// local (embeddings de Ollama):
//   - OpenCode: se aplica ya, también con un turno en marcha (como el barrido entre pasos: el turno en curso solo
//     pierde piezas con su propia vigencia de 3 o más, nunca las de sus últimos pasos);
//   - Claude Code: solo si ningún proceso tiene la sesión abierta; si no, queda pendiente para el próximo turno;
//   - Antigravity: no deja tocar su historial: el relevo por vigencia se fuerza en el próximo envío (lo hace la
//     interfaz).

// ForceSweep barre la sesión del agente ahora. folder: la carpeta de la sesión; msg: el último mensaje del usuario
// (la relación se mide frente a él); running: hay un turno en marcha.
func ForceSweep(ctx context.Context, agent, ses, folder, msg string, running bool) (*PruneReport, error) {
	if !sesionValida.MatchString(ses) {
		return nil, errors.New("sesión no válida")
	}
	if folder == "" {
		return nil, errors.New("la sesión no tiene carpeta")
	}
	p := Peticion{Carpeta: folder, SesionAgente: ses, Prune: PruneAuto, Mensajes: []Mensaje{{Rol: "user", Texto: msg}}}
	c := collectorFor(ses)
	var rep *PruneReport
	var err error
	ce, cancela := context.WithTimeout(ctx, 90*time.Second)
	defer cancela()
	switch agent {
	case "opencode":
		if !idSesionOC.MatchString(ses) {
			return nil, errors.New("sesión de OpenCode no válida")
		}
		var a *Agente
		for _, x := range Agentes() {
			if ag, ok := x.(*Agente); ok && ag.Clave == "opencode" {
				a = ag
			}
		}
		if a == nil {
			return nil, errors.New("OpenCode no está disponible")
		}
		young := pruneYoung
		if running {
			young++ // el turno en curso ya está en su historial
		}
		e := c.do(ce, func() {
			defer c.setPhase("", 0)
			c.setPhase("barriendo", 0)
			if rep, err = c.sweep(ce, a, p, msg, young, nil, true); err == nil {
				c.publish(rep)
			}
		})
		if e != nil {
			return nil, e
		}
	case "claude-code":
		e := c.do(ce, func() {
			defer c.setPhase("", 0)
			c.setPhase("barriendo", 0)
			if rep, _, err = c.sweepClaude(ce, p, msg, true); err == nil {
				c.publish(rep)
			}
		})
		if e != nil {
			return nil, e
		}
	default:
		return nil, errors.New("este agente no deja podar su historial: se releva en el próximo envío")
	}
	if err != nil {
		depuracion.Log(agent, "barrido pedido %s: %v", ses, err)
		return nil, err
	}
	st := c.status()
	rep.Status = &st
	return rep, nil
}
