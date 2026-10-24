package servidor

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"planesgo-code/internal/bugreport"
	"planesgo-code/internal/sesiones"
)

// The failures of pgocode itself become issues of the PlanesGo repository (internal/bugreport), where the automatic
// PSF sessions of the PLANESGO project can resolve them: the exceptions of the interface (POST /api/diagnostico) and
// the panics while serving a request (recovers). The crashes are watched from main (bugreport.WatchCrashes).

// recovers: a panic in a request answers 500 and is reported, instead of only cutting the connection.
func (s *Servidor) recovers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler { // the deliberate way to cut a response: not a failure
				panic(v)
			}
			stack := debug.Stack()
			log.Printf("pánico en %s %s: %v\n%s", r.Method, r.URL.Path, v, stack)
			s.Errores.Report(bugreport.Report{Source: bugreport.FromServer, Message: fmt.Sprint(v), Stack: string(stack), Where: r.Method + " " + r.URL.Path})
			jsonError(w, http.StatusInternalServerError, "error interno de pgocode")
		}()
		h.ServeHTTP(w, r)
	})
}

// errorReportsView: the «Errores» section of the application properties.
type errorReportsView struct {
	Enabled   bool              `json:"enabled"`
	Chosen    bool              `json:"chosen"` // the person chose; otherwise, the default
	Repo      string            `json:"repo"`
	Access    bool              `json:"access"` // the gh account of this computer can open issues there
	Public    bool              `json:"public"` // anyone can read the issues of the repository
	Dev       bool              `json:"dev"`
	MaxPerDay int               `json:"max_per_day"`
	Recent    []bugreport.Known `json:"recent"`
}

func (s *Servidor) errorReportsView(r *http.Request) errorReportsView {
	access, public := s.Errores.CanUse(r.Context())
	on, chosen := s.Errores.Enabled(r.Context())
	return errorReportsView{Enabled: on, Chosen: chosen, Repo: s.Errores.Repo, Access: access, Public: public,
		Dev: s.Version == "dev", MaxPerDay: bugreport.MaxNewPerDay, Recent: s.Errores.Recent(10)}
}

// GET /api/errores
func (s *Servidor) errorReports(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	if s.Errores == nil {
		jsonError(w, http.StatusServiceUnavailable, "los informes de errores no están disponibles")
		return
	}
	jsonOK(w, s.errorReportsView(r))
}

// POST /api/errores {enabled}: the choice of the person.
func (s *Servidor) errorReportsSave(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Enabled bool `json:"enabled"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if s.Errores == nil {
		jsonError(w, http.StatusServiceUnavailable, "los informes de errores no están disponibles")
		return
	}
	if err := s.Errores.SetEnabled(p.Enabled); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("informes de errores en GitHub: %v (%s)", p.Enabled, ses.Login)
	jsonOK(w, s.errorReportsView(r))
}
