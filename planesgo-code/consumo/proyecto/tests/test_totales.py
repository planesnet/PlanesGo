import unittest

from facturacion.clientes import Cliente
from facturacion.lineas import Linea
from facturacion.productos import Producto
from facturacion.totales import totales_factura


class TestTotales(unittest.TestCase):
    def test_factura_simple(self):
        t = totales_factura([Linea(Producto("S1", "Silla", 50.0), 2)])
        self.assertEqual(t, {"base": 100.0, "iva": 21.0, "recargo": 0.0, "irpf": 0.0, "total": 121.0})

    def test_profesional(self):
        c = Cliente("1", "Prof", retencion_irpf=15)
        t = totales_factura([Linea(Producto("H", "Hora", 40.0), 10)], c)
        self.assertEqual(t["total"], 424.0)


if __name__ == "__main__":
    unittest.main()
