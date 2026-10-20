// Package github lee el estado de un repositorio de GitHub (issues y pull requests) con la sesión de «gh» del
// usuario, en una sola consulta GraphQL, y lo clasifica para el flujo de pgocode: peticiones nuevas frente a
// mantenimiento, y la fase de cada issue (por empezar, en desarrollo, en revisión, en test, cerrada).
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var nombreRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

// DeRemoto saca «dueño/repo» de un remoto de GitHub (https o ssh); vacío si no es de GitHub.
func DeRemoto(remoto string) string {
	r := strings.TrimSuffix(strings.TrimRight(remoto, "/"), ".git")
	for _, p := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if strings.HasPrefix(r, p) {
			if n := strings.TrimPrefix(r, p); nombreRepo.MatchString(n) {
				return n
			}
		}
	}
	return ""
}

type Etiqueta struct {
	Nombre string `json:"nombre"`
	Color  string `json:"color"`
}

type PRCorta struct {
	Numero   int    `json:"numero"`
	Estado   string `json:"estado"` // OPEN | CLOSED | MERGED
	Borrador bool   `json:"borrador"`
}

type Issue struct {
	Numero      int        `json:"numero"`
	Titulo      string     `json:"titulo"`
	URL         string     `json:"url"`
	Estado      string     `json:"estado"` // OPEN | CLOSED
	Tipo        string     `json:"tipo"`   // Feature | Bug | Task | ""
	Clase       string     `json:"clase"`  // peticion | mantenimiento | sin-clasificar
	Fase        string     `json:"fase"`   // por-empezar | desarrollo | revision | test | cerrada
	Etiquetas   []Etiqueta `json:"etiquetas"`
	Autor       string     `json:"autor"`
	Asignados   []string   `json:"asignados"`
	Comentarios int        `json:"comentarios"`
	Hito        string     `json:"hito,omitempty"`
	PRs         []PRCorta  `json:"prs"`
	Creada      time.Time  `json:"creada"`
	Actualizada time.Time  `json:"actualizada"`
	Cerrada     time.Time  `json:"cerrada,omitzero"`
}

type PR struct {
	Numero      int        `json:"numero"`
	Titulo      string     `json:"titulo"`
	URL         string     `json:"url"`
	Estado      string     `json:"estado"` // OPEN | CLOSED | MERGED
	Borrador    bool       `json:"borrador"`
	Rama        string     `json:"rama"`
	Base        string     `json:"base"`
	Autor       string     `json:"autor"`
	Revision    string     `json:"revision"`   // APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED | ""
	Fusionable  string     `json:"fusionable"` // MERGEABLE | CONFLICTING | UNKNOWN
	Checks      string     `json:"checks"`     // SUCCESS | FAILURE | ERROR | PENDING | EXPECTED | ""
	Mas         int        `json:"mas"`        // líneas añadidas
	Menos       int        `json:"menos"`      // líneas quitadas
	Ficheros    int        `json:"ficheros"`
	Commits     int        `json:"commits"`
	Comentarios int        `json:"comentarios"`
	Issues      []int      `json:"issues"` // issues que cierra
	Etiquetas   []Etiqueta `json:"etiquetas"`
	Creada      time.Time  `json:"creada"`
	Actualizada time.Time  `json:"actualizada"`
	Fusionada   time.Time  `json:"fusionada,omitzero"`
}

type Estado struct {
	Repo      string    `json:"repo"`
	URL       string    `json:"url"`
	Defecto   string    `json:"defecto"` // rama por defecto
	Issues    []Issue   `json:"issues"`  // abiertas y cerradas en los últimos días
	PRs       []PR      `json:"prs"`     // abiertas y fusionadas/cerradas recientes
	Abiertas  int       `json:"abiertas"`
	Cerradas  int       `json:"cerradas"`
	PRsAbiert int       `json:"prs_abiertas"`
	Leido     time.Time `json:"leido"`
}

