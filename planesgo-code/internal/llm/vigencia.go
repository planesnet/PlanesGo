package llm

import (
	"math"
	"strings"
	"time"
)

// Vigencia: lo vigente que está cada pieza del historial del agente (pregunta, respuesta, salida de herramienta,
// imagen, adjunto) y cada intercambio, como un número real ≥ 0. 0 es absolutamente vital; 1, 2, 3… cada vez más
// viejo y menos importante. Con vigCut (3) o más, fuera del contexto del agente (lo que antes era L3).
//
//	vigencia = vigAgeK·log_φ(1 + intercambios desde entonces − los jóvenes)   (φ: el número áureo)
//	         + vigStepK·log_φ(1 + pasos del agente desde entonces − los protegidos)   (solo las piezas)
//	         + vigTimeK·ln(1 + horas desde entonces)
//	         + vigRelK·(vigRelPivot − relación con lo último)   (sin embeddings, este término no está)
//	         + vigSupersededJump si está superada (lectura vieja, orden repetida, imagen ya vista, edición antigua)
//
// Vale 0 el primer intercambio (las instrucciones), el texto del usuario y la respuesta de cada intercambio, y las
// piezas de los últimos pruneProtectSteps pasos del agente (lo que necesita para el paso siguiente). Los intercambios
// recientes (el turno en curso y los dos anteriores) cuentan enteros como vitales, pero cada pieza suya tiene su
// propia vigencia por pasos, relación con lo que se está haciendo ahora, tiempo y si está superada: así un turno
// largo puede retirar sus lecturas viejas sin esperar a que el turno acabe. Los pesos (vigParams) se calibran por
// proyecto con el propio historial (calibration.go); estas constantes son los valores generales.
// Calibrada con la conversación real de INTRADEMA (43 mensajes en 12 horas) para que el corte en 3 coincida con la
// poda por niveles que había (TestVigenciaCalibration): el 95,0 % de los intercambios y el 99,3 % de las partes
// cambiadas (con la antigüedad lineal de antes, el 93,6 %). La relación manda, y la antigüedad y el tiempo empujan poco a poco. Más empinada (vigRelK 30) coincide
// algo más (95 %), pero el desvanecido pasa a ser casi todo o nada.

const (
	vigCut = 3.0
	// vigAgeK: la antigüedad crece como la serie de Fibonacci: un escalón cada vez que los intercambios desde
	// entonces se multiplican por φ (1, 2, 3, 5, 8, 13, 21…). Mucha diferencia entre lo reciente y poca entre lo
	// antiguo, como la curva del olvido; por sí sola casi nunca llega al corte (lo que manda es la relación).
	vigAgeK = 0.12
	// vigStepK: la antigüedad de una pieza en pasos del agente (llamadas al modelo desde entonces), más allá de los
	// protegidos: también como la serie de Fibonacci (8 pasos más: 0,8·log_φ(9) = 3,65).
	vigStepK = 0.8
	// vigTimeK: lo que suma el tiempo: ln(1+h) (1 h: 0,10; 6 h: 0,29; un día: 0,48; una semana: 0,77).
	vigTimeK = 0.15
	// vigRelK y vigRelPivot: la relación con lo último (similitud coseno con 256 dimensiones). En el pivote no suma
	// nada; por encima resta y por debajo suma (0,40: 2; 0,35: 3; 0,30: 4). Con antigüedad y tiempo típicos, el
	// corte cae cerca del umbral de antes (pruneThreshold, 0,38).
	vigRelK     = 20.0
	vigRelPivot = 0.50
	// vigSupersededJump: lo superado salta por encima del corte.
	vigSupersededJump = 3.0
)

// vigParams: los pesos de la vigencia (el corte sigue en vigCut). Por proyecto, calibrados con su historial
// (calibration.go); sin calibrar, defaultVig.
type vigParams struct {
	AgeK     float64 `json:"antiguedad"`
	StepK    float64 `json:"pasos"`
	TimeK    float64 `json:"tiempo"`
	RelK     float64 `json:"relacion"`
	RelPivot float64 `json:"pivote"`
}

// defaultVig: los valores generales.
var defaultVig = vigParams{AgeK: vigAgeK, StepK: vigStepK, TimeK: vigTimeK, RelK: vigRelK, RelPivot: vigRelPivot}

