from .impuestos import iva_linea, retencion_irpf, recargo_equivalencia
from .redondeo import redondea


def totales_factura(lineas, cliente=None):
    base = redondea(sum(l.subtotal() for l in lineas))
    iva = redondea(sum(iva_linea(l, cliente) for l in lineas))
    recargo = redondea(sum(recargo_equivalencia(l.subtotal(), l.porcentaje_iva(), cliente) for l in lineas))
    irpf = retencion_irpf(base, cliente)
    return {"base": base, "iva": iva, "recargo": recargo, "irpf": irpf,
            "total": redondea(base + iva + recargo - irpf)}
