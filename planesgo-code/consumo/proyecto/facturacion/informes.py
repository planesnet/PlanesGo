from collections import defaultdict

from .fechas import trimestre
from .impuestos.calculo import desglose


def resumen_trimestral(facturas):
    """Base y cuota de IVA por trimestre (modelo 303): facturas = [(fecha, lineas, cliente)]."""
    r = defaultdict(lambda: [0.0, 0.0])
    for fecha, lineas, cliente in facturas:
        for _, (base, cuota) in desglose(lineas, cliente).items():
            r[trimestre(fecha)][0] += base
            r[trimestre(fecha)][1] += cuota
    return dict(r)
