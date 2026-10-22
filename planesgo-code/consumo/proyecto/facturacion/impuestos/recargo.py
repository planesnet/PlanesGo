"""Recargo de equivalencia (comerciantes minoristas): se suma al IVA según su tipo."""
from ..redondeo import redondea

RECARGOS = {21.0: 5.2, 10.0: 1.4, 4.0: 0.5, 0.0: 0.0}


def recargo_equivalencia(base, porcentaje_iva, cliente):
    if cliente is None or not cliente.recargo_equivalencia:
        return 0.0
    return redondea(base * RECARGOS.get(porcentaje_iva, 0.0) / 100)