const consulta = `query($o:String!,$n:String!){repository(owner:$o,name:$n){
 url defaultBranchRef{name}
 abiertas:issues(states:OPEN){totalCount} cerradas:issues(states:CLOSED){totalCount}
 prsAbiertas:pullRequests(states:OPEN){totalCount}
 issuesAbiertas:issues(states:OPEN,first:100,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{...I}}
 issuesCerradas:issues(states:CLOSED,first:15,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{...I}}
 prs:pullRequests(states:OPEN,first:50,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{...P}}
 prsHechas:pullRequests(states:[MERGED,CLOSED],first:10,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{...P}}
}}
fragment I on Issue{number title url state createdAt updatedAt closedAt author{login} issueType{name}
 assignees(first:5){nodes{login}} labels(first:10){nodes{name color}} comments{totalCount} milestone{title}
 closedByPullRequestsReferences(first:5,includeClosedPrs:true){nodes{number state isDraft}}}
fragment P on PullRequest{number title url state isDraft headRefName baseRefName createdAt updatedAt mergedAt
 author{login} reviewDecision mergeable additions deletions changedFiles commits{totalCount} comments{totalCount}
 labels(first:10){nodes{name color}} closingIssuesReferences(first:10){nodes{number}}
 ultimo:commits(last:1){nodes{commit{statusCheckRollup{state}}}}}`

type nodosEtiq struct {
	Nodes []struct{ Name, Color string } `json:"nodes"`
}

func (n nodosEtiq) lista() []Etiqueta {
	l := []Etiqueta{}
	for _, x := range n.Nodes {
		l = append(l, Etiqueta{x.Name, x.Color})
	}
	return l
}

type issueGQL struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ClosedAt  time.Time `json:"closedAt"`
	Author    struct{ Login string }
	IssueType *struct{ Name string }
	Assignees struct{ Nodes []struct{ Login string } }
	Labels    nodosEtiq
	Comments  struct{ TotalCount int }
	Milestone *struct{ Title string }
	PRs       struct {
		Nodes []struct {
			Number  int
			State   string
			IsDraft bool
		}
	} `json:"closedByPullRequestsReferences"`
}

type prGQL struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	State          string    `json:"state"`
	IsDraft        bool      `json:"isDraft"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	MergedAt       time.Time `json:"mergedAt"`
	Author         struct{ Login string }
	ReviewDecision string `json:"reviewDecision"`
	Mergeable      string `json:"mergeable"`
	Additions      int
	Deletions      int
	ChangedFiles   int
	Commits        struct{ TotalCount int }
	Comments       struct{ TotalCount int }
	Labels         nodosEtiq
	Closing        struct{ Nodes []struct{ Number int } } `json:"closingIssuesReferences"`
	Ultimo         struct {
		Nodes []struct {
			Commit struct {
				Rollup *struct{ State string } `json:"statusCheckRollup"`
			}
		}
	}
}

// Clasifica: el tipo de issue de GitHub manda; si no hay, las etiquetas (en inglés o en español).
func clase(tipo string, et []Etiqueta) string {
	switch strings.ToLower(tipo) {
	case "feature":
		return "peticion"
	case "bug", "task":
		return "mantenimiento"
	}
	for _, e := range et {
		switch strings.ToLower(e.Nombre) {
		case "enhancement", "mejora", "feature", "petición", "peticion", "nueva funcionalidad":
			return "peticion"
		case "bug", "corrección", "correccion", "mantenimiento", "maintenance", "fix", "error":
			return "mantenimiento"
		}
	}
	return "sin-clasificar"
}

// Fase de una issue: cerrada; en test (etiqueta); en revisión (PR abierta lista); en desarrollo (etiqueta o PR
// en borrador); o por empezar.
func fase(estado string, et []Etiqueta, prs []PRCorta) string {
	if estado == "CLOSED" {
		return "cerrada"
	}
	tiene := func(nombres ...string) bool {
		for _, e := range et {
			for _, n := range nombres {
				if strings.EqualFold(e.Nombre, n) {
					return true
				}
			}
		}
		return false
	}
	if tiene("en-test", "en test", "testing", "qa", "en-dev-alfa") {
		return "test"
	}
	for _, p := range prs {
		if p.Estado == "OPEN" && !p.Borrador {
			return "revision"
		}
	}
	if tiene("en-desarrollo", "en desarrollo", "in progress", "wip") {
		return "desarrollo"
	}
	for _, p := range prs {
		if p.Estado == "OPEN" {
			return "desarrollo"
		}
	}
	return "por-empezar"
}

func convierteIssue(x issueGQL) Issue {
	i := Issue{Numero: x.Number, Titulo: x.Title, URL: x.URL, Estado: x.State, Etiquetas: x.Labels.lista(), Autor: x.Author.Login,
		Asignados: []string{}, Comentarios: x.Comments.TotalCount, PRs: []PRCorta{}, Creada: x.CreatedAt, Actualizada: x.UpdatedAt, Cerrada: x.ClosedAt}
	if x.IssueType != nil {
		i.Tipo = x.IssueType.Name
	}
	if x.Milestone != nil {
		i.Hito = x.Milestone.Title
	}
	for _, a := range x.Assignees.Nodes {
		i.Asignados = append(i.Asignados, a.Login)
	}
	for _, p := range x.PRs.Nodes {
		i.PRs = append(i.PRs, PRCorta{p.Number, p.State, p.IsDraft})
	}
	i.Clase = clase(i.Tipo, i.Etiquetas)
	i.Fase = fase(i.Estado, i.Etiquetas, i.PRs)
	return i
}

func conviertePR(x prGQL) PR {
	p := PR{Numero: x.Number, Titulo: x.Title, URL: x.URL, Estado: x.State, Borrador: x.IsDraft, Rama: x.HeadRefName, Base: x.BaseRefName,
		Autor: x.Author.Login, Revision: x.ReviewDecision, Fusionable: x.Mergeable, Mas: x.Additions, Menos: x.Deletions,
		Ficheros: x.ChangedFiles, Commits: x.Commits.TotalCount, Comentarios: x.Comments.TotalCount, Issues: []int{},
		Etiquetas: x.Labels.lista(), Creada: x.CreatedAt, Actualizada: x.UpdatedAt, Fusionada: x.MergedAt}
	for _, n := range x.Closing.Nodes {
		p.Issues = append(p.Issues, n.Number)
	}
	if len(x.Ultimo.Nodes) > 0 && x.Ultimo.Nodes[0].Commit.Rollup != nil {
		p.Checks = x.Ultimo.Nodes[0].Commit.Rollup.State
	}
	return p
}

// Interpreta la respuesta de la consulta.
func interpreta(repo string, b []byte) (*Estado, error) {
	var r struct {
		Data struct {
			Repository *struct {
				URL                             string                 `json:"url"`
				Defecto                         *struct{ Name string } `json:"defaultBranchRef"`
				Abiertas, Cerradas, PrsAbiertas struct{ TotalCount int }
				IssuesAbiertas, IssuesCerradas  struct{ Nodes []issueGQL }
				Prs, PrsHechas                  struct{ Nodes []prGQL }
			} `json:"repository"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if len(r.Errors) > 0 && r.Data.Repository == nil {
		return nil, errors.New(r.Errors[0].Message)
	}
	x := r.Data.Repository
	if x == nil {
		return nil, fmt.Errorf("no se encuentra el repositorio %s en GitHub", repo)
	}
	e := &Estado{Repo: repo, URL: x.URL, Abiertas: x.Abiertas.TotalCount, Cerradas: x.Cerradas.TotalCount,
		PRsAbiert: x.PrsAbiertas.TotalCount, Issues: []Issue{}, PRs: []PR{}, Leido: time.Now()}
	if x.Defecto != nil {
		e.Defecto = x.Defecto.Name
	}
	for _, i := range append(x.IssuesAbiertas.Nodes, x.IssuesCerradas.Nodes...) {
		e.Issues = append(e.Issues, convierteIssue(i))
	}
	for _, p := range append(x.Prs.Nodes, x.PrsHechas.Nodes...) {
		e.PRs = append(e.PRs, conviertePR(p))
	}
	return e, nil
}

