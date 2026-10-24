package facturacion

import "time"

// Vencimiento es la fecha de vencimiento de una factura emitida a tantos días.
func Vencimiento(emision time.Time, dias int) time.Time {
	return emision.AddDate(0, 0, dias)
}

// Trimestre es el trimestre natural (1-4) de una fecha.
func Trimestre(fecha time.Time) int {
	return (int(fecha.Month())-1)/3 + 1
}
