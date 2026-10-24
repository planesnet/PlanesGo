package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// What the automatic error reports of pgocode (internal/bugreport) need from GitHub: find the open issue of a
// failure, open a new one, comment on it and know whether the user can use the repository at all.

// ReportLabel marks the issues opened by pgocode for its own failures.
const ReportLabel = "pgocode-error"

// FindOpenIssue: the number of an open issue whose body contains word (0 if there is none).
func (l *Lector) FindOpenIssue(ctx context.Context, repo, word string) (int, error) {
	if !nombreRepo.MatchString(repo) || strings.ContainsAny(word, " \"\n") || word == "" {
		return 0, errors.New("búsqueda no válida")
	}
	b, err := gh(ctx, "", "issue", "list", "-R", repo, "--state", "open", "--search", word+" in:body", "--json", "number", "--limit", "1")
	if err != nil {
		return 0, err
	}
	var found []struct{ Number int }
	if err := json.Unmarshal(b, &found); err != nil {
		return 0, err
	}
	if len(found) == 0 {
		return 0, nil
	}
	return found[0].Number, nil
}

// OpenReportIssue opens an issue with the ReportLabel (created the first time if the repository lacks it).
func (l *Lector) OpenReportIssue(ctx context.Context, repo, title, body string) (int, string, error) {
	if !nombreRepo.MatchString(repo) || strings.TrimSpace(title) == "" {
		return 0, "", errors.New("issue no válida")
	}
	args := []string{"issue", "create", "-R", repo, "--title", title, "--body-file", "-", "--label", ReportLabel}
	out, err := gh(ctx, body, args...)
	if err != nil && strings.Contains(err.Error(), ReportLabel) { // the label does not exist yet
		gh(ctx, "", "label", "create", ReportLabel, "-R", repo, "--color", "B60205", "--description", "Fallo de pgocode informado automáticamente")
		out, err = gh(ctx, body, args...)
	}
	if err != nil {
		return 0, "", err
	}
	url := strings.TrimSpace(string(out))
	n, err := strconv.Atoi(url[strings.LastIndex(url, "/")+1:])
	if err != nil {
		return 0, "", fmt.Errorf("GitHub no ha devuelto el número de la issue: %q", url)
	}
	l.forget(repo)
	return n, url, nil
}

// CommentIssue adds a comment to an issue (the text goes through stdin).
func (l *Lector) CommentIssue(ctx context.Context, repo string, n int, body string) error {
	if err := checkRef(repo, n); err != nil {
		return err
	}
	_, err := gh(ctx, body, "issue", "comment", fmt.Sprint(n), "-R", repo, "--body-file", "-")
	return err
}

// CanUse: whether the gh account of this computer can read the repository (and so open issues in it).
func (l *Lector) CanUse(ctx context.Context, repo string) bool {
	if !nombreRepo.MatchString(repo) {
		return false
	}
	b, err := gh(ctx, "", "repo", "view", repo, "--json", "viewerPermission", "-q", ".viewerPermission")
	p := strings.TrimSpace(string(b))
	return err == nil && p != "" && p != "NONE"
}
