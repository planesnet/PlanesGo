import unittest

from facturacion.lineas import Linea
from facturacion.productos import Producto

TORNILLO = Producto("T1", "Tornillo", 2.50)


class TestLineas(unittest.TestCase):
    def test_sin_descuento(self):
        self.assertEqual(Linea(TORNILLO, 4).subtotal(), 10.0)

    def test_descuento_porcentaje(self):
        self.assertEqual(Linea(TORNILLO, 4, descuento=10).subtotal(), 9.0)

    def test_descuento_redondeo(self):
        self.assertEqual(Linea(Producto("P", "Pieza", 3.33), 3, descuento=15).subtotal(), 8.49)


if __name__ == "__main__":
    unittest.main()
