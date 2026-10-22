import unittest

from almacen.envios import coste_envio, zona_de


class TestEnvios(unittest.TestCase):
    def test_zona(self):
        self.assertEqual(zona_de("28013"), 1)
        self.assertEqual(zona_de("35001"), 5)

    def test_coste(self):
        self.assertEqual(coste_envio(0.5, 1), 3.9)


if __name__ == "__main__":
    unittest.main()
