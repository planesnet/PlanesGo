package bugreport

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The interface is minified (one line of thousands of columns), so its stack frames point nowhere useful. The build
// leaves hidden source maps next to the bundles (vite «sourcemap: 'hidden'»: not referenced from the code), and here
// each frame «…/assets/index-X.js:1:2345» becomes the original file and line, the ones an agent can open.

// frameJS: a frame of the bundle in a browser stack.
var frameJS = regexp.MustCompile(`(?:https?://[^/\s]+)?/(assets/[\w.-]+\.js):(\d+):(\d+)`)

// segment: one mapping of a generated line (0-based).
type segment struct{ col, src, line, srcCol, name int }

type sourceMap struct {
	sources []string
	names   []string
	lines   [][]segment
}

// Maps resolves the frames of the interface with the source maps of files (the embedded web/dist).
type Maps struct {
	files fs.FS
	root  string // repository path of the bundles («planesgo-code/web/dist/assets»): the sources are relative to it
	mu    sync.Mutex
	cache map[string]*sourceMap // nil value: that bundle has no map
}

// NewMaps: files is the compiled interface (nil: frames are left as they are).
func NewMaps(files fs.FS, root string) *Maps {
	return &Maps{files: files, root: root, cache: map[string]*sourceMap{}}
}

// Resolve rewrites every bundle frame of stack as «file:line (name)»; frames it cannot map stay as they were.
func (m *Maps) Resolve(stack string) string {
	if m == nil || m.files == nil {
		return stack
	}
	return frameJS.ReplaceAllStringFunc(stack, func(f string) string {
		p := frameJS.FindStringSubmatch(f)
		line, _ := strconv.Atoi(p[2])
		col, _ := strconv.Atoi(p[3])
		if sm := m.load(p[1]); sm != nil {
			if s, ok := sm.find(line-1, col-1); ok {
				out := fmt.Sprintf("%s:%d:%d", path.Join(m.root, sm.sources[s.src]), s.line+1, s.srcCol+1)
				if s.name >= 0 && s.name < len(sm.names) {
					out += " (" + sm.names[s.name] + ")"
				}
				return out
			}
		}
		return f
	})
}

func (m *Maps) load(bundle string) *sourceMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sm, ok := m.cache[bundle]; ok {
		return sm
	}
	var sm *sourceMap
	if b, err := fs.ReadFile(m.files, bundle+".map"); err == nil {
		sm, _ = parseMap(b)
	}
	m.cache[bundle] = sm
	return sm
}

func parseMap(b []byte) (*sourceMap, error) {
	var raw struct {
		Version  int      `json:"version"`
		Sources  []string `json:"sources"`
		Names    []string `json:"names"`
		Mappings string   `json:"mappings"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.Version != 3 {
		return nil, fmt.Errorf("source map v%d", raw.Version)
	}
	sm := &sourceMap{sources: raw.Sources, names: raw.Names}
	src, line, srcCol, name := 0, 0, 0, 0 // relative across the whole file; the column restarts on each line
	for _, l := range strings.Split(raw.Mappings, ";") {
		var segs []segment
		col := 0
		for _, g := range strings.Split(l, ",") {
			if g == "" {
				continue
			}
			v, err := decodeVLQ(g)
			if err != nil {
				return nil, err
			}
			col += v[0]
			if len(v) < 4 {
				continue // a segment with no source
			}
			src, line, srcCol = src+v[1], line+v[2], srcCol+v[3]
			n := -1
			if len(v) >= 5 {
				name += v[4]
				n = name
			}
			if src < 0 || src >= len(sm.sources) {
				return nil, fmt.Errorf("source map: fuente %d", src)
			}
			segs = append(segs, segment{col, src, line, srcCol, n})
		}
		sm.lines = append(sm.lines, segs)
	}
	return sm, nil
}

// find: the last mapping of the line at or before the column.
func (sm *sourceMap) find(line, col int) (segment, bool) {
	if line < 0 || line >= len(sm.lines) {
		return segment{}, false
	}
	segs := sm.lines[line]
	i := sort.Search(len(segs), func(i int) bool { return segs[i].col > col }) - 1
	if i < 0 {
		return segment{}, false
	}
	return segs[i], true
}

const base64VLQ = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// decodeVLQ decodes the base64 VLQ numbers of one segment.
func decodeVLQ(s string) ([]int, error) {
	var out []int
	v, shift := 0, 0
	for i := 0; i < len(s); i++ {
		d := strings.IndexByte(base64VLQ, s[i])
		if d < 0 {
			return nil, fmt.Errorf("source map: carácter %q", s[i])
		}
		v += (d & 31) << shift
		if d&32 != 0 {
			shift += 5
			if shift > 30 {
				return nil, fmt.Errorf("source map: número demasiado largo")
			}
			continue
		}
		n := v >> 1
		if v&1 != 0 {
			n = -n
		}
		out = append(out, n)
		v, shift = 0, 0
	}
	if shift != 0 {
		return nil, fmt.Errorf("source map: segmento cortado")
	}
	return out, nil
}
