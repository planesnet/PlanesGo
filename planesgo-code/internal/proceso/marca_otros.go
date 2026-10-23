//go:build !linux

package proceso

import (
	"net"
	"os"
)

// Fuera de Linux no se buscan los procesos por su entorno: queda la vigilancia y la cancelación por grupo.
func ConMarca(string) []int { return nil }

func porMarca() map[string][]int { return nil }

func termina(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}

func mata(int) {}

func GruposConMarca(string) map[int][]int { return nil }

func Orden(int) string { return "" }

func MarcaDe(int) string { return "" }

// Fuera de Linux no se sabe qué proceso está al otro lado de una conexión.
func PeerPid(client, server net.Addr) int { return 0 }

func Parent(int) int { return 0 }

func MarkedLineage(int, []string) bool { return false }
