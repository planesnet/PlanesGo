package imputacion

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"time"
)

// Validación del registro de tiempos: vuelve a calcular el día a partir de los sucesos y avisa de lo que no cuadra
// (periodos descuadrados, tiempo humano contado en dos sesiones a la vez, acumulados que saltan, horas en Odoo
// distintas de las calculadas, errores, turnos sin cerrar, procesos que siguen vivos, consumo sin periodo…).

type Aviso struct {
	T        string `json:"t,omitempty"`
	Gravedad string `json:"gravedad"` // error | aviso | info
	Tipo     string `json:"tipo"`
	Sesion   string `json:"sesion,omitempty"`
	Texto    string `json:"texto"`
}

// LineaDia: lo calculado para una línea del día (fecha, proyecto, tarea, clase y agente) y lo último escrito.
type LineaDia struct {
	Fecha      string  `json:"fecha"`
	Proyecto   int     `json:"proyecto"`
	TareaID    int     `json:"tarea_id"`
	Clase      string  `json:"clase"` // h | m
	Agente     string  `json:"agente,omitempty"`
	Periodos   int     `json:"periodos"`
	Segundos   float64 `json:"segundos"` // suma de lo sumado por los periodos cerrados
	Calculadas float64 `json:"horas_calculadas"`
	Odoo       float64 `json:"horas_odoo"` // lo último escrito en Odoo (-1: no se ha escrito)
	LineaOdoo  int     `json:"linea_odoo,omitempty"`
}

type ResumenSesion struct {
	Sesion       string  `json:"sesion"`
	Proyecto     int     `json:"proyecto"`
	Agente       string  `json:"agente,omitempty"`
	Periodos     int     `json:"periodos"`
	Turnos       int     `json:"turnos"`
	MaquinaS     float64 `json:"maquina_s"`
	HombreS      float64 `json:"hombre_s"`
	Herramientas int     `json:"herramientas"`
	Subagentes   int     `json:"subagentes"`
	SegundoPlano int     `json:"segundo_plano"`
	Tokens       int     `json:"tokens"`
	Coste        float64 `json:"coste"`
}

type Informe struct {
	Sucesos  int             `json:"sucesos"`
	Lineas   []LineaDia      `json:"lineas"`
	Sesiones []ResumenSesion `json:"sesiones"`
	Avisos   []Aviso         `json:"avisos"`
	Errores  int             `json:"errores"`
}

type suceso struct {
	T          time.Time
	Ev         string  `json:"ev"`
	Sesion     string  `json:"sesion"`
	UID        int     `json:"uid"`
	Proyecto   int     `json:"proyecto"`
	TareaID    int     `json:"tarea_id"`
	Agente     string  `json:"agente"`
	Fecha      string  `json:"fecha"`
	Clase      string  `json:"clase"`
	LineaAg    string  `json:"linea_agente"`
	MasS       float64 `json:"mas_s"`
	CerradoS   float64 `json:"cerrado_s"`
	Inicio     string  `json:"inicio"`
	Fin        string  `json:"fin"`
	MaquinaS   float64 `json:"maquina_s"`
	HombreS    float64 `json:"hombre_s"`
	SinTarea   bool    `json:"sin_tarea"`
	Motivo     string  `json:"motivo"`
	Segundos   float64 `json:"segundos"`
	Horas      float64 `json:"horas"`
	Antes      float64 `json:"antes"`
	Linea      int     `json:"linea"`
	Error      string  `json:"error"`
	TurnoS     float64 `json:"turno_s"`
	Vivos      int     `json:"procesos_vivos"`
	Cancelada  bool    `json:"cancelada"`
	Tipo       string  `json:"tipo"`
	Nombre     string  `json:"nombre"`
	Estado     string  `json:"estado"`
	ID         string  `json:"id"`
	Entrada    int     `json:"entrada"`
	Salida     int     `json:"salida"`
	Coste      float64 `json:"coste"`
	TurnoCorte bool    `json:"turno_cortado"`
	Solapado   float64 `json:"solapado_s"`
	Ritmo      int     `json:"ritmo"`
	// Tramos humanos del periodo ([inicio, fin]); los registros de antes de los tramos no los traen.
	Tramos *[][2]string `json:"tramos"`
}

// PeriodoLargo: un periodo más largo que esto es sospechoso (alguien se dejó la sesión abierta, un turno colgado…).
const PeriodoLargo = 4 * time.Hour

