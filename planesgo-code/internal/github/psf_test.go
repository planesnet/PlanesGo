package github

import (
	"strings"
	"testing"
)

func TestParseIssueDetail(t *testing.T) {
	d, err := parseIssueDetail([]byte(`{"number":7,"title":"T","body":"B","url":"u","state":"OPEN","labels":[{"name":"bug"}],
		"comments":[{"author":{"login":"luis"},"body":"ojo","createdAt":"2026-10-24T10:00:00Z"}]}`))
	if err != nil || d.Number != 7 || d.Labels[0] != "bug" || d.Comments[0].Author != "luis" || d.Comments[0].Body != "ojo" {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestParseChecks(t *testing.T) {
	for j, q := range map[string]string{
		`{"state":"OPEN","statusCheckRollup":[]}`:                                                                                             "",
		`{"state":"OPEN","statusCheckRollup":[{"name":"go","status":"COMPLETED","conclusion":"SUCCESS"}]}`:                                    "SUCCESS",
		`{"state":"OPEN","statusCheckRollup":[{"name":"go","status":"IN_PROGRESS","conclusion":""}]}`:                                         "PENDING",
		`{"state":"OPEN","statusCheckRollup":[{"name":"go","status":"COMPLETED","conclusion":"FAILURE"},{"context":"ci","state":"SUCCESS"}]}`: "FAILURE",
		`{"state":"OPEN","statusCheckRollup":[{"context":"ci","state":"PENDING"}]}`:                                                           "PENDING",
	} {
		c, err := parseChecks([]byte(j))
		if err != nil || c.Checks != q {
			t.Errorf("%s: %v %q", j, err, c.Checks)
		}
	}
	c, _ := parseChecks([]byte(`{"state":"OPEN","statusCheckRollup":[{"name":"go","status":"COMPLETED","conclusion":"FAILURE"}]}`))
	if len(c.Failing) != 1 || c.Failing[0] != "go" {
		t.Errorf("failing: %v", c.Failing)
	}
}

func TestPSFBody(t *testing.T) {
	b := PSFBody(7, "Fix", ".psf/specs/issue-7-spec.md", 2)
	if !strings.HasPrefix(b, "Closes #7") || strings.Count(b, "- [x]") != 2 || strings.Count(b, "- [ ]") != 3 {
		t.Error(b)
	}
}
