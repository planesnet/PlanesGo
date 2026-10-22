import unittest

from almacen.inventario import MARGEN_SEGURIDAD, Inventario, SinStock


class TestInventario(unittest.TestCase):
    def test_reserva(self):
        inv = Inventario()
        inv.entrada("AL-0001", 20)
        inv.reserva_stock("AL-0001", 20 - MARGEN_SEGURIDAD)
        self.assertEqual(inv.disponible("AL-0001"), 0)

    def test_sin_stock(self):
        inv = Inventario()
        inv.entrada("AL-0002", 5)
        with self.assertRaises(SinStock):
            inv.reserva_stock("AL-0002", 1)


if __name__ == "__main__":
    unittest.main()
