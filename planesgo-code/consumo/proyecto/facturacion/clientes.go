// Package facturacion es una librería de facturación de ejemplo.
package facturacion

var paisesUE = map[string]bool{"DE": true, "FR": true, "IT": true, "PT": true, "NL": true, "BE": true, "AT": true, "IE": true}

// Cliente es el destinatario de una factura.
type Cliente struct {
	NIF                 string
	Nombre              string
	Pais                string // vacío: "ES"
	RecargoEquivalencia bool
	RetencionIRPF       float64 // porcentaje
}

// EsNacional dice si el cliente es de España.
func (c Cliente) EsNacional() bool {
	return c.Pais == "" || c.Pais == "ES"
}

// EsIntracomunitario dice si el cliente es de otro país de la UE.
func (c Cliente) EsIntracomunitario() bool {
	return paisesUE[c.Pais]
}
