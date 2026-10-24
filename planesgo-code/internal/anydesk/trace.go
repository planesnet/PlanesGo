// Package anydesk follows AnyDesk's own trace files to know when its sessions start and end, both outgoing (this
// computer controls another one) and incoming (someone controls this one), without its REST API.
//
// Every AnyDesk process writes lines like
//
//	info 2026-10-23 13:20:15.521      front   main 1987590 1987590                     unix_app.frontend - Sending a connection request for address 123456789.
//	info 2026-10-23 13:20:20.655      front   ctrl 1987590 1987688                  app.frontend_session - Session started.
//	info 2026-09-21 09:59:46.282    backend   wrk1 1382027 1382040                   app.backend_session - Incoming session request: Ana (123456789)
//
// (level, date, time, process role, thread, PID, TID, an optional fiber, component, « - », message). The session
// lines come from app.frontend_session (outgoing) and app.backend_session (incoming), one session at a time per
// process; the service's own lines (app.session, by fiber) are not used, so the same session is never counted twice
// when both the user's trace and the service's are followed.
package anydesk

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Line: one parsed trace line.
type Line struct {
	Time      time.Time
	Level     string
	Role      string // front, frontend, backend, service, gsvc…
	PID       int
	Component string
	Message   string
}

var lineRe = regexp.MustCompile(`^\s*([a-z]+)\s+(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d+)\s+(?:\d+\s+)?(\S+) - (.*)$`)

// ParseLine reads one trace line (AnyDesk writes the date in UTC); ok false if it does not have the expected format.
func ParseLine(s string) (Line, bool) {
	m := lineRe.FindStringSubmatch(strings.TrimRight(s, "\r\n"))
	if m == nil {
		return Line{}, false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05.000", m[2], time.UTC) // AnyDesk writes UTC (checked against the file times)
	if err != nil {
		return Line{}, false
	}
	pid, err := strconv.Atoi(m[5])
	if err != nil {
		return Line{}, false
	}
	return Line{Time: t, Level: m[1], Role: m[3], PID: pid, Component: m[7], Message: strings.TrimSpace(m[8])}, true
}

// ignorable: lines without information that are not a format problem (blank lines, the banners of
// «* AnyDesk Linux Startup *»).
func ignorable(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.Trim(s, "* ") == ""
}

// Direction of a session.
type Direction string

const (
	Outgoing Direction = "outgoing" // from this computer to another one
	Incoming Direction = "incoming" // from another computer to this one
)

// Session: an AnyDesk session seen in a trace.
type Session struct {
	Key       string // unique: the trace, the process and the session's number in it
	Direction Direction
	Peer      string // the other side's AnyDesk ID or alias, normalized ("" if the trace did not say)
	PeerName  string // incoming: the name the other side shows ("" if none)
	PID       int
	Start     time.Time
}

// Change: a session that has just started (Open) or ended.
type Change struct {
	Session Session
	Open    bool
	Time    time.Time
}

type pending struct {
	peer, name string
}

// Tracker turns trace lines into session changes. Not safe for concurrent use.
type Tracker struct {
	Source  string // which trace (part of the keys)
	Lines   int    // lines seen
	Unknown int    // lines with a format it does not understand
	pending map[int]pending
	open    map[int]*Session
	n       int
	newest  time.Time // the newest line read
	skipTo  time.Time // after Rewind: lines up to this time were already read
}

// NewTracker for the trace source (a path or any name that tells traces apart).
func NewTracker(source string) *Tracker {
	return &Tracker{Source: source, pending: map[int]pending{}, open: map[int]*Session{}}
}

// Feed reads one line and returns the sessions it starts or ends.
func (t *Tracker) Feed(raw string) []Change {
	if ignorable(raw) {
		return nil
	}
	t.Lines++
	l, ok := ParseLine(raw)
	if !ok {
		t.Unknown++
		return nil
	}
	if !t.skipTo.IsZero() {
		if !l.Time.After(t.skipTo) {
			return nil
		}
		t.skipTo = time.Time{}
	}
	if l.Time.After(t.newest) {
		t.newest = l.Time
	}
	msg, comp := l.Message, l.Component
	switch {
	case strings.HasPrefix(msg, "Sending a connection request for address "):
		t.pending[l.PID] = pending{peer: NormalizePeer(strings.TrimPrefix(msg, "Sending a connection request for address "))}
	case strings.HasSuffix(comp, "frontend_session") && msg == "Session started.":
		return t.start(l, Outgoing)
	case strings.HasSuffix(comp, "backend_session") && strings.HasPrefix(msg, "Incoming session request:"):
		name, peer := incomingRequest(strings.TrimPrefix(msg, "Incoming session request:"))
		t.pending[l.PID] = pending{peer: peer, name: name}
	case strings.HasSuffix(comp, "backend_session") && strings.HasPrefix(msg, "Session started"):
		return t.start(l, Incoming)
	case (strings.HasSuffix(comp, "frontend_session") || strings.HasSuffix(comp, "backend_session")) && msg == "Session stopped.":
		return t.stop(l.PID, l.Time)
	case strings.HasSuffix(comp, ".frontend") && msg == "Frontend shutdown.",
		strings.HasSuffix(comp, ".backend") && (msg == "Backend is shutting down." || msg == "Backend has been stopped." || msg == "Deleting backend."),
		comp == "main" && strings.HasPrefix(msg, "Exiting"):
		delete(t.pending, l.PID)
		return t.stop(l.PID, l.Time)
	}
	return nil
}

func (t *Tracker) start(l Line, d Direction) []Change {
	ch := t.stop(l.PID, l.Time) // one session at a time per process: a start without a stop ends the previous one
	p := t.pending[l.PID]
	delete(t.pending, l.PID)
	t.n++
	s := &Session{Key: fmt.Sprintf("%s#%d#%d", t.Source, l.PID, t.n), Direction: d, Peer: p.peer, PeerName: p.name, PID: l.PID, Start: l.Time}
	t.open[l.PID] = s
	return append(ch, Change{Session: *s, Open: true, Time: l.Time})
}

func (t *Tracker) stop(pid int, at time.Time) []Change {
	s := t.open[pid]
	if s == nil {
		return nil
	}
	delete(t.open, pid)
	return []Change{{Session: *s, Time: at}}
}

// Rewind: the trace was cut or replaced and is read again from its start; its lines up to the newest one already
// read are skipped (only until a newer one comes: within the trace, lines from different threads are not always in
// order).
func (t *Tracker) Rewind() { t.skipTo = t.newest }

// Open: the sessions open now.
func (t *Tracker) Open() []Session {
	l := make([]Session, 0, len(t.open))
	for _, s := range t.open {
		l = append(l, *s)
	}
	return l
}

// CloseDead ends the open sessions whose process no longer exists (AnyDesk closed without writing it).
func (t *Tracker) CloseDead(alive func(pid int) bool, now time.Time) []Change {
	var ch []Change
	for pid := range t.open {
		if !alive(pid) {
			delete(t.pending, pid)
			ch = append(ch, t.stop(pid, now)...)
		}
	}
	return ch
}

// incomingRequest reads « NAME (ID)» («-» or nothing: no name).
func incomingRequest(s string) (name, peer string) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "("); i >= 0 && strings.HasSuffix(s, ")") {
		name, peer = strings.TrimSpace(s[:i]), s[i+1:len(s)-1]
	} else {
		peer = s
	}
	if name == "-" {
		name = ""
	}
	return name, NormalizePeer(peer)
}

