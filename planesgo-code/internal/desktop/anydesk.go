package desktop

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"time"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/proceso"
)

// AnyDesk: the helper opens the installed client (AnyDesk asks for the password itself; pgocode never sees it) and
// follows AnyDesk's traces (internal/anydesk) to know when its sessions, outgoing or incoming, start and end.

var (
	anydeskPoll  = 2 * time.Second // how often the traces are read (shorter in the tests)
	anydeskAlive = anydesk.Alive   // whether an AnyDesk process is still running (replaced in the tests)
)

const (
	anydeskBacklog    = 8 << 20          // on start, the end of each trace is read again to know what is open
	anydeskLaunchLife = 10 * time.Minute // a launch from pgocode waits this long for its session to start
)

// anydeskLaunch: a client opened from pgocode, waiting for its session to start.
type anydeskLaunch struct {
	peer    string
	project int
	at      time.Time
}

// takeLaunch: the project of a session that starts, from the launch from pgocode it belongs to (0 if none), and
// the launches still waiting. Old launches are dropped; an outgoing session without a peer takes the oldest one.
func takeLaunch(launches []anydeskLaunch, s anydesk.Session, now time.Time) (int, []anydeskLaunch) {
	project := 0
	kept := make([]anydeskLaunch, 0, len(launches))
	for _, l := range launches {
		switch {
		case now.Sub(l.at) > anydeskLaunchLife:
		case project == 0 && s.Direction == anydesk.Outgoing && (l.peer == s.Peer || s.Peer == ""):
			project = l.project
		default:
			kept = append(kept, l)
		}
	}
	return project, kept
}

// startAnyDesk opens AnyDesk on peer (an ID or alias). Never through a shell; the ID, validated, is the only
// argument, and nothing of pgocode's environment reaches it.
func startAnyDesk(peer string) (string, error) {
	id, ok := anydesk.ValidID(peer)
	if !ok {
		return "", errors.New("el ID de AnyDesk son solo cifras (p. ej. 123 456 789) o un alias como equipo@ad")
	}
	bin := anydesk.Client()
	if bin == "" {
		return "", errors.New("AnyDesk no está instalado en este equipo")
	}
	cmd := exec.CommandContext(context.Background(), bin, id)
	proceso.GrupoPropio(cmd)
	cmd.Env = proceso.CleanEnviron()
	if err := cmd.Start(); err != nil {
		return "", errors.New("no arranca AnyDesk: " + err.Error())
	}
	go cmd.Wait() // it hands the connection to AnyDesk's service and ends (or stays as its window)
	return id, nil
}

// anydeskTrace: one followed trace.
type anydeskTrace struct {
	f       *anydesk.Follower
	t       *anydesk.Tracker
	first   bool
	warned  int
	lastErr string
}

// poll reads the trace's new lines and returns its changes. The first time (initial), it only learns what is open
// now (the past is not reported again).
func (x *anydeskTrace) poll() (ch []anydesk.Change, initial bool) {
	lines, err := x.f.Poll()
	if err != nil {
		if e := err.Error(); e != x.lastErr {
			x.lastErr = e
			log.Printf("anydesk: no se puede leer %s: %v", x.f.Path, err)
		}
	} else {
		x.lastErr = ""
	}
	for i, l := range lines {
		if i == x.f.ResetAt {
			x.t.Rewind()
		}
		ch = append(ch, x.t.Feed(l)...)
	}
	// A line cut when AnyDesk trims its trace is normal; more is a format that changed (another AnyDesk version).
	if x.t.Unknown >= 5 && x.t.Unknown >= 2*x.warned {
		log.Printf("anydesk: %s: %d de %d líneas con un formato que no se entiende (¿otra versión de AnyDesk?)", x.f.Path, x.t.Unknown, x.t.Lines)
		x.warned = x.t.Unknown
	}
	if !x.first {
		x.first = true
		ch = nil
		for _, o := range x.t.Open() {
			ch = append(ch, anydesk.Change{Session: o, Open: true, Time: o.Start})
		}
		return ch, true
	}
	return ch, false
}

// watchAnyDesk follows AnyDesk's traces until ctx ends and hands apply the sessions that start and end (also those
// whose process died without saying it, checked every deadEvery).
func watchAnyDesk(ctx context.Context, files []string, deadEvery time.Duration, apply func(anydesk.Change)) {
	var traces []*anydeskTrace
	for _, p := range files {
		traces = append(traces, &anydeskTrace{f: anydesk.NewFollower(p, anydeskBacklog), t: anydesk.NewTracker(p)})
	}
	defer func() {
		for _, x := range traces {
			x.f.Close()
		}
	}()
	t := time.NewTicker(anydeskPoll)
	defer t.Stop()
	lastDead := time.Now()
	for {
		for _, x := range traces {
			ch, initial := x.poll()
			for _, c := range ch {
				if !initial || anydeskAlive(c.Session.PID) { // a process gone without saying it: it is over
					apply(c)
				}
			}
		}
		if now := time.Now(); now.Sub(lastDead) >= deadEvery {
			lastDead = now
			for _, x := range traces {
				for _, c := range x.t.CloseDead(anydeskAlive, now) {
					apply(c)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
