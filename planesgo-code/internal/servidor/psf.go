package servidor

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"planesgo-code/internal/github"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// Automatic Planes Software Factory (PSF 1.0) session for a GitHub issue. The UI drives the phases, agents and
// reviewers; this file holds the git and GitHub side: the psf/issue-<N> branch in its worktree, the draft PR, the
// spec, pushing what the Builder does, marking the PR ready and, once a person confirms, merging it. Every route
// goes through the project and folder checks.

// maxPSFDiff: the diff size handed to each reviewer (they can read the rest from the repository).
const maxPSFDiff = 60_000

type psfRequest struct {
	Project int    `json:"project"`
	Folder  string `json:"folder"`
	Issue   int    `json:"issue"`
	Session string `json:"session"`
	PR      int    `json:"pr"`
	Base    string `json:"base"`
	Title   string `json:"title"`
	Text    string `json:"text"`
}

// psfRead decodes the request and checks project, folder and GitHub repository.
func (s *Servidor) psfRead(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) (psfRequest, string, bool) {
	var p psfRequest
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return p, "", false
	}
	if p.Issue < 0 || p.PR < 0 || len(p.Text) > 200_000 || len(p.Title) > 300 {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return p, "", false
	}
	name, ok := s.repoGitHub(w, r, ses, p.Project, p.Folder)
	return p, name, ok
}

// psfFolder: the worktree of the issue branch (the same one the chat uses).
func (s *Servidor) psfFolder(w http.ResponseWriter, r *http.Request, p psfRequest) (string, bool) {
	if p.Issue <= 0 {
		jsonError(w, http.StatusBadRequest, "falta la issue")
		return "", false
	}
	dir, err := repo.CarpetaDeRama(r.Context(), p.Folder, repo.PSFBranch(p.Issue), s.DirRamas)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return dir, true
}

// psfStart: phase 1. The whole issue, the branch from the merge branch (the remote default one unless the project
// chose another) and the draft PR into it.
func (s *Servidor) psfStart(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	issue, err := s.GitHub.IssueDetail(r.Context(), name, p.Issue)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	if issue.State != "OPEN" {
		jsonError(w, http.StatusBadRequest, "la issue está cerrada")
		return
	}
	mainDir, err := repo.RepoPrincipal(r.Context(), p.Folder)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	lock := s.bloqueoRepo(mainDir)
	lock.Lock()
	ws, err := repo.PreparePSFBranch(r.Context(), p.Folder, p.Issue, p.Base, s.DirRamas) // Base: the project's merge branch («» = default)
	lock.Unlock()
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	ws.Sesion, ws.Proyecto = p.Session, p.Project
	s.apuntaCopia(ws.CopiaCreada)
	spec := repo.PSFSpecPath(p.Issue)
	pr, err := s.GitHub.CreateDraftPR(r.Context(), name, ws.Base, ws.Rama, psfDraftTitle(p.Issue, issue.Title),
		github.PSFBody(p.Issue, issue.Title, spec, 1))
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("psf: issue #%d of %s on %s, PR #%d (%s)", p.Issue, name, ws.Rama, pr.Number, ses.Login)
	jsonOK(w, map[string]any{"repo": name, "issue": issue, "branch": ws.Rama, "base": ws.Base, "folder": ws.Carpeta,
		"pr": pr.Number, "pr_url": pr.URL, "spec_path": spec})
}

// psfSpec: phase 2, when the spec is approved. It is committed to the branch, pushed and posted on the PR.
func (s *Servidor) psfSpec(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	if strings.TrimSpace(p.Text) == "" || p.PR <= 0 {
		jsonError(w, http.StatusBadRequest, "falta la especificación o el PR")
		return
	}
	dir, ok := s.psfFolder(w, r, p)
	if !ok {
		return
	}
	if err := repo.WritePSFSpec(r.Context(), dir, repo.PSFBranch(p.Issue), p.Issue, p.Text); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	body := "## Especificación aprobada (PSF · Spec Writer)\n\n" + truncate(p.Text, 60_000)
	if err := s.GitHub.CommentPR(r.Context(), name, p.PR, body); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.psfChecklist(r, name, p, 2)
	jsonOK(w, map[string]bool{"ok": true})
}