// pruneProtectSteps: las piezas de los últimos pasos del agente no se tocan nunca (vigencia 0). Medido en las
// sesiones de INTRADEMA: el 90 % de las ediciones de un fichero llegan como mucho 2-11 pasos después de leerlo
// (mediana 1); con 8, lo que el agente está usando sigue entero, y lo que queda más atrás lo decide su vigencia
// (la relación con lo que se está haciendo ahora lo mantiene si sigue en uso).
const pruneProtectSteps = 8

// vigFactors: de qué sale la vigencia de un intercambio o una pieza.
type vigFactors struct {
	Pinned     bool     `json:"fija,omitempty"`     // vital: 0 (el primero, o una pieza de los últimos pasos)
	Ago        int      `json:"hace,omitempty"`     // intercambios desde entonces (1: el anterior)
	Young      int      `json:"jovenes,omitempty"`  // intercambios jóvenes (no suman antigüedad)
	Steps      int      `json:"pasos,omitempty"`    // pieza: pasos del agente desde entonces
	Sim        *float64 `json:"relacion,omitempty"` // relación con lo último (nil: sin embeddings)
	At         int64    `json:"en,omitempty"`       // cuándo (ms; 0: sin fecha)
	Superseded string   `json:"superada,omitempty"` // por qué está superada («» si no)
}

// phi: el número áureo.
var phi = (1 + math.Sqrt(5)) / 2

// logPhi: log_φ(1+n) (0 si n ≤ 0).
func logPhi(n int) float64 {
	if n <= 0 {
		return 0
	}
	return math.Log1p(float64(n)) / math.Log(phi)
}

// vigAge: lo que suma la antigüedad de n intercambios (más allá de los jóvenes): vigAgeK·log_φ(1+n).
func vigAge(n int) float64 { return vigAgeK * logPhi(n) }

// vigHours: las horas desde at (0 si no hay fecha).
func vigHours(at int64, now time.Time) float64 {
	if at <= 0 {
		return 0
	}
	return max(float64(now.UnixMilli()-at)/3.6e6, 0)
}

// vigencia: el número (con dos decimales), con los valores generales.
func vigencia(f vigFactors, now time.Time) float64 { return defaultVig.of(f, now) }

// of: la vigencia con estos pesos.
func (k vigParams) of(f vigFactors, now time.Time) float64 {
	return math.Round(k.raw(f, vigHours(f.At, now))*100) / 100
}

// raw: la vigencia sin redondear, con las horas ya calculadas (la calibración la evalúa miles de veces).
func (k vigParams) raw(f vigFactors, hours float64) float64 {
	v := 0.0
	if !f.Pinned {
		v = k.AgeK*logPhi(f.Ago-f.Young) + k.StepK*logPhi(f.Steps-pruneProtectSteps) + k.TimeK*math.Log1p(hours)
		if f.Sim != nil {
			v += k.RelK * (k.RelPivot - *f.Sim)
		}
		v = max(v, 0)
	}
	if f.Superseded != "" {
		v += vigSupersededJump
	}
	return v
}

func simPtr(s float64) *float64 { return &s }

// Vigencia: lo que ve la interfaz de un intercambio o una pieza (y el porqué).
type Vigencia struct {
	Value      float64  `json:"vigencia"`
	Ago        int      `json:"hace,omitempty"`     // intercambios desde entonces
	Steps      int      `json:"pasos,omitempty"`    // pasos del agente desde entonces (piezas)
	Sim        *float64 `json:"relacion,omitempty"` // relación con lo último
	Hours      float64  `json:"horas,omitempty"`    // horas desde entonces
	Superseded string   `json:"superada,omitempty"` // por qué está superada
	Note       string   `json:"nota,omitempty"`     // p. ej. «salida recortada»
	Out        bool     `json:"fuera,omitempty"`    // fuera del contexto del agente ahora
}

// PrunePiece: la vigencia de una pieza (una herramienta, por su id: el mismo de Herramienta.ID).
type PrunePiece struct {
	ID string `json:"id"`
	Vigencia
}

// vigRecord: lo que se guarda de un intercambio o pieza para volver a calcular su vigencia más tarde (el tiempo
// sigue corriendo aunque no haya turnos).
type vigRecord struct {
	ID       string     `json:"id"`
	F        vigFactors `json:"f"`
	Exchange int        `json:"x"` // pieza: su intercambio (índice en Exchanges)
	Note     string     `json:"nota,omitempty"`
	Out      bool       `json:"fuera,omitempty"`
}

