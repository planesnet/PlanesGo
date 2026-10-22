import csv
import io

from .totales import totales_factura


def a_csv(lineas, cliente=None):
    """Exporta las líneas y los totales (base, IVA, recargo, IRPF) a CSV."""
    salida = io.StringIO()
    w = csv.writer(salida, delimiter=";")
    w.writerow(["codigo", "producto", "cantidad", "descuento", "base", "iva_%"])
    for l in lineas:
        w.writerow([l.producto.codigo, l.producto.nombre, l.cantidad, l.descuento, l.subtotal(), l.porcentaje_iva()])
    t = totales_factura(lineas, cliente)
    for k in ("base", "iva", "recargo", "irpf", "total"):
        w.writerow([k, "", "", "", t[k], ""])
    return salida.getvalue()