// Valida lee un registro de tiempos y devuelve el informe. hoy: el día aún no ha acabado (lo que sigue abierto o
// pendiente de escribir no es un error).
func Valida(r io.Reader, hoy bool) (Informe, error) {
	inf := Informe{Lineas: []LineaDia{}, Sesiones: []ResumenSesion{}, Avisos: []Aviso{}}
	av := func(s *suceso, grav, tipo, f string, a ...any) {
		x := Aviso{Gravedad: grav, Tipo: tipo, Texto: fmt.Sprintf(f, a...)}
		if s != nil {
			x.T, x.Sesion = s.T.Format("15:04:05"), s.Sesion
		}
		if grav == "error" {
			inf.Errores++
		}
		inf.Avisos = append(inf.Avisos, x)
	}
	lineas := map[string]*LineaDia{}
	clave := func(fecha string, proy, tarea int, clase, agente string) string {
		return fmt.Sprintf("%s|%d|%d|%s|%s", fecha, proy, tarea, clase, agente)
	}
	linea := func(fecha string, proy, tarea int, clase, agente string) *LineaDia {
		k := clave(fecha, proy, tarea, clase, agente)
		if lineas[k] == nil {
			lineas[k] = &LineaDia{Fecha: fecha, Proyecto: proy, TareaID: tarea, Clase: clase, Agente: agente, Odoo: -1}
		}
		return lineas[k]
	}
	acumulado := map[string]float64{} // último cerrado_s visto por línea
	sesiones := map[string]*ResumenSesion{}
	ses := func(s *suceso) *ResumenSesion {
		if sesiones[s.Sesion] == nil {
			sesiones[s.Sesion] = &ResumenSesion{Sesion: s.Sesion}
		}
		x := sesiones[s.Sesion]
		if s.Proyecto != 0 {
			x.Proyecto = s.Proyecto
		}
		if s.Agente != "" && s.Agente != "Agente" {
			x.Agente = s.Agente
		}
		return x
	}
	type intervalo struct {
		sesion   string
		uid      int
		ini, fin time.Time
	}
	var humanos []intervalo
	solapados := map[string]float64{} // tiempo humano solapado que ya se descontó, por sesión
	turnos := map[string]int{}        // turnos vivos por sesión
	turnoDesde := map[string]time.Time{}
	var ultimo time.Time

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var crudo struct {
			T string `json:"t"`
		}
		var s suceso
		if json.Unmarshal(sc.Bytes(), &s) != nil || json.Unmarshal(sc.Bytes(), &crudo) != nil {
			av(nil, "aviso", "registro", "línea del registro ilegible")
			continue
		}
		s.T, _ = time.Parse(time.RFC3339Nano, crudo.T)
		ultimo = s.T
		inf.Sucesos++
		switch s.Ev {
		case "turno-empieza":
			ses(&s).Turnos++
			if turnos[s.Sesion] == 0 {
				turnoDesde[s.Sesion] = s.T
			}
			turnos[s.Sesion]++
		case "turno-acaba":
			if turnos[s.Sesion] == 0 {
				av(&s, "aviso", "turno", "fin de turno sin inicio (¿pgocode se reinició a mitad?)")
			} else {
				turnos[s.Sesion]--
			}
			if s.TurnoS > PeriodoLargo.Seconds()/2 {
				av(&s, "aviso", "turno", "turno del agente de %s", dur(s.TurnoS))
			}
		case "periodo-recupera":
			if s.TurnoCorte {
				av(&s, "info", "periodo", "periodo recuperado tras reiniciar pgocode con un turno a medias: su tiempo de máquina acaba en el último trabajo anotado")
			}
			turnos[s.Sesion] = 0
		case "periodo-cierra":
			x := ses(&s)
			x.Periodos++
			x.MaquinaS += s.MaquinaS
			x.HombreS += s.HombreS
			ini, e1 := time.Parse(time.RFC3339Nano, s.Inicio)
			fin, e2 := time.Parse(time.RFC3339Nano, s.Fin)
			if e1 != nil || e2 != nil {
				av(&s, "error", "periodo", "periodo cerrado sin inicio o fin legibles")
				break
			}
			total := fin.Sub(ini).Seconds()
			if s.MaquinaS < 0 || s.HombreS < 0 {
				av(&s, "error", "periodo", "tiempos negativos: máquina %.0f s, hombre %.0f s", s.MaquinaS, s.HombreS)
			}
			if s.Tramos == nil {
				// Registro antiguo: el tiempo del periodo se repartía entre máquina y hombre.
				if math.Abs(s.MaquinaS+s.HombreS-total) > 1 {
					av(&s, "error", "periodo", "descuadre: máquina %.0f s + hombre %.0f s ≠ duración %.0f s", s.MaquinaS, s.HombreS, total)
				}
				if s.HombreS > 0 {
					humanos = append(humanos, intervalo{s.Sesion, s.UID, ini, fin})
				}
			} else {
				// Máquina y hombre son independientes: cada uno cabe en el periodo; el hombre son sus tramos, y cada
				// tramo dura al menos 15 minutos (salvo que lo corte la medianoche).
				if s.MaquinaS > total+1 {
					av(&s, "error", "periodo", "máquina %.0f s en un periodo de %.0f s", s.MaquinaS, total)
				}
				var ts []Tramo
				for _, t := range *s.Tramos {
					a, e1 := time.Parse(time.RFC3339Nano, t[0])
					b, e2 := time.Parse(time.RFC3339Nano, t[1])
					if e1 != nil || e2 != nil || !b.After(a) {
						av(&s, "error", "humano", "tramo humano ilegible o vacío: %v", t)
						continue
					}
					medianoche := b.Hour() == 0 && b.Minute() == 0 && b.Second() == 0
					if b.Sub(a) < MinHumano-time.Second && !medianoche {
						av(&s, "error", "humano", "tramo humano de %s (%s–%s): el mínimo es de %s", dur(b.Sub(a).Seconds()), a.Format("15:04"), b.Format("15:04"), dur(MinHumano.Seconds()))
					}
					if a.Before(ini.Add(-time.Second)) || b.After(fin.Add(time.Second)) {
						av(&s, "error", "humano", "tramo humano %s–%s fuera del periodo %s–%s", a.Format("15:04:05"), b.Format("15:04:05"), ini.Format("15:04:05"), fin.Format("15:04:05"))
					}
					ts = append(ts, Tramo{a, b})
					humanos = append(humanos, intervalo{s.Sesion, s.UID, a, b})
				}
				if u := union(ts); math.Abs(u-s.HombreS) > 1 {
					av(&s, "error", "humano", "hombre %.0f s, pero sus tramos suman %.0f s", s.HombreS, u)
				}
			}
			if total > PeriodoLargo.Seconds() {
				av(&s, "aviso", "periodo", "periodo de %s (%s): ¿sesión olvidada o turno colgado?", dur(total), s.Motivo)
			}
			if s.SinTarea {
				av(&s, "aviso", "tarea", "periodo cerrado sin tarea de Odoo resuelta: se sumará al resolverla")
			}
			turnos[s.Sesion] = 0
		case "dia-suma":
			ag := ""
			if s.Clase == "m" {
				ag = s.LineaAg
			}
			l := linea(s.Fecha, s.Proyecto, s.TareaID, s.Clase, ag)
			k := clave(s.Fecha, s.Proyecto, s.TareaID, s.Clase, ag)
			if prev, ok := acumulado[k]; ok && math.Abs(prev+s.MasS-s.CerradoS) > 0.01 {
				av(&s, "aviso", "acumulado", "la línea %s salta de %.0f s a %.0f s sumando %.0f s (se leyó de Odoo o del estado guardado)", l.nombre(), prev, s.CerradoS, s.MasS)
			}
			acumulado[k] = s.CerradoS
			l.Periodos++
			l.Segundos = s.CerradoS
		case "odoo":
			l := linea(s.Fecha, s.Proyecto, s.TareaID, s.Clase, map[bool]string{true: s.LineaAg, false: ""}[s.Clase == "m"])
			if s.Error != "" {
				av(&s, "error", "odoo", "no se pudo escribir la línea %s (%.2f h): %s", l.nombre(), s.Horas, s.Error)
				break
			}
			// Un trabajo recién empezado (con ritmo y aún 0 s) ya cuenta su primer bloque.
			seg := s.Segundos
			if seg < 0.5 && s.Ritmo > 0 {
				seg = 1
			}
			if r := Redondea(seg, s.Clase); math.Abs(r-s.Horas) > 0.0001 {
				av(&s, "error", "redondeo", "línea %s: %.0f s se redondean a %.4f h, pero se escribieron %.4f h", l.nombre(), s.Segundos, r, s.Horas)
			}
			if s.Antes > 0 && s.Horas+0.0001 < s.Antes {
				av(&s, "aviso", "odoo", "la línea %s baja de %.2f h a %.2f h", l.nombre(), s.Antes, s.Horas)
			}
			l.Odoo, l.LineaOdoo = s.Horas, s.Linea
		case "error":
			av(&s, "error", "imputacion", "%s", s.Error)
		case "periodo-solape":
			solapados[s.Sesion] += s.Solapado
		case "uso":
			x := ses(&s)
			x.Tokens += s.Entrada + s.Salida
			x.Coste += s.Coste
		case "uso-sin-periodo":
			av(&s, "error", "uso", "consumo sin periodo abierto (%d tokens): no se imputa", s.Entrada+s.Salida)
		case "herramienta":
			if s.Estado == "curso" {
				x := ses(&s)
				x.Herramientas++
				switch s.Tipo {
				case "subagente":
					x.Subagentes++
				case "segundo_plano":
					x.SegundoPlano++
				}
			}
		case "agente-fin":
			if s.Error != "" && !s.Cancelada {
				av(&s, "aviso", "agente", "el agente acabó con error: %s", s.Error)
			}
			if s.Vivos > 0 {
				av(&s, "aviso", "procesos", "tras el turno siguen vivos %d grupos de procesos de agentes (segundo plano)", s.Vivos)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return inf, err
	}

	// Tiempo humano contado dos veces: lo de cada periodo que cae en tiempo humano ya contado antes (del mismo
	// usuario, otra sesión), como hace la imputación; si se anotó ese descuento («periodo-solape»), no es un fallo.
	sort.Slice(humanos, func(i, j int) bool { return humanos[i].ini.Before(humanos[j].ini) })
	porUsuario := map[int][]Tramo{}
	for _, b := range humanos {
		antes := append([]Tramo(nil), porUsuario[b.uid]...)
		sol := solape(antes, b.ini, b.fin)
		if sol >= 60 && solapados[b.sesion]+60 < sol {
			av(&suceso{T: b.ini, Sesion: b.sesion}, "aviso", "solape", "%s de tiempo humano de la sesión %s ya contado en otra sesión a la vez (%s–%s): se cuenta dos veces",
				dur(sol-solapados[b.sesion]), b.sesion, b.ini.Format("15:04"), b.fin.Format("15:04"))
		}
		solapados[b.sesion] = math.Max(0, solapados[b.sesion]-sol)
		porUsuario[b.uid] = append(porUsuario[b.uid], Tramo{b.ini, b.fin})
	}
	// Turnos sin acabar.
	for s, n := range turnos {
		if n > 0 && (!hoy || ultimo.Sub(turnoDesde[s]) > PeriodoLargo/2) {
			av(&suceso{T: turnoDesde[s], Sesion: s}, "aviso", "turno", "turno del agente sin acabar desde las %s", turnoDesde[s].Format("15:04"))
		}
	}
	// Lo calculado frente a lo escrito en Odoo.
	for _, l := range lineas {
		l.Calculadas = Redondea(l.Segundos, l.Clase)
		switch {
		case l.Segundos > 0 && l.Odoo < 0:
			av(nil, map[bool]string{true: "info", false: "error"}[hoy], "odoo", "la línea %s (%.2f h calculadas) no se ha escrito en Odoo", l.nombre(), l.Calculadas)
		case l.Odoo >= 0 && l.Odoo+0.0001 < l.Calculadas:
			av(nil, map[bool]string{true: "info", false: "error"}[hoy], "odoo", "la línea %s tiene %.2f h en Odoo y se calculan %.2f h", l.nombre(), l.Odoo, l.Calculadas)
		}
		inf.Lineas = append(inf.Lineas, *l)
	}
	sort.Slice(inf.Lineas, func(i, j int) bool { return inf.Lineas[i].nombre() < inf.Lineas[j].nombre() })
	for _, x := range sesiones {
		inf.Sesiones = append(inf.Sesiones, *x)
	}
	sort.Slice(inf.Sesiones, func(i, j int) bool { return inf.Sesiones[i].Sesion < inf.Sesiones[j].Sesion })
	return inf, nil
}

func (l *LineaDia) nombre() string {
	if l.Clase == "m" {
		return fmt.Sprintf("%s/%d/%d/M %s", l.Fecha, l.Proyecto, l.TareaID, l.Agente)
	}
	return fmt.Sprintf("%s/%d/%d/H", l.Fecha, l.Proyecto, l.TareaID)
}

func minT(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func dur(s float64) string {
	d := time.Duration(s) * time.Second
	if d >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
}
