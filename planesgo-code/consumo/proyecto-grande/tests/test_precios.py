import unittest

from almacen.precios import aplica_descuento, descuento_volumen


class TestPrecios(unittest.TestCase):
    def test_tramos(self):
        self.assertEqual(descuento_volumen(199.99), 0)
        self.assertEqual(descuento_volumen(350), 2)
        self.assertEqual(descuento_volumen(2000), 10)

    def test_limites(self):
        self.assertEqual(descuento_volumen(200), 2)
        self.assertEqual(descuento_volumen(500), 5)
        self.assertEqual(descuento_volumen(1000), 10)

    def test_aplica(self):
        self.assertEqual(aplica_descuento(500), 475.0)


if __name__ == "__main__":
    unittest.main()
