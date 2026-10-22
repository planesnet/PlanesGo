import re

_PATRON = re.compile(r"^(?P<serie>[A-Z]+)(?P<anio>\d{4})-(?P<num>\d{5})$")


def siguiente(ultimo, serie="F", anio=2026):
    """Siguiente número de factura de la serie (F2026-00001…); empieza de nuevo cada año."""
    m = _PATRON.match(ultimo or "")
    if not m or m["serie"] != serie or int(m["anio"]) != anio:
        return f"{serie}{anio}-00001"
    return f"{serie}{anio}-{int(m['num']) + 1:05d}"
