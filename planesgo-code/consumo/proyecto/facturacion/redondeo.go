package facturacion

import (
	"math/big"
	"strconv"
)

// Redondea hace el redondeo comercial (mitad hacia arriba), como exige la normativa de facturación. Parte del
// número como se escribe (0.105 es 0.105, no 0.10499999…).
func Redondea(valor float64, decimales int) float64 {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(valor, 'g', -1, 64))
	escala := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimales)), nil)
	r.Mul(r, new(big.Rat).SetInt(escala))
	// QuoRem trunca hacia cero: si lo que queda es la mitad o más, se aleja del cero.
	entero, resto := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if new(big.Int).Mul(resto.Abs(resto), big.NewInt(2)).Cmp(r.Denom()) >= 0 {
		entero.Add(entero, big.NewInt(int64(r.Sign())))
	}
	f, _ := new(big.Rat).SetFrac(entero, escala).Float64()
	return f
}
