package proceso

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Quién está al otro lado de una conexión TCP local: el proceso dueño del socket del cliente (por /proc/net/tcp y
// los descriptores de /proc/<pid>/fd), y si él o alguno de sus antepasados lleva una marca de agente o de programa
// lanzado (PGO_EJECUCION). Sirve para que el servidor de pgocode no atienda la interfaz a un agente aunque tenga la
// cookie (docs/odoo-blindaje.md).

// PeerPid: el proceso que abrió el extremo «cliente» de la conexión local→remota (0 si no se sabe).
func PeerPid(client, server net.Addr) int {
	c, ok1 := client.(*net.TCPAddr)
	s, ok2 := server.(*net.TCPAddr)
	if !ok1 || !ok2 {
		return 0
	}
	inode := socketInode(c, s)
	if inode == "" {
		return 0
	}
	return socketOwner(inode)
}

// hexAddr: la dirección como la escribe /proc/net/tcp{,6} (cada palabra de 32 bits en el orden del equipo).
func hexAddr(a *net.TCPAddr, v6 bool) string {
	var b []byte
	if v6 {
		b = a.IP.To16()
	} else {
		b = a.IP.To4()
	}
	if b == nil {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(b); i += 4 {
		fmt.Fprintf(&sb, "%02X%02X%02X%02X", b[i+3], b[i+2], b[i+1], b[i])
	}
	return fmt.Sprintf("%s:%04X", sb.String(), a.Port)
}

func socketInode(c, s *net.TCPAddr) string {
	for _, f := range []struct {
		fichero string
		v6      bool
	}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
		lc, ls := hexAddr(c, f.v6), hexAddr(s, f.v6)
		if lc == "" || ls == "" {
			continue
		}
		fh, err := os.Open(f.fichero)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			x := strings.Fields(sc.Text())
			if len(x) > 9 && x[1] == lc && x[2] == ls {
				fh.Close()
				return x[9]
			}
		}
		fh.Close()
	}
	return ""
}

func socketOwner(inode string) int {
	objetivo := "socket:[" + inode + "]"
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	yo := os.Getpid()
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == yo {
			continue
		}
		dir := "/proc/" + e.Name() + "/fd"
		fds, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			if l, err := os.Readlink(dir + "/" + fd.Name()); err == nil && l == objetivo {
				return pid
			}
		}
	}
	return 0
}

// Parent: el padre de un proceso (0 si no se sabe).
func Parent(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	// pid (nombre) estado ppid …; el nombre puede llevar espacios y paréntesis: se corta en el último «)».
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 2 {
		return 0
	}
	pp, _ := strconv.Atoi(f[1])
	return pp
}

// MarkedLineage: el proceso o alguno de sus antepasados lleva una marca que empieza por uno de los prefijos.
func MarkedLineage(pid int, prefixes []string) bool {
	for i := 0; pid > 1 && i < 64; i++ {
		if m := MarcaDe(pid); m != "" {
			for _, p := range prefixes {
				if strings.HasPrefix(m, p) {
					return true
				}
			}
		}
		pid = Parent(pid)
	}
	return false
}