// psfSync: after each Builder turn. Commits whatever is pending, pushes and returns the diff against the base.
func (s *Servidor) psfSync(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, _, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	if p.Base == "" || strings.HasPrefix(p.Base, "-") || strings.Contains(p.Base, "..") {
		jsonError(w, http.StatusBadRequest, "falta la rama base")
		return
	}
	dir, ok := s.psfFolder(w, r, p)
	if !ok {
		return
	}
	msg := strings.TrimSpace(p.Text)
	if msg == "" || len(msg) > 300 {
		msg = "feat(issue-" + strconv.Itoa(p.Issue) + "): implement spec"
	}
	res, err := repo.SyncPSF(r.Context(), dir, repo.PSFBranch(p.Issue), p.Base, msg, maxPSFDiff)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, res)
}

// psfReady: phase 5, with all 4 reviewers approving. Posts the audit summary and marks the PR ready.
func (s *Servidor) psfReady(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	if p.PR <= 0 {
		jsonError(w, http.StatusBadRequest, "falta el PR")
		return
	}
	if strings.TrimSpace(p.Text) != "" {
		if err := s.GitHub.CommentPR(r.Context(), name, p.PR, truncate(p.Text, 60_000)); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	s.psfChecklist(r, name, p, 4)
	// Out of draft, the title loses its «wip:» (it becomes the squash commit title on the base branch).
	if p.Issue > 0 && p.Title != "" {
		if err := s.GitHub.EditPR(r.Context(), name, p.PR, map[string]string{"title": psfReadyTitle(p.Issue, p.Title)}); err != nil {
			log.Printf("psf: title of PR #%d: %v", p.PR, err)
		}
	}
	if err := s.GitHub.MarkReady(r.Context(), name, p.PR); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("psf: PR #%d of %s ready for merge (%s)", p.PR, name, ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
}

// psfComment: a comment on the PR (e.g. why the session got blocked).
func (s *Servidor) psfComment(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	if p.PR <= 0 || strings.TrimSpace(p.Text) == "" {
		jsonError(w, http.StatusBadRequest, "falta el PR o el comentario")
		return
	}
	if err := s.GitHub.CommentPR(r.Context(), name, p.PR, truncate(p.Text, 60_000)); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// psfChecks: the PR checks status (shown before merging).
func (s *Servidor) psfChecks(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	c, err := s.GitHub.Checks(r.Context(), name, p.PR)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, c)
}

// psfMerge: confirmed by a person. Squash merge, remote branch deleted; the issue closes through «Closes #N».
func (s *Servidor) psfMerge(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	p, name, ok := s.psfRead(w, r, ses)
	if !ok {
		return
	}
	s.psfChecklist(r, name, p, 5)
	if err := s.GitHub.Merge(r.Context(), name, p.PR); err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("psf: PR #%d of %s merged by %s", p.PR, name, ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
}

// psfChecklist updates the PR checklist (a failure is only logged: it never stops the process).
func (s *Servidor) psfChecklist(r *http.Request, name string, p psfRequest, done int) {
	if p.PR <= 0 || p.Issue <= 0 {
		return
	}
	if err := s.GitHub.EditPRBody(r.Context(), name, p.PR, github.PSFBody(p.Issue, p.Title, repo.PSFSpecPath(p.Issue), done)); err != nil {
		log.Printf("psf: checklist of PR #%d: %v", p.PR, err)
	}
}

// psfDraftTitle and psfReadyTitle: the PR title while in draft and once ready.
func psfDraftTitle(issue int, title string) string { return "wip: " + psfReadyTitle(issue, title) }
func psfReadyTitle(issue int, title string) string {
	return "[Issue #" + strconv.Itoa(issue) + "] " + title
}

func truncate(t string, n int) string {
	if len(t) <= n {
		return t
	}
	return strings.ToValidUTF8(t[:n], "") + "\n\n… (recortado)"
}
