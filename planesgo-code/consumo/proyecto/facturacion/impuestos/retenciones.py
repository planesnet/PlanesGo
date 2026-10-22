"""Retenciones de IRPF (profesionales): no es un impuesto de la línea, se resta del total."""
from ..redondeo import redondea


def retencion_irpf(base, cliente):
    if cliente is None or not cliente.retencion_irpf:
        return 0.0
    return redondea(base * cliente.retencion_irpf / 100)
