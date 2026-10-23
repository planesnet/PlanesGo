package llm

import "sync"

// Calibración de la vigencia por proyecto (ver calibrate.go): los pesos de cada carpeta de proyecto, en los datos de
// pgocode (nunca en el repositorio del usuario).

var (
	muVigParams sync.Mutex
	// projectVig: los pesos calibrados de cada carpeta (cargados de disco la primera vez).
	projectVig map[string]vigParams
)

// vigParamsFor: los pesos de la vigencia del proyecto (los generales si no está calibrado o no hay carpeta).
func vigParamsFor(folder string) vigParams {
	if folder == "" {
		return defaultVig
	}
	muVigParams.Lock()
	defer muVigParams.Unlock()
	if k, ok := projectVig[folder]; ok {
		return k
	}
	return defaultVig
}

// params: los pesos de la sesión del recolector (los de su proyecto).
func (c *pruneCollector) params() vigParams { return vigParamsFor(c.state.Folder) }

// CalibrationInfo: lo que ve la interfaz de la calibración del proyecto (la ventanita del hilo de limpieza).
type CalibrationInfo struct {
	Params   vigParams `json:"pesos"`
	General  bool      `json:"generales,omitempty"` // sin calibrar (poco historial): los valores generales
	At       int64     `json:"fecha,omitempty"`     // cuándo se calibró (ms)
	Pieces   int       `json:"piezas,omitempty"`    // piezas etiquetadas
	Sessions int       `json:"sesiones,omitempty"`
	// Con los pesos del proyecto (y, para comparar, con los generales): piezas retiradas, tokens retirados,
	// tokens·paso fuera y podadas de más (relecturas por poda de más).
	Cleared       int   `json:"retiradas,omitempty"`
	Tokens        int   `json:"tokens,omitempty"`
	TokenSteps    int64 `json:"tokens_paso,omitempty"`
	Over          int   `json:"de_mas,omitempty"`
	BaseCleared   int   `json:"retiradas_generales,omitempty"`
	BaseTokens    int   `json:"tokens_generales,omitempty"`
	BaseTokenStep int64 `json:"tokens_paso_generales,omitempty"`
	BaseOver      int   `json:"de_mas_generales,omitempty"`
}
