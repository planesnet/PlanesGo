//go:build !linux

package proceso

// EnRed: un programa del usuario que escucha en algún puerto.
type EnRed struct {
	Grupo    int    `json:"grupo"`
	Pid      int    `json:"pid"`
	Puertos  []int  `json:"puertos"`
	Nombre   string `json:"nombre"`
	Orden    string `json:"orden"`
	Carpeta  string `json:"carpeta"`
	Segundos int64  `json:"segundos"`
}

// Escuchando: fuera de Linux, de momento ninguno.
func Escuchando() []EnRed { return nil }
