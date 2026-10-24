package servidor

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"planesgo-code/internal/repo"
	"planesgo-code/internal/selector"
	"planesgo-code/internal/sesiones"
)

// Evidence and supervision of the automatic PSF sessions. The evidence is the program itself: after each Builder
// round pgocode photographs, without tokens, the screen of the project's program running in the issue worktree
// (psf_screen.go) and stores the pictures as evidence of the session in the project folder (Studio TV, «Capturas»).
// Every step also goes to the pgocode log (tiempos/*.jsonl), so the work of the sessions can be supervised and the
// process improved; the local AI writes the documentation of each session.

// psfEvidence stores a PSF picture as evidence of the session (failures are only logged: it never stops the flow).
func (s *Servidor) psfEvidence(p psfRequest, step, summary string, data []byte) {
	if p.Session == "" || p.Folder == "" {
		return
	}
	c := Captura{Sesion: p.Session, Titulo: fmt.Sprintf("#%d %s", p.Issue, p.Title), Herramienta: "PSF · " + step,
		Resumen: recortaTexto(summary, 160), Rama: repo.PSFBranch(p.Issue)}
	if _, err := saveEvidence(p.Folder, c, "psf-"+step, ".svg", data); err != nil {
		log.Printf("psf: evidence of %s: %v", step, err)
	}
}

// psfProgramPictures: the program of the project, running in the issue worktree, at three moments (the evidence of
// the change). Failures are logged: the pictures never stop the flow.
func (s *Servidor) psfProgramPictures(p psfRequest, dir, round string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	shots, err := captureProgram(ctx, dir, p.Program, []float64{3, 6, 10})
	for i, svg := range shots {
		s.psfEvidence(p, round, fmt.Sprintf("%s · foto %d de %d", p.Program, i+1, len(shots)), svg)
	}
	f := map[string]any{"orden": p.Program, "fotos": len(shots), "ms": time.Since(start).Milliseconds(), "ronda": round}
	if err != nil {
		f["error"] = err.Error()
	}
	s.psfLog(p, "fotos", f)
}

// psfLog writes one PSF step to the pgocode log (tiempos/*.jsonl), with the issue, PR and branch.
func (s *Servidor) psfLog(p psfRequest, ev string, fields map[string]any) {
	if s.Imputacion == nil || p.Session == "" {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["issue"], fields["pr"], fields["rama"] = p.Issue, p.PR, repo.PSFBranch(p.Issue)
	if p.Iteration > 0 {
		fields["iteracion"] = p.Iteration
	}
	s.Imputacion.AnotaAgente(p.Session, "psf-"+ev, fields)
}

// psfEvent: what the interface decides (phase changes, verdicts, the Approver, a person's decisions, errors) goes to
// the same log as the server steps.
func (s *Servidor) psfEvent(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		psfRequest
		Event string         `json:"event"`
		Data  map[string]any `json:"data"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Project); !ok {
		return
	}
	ev := strings.Map(func(c rune) rune {
		if c >= 'a' && c <= 'z' || c == '-' {
			return c
		}
		return -1
	}, p.Event)
	if ev == "" || len(ev) > 40 || len(p.Data) > 30 {
		jsonError(w, http.StatusBadRequest, "evento no válido")
		return
	}
	for k, v := range p.Data {
		if t, ok := v.(string); ok && len(t) > 4000 {
			p.Data[k] = t[:4000] + "…"
		}
	}
	p.Data["login"] = ses.Login
	s.psfLog(p.psfRequest, ev, p.Data)
	jsonOK(w, map[string]bool{"ok": true})
}

// psfDocSystem: what the local AI is asked when documenting an automatic session.
const psfDocSystem = "Eres el documentalista de Planes Software Factory. Con los datos de una issue resuelta por una sesión " +
	"automática, escribe en español un resumen breve en Markdown para el pull request, con tres apartados: «Qué se pedía», " +
	"«Qué se ha hecho» (con los ficheros) y «Cómo se ha revisado». Sé fiel a los datos: no inventes nada que no aparezca."

// psfDocument: the documentation of a session, written by the local AI (no paid tokens; with no local AI, a plain
// template). It goes to the PR as a comment, to the session pictures and to the log.
func (s *Servidor) psfDocument(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	if p.PR <= 0 || strings.TrimSpace(p.Text) == "" {
		jsonError(w, http.StatusBadRequest, "faltan el PR o los datos de la sesión")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	start := time.Now()
	doc, engine, err := selector.LocalChat(ctx, psfDocSystem, truncate(p.Text, 12_000), 700)
	doc = strings.TrimSpace(doc)
	if err != nil || doc == "" {
		engine = "plantilla"
		doc = "## Qué se pedía\n\n" + p.Title + "\n\n## Datos de la sesión\n\n" + truncate(p.Text, 6000)
		if err != nil {
			log.Printf("psf: local AI for the documentation: %v", err)
		}
	}
	body := fmt.Sprintf("### PSF · Documentación de la sesión\n\n%s\n\n_Redactado por %s, sin tokens de pago._", doc, map[bool]string{true: "una plantilla de pgocode", false: "la IA local (" + engine + ")"}[engine == "plantilla"])
	if err := s.GitHub.CommentPR(r.Context(), name, p.PR, body); err != nil {
		s.psfFail(w, p, "documentacion", http.StatusBadGateway, err)
		return
	}
	s.psfLog(p, "documentacion", map[string]any{"motor": engine, "ms": time.Since(start).Milliseconds(), "caracteres": len(doc)})
	jsonOK(w, map[string]string{"text": doc, "engine": engine})
}
