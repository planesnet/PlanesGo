package proceso

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// EnRed: un programa del usuario que escucha en algún puerto (un Odoo, un servidor de desarrollo…), agrupado
// por su grupo de procesos (un Odoo con sus trabajadores es uno).
type EnRed struct {
	Grupo    int    `json:"grupo"`
	Pid      int    `json:"pid"`
	Puertos  []int  `json:"puertos"`
	Nombre   string `json:"nombre"`
	Orden    string `json:"orden"`
	Carpeta  string `json:"carpeta"`
	Segundos int64  `json:"segundos"` // cuánto lleva en marcha
}

var usuarioSS = regexp.MustCompile(`\("([^"]+)",pid=(\d+),`)
var puertoSS = regexp.MustCompile(`:(\d+)\s`)

// Escuchando: los programas del usuario con algún puerto TCP en escucha (sin pgocode).
func Escuchando() []EnRed {
	out, err := exec.Command("ss", "-ltnpH").Output()
	if err != nil {
		return nil
	}
	yo := os.Getpid()
	grupos := map[int]*EnRed{}
	for _, l := range strings.Split(string(out), "\n") {
		pm := puertoSS.FindStringSubmatch(l)
		if pm == nil {
			continue
		}
		puerto, _ := strconv.Atoi(pm[1])
		for _, u := range usuarioSS.FindAllStringSubmatch(l, -1) {
			pid, _ := strconv.Atoi(u[2])
			if pid == yo || pid <= 1 {
				continue
			}
			pg, err := syscall.Getpgid(pid)
			if err != nil || pg <= 1 {
				continue
			}
			g := grupos[pg]
			if g == nil {
				cwd, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
				g = &EnRed{Grupo: pg, Pid: pid, Nombre: u[1], Orden: Orden(pid), Carpeta: cwd, Segundos: edad(pid)}
				grupos[pg] = g
			}
			if !contieneInt(g.Puertos, puerto) {
				g.Puertos = append(g.Puertos, puerto)
			}
		}
	}
	l := make([]EnRed, 0, len(grupos))
	for _, g := range grupos {
		sort.Ints(g.Puertos)
		l = append(l, *g)
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Segundos < l[j].Segundos })
	return l
}

func contieneInt(l []int, x int) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

// edad: segundos desde que arrancó el proceso (/proc/<pid>/stat, campo 22, en ticks desde el arranque).
func edad(pid int) int64 {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	s := string(b)
	if i := strings.LastIndexByte(s, ')'); i >= 0 {
		s = s[i+2:]
	}
	f := strings.Fields(s)
	if len(f) < 20 {
		return 0
	}
	ticks, _ := strconv.ParseInt(f[19], 10, 64)
	u, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	arriba, _ := strconv.ParseFloat(strings.Fields(string(u))[0], 64)
	return int64(arriba - float64(ticks)/100)
}
