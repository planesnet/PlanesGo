package facturacion

// Linea es una línea de factura: un producto, la cantidad y el descuento.
type Linea struct {
	Producto  Producto
	Cantidad  float64
	Descuento float64 // porcentaje (0-100)
}

// Bruto es el precio por la cantidad, sin descuento.
func (l Linea) Bruto() float64 {
	return l.Producto.Precio * l.Cantidad
}

// Subtotal es la base imponible de la línea: el bruto menos el descuento (en porcentaje).
func (l Linea) Subtotal() float64 {
	return Redondea(l.Bruto()-l.Descuento, 2)
}

// PorcentajeIVA es el tipo de IVA del producto de la línea.
func (l Linea) PorcentajeIVA() float64 {
	return l.Producto.PorcentajeIVA()
}
