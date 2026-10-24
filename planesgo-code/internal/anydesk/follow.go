package anydesk

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
)

const (
	followChunk = 64 << 10
	followMax   = 16 << 20 // read at most this much per Poll (the rest, in the next ones)
)

// Follower reads a file like «tail -F»: the complete lines appended since the last Poll, starting over when the file
// is truncated or replaced (rotation). Not safe for concurrent use.
type Follower struct {
	Path string
	// Backlog: on the first Poll, start this many bytes before the end (0: at the end); the first, cut line is
	// dropped.
	Backlog int64
	// ResetAt: after a Poll, where the lines of a truncated or replaced file start (-1: none was). Those may repeat
	// what was already read (AnyDesk keeps the end of its trace when it cuts it): see Tracker.Rewind.
	ResetAt int
	// keepOpen: where renaming an open file is allowed (not Windows: AnyDesk could not rotate it), the open file is
	// kept and drained before following the new one, so no line is lost on a rotation.
	keepOpen bool
	f        *os.File
	info     fs.FileInfo
	off      int64
	partial  []byte
	started  bool
	skip     bool
}

// NewFollower for path, starting backlog bytes before its current end.
func NewFollower(path string, backlog int64) *Follower {
	return &Follower{Path: path, Backlog: backlog, keepOpen: runtime.GOOS != "windows"}
}

// Close releases the open file.
func (f *Follower) Close() {
	if f.f != nil {
		f.f.Close()
		f.f = nil
	}
}

// Poll returns the new complete lines (without «\n»). A missing file is not an error: there is nothing yet.
func (f *Follower) Poll() ([]string, error) {
	var lines []string
	f.ResetAt = -1
	first := !f.started // a file that appears later is read from its start
	f.started = true
	st, err := os.Stat(f.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if f.info != nil {
		replaced := err != nil || !os.SameFile(f.info, st)
		if f.f != nil {
			if cur, e := f.f.Stat(); e == nil && cur.Size() < f.off {
				f.off, f.partial, f.ResetAt = 0, nil, 0 // truncated in place
			}
			var e error
			if lines, e = f.read(f.f, lines); e != nil {
				return lines, e
			}
		}
		if replaced {
			if len(f.partial) > 0 { // the old file's last line, without «\n»: complete now
				lines = append(lines, string(f.partial))
			}
			f.Close()
			f.info, f.off, f.partial, f.ResetAt = nil, 0, nil, len(lines)
		} else if f.f == nil && st.Size() < f.off {
			f.off, f.partial, f.ResetAt = 0, nil, len(lines)
		}
	}
	if err != nil {
		return lines, nil
	}
	if f.info == nil {
		f.info = st
		if first && st.Size() > f.Backlog {
			f.off, f.skip = st.Size()-f.Backlog, f.Backlog > 0
		}
	}
	if f.f == nil {
		h, e := os.Open(f.Path)
		if e != nil {
			if errors.Is(e, fs.ErrNotExist) {
				f.info = nil
				return lines, nil
			}
			return lines, e
		}
		if f.keepOpen {
			f.f = h
		} else {
			defer h.Close()
		}
		lines, err = f.read(h, lines)
		return lines, err
	}
	return lines, nil
}

// read appends the lines from f.off to the end of h.
func (f *Follower) read(h *os.File, lines []string) ([]string, error) {
	buf := make([]byte, followChunk)
	for total := 0; total < followMax; {
		n, err := h.ReadAt(buf, f.off)
		if n > 0 {
			total += n
			f.off += int64(n)
			data := append(f.partial, buf[:n]...)
			f.partial = nil
			for {
				i := bytes.IndexByte(data, '\n')
				if i < 0 {
					break
				}
				if f.skip {
					f.skip = false
				} else {
					lines = append(lines, string(bytes.TrimRight(data[:i], "\r")))
				}
				data = data[i+1:]
			}
			f.partial = append([]byte(nil), data...)
		}
		if err == io.EOF || n == 0 {
			return lines, nil
		}
		if err != nil {
			return lines, err
		}
	}
	return lines, nil
}
