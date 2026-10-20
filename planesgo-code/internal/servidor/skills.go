package servidor

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"planesgo-code/internal/depuracion"

	"planesgo-code/internal/sesiones"
	"planesgo-code/internal/skills"
)

// skillsCarpeta comprueba proyecto y carpeta (la carpeta puede faltar: solo las skills del usuario).
func (s *Servidor) skillsCarpeta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) (string, bool) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return "", false
	}
	c := r.URL.Query().Get("carpeta")
	if c != "" && !s.Indice.Pertenece(id, c) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return "", false
	}
	return c, true
}

// skillsInventario: las skills que ven los agentes en la carpeta del proyecto.
func (s *Servidor) skillsInventario(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	c, ok := s.skillsCarpeta(w, r, ses)
	if !ok {
		return
	}
	l := skills.Inventario(c)
	// Los vectores de las skills nuevas o cambiadas se calculan ya, en segundo plano, para elegir al momento.
	if s.Selector != nil {
		go func() {
			ctx, cancela := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancela()
			if err := s.Selector.Prepara(ctx, l); err != nil {
				depuracion.Log("skills", "preparar embeddings: %v", err)
			}
		}()
	}
	jsonOK(w, l)
}

// skillsElegir: qué skill del agente encaja con la petición (embeddings), las cinco mejores y si alguna pasa el umbral.
func (s *Servidor) skillsElegir(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Agente   string `json:"agente"`
		Texto    string `json:"texto"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if p.Carpeta != "" && !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return
	}
	if s.Selector == nil {
		jsonError(w, http.StatusServiceUnavailable, "selector de skills no disponible")
		return
	}
	// Solo las skills que ve el agente de la sesión, sin repetir nombre (la primera: la más concreta).
	var del []skills.Skill
	vistos := map[string]bool{}
	for _, k := range skills.Inventario(p.Carpeta) {
		if k.Rota || vistos[k.Nombre] || (p.Agente != "" && !contiene(k.Agentes, p.Agente)) {
			continue
		}
		vistos[k.Nombre] = true
		del = append(del, k)
	}
	ini := time.Now()
	c, motor, err := s.Selector.Elige(r.Context(), p.Texto, del)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	res := map[string]any{"candidatos": c[:min(5, len(c))], "umbral": motor.Umbral, "motor": motor.Nombre, "ms": time.Since(ini).Milliseconds()}
	if len(c) > 0 && c[0].Puntuacion >= motor.Umbral {
		res["elegida"] = c[0]
	}
	depuracion.Log("skills", "elegir con %s (%s, %d skills) %q → %v en %s", motor.Nombre, p.Agente, len(del), recortaTexto(p.Texto, 80), res["elegida"], time.Since(ini).Round(time.Millisecond))
	jsonOK(w, res)
}

func contiene(l []string, x string) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

func recortaTexto(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// skillsTexto: el SKILL.md de una skill del inventario (solo esas rutas).
func (s *Servidor) skillsTexto(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	c, ok := s.skillsCarpeta(w, r, ses)
	if !ok {
		return
	}
	ruta := r.URL.Query().Get("ruta")
	for _, k := range skills.Inventario(c) {
		if k.Ruta == ruta && !k.Rota {
			b, err := os.ReadFile(ruta)
			if err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, map[string]string{"texto": string(b)})
			return
		}
	}
	jsonError(w, http.StatusNotFound, "esa skill no está en el inventario")
}
