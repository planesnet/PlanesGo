package proyectos

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"planesgo-code/internal/repo"
)

// LinkFile links a folder to its Odoo project: {"odoo_project_id": 589, "odoo_project_name": "…"}. legacyLinkFile is
// the name it had in PlanesGo (with fields of its own nobody reads any more): still read, and migrated to LinkFile
// where it is not in git (MigrateLinks).
const (
	LinkFile       = ".pgo.json"
	legacyLinkFile = ".planesgo.json"
)

// Link: the content of LinkFile.
type Link struct {
	ProjectID   int    `json:"odoo_project_id"`
	ProjectName string `json:"odoo_project_name,omitempty"`
}

// ReadLink: the project of a folder (LinkFile, or else the PlanesGo one); ok false if it has none.
func ReadLink(dir string) (Link, bool) {
	for _, name := range []string{LinkFile, legacyLinkFile} {
		if l, ok := readLinkFile(filepath.Join(dir, name)); ok {
			return l, true
		}
	}
	return Link{}, false
}

func readLinkFile(path string) (Link, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Link{}, false
	}
	var l Link
	if json.Unmarshal(b, &l) != nil || l.ProjectID <= 0 {
		return Link{}, false
	}
	return l, true
}

// WriteLink writes the folder's LinkFile, kept out of git locally (.git/info/exclude: nothing changes in the
// repository).
func WriteLink(dir string, l Link) error {
	if l.ProjectID <= 0 {
		return errors.New("falta el proyecto de Odoo")
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	tmp := filepath.Join(dir, LinkFile+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, LinkFile)); err != nil {
		return err
	}
	repo.ExcludeLocally(dir, LinkFile)
	return nil
}

// MigrateLinks turns the PlanesGo link files under roots into LinkFile (kept out of git). With removeOld, the old
// file goes too, unless it is in git (that is a change of the repository: it stays, and is still read); without
// it, both stay (versions of pgocode that only read the old name keep working). Returns the folders whose old file
// is gone and the ones where it stays. It can run again: what is done is not redone.
func MigrateLinks(roots []string, removeOld bool) (migrated, kept []string) {
	for _, dir := range linkDirs(roots, legacyLinkFile) {
		old := filepath.Join(dir, legacyLinkFile)
		l, ok := readLinkFile(old)
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, LinkFile)); err != nil {
			if err := WriteLink(dir, Link{ProjectID: l.ProjectID, ProjectName: l.ProjectName}); err != nil {
				log.Printf("vínculo de %s: %v", dir, err)
				continue
			}
		}
		if !removeOld || repo.Tracked(old) {
			kept = append(kept, dir)
			continue
		}
		if err := os.Remove(old); err != nil {
			log.Printf("vínculo de %s: %v", dir, err)
			continue
		}
		migrated = append(migrated, dir)
	}
	return migrated, kept
}

// linkDirs: the folders under roots (up to Profundidad, without hidden or generated ones) with a file of one of
// those names, each once.
func linkDirs(roots []string, names ...string) []string {
	var l []string
	seen := map[string]bool{}
	for _, r := range roots {
		r = filepath.Clean(r)
		base := strings.Count(r, string(filepath.Separator))
		filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				// Without hidden folders (.git, .pgocode, .claude with the subagents' copies…).
				if p != r && (saltar[d.Name()] || strings.HasPrefix(d.Name(), ".") || strings.Count(p, string(filepath.Separator))-base > Profundidad) {
					return fs.SkipDir
				}
				return nil
			}
			if dir := filepath.Dir(p); slices.Contains(names, d.Name()) && !seen[dir] {
				seen[dir] = true
				l = append(l, dir)
			}
			return nil
		})
	}
	return l
}
