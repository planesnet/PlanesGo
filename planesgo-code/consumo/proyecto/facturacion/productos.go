package facturacion

// TiposIVA son los porcentajes de cada tipo de IVA.
var TiposIVA = map[string]float64{"general": 21, "reducido": 10, "superreducido": 4, "exento": 0}

// Producto es un artículo del catálogo.
type Producto struct {
	Codigo  string
	Nombre  string
	Precio  float64
	TipoIVA string // vacío: "general"
}

// PorcentajeIVA es el porcentaje de IVA del producto.
func (p Producto) PorcentajeIVA() float64 {
	if p.TipoIVA == "" {
		return TiposIVA["general"]
	}
	return TiposIVA[p.TipoIVA]
}
