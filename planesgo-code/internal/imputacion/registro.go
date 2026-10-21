package imputacion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Registro de tiempos: auditoría detallada de todo lo que interviene en la imputación, para validar que el
// sistema imputa bien y encontrar los fallos. Un fichero JSONL por día (<Dir>/AAAA-MM-DD.jsonl), una línea por
// suceso: actividad humana, turnos del agente, sus herramientas (subagentes, tareas en segundo plano), estados,
// periodos que se abren y se cierran, lo que se suma a cada línea del día y cada escritura en Odoo.
// Valida (validacion.go) lo vuelve a calcular y avisa de lo que no cuadra.

// IntervaloHumano: como mucho una línea de actividad humana por sesión cada tanto (la actividad llega a menudo).
const IntervaloHumano = 30 * time.Second

type Registro struct {
	Dir   string
	Ahora func() time.Time
	// Version del programa: va en cada suceso (con el pid) para saber qué versión hizo cada cosa.
	Version string
	// Dias: cuántos días de registros se guardan (0: 90).
	Dias int

	mu     sync.Mutex
	humano map[string]time.Time // última actividad humana anotada por sesión

	limpiado string // día en que se borraron los registros viejos
}

// limpia (con el cerrojo): borra los registros de hace más de Dias días.
func (r *Registro) limpia(ahora time.Time) {
	dias := r.Dias
	if dias <= 0 {
		dias = 90
	}
	limite := ahora.AddDate(0, 0, -dias).Format("2006-01-02")
	l, _ := filepath.Glob(filepath.Join(r.Dir, "????-??-??.jsonl"))
	for _, f := range l {
		if strings.TrimSuffix(filepath.Base(f), ".jsonl") < limite {
			os.Remove(f)
		}
	}
}

func (r *Registro) ahora() time.Time {
	if r.Ahora != nil {
		return r.Ahora()
	}
	return time.Now()
}

// Fichero: el registro de un día.
func (r *Registro) Fichero(dia string) string { return filepath.Join(r.Dir, dia+".jsonl") }

// Anota un suceso. campos lleva lo propio de cada uno; «t» y «ev» los pone el registro.
func (r *Registro) Anota(ev string, campos map[string]any) {
	if r == nil || r.Dir == "" {
		return
	}
	ahora := r.ahora()
	l := map[string]any{"t": ahora.Format(time.RFC3339Nano), "ev": ev, "pid": os.Getpid()}
	if r.Version != "" {
		l["v"] = r.Version
	}
	for k, v := range campos {
		if v != nil && v != "" {
			l[k] = v
		}
	}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return
	}
	if dia := ahora.Format("2006-01-02"); dia != r.limpiado {
		r.limpiado = dia
		r.limpia(ahora)
	}
	if f, err := os.OpenFile(r.Fichero(ahora.Format("2006-01-02")), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.Write(append(b, '\n'))
		f.Close()
	}
}

// humanoToca: si toca anotar la actividad humana de la sesión (y desde cuándo no se anotaba).
func (r *Registro) humanoToca(sesion string) (bool, time.Duration) {
	if r == nil {
		return false, 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.humano == nil {
		r.humano = map[string]time.Time{}
	}
	ahora := r.ahora()
	antes := r.humano[sesion]
	if !antes.IsZero() && ahora.Sub(antes) < IntervaloHumano {
		return false, 0
	}
	r.humano[sesion] = ahora
	if antes.IsZero() {
		return true, 0
	}
	return true, ahora.Sub(antes)
}

// TipoHerramienta clasifica una herramienta de un agente: subagente, tarea en segundo plano o herramienta.
func TipoHerramienta(nombre, entrada string) string {
	switch nombre {
	case "Agent", "Task", "task", "invoke_subagent", "browser_subagent":
		return "subagente"
	case "Monitor", "manage_task", "command_status":
		return "segundo_plano"
	}
	e := strings.ReplaceAll(entrada, " ", "")
	if strings.Contains(e, `"run_in_background":true`) || strings.Contains(e, `"background":true`) || strings.Contains(e, `"WaitMsBeforeAsync"`) {
		return "segundo_plano"
	}
	return "herramienta"
}

// ---- anotaciones del gestor ----

// anota un suceso de un periodo (con sus datos de sesión, proyecto, tarea y agente).
func (g *Gestor) anota(ev string, p *Periodo, campos map[string]any) {
	if g.Tiempos == nil {
		return
	}
	if campos == nil {
		campos = map[string]any{}
	}
	if p != nil {
		campos["sesion"] = p.Sesion
		campos["proyecto"] = p.T.Proyecto
		campos["tarea_id"] = p.TareaID
		campos["tarea"] = tareaDe(p.T)
		campos["agente"] = agenteDe(p.T.Modelo)
		if p.T.UID > 0 {
			campos["uid"] = p.T.UID
		}
	}
	g.Tiempos.Anota(ev, campos)
}

// AnotaAgente: lo que hace el agente de una sesión durante su turno (herramientas, subagentes, tareas en segundo
// plano, consumo, procesos), desde quien lee su salida.
func (g *Gestor) AnotaAgente(sesion, ev string, campos map[string]any) {
	if g.Tiempos == nil {
		return
	}
	if campos == nil {
		campos = map[string]any{}
	}
	campos["sesion"] = sesion
	g.mu.Lock()
	if p := g.periodos[sesion]; p != nil {
		campos["proyecto"] = p.T.Proyecto
		campos["tarea_id"] = p.TareaID
		campos["agente"] = agenteDe(p.T.Modelo)
	}
	g.mu.Unlock()
	g.Tiempos.Anota(ev, campos)
}

func seg(d time.Duration) float64 { return float64(d.Milliseconds()) / 1000 }
