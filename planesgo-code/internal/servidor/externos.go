package servidor

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"planesgo-code/internal/proceso"
	"planesgo-code/internal/sesiones"
)

// Programas externos: los que tienes en marcha escuchando en un puerto (un Odoo, un servidor de desarrollo…) que
// no lanzó pgocode (los arrancó un agente fuera de pgocode, o antes de que pgocode los adoptara, o tú). Se ven
// por proyecto (su carpeta) y se pueden parar. Solo los de tus carpetas de trabajo, nunca los del sistema.

// dentro: si la ruta es la carpeta o está dentro de alguna de la lista.
func dentro(ruta string, carpetas []string) bool {
	ruta = filepath.Clean(ruta)
	for _, c := range carpetas {
		c = filepath.Clean(c)
		if ruta == c || strings.HasPrefix(ruta, c+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// externos: los programas externos en escucha, separados en los del proyecto y los demás de las carpetas de trabajo.
func (s *Servidor) externos(proyecto int) (delProyecto, otros []proceso.EnRed) {
	propios := map[int]bool{proceso.MiGrupo(): true}
	if s.Procesos != nil {
		for g := range s.Procesos.Grupos() {
			propios[g] = true
		}
	}
	carpetas := append(append([]string(nil), s.Indice.Carpetas(false)[proyecto]...), s.Indice.Registradas(proyecto)...)
	trabajo := append([]string(nil), s.Indice.Raices...)
	delProyecto, otros = []proceso.EnRed{}, []proceso.EnRed{}
	for _, x := range proceso.Escuchando() {
		if propios[x.Grupo] {
			continue
		}
		// Parte de un programa de pgocode (lleva su marca, aunque esté en otro grupo): su puerto es la dirección
		// de ese programa, y no es externo.
		if s.Procesos != nil && len(x.Puertos) > 0 {
			if n, ok := s.Procesos.DeMarca(proceso.MarcaDe(x.Pid)); ok {
				s.Procesos.PonURL(n, "http://localhost:"+strconv.Itoa(x.Puertos[0])+"/")
				continue
			}
		}
		switch {
		case len(carpetas) > 0 && (dentro(x.Carpeta, carpetas) || ordenEn(x.Orden, carpetas)):
			delProyecto = append(delProyecto, x)
		case dentro(x.Carpeta, trabajo):
			otros = append(otros, x)
		}
	}
	return delProyecto, otros
}

func ordenEn(orden string, carpetas []string) bool {
	for _, c := range carpetas {
		if strings.Contains(orden, filepath.Clean(c)+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (s *Servidor) procesosExternos(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	p, o := s.externos(id)
	jsonOK(w, map[string]any{"proyecto": p, "otros": o})
}

// procesosExternosPara: para un programa externo (su grupo entero): que termine y, si no, a los 5 s a la fuerza.
// Solo uno de los que se listan (de las carpetas de trabajo), nunca otro proceso del equipo.
func (s *Servidor) procesosExternosPara(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int `json:"proyecto"`
		Grupo    int `json:"grupo"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	a, b := s.externos(p.Proyecto)
	for _, x := range append(a, b...) {
		if x.Grupo == p.Grupo {
			proceso.SenalGrupo(x.Grupo, false)
			go func(g int) {
				time.Sleep(5 * time.Second)
				if proceso.GrupoVivo(g) {
					proceso.SenalGrupo(g, true)
				}
			}(x.Grupo)
			jsonOK(w, map[string]bool{"ok": true})
			return
		}
	}
	jsonError(w, http.StatusNotFound, "ese programa ya no está en marcha o no es de tus carpetas de trabajo")
}
