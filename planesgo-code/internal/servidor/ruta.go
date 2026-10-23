package servidor

import (
	"net/http"

	"planesgo-code/internal/router"
	"planesgo-code/internal/sesiones"
)

// rutaAnota: POST /api/ruta: la decisión del enrutador de intenciones para un mensaje (web/src/router.ts), al
// registro de tiempos como suceso «ruta» (destino, quién lo ejecuta, motor que decidió, motivo, modificadores y ms
// que tardó en decidirse). Sirve para medir cuántos mensajes van a cada sitio y cuánto se ahorra (docs/enrutador.md).
func (s *Servidor) rutaAnota(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	var d router.Decision
	if err := lee(r, &d); err != nil {
		errorLectura(w, err)
		return
	}
	if err := d.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Imputacion != nil {
		s.Imputacion.AnotaAgente(d.Sesion, "ruta", d.Fields())
	}
	jsonOK(w, map[string]bool{"ok": true})
}