// Lector guarda en caché lo leído (un minuto), para no repetir la consulta en cada vistazo.
type Lector struct {
	mu    sync.Mutex
	cache map[string]*Estado
}

// Lee el estado de repo («dueño/nombre»); recargar ignora la caché.
func (l *Lector) Lee(ctx context.Context, repo string, recargar bool) (*Estado, error) {
	if !nombreRepo.MatchString(repo) {
		return nil, errors.New("repositorio no válido")
	}
	l.mu.Lock()
	if l.cache == nil {
		l.cache = map[string]*Estado{}
	}
	if e := l.cache[repo]; e != nil && !recargar && time.Since(e.Leido) < time.Minute {
		l.mu.Unlock()
		return e, nil
	}
	l.mu.Unlock()
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errors.New("falta la orden «gh» de GitHub (instálala y entra con «gh auth login»)")
	}
	dueno, nombre, _ := strings.Cut(repo, "/")
	ctx, cancela := context.WithTimeout(ctx, 30*time.Second)
	defer cancela()
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "-f", "query="+consulta, "-F", "o="+dueno, "-F", "n="+nombre)
	var errSalida strings.Builder
	cmd.Stderr = &errSalida
	b, err := cmd.Output()
	if err != nil && len(b) == 0 {
		return nil, fmt.Errorf("GitHub: %s", strings.TrimSpace(errSalida.String()+" "+err.Error()))
	}
	e, err := interpreta(repo, b)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.cache[repo] = e
	l.mu.Unlock()
	return e, nil
}
