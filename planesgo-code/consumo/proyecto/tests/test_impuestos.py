import unittest

from facturacion.clientes import Cliente
from facturacion.impuestos import iva_linea, retencion_irpf, recargo_equivalencia
from facturacion.lineas import Linea
from facturacion.productos import Producto

LIBRO = Producto("L1", "Libro", 20.0, "superreducido")
SILLA = Producto("S1", "Silla", 50.0)


class TestImpuestos(unittest.TestCase):
    def test_iva_general(self):
        self.assertEqual(iva_linea(Linea(SILLA, 2)), 21.0)

    def test_iva_superreducido(self):
        self.assertEqual(iva_linea(Linea(LIBRO, 1)), 0.8)

    def test_exportacion_exenta(self):
        self.assertEqual(iva_linea(Linea(SILLA, 1), Cliente("X", "GmbH", pais="DE")), 0.0)

    def test_irpf(self):
        self.assertEqual(retencion_irpf(1000, Cliente("1", "Prof", retencion_irpf=15)), 150.0)

    def test_recargo(self):
        self.assertEqual(recargo_equivalencia(100, 21.0, Cliente("1", "Tienda", recargo_equivalencia=True)), 5.2)


if __name__ == "__main__":
    unittest.main()
