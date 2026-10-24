package facturacion

import (
	"fmt"
	"regexp"
	"strconv"
)

var patronNumero = regexp.MustCompile(`^([A-Z]+)(\d{4})-(\d{5})$`)

// Siguiente es el siguiente número de factura de la serie (F2026-00001…); empieza de nuevo cada año.
func Siguiente(ultimo, serie string, anio int) string {
	m := patronNumero.FindStringSubmatch(ultimo)
	if m == nil || m[1] != serie || m[2] != strconv.Itoa(anio) {
		return fmt.Sprintf("%s%d-00001", serie, anio)
	}
	n, _ := strconv.Atoi(m[3])
	return fmt.Sprintf("%s%d-%05d", serie, anio, n+1)
}