// NormalizePeer: an AnyDesk address as the traces and pgocode compare it: without the final dot, the terminal user
// («123456789/dominio/usuario»), quotes or spaces between the digit groups; aliases in lowercase.
func NormalizePeer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	s = strings.Trim(s, `'"`)
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if d := strings.ReplaceAll(s, " ", ""); d != "" && strings.Trim(d, "0123456789") == "" {
		return d
	}
	return strings.ToLower(s)
}

var (
	idRe    = regexp.MustCompile(`^[0-9]{6,12}$`)
	aliasRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}@[a-z0-9][a-z0-9_-]{0,31}$`)
)

// ValidID: the AnyDesk ID (digits, spaces between groups allowed) or alias («nombre@ad») the person typed, ready
// to be an argument of the client; ok false if it is anything else (never starts with «-»).
func ValidID(s string) (string, bool) {
	s = strings.TrimSpace(s)
	// «123 456 789/dominio/usuario», as AnyDesk shows a computer with its user: only the ID is used.
	if id, _, ok := strings.Cut(s, "/"); ok {
		if d := strings.ReplaceAll(id, " ", ""); d != "" && strings.Trim(d, "0123456789") == "" {
			s = id
		}
	}
	if strings.ContainsAny(s, "/.'\"\t\r\n") && !aliasRe.MatchString(strings.ToLower(s)) {
		return "", false
	}
	n := NormalizePeer(s)
	if idRe.MatchString(n) || aliasRe.MatchString(n) {
		return n, true
	}
	return "", false
}