// vigState: la vigencia de una sesión del agente (en el estado del recolector).
type vigState struct {
	Exchanges []vigRecord `json:"intercambios"`
	Pieces    []vigRecord `json:"piezas"`
	K         *vigParams  `json:"k,omitempty"` // los pesos con que se calculó (nil: los generales)
}

// params: los pesos del estado.
func (s *vigState) params() vigParams {
	if s.K != nil {
		return *s.K
	}
	return defaultVig
}

func (r vigRecord) view(v float64, now time.Time) Vigencia {
	w := Vigencia{Value: v, Superseded: r.F.Superseded, Note: r.Note, Out: r.Out}
	if !r.F.Pinned {
		w.Ago, w.Steps = r.F.Ago, r.F.Steps
		w.Hours = math.Round(vigHours(r.F.At, now)*100) / 100
		if r.F.Sim != nil {
			w.Sim = simPtr(math.Round(*r.F.Sim*100) / 100)
		}
	}
	return w
}

// report: la vigencia de cada intercambio (por su punto) y pieza en el momento now. La de una pieza no baja de la
// de su intercambio.
func (s *vigState) report(now time.Time) ([]PruneExchange, []PrunePiece) {
	k := s.params()
	xs := make([]PruneExchange, 0, len(s.Exchanges))
	vx := make([]float64, len(s.Exchanges))
	for i, r := range s.Exchanges {
		vx[i] = k.of(r.F, now)
		if r.ID != "" {
			xs = append(xs, PruneExchange{Point: r.ID, Vigencia: r.view(vx[i], now)})
		}
	}
	ps := make([]PrunePiece, 0, len(s.Pieces))
	for _, r := range s.Pieces {
		v := k.of(r.F, now)
		if r.Exchange >= 0 && r.Exchange < len(vx) {
			v = max(v, vx[r.Exchange])
		}
		ps = append(ps, PrunePiece{ID: r.ID, Vigencia: r.view(v, now)})
	}
	return xs, ps
}

// vigSnapshot: la vigencia del plan, con lo que está fuera ahora (outX y outP: si el barrido se aplica, lo del
// plan; si no, lo que ya estaba fuera en el historial del agente).
func vigSnapshot(plan prunePlan, outX func(*pruneExchange) bool, outP func(*pieceVig) bool) *vigState {
	s := &vigState{Exchanges: []vigRecord{}, Pieces: []vigRecord{}}
	if plan.K != defaultVig {
		k := plan.K
		s.K = &k
	}
	idx := map[*pruneExchange]int{}
	for i, x := range plan.Exchanges {
		idx[x] = i
		s.Exchanges = append(s.Exchanges, vigRecord{ID: x.Point, F: x.Vig, Out: outX(x)})
	}
	for _, p := range plan.Pieces {
		if p.CallID == "" {
			continue
		}
		s.Pieces = append(s.Pieces, vigRecord{ID: p.CallID, F: p.F, Exchange: idx[p.X], Note: p.Note, Out: outP(p)})
	}
	return s
}

// appliedOut: si una parte está ya fuera del contexto del agente (vaciada por pgocode o por OpenCode).
func appliedOut(p *ocPart) bool {
	if p.v.State.Time.Compacted != 0 {
		return true
	}
	return p.Current != "" && (strings.Contains(p.Current, `"compacted":`) || strings.Contains(p.Current, ClearedText))
}

// compactedNote: por qué está fuera lo anterior a la última compactación del agente.
const compactedNote = "resumido por la compactación del agente"

// addCompacted: lo anterior a la última compactación del agente (before: sus mensajes, sin el de la compactación ni
// el resumen) sale como fuera del contexto: el agente solo tiene su resumen. Cada intercambio (por su punto) y cada
// herramienta (por su id), con vigencia 3 y el porqué.
func (s *vigState) addCompacted(before []*ocMessage) {
	if len(before) == 0 {
		return
	}
	f := vigFactors{Superseded: compactedNote}
	for _, x := range pruneExchanges(before) {
		s.Exchanges = append(s.Exchanges, vigRecord{ID: x.Point, F: f, Out: true})
	}
	for _, m := range before {
		for _, p := range m.Parts {
			if p.v.Type == "tool" && p.v.CallID != "" {
				s.Pieces = append(s.Pieces, vigRecord{ID: p.v.CallID, F: f, Exchange: -1, Out: true})
			}
		}
	}
}
