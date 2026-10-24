// Package sysskills holds pgocode's own skills (the system ones): what each agent of an automatic session needs to
// know to do its part, written as Agent Skills (a folder with a SKILL.md and its chapters), embedded in the binary and
// read only. They are kept apart from the skills of the user or of a project, which pgocode does not use for this.
//
// A skill is read like a book: its SKILL.md (a frontmatter with name and description, and the essentials) goes to
// the agent with the index of its chapters (capitulos/*.md, each with a description of when to read it); the agent
// reads only the chapters its task needs. Install leaves them in the agent's folder so that any agent can read them
// with its own tools.
package sysskills

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed sistema
var files embed.FS

// Dir: where Install leaves the system skills inside an agent's folder (.pgocode is kept out of git).
const Dir = ".pgocode/skills-sistema"

// Skill: a system skill. Body is its SKILL.md without the frontmatter.
type Skill struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Body        string    `json:"body"`
	Chapters    []Chapter `json:"chapters"`
}

// Chapter: one chapter of a skill; Path is where Install left it (relative to the agent's folder).
type Chapter struct {
	Name        string `json:"name"`
	Description string `json:"description"` // when to read it
	Path        string `json:"path,omitempty"`
	body        string
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Names: the system skills.
func Names() []string {
	l, _ := fs.ReadDir(files, "sistema")
	var n []string
	for _, e := range l {
		if e.IsDir() {
			n = append(n, e.Name())
		}
	}
	return n
}

// Get: a system skill and its chapters (in file order).
func Get(name string) (Skill, error) {
	if !validName.MatchString(name) {
		return Skill{}, errors.New("nombre de skill no válido")
	}
	b, err := files.ReadFile(path.Join("sistema", name, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("no hay ninguna skill del sistema «%s»", name)
	}
	meta, body := frontmatter(string(b))
	if meta["name"] != name || meta["description"] == "" {
		return Skill{}, fmt.Errorf("la skill «%s» no tiene nombre o descripción", name)
	}
	sk := Skill{Name: name, Description: meta["description"], Body: body, Chapters: []Chapter{}}
	l, _ := fs.ReadDir(files, path.Join("sistema", name, "capitulos"))
	for _, e := range l {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		c, _ := files.ReadFile(path.Join("sistema", name, "capitulos", e.Name()))
		m, text := frontmatter(string(c))
		sk.Chapters = append(sk.Chapters, Chapter{Name: strings.TrimSuffix(e.Name(), ".md"), Description: m["description"], body: text})
	}
	sort.Slice(sk.Chapters, func(i, j int) bool { return sk.Chapters[i].Name < sk.Chapters[j].Name })
	return sk, nil
}

// Install leaves the skill's files in dir (an agent's folder) under Dir and returns it with the chapters' paths.
// What was there is replaced: the copy is always the one of this pgocode.
func Install(name, dir string) (Skill, error) {
	sk, err := Get(name)
	if err != nil {
		return sk, err
	}
	rel := path.Join(Dir, name)
	base := filepath.Join(dir, filepath.FromSlash(rel))
	// The folder is a repository nobody checked: none of the folders on the way may be a link it brings (the files
	// would be written wherever it points).
	for p := dir; ; {
		next, ok := strings.CutPrefix(base, p+string(filepath.Separator))
		if !ok {
			break
		}
		first, _, _ := strings.Cut(next, string(filepath.Separator))
		p = filepath.Join(p, first)
		if st, err := os.Lstat(p); err == nil && !st.IsDir() {
			return sk, fmt.Errorf("%s no es una carpeta: no se instala la skill", p)
		}
	}
	if err := os.RemoveAll(base); err != nil {
		return sk, err
	}
	if err := os.MkdirAll(filepath.Join(base, "capitulos"), 0o755); err != nil {
		return sk, err
	}
	whole, _ := files.ReadFile(path.Join("sistema", name, "SKILL.md"))
	if err := os.WriteFile(filepath.Join(base, "SKILL.md"), whole, 0o644); err != nil {
		return sk, err
	}
	for i, c := range sk.Chapters {
		b, _ := files.ReadFile(path.Join("sistema", name, "capitulos", c.Name+".md"))
		if err := os.WriteFile(filepath.Join(base, "capitulos", c.Name+".md"), b, 0o644); err != nil {
			return sk, err
		}
		sk.Chapters[i].Path = path.Join(rel, "capitulos", c.Name+".md")
	}
	return sk, nil
}

// frontmatter splits a Markdown file into its «---» header (simple «key: value» lines) and its body.
func frontmatter(s string) (map[string]string, string) {
	meta := map[string]string{}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return meta, strings.TrimSpace(s)
	}
	head, body, ok := strings.Cut(s[4:], "\n---\n")
	if !ok {
		return meta, strings.TrimSpace(s)
	}
	for _, l := range strings.Split(head, "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok {
			meta[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return meta, strings.TrimSpace(body)
}
