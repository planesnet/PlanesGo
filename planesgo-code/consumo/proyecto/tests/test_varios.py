import unittest
from datetime import date

from facturacion.fechas import trimestre, vencimiento
from facturacion.numeracion import siguiente
from facturacion.validacion import nif_valido


class TestVarios(unittest.TestCase):
    def test_numeracion(self):
        self.assertEqual(siguiente("F2026-00041"), "F2026-00042")
        self.assertEqual(siguiente("F2025-00041"), "F2026-00001")

    def test_nif(self):
        self.assertTrue(nif_valido("12345678Z"))
        self.assertFalse(nif_valido("12345678A"))

    def test_fechas(self):
        self.assertEqual(trimestre(date(2026, 5, 1)), 2)
        self.assertEqual(vencimiento(date(2026, 1, 31)), date(2026, 3, 2))


if __name__ == "__main__":
    unittest.main()
