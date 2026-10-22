import unittest

from almacen.pedidos import Pedido, TransicionInvalida


class TestPedidos(unittest.TestCase):
    def test_importe(self):
        p = Pedido("P-1", "C-0001")
        p.anade("AL-0001", 10, 0.5)
        p.anade("AL-0002", 2, 3.25)
        self.assertEqual(p.importe(), 11.5)

    def test_transicion_invalida(self):
        p = Pedido("P-2", "C-0001")
        with self.assertRaises(TransicionInvalida):
            p.cambia_estado("ENVIADO")


if __name__ == "__main__":
    unittest.main()
