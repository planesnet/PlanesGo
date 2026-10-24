package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Planes Software Factory (PSF): what the automatic session of an issue needs from GitHub: the whole issue, its
// draft PR, the process comments, marking it ready and, once a person confirms, merging it.

// IssueComment is a comment on an issue.
type IssueComment struct {
	Author string    `json:"author"`
	Body   string    `json:"body"`
	Date   time.Time `json:"date"`
}

// IssueDetail: the issue with everything the Spec Writer reads (title, body, labels and comments).
type IssueDetail struct {
	Number   int            `json:"number"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	URL      string         `json:"url"`
	State    string         `json:"state"`
	Labels   []string       `json:"labels"`
	Comments []IssueComment `json:"comments"`
}

// gh runs a «gh» command (with optional stdin) outside any local repository, so «--delete-branch» only touches the
// remote branch and never the worktree.
func gh(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("falta la orden «gh» de GitHub (instálala y entra con «gh auth login»)")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = os.TempDir()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return b, fmt.Errorf("GitHub: %s", strings.TrimSpace(stderr.String()+" "+err.Error()))
	}
	return b, nil
}

func checkRef(repo string, n int) error {
	if !nombreRepo.MatchString(repo) || n <= 0 {
		return errors.New("repositorio o número no válido")
	}
	return nil
}

// IssueDetail reads the whole issue.
func (l *Lector) IssueDetail(ctx context.Context, repo string, n int) (IssueDetail, error) {
	if err := checkRef(repo, n); err != nil {
		return IssueDetail{}, err
	}
	b, err := gh(ctx, "", "issue", "view", fmt.Sprint(n), "-R", repo, "--json", "number,title,body,url,state,labels,comments")
	if err != nil {
		return IssueDetail{}, err
	}
	return parseIssueDetail(b)
}

func parseIssueDetail(b []byte) (IssueDetail, error) {
	var r struct {
		Number      int
		Title, Body string
		URL, State  string
		Labels      []struct{ Name string }
		Comments    []struct {
			Author    struct{ Login string }
			Body      string
			CreatedAt time.Time
		}
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return IssueDetail{}, err
	}
	d := IssueDetail{Number: r.Number, Title: r.Title, Body: r.Body, URL: r.URL, State: r.State, Labels: []string{}, Comments: []IssueComment{}}
	for _, x := range r.Labels {
		d.Labels = append(d.Labels, x.Name)
	}
	for _, c := range r.Comments {
		d.Comments = append(d.Comments, IssueComment{c.Author.Login, c.Body, c.CreatedAt})
	}
	return d, nil
}

// PRRef: a PR by number and URL.
type PRRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Draft  bool   `json:"draft"`
	State  string `json:"state"`
}

// OpenPRFor finds the open PR of a branch (nil when there is none).
func (l *Lector) OpenPRFor(ctx context.Context, repo, branch string) (*PRRef, error) {
	if !nombreRepo.MatchString(repo) || branch == "" || strings.HasPrefix(branch, "-") {
		return nil, errors.New("repositorio o rama no válidos")
	}
	b, err := gh(ctx, "", "pr", "list", "-R", repo, "--head", branch, "--state", "open", "--json", "number,url,isDraft,state")
	if err != nil {
		return nil, err
	}
	var prs []struct {
		Number  int
		URL     string
		IsDraft bool
		State   string
	}
	if err := json.Unmarshal(b, &prs); err != nil || len(prs) == 0 {
		return nil, nil
	}
	return &PRRef{prs[0].Number, prs[0].URL, prs[0].IsDraft, prs[0].State}, nil
}

// CreateDraftPR opens the draft PR from branch into base, or returns the one already open.
func (l *Lector) CreateDraftPR(ctx context.Context, repo, base, branch, title, body string) (PRRef, error) {
	if p, err := l.OpenPRFor(ctx, repo, branch); err != nil {
		return PRRef{}, err
	} else if p != nil {
		return *p, nil
	}
	if strings.HasPrefix(base, "-") || strings.TrimSpace(title) == "" {
		return PRRef{}, errors.New("PR no válido")
	}
	b, err := gh(ctx, body, "pr", "create", "-R", repo, "--base", base, "--head", branch, "--title", title, "--body-file", "-", "--draft")
	if err != nil {
		return PRRef{}, err
	}
	u := strings.TrimSpace(string(b))
	if i := strings.LastIndex(u, "\n"); i >= 0 {
		u = u[i+1:]
	}
	var n int
	if i := strings.LastIndex(u, "/pull/"); i >= 0 {
		fmt.Sscan(u[i+6:], &n)
	}
	if n == 0 {
		return PRRef{}, fmt.Errorf("GitHub no ha devuelto el PR creado: %s", u)
	}
	l.forget(repo)
	return PRRef{Number: n, URL: u, Draft: true, State: "OPEN"}, nil
}

// CommentPR adds a comment to the PR (the text goes through stdin: no command-line limits).
func (l *Lector) CommentPR(ctx context.Context, repo string, pr int, body string) error {
	if err := checkRef(repo, pr); err != nil {
		return err
	}
	_, err := gh(ctx, body, "pr", "comment", fmt.Sprint(pr), "-R", repo, "--body-file", "-")
	return err
}

// EditPR changes PR fields («title», «body»). Through the REST API: «gh pr edit» fails on some gh versions because
// it still asks for the retired Projects (classic) cards.
func (l *Lector) EditPR(ctx context.Context, repo string, pr int, fields map[string]string) error {
	if err := checkRef(repo, pr); err != nil {
		return err
	}
	in, _ := json.Marshal(fields)
	_, err := gh(ctx, string(in), "api", "-X", "PATCH", fmt.Sprintf("repos/%s/pulls/%d", repo, pr), "--input", "-", "--silent")
	return err
}

// EditPRBody replaces the PR description.
func (l *Lector) EditPRBody(ctx context.Context, repo string, pr int, body string) error {
	return l.EditPR(ctx, repo, pr, map[string]string{"body": body})
}

// MarkReady takes the PR out of draft.
func (l *Lector) MarkReady(ctx context.Context, repo string, pr int) error {
	if err := checkRef(repo, pr); err != nil {
		return err
	}
	_, err := gh(ctx, "", "pr", "ready", fmt.Sprint(pr), "-R", repo)
	l.forget(repo)
	return err
}

// Discard closes the PR without merging (and deletes its remote branch) and closes the issue as «not planned».
func (l *Lector) Discard(ctx context.Context, repo string, pr, issue int) error {
	if err := checkRef(repo, pr); err != nil {
		return err
	}
	if _, err := gh(ctx, "", "pr", "close", fmt.Sprint(pr), "-R", repo, "--delete-branch"); err != nil {
		return err
	}
	if issue > 0 {
		if _, err := gh(ctx, "", "issue", "close", fmt.Sprint(issue), "-R", repo, "--reason", "not planned"); err != nil {
			return err
		}
	}
	l.forget(repo)
	return nil
}

// PRChecks: the PR checks status and whether it can be merged.
type PRChecks struct {
	State     string   `json:"state"`     // OPEN | MERGED | CLOSED
	Draft     bool     `json:"draft"`     //
	Checks    string   `json:"checks"`    // SUCCESS | FAILURE | PENDING | "" (no checks)
	Mergeable string   `json:"mergeable"` // MERGEABLE | CONFLICTING | UNKNOWN
	Failing   []string `json:"failing"`   // failed checks
}

// Checks reads the PR checks status.
func (l *Lector) Checks(ctx context.Context, repo string, pr int) (PRChecks, error) {
	if err := checkRef(repo, pr); err != nil {
		return PRChecks{}, err
	}
	b, err := gh(ctx, "", "pr", "view", fmt.Sprint(pr), "-R", repo, "--json", "state,isDraft,mergeable,statusCheckRollup")
	if err != nil {
		return PRChecks{}, err
	}
	return parseChecks(b)
}

func parseChecks(b []byte) (PRChecks, error) {
	var r struct {
		State     string
		IsDraft   bool
		Mergeable string
		Rollup    []struct {
			Name, Context      string
			Status, Conclusion string
			State              string
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return PRChecks{}, err
	}
	c := PRChecks{State: r.State, Draft: r.IsDraft, Mergeable: r.Mergeable, Failing: []string{}}
	pending := false
	for _, x := range r.Rollup {
		name := x.Name
		if name == "" {
			name = x.Context
		}
		result := strings.ToUpper(x.Conclusion + x.State) // CheckRun: conclusion; StatusContext: state
		switch {
		case x.Status != "" && x.Status != "COMPLETED", result == "PENDING", result == "EXPECTED":
			pending = true
		case result == "FAILURE", result == "ERROR", result == "TIMED_OUT", result == "CANCELLED", result == "ACTION_REQUIRED", result == "STARTUP_FAILURE":
			c.Failing = append(c.Failing, name)
		}
	}
	switch {
	case len(r.Rollup) == 0:
	case len(c.Failing) > 0:
		c.Checks = "FAILURE"
	case pending:
		c.Checks = "PENDING"
	default:
		c.Checks = "SUCCESS"
	}
	return c, nil
}

// Merge squash-merges the PR and deletes the remote branch. Only with green checks (or none) and no conflicts.
func (l *Lector) Merge(ctx context.Context, repo string, pr int) error {
	c, err := l.Checks(ctx, repo, pr)
	if err != nil {
		return err
	}
	switch {
	case c.State != "OPEN":
		return errors.New("el PR ya no está abierto")
	case c.Draft:
		return errors.New("el PR sigue en borrador")
	case c.Checks == "FAILURE":
		return fmt.Errorf("hay checks fallando: %s", strings.Join(c.Failing, ", "))
	case c.Checks == "PENDING":
		return errors.New("los checks de GitHub aún no han terminado")
	case c.Mergeable == "CONFLICTING":
		return errors.New("el PR tiene conflictos con la rama base")
	}
	_, err = gh(ctx, "", "pr", "merge", fmt.Sprint(pr), "-R", repo, "--squash", "--delete-branch")
	l.forget(repo)
	return err
}

func (l *Lector) forget(repo string) {
	l.mu.Lock()
	delete(l.cache, repo)
	l.mu.Unlock()
}

// PSFSteps: the PR checklist items.
var PSFSteps = []string{
	"Ingestión, rama y PR",
	"Especificación aprobada",
	"Builder (TDD) en verde",
	"Comité de revisión 4/4",
	"Listo para merge (confirmación humana)",
}

// PSFBody: the PR description with the PSF checklist (the first «done» steps ticked).
func PSFBody(issue int, title, specPath string, done int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Closes #%d\n\n**%s**\n\nPR abierto por la sesión automática de Planes Software Factory (PSF 1.0) de pgocode.\n\n", issue, title)
	for i, s := range PSFSteps {
		mark := " "
		if i < done {
			mark = "x"
		}
		fmt.Fprintf(&b, "- [%s] %d. %s\n", mark, i+1, s)
	}
	fmt.Fprintf(&b, "\nEspecificación: `%s`\n", specPath)
	return b.String()
}
