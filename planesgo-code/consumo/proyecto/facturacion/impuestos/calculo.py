"""Cálculo de impuestos indirectos de las líneas de factura."""
from ..redondeo import redondea


def exenta(linea, cliente):
    """Operaciones sin IVA: exportaciones y entregas intracomunitarias a empresas."""
    if cliente is None:
        return False
    return not cliente.es_nacional()


def cuota_iva(base, porcentaje):
    """Cuota de IVA de una base con un porcentaje dado."""
    return redondea(base * porcentaje / 100)


def iva_linea(linea, cliente=None):
    """Importe del IVA de una línea de factura."""
    if exenta(linea, cliente):
        return 0.0
    return cuota_iva(linea.subtotal(), linea.porcentaje_iva())


def desglose(lineas, cliente=None):
    """Bases y cuotas agrupadas por porcentaje de IVA."""
    grupos = {}
    for linea in lineas:
        p = 0.0 if exenta(linea, cliente) else linea.porcentaje_iva()
        base, cuota = grupos.get(p, (0.0, 0.0))
        grupos[p] = (base + linea.subtotal(), cuota + iva_linea(linea, cliente))
    return {p: (redondea(b), redondea(c)) for p, (b, c) in grupos.items()}
