from decimal import Decimal, ROUND_HALF_UP


def redondea(valor, decimales=2):
    """Redondeo comercial (mitad hacia arriba), como exige la normativa de facturación."""
    q = Decimal(1).scaleb(-decimales)
    return float(Decimal(str(valor)).quantize(q, rounding=ROUND_HALF_UP))
