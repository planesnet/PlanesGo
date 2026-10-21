//go:build !linux

package proceso

import "os"

// Fuera de Linux no se buscan los procesos por su entorno: queda la vigilancia y la cancelación por grupo.
func ConMarca(string) []int { return nil }

func termina(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}

func mata(int) {}

func GruposConMarca(string) map[int][]int { return nil }

func Orden(int) string { return "" }

func MarcaDe(int) string { return "" }
