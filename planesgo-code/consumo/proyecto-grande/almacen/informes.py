"""Informes del almacén: listados de stock, ventas por familia y por cliente, y textos para imprimir."""

from .inventario import CATALOGO
from .pedidos import CLIENTES

ANCHO = 78


def cabecera(titulo):
    return titulo.upper().center(ANCHO, "=")


def fila(*columnas, anchos=(12, 40, 12, 12)):
    return "".join(str(c)[:a].ljust(a) for c, a in zip(columnas, anchos))


def informe_stock_tornilleria(inventario, ventas=None):
    """Existencias de la familia tornilleria, una fila por producto."""
    lineas = [cabecera("Existencias · tornilleria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "tornilleria":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_tornilleria(inventario, ventas=None):
    """Ventas de la familia tornilleria, una fila por producto."""
    lineas = [cabecera("Ventas · tornilleria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "tornilleria":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_tornilleria(inventario, ventas=None):
    """Productos bajo mínimo de la familia tornilleria, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · tornilleria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "tornilleria":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_stock_electricidad(inventario, ventas=None):
    """Existencias de la familia electricidad, una fila por producto."""
    lineas = [cabecera("Existencias · electricidad")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "electricidad":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_electricidad(inventario, ventas=None):
    """Ventas de la familia electricidad, una fila por producto."""
    lineas = [cabecera("Ventas · electricidad")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "electricidad":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_electricidad(inventario, ventas=None):
    """Productos bajo mínimo de la familia electricidad, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · electricidad")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "electricidad":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_stock_fontaneria(inventario, ventas=None):
    """Existencias de la familia fontaneria, una fila por producto."""
    lineas = [cabecera("Existencias · fontaneria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "fontaneria":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_fontaneria(inventario, ventas=None):
    """Ventas de la familia fontaneria, una fila por producto."""
    lineas = [cabecera("Ventas · fontaneria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "fontaneria":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_fontaneria(inventario, ventas=None):
    """Productos bajo mínimo de la familia fontaneria, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · fontaneria")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "fontaneria":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_stock_pintura(inventario, ventas=None):
    """Existencias de la familia pintura, una fila por producto."""
    lineas = [cabecera("Existencias · pintura")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "pintura":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_pintura(inventario, ventas=None):
    """Ventas de la familia pintura, una fila por producto."""
    lineas = [cabecera("Ventas · pintura")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "pintura":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_pintura(inventario, ventas=None):
    """Productos bajo mínimo de la familia pintura, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · pintura")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "pintura":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_stock_herramienta(inventario, ventas=None):
    """Existencias de la familia herramienta, una fila por producto."""
    lineas = [cabecera("Existencias · herramienta")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "herramienta":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_herramienta(inventario, ventas=None):
    """Ventas de la familia herramienta, una fila por producto."""
    lineas = [cabecera("Ventas · herramienta")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "herramienta":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_herramienta(inventario, ventas=None):
    """Productos bajo mínimo de la familia herramienta, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · herramienta")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "herramienta":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_stock_jardin(inventario, ventas=None):
    """Existencias de la familia jardin, una fila por producto."""
    lineas = [cabecera("Existencias · jardin")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "jardin":
            continue
        n = inventario.existencias.get(sku, 0)
        lineas.append(fila(sku, p.nombre, n, p.ubicacion))
        total += n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_ventas_jardin(inventario, ventas=None):
    """Ventas de la familia jardin, una fila por producto."""
    lineas = [cabecera("Ventas · jardin")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "jardin":
            continue
        n = inventario.existencias.get(sku, 0)
        v = (ventas or {}).get(sku, 0)
        lineas.append(fila(sku, p.nombre, v, round(v * p.precio, 2)))
        total += v * p.precio
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


def informe_rotura_jardin(inventario, ventas=None):
    """Productos bajo mínimo de la familia jardin, una fila por producto."""
    lineas = [cabecera("Productos bajo mínimo · jardin")]
    total = 0.0
    for sku, p in sorted(CATALOGO.items()):
        if p.familia != "jardin":
            continue
        n = inventario.existencias.get(sku, 0)
        if n >= p.stock_minimo:
            continue
        lineas.append(fila(sku, p.nombre, n, p.stock_minimo))
        total += p.stock_minimo - n
    lineas.append("-" * ANCHO)
    lineas.append(fila("", "Total", "", round(total, 2)))
    return "\n".join(lineas)


# Textos de los informes por cliente.
PIES = {
    "C-0001": "Condiciones: 60 días. Entregar en Santa Cruz de Tenerife (CP 38780). Grupo de tarifa: general.",
    "C-0002": "Condiciones: 30 días. Entregar en Baleares (CP 07252). Grupo de tarifa: general.",
    "C-0003": "Condiciones: contado. Entregar en Alicante (CP 03972). Grupo de tarifa: mayorista.",
    "C-0004": "Condiciones: contado. Entregar en Baleares (CP 07135). Grupo de tarifa: mayorista.",
    "C-0005": "Condiciones: transferencia 15 días. Entregar en Huelva (CP 21344). Grupo de tarifa: profesional.",
    "C-0006": "Condiciones: 60 días. Entregar en Girona (CP 17952). Grupo de tarifa: general.",
    "C-0007": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50987). Grupo de tarifa: profesional.",
    "C-0008": "Condiciones: contado. Entregar en Albacete (CP 02446). Grupo de tarifa: profesional.",
    "C-0009": "Condiciones: 30 días. Entregar en Las Palmas (CP 35579). Grupo de tarifa: mayorista.",
    "C-0010": "Condiciones: contado. Entregar en Cádiz (CP 11446). Grupo de tarifa: mayorista.",
    "C-0011": "Condiciones: contado. Entregar en Zamora (CP 49768). Grupo de tarifa: general.",
    "C-0012": "Condiciones: 30 días. Entregar en Baleares (CP 07503). Grupo de tarifa: profesional.",
    "C-0013": "Condiciones: 30 días. Entregar en Girona (CP 17591). Grupo de tarifa: profesional.",
    "C-0014": "Condiciones: contado. Entregar en Ourense (CP 32169). Grupo de tarifa: mayorista.",
    "C-0015": "Condiciones: contado. Entregar en Ceuta (CP 51713). Grupo de tarifa: mayorista.",
    "C-0016": "Condiciones: transferencia 15 días. Entregar en Baleares (CP 07410). Grupo de tarifa: mayorista.",
    "C-0017": "Condiciones: 60 días. Entregar en Soria (CP 42025). Grupo de tarifa: general.",
    "C-0018": "Condiciones: 30 días. Entregar en Madrid (CP 28922). Grupo de tarifa: profesional.",
    "C-0019": "Condiciones: contado. Entregar en Burgos (CP 09544). Grupo de tarifa: profesional.",
    "C-0020": "Condiciones: transferencia 15 días. Entregar en Cáceres (CP 10888). Grupo de tarifa: mayorista.",
    "C-0021": "Condiciones: 60 días. Entregar en Murcia (CP 30449). Grupo de tarifa: mayorista.",
    "C-0022": "Condiciones: 30 días. Entregar en Huesca (CP 22473). Grupo de tarifa: mayorista.",
    "C-0023": "Condiciones: 30 días. Entregar en Gipuzkoa (CP 20772). Grupo de tarifa: mayorista.",
    "C-0024": "Condiciones: 60 días. Entregar en Valladolid (CP 47334). Grupo de tarifa: general.",
    "C-0025": "Condiciones: contado. Entregar en Ciudad Real (CP 13264). Grupo de tarifa: general.",
    "C-0026": "Condiciones: 30 días. Entregar en Lleida (CP 25154). Grupo de tarifa: profesional.",
    "C-0027": "Condiciones: contado. Entregar en Granada (CP 18200). Grupo de tarifa: mayorista.",
    "C-0028": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14906). Grupo de tarifa: profesional.",
    "C-0029": "Condiciones: transferencia 15 días. Entregar en La Rioja (CP 26874). Grupo de tarifa: mayorista.",
    "C-0030": "Condiciones: transferencia 15 días. Entregar en Sevilla (CP 41303). Grupo de tarifa: general.",
    "C-0031": "Condiciones: transferencia 15 días. Entregar en Cantabria (CP 39755). Grupo de tarifa: general.",
    "C-0032": "Condiciones: transferencia 15 días. Entregar en Toledo (CP 45587). Grupo de tarifa: general.",
    "C-0033": "Condiciones: transferencia 15 días. Entregar en Soria (CP 42127). Grupo de tarifa: profesional.",
    "C-0034": "Condiciones: contado. Entregar en Sevilla (CP 41717). Grupo de tarifa: profesional.",
    "C-0035": "Condiciones: 30 días. Entregar en Valencia (CP 46729). Grupo de tarifa: profesional.",
    "C-0036": "Condiciones: transferencia 15 días. Entregar en Murcia (CP 30020). Grupo de tarifa: mayorista.",
    "C-0037": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50010). Grupo de tarifa: profesional.",
    "C-0038": "Condiciones: 30 días. Entregar en Girona (CP 17556). Grupo de tarifa: general.",
    "C-0039": "Condiciones: transferencia 15 días. Entregar en Jaén (CP 23103). Grupo de tarifa: mayorista.",
    "C-0040": "Condiciones: 60 días. Entregar en Asturias (CP 33016). Grupo de tarifa: mayorista.",
    "C-0041": "Condiciones: transferencia 15 días. Entregar en Bizkaia (CP 48970). Grupo de tarifa: general.",
    "C-0042": "Condiciones: contado. Entregar en Asturias (CP 33781). Grupo de tarifa: mayorista.",
    "C-0043": "Condiciones: transferencia 15 días. Entregar en Girona (CP 17280). Grupo de tarifa: profesional.",
    "C-0044": "Condiciones: transferencia 15 días. Entregar en Ávila (CP 05428). Grupo de tarifa: mayorista.",
    "C-0045": "Condiciones: 60 días. Entregar en Baleares (CP 07229). Grupo de tarifa: mayorista.",
    "C-0046": "Condiciones: transferencia 15 días. Entregar en A Coruña (CP 15820). Grupo de tarifa: profesional.",
    "C-0047": "Condiciones: contado. Entregar en Burgos (CP 09951). Grupo de tarifa: mayorista.",
    "C-0048": "Condiciones: 30 días. Entregar en Soria (CP 42575). Grupo de tarifa: general.",
    "C-0049": "Condiciones: 30 días. Entregar en Murcia (CP 30301). Grupo de tarifa: profesional.",
    "C-0050": "Condiciones: transferencia 15 días. Entregar en Granada (CP 18721). Grupo de tarifa: mayorista.",
    "C-0051": "Condiciones: transferencia 15 días. Entregar en Teruel (CP 44190). Grupo de tarifa: general.",
    "C-0052": "Condiciones: 60 días. Entregar en Cuenca (CP 16670). Grupo de tarifa: profesional.",
    "C-0053": "Condiciones: contado. Entregar en Madrid (CP 28638). Grupo de tarifa: mayorista.",
    "C-0054": "Condiciones: transferencia 15 días. Entregar en Gipuzkoa (CP 20874). Grupo de tarifa: general.",
    "C-0055": "Condiciones: 30 días. Entregar en Ceuta (CP 51965). Grupo de tarifa: mayorista.",
    "C-0056": "Condiciones: 30 días. Entregar en Tarragona (CP 43011). Grupo de tarifa: general.",
    "C-0057": "Condiciones: 30 días. Entregar en Cantabria (CP 39103). Grupo de tarifa: general.",
    "C-0058": "Condiciones: 30 días. Entregar en Jaén (CP 23803). Grupo de tarifa: general.",
    "C-0059": "Condiciones: contado. Entregar en Segovia (CP 40912). Grupo de tarifa: mayorista.",
    "C-0060": "Condiciones: 30 días. Entregar en Ourense (CP 32709). Grupo de tarifa: mayorista.",
    "C-0061": "Condiciones: contado. Entregar en Tarragona (CP 43903). Grupo de tarifa: mayorista.",
    "C-0062": "Condiciones: 30 días. Entregar en Lugo (CP 27239). Grupo de tarifa: profesional.",
    "C-0063": "Condiciones: 30 días. Entregar en Navarra (CP 31478). Grupo de tarifa: mayorista.",
    "C-0064": "Condiciones: contado. Entregar en Ourense (CP 32168). Grupo de tarifa: general.",
    "C-0065": "Condiciones: transferencia 15 días. Entregar en Toledo (CP 45576). Grupo de tarifa: mayorista.",
    "C-0066": "Condiciones: transferencia 15 días. Entregar en León (CP 24436). Grupo de tarifa: mayorista.",
    "C-0067": "Condiciones: contado. Entregar en Sevilla (CP 41369). Grupo de tarifa: general.",
    "C-0068": "Condiciones: transferencia 15 días. Entregar en Melilla (CP 52096). Grupo de tarifa: profesional.",
    "C-0069": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14735). Grupo de tarifa: mayorista.",
    "C-0070": "Condiciones: 60 días. Entregar en Baleares (CP 07882). Grupo de tarifa: profesional.",
    "C-0071": "Condiciones: 30 días. Entregar en Pontevedra (CP 36789). Grupo de tarifa: profesional.",
    "C-0072": "Condiciones: contado. Entregar en Madrid (CP 28257). Grupo de tarifa: profesional.",
    "C-0073": "Condiciones: 60 días. Entregar en Ourense (CP 32413). Grupo de tarifa: mayorista.",
    "C-0074": "Condiciones: 30 días. Entregar en Jaén (CP 23998). Grupo de tarifa: mayorista.",
    "C-0075": "Condiciones: 60 días. Entregar en Huesca (CP 22196). Grupo de tarifa: mayorista.",
    "C-0076": "Condiciones: contado. Entregar en Santa Cruz de Tenerife (CP 38996). Grupo de tarifa: general.",
    "C-0077": "Condiciones: contado. Entregar en Las Palmas (CP 35587). Grupo de tarifa: profesional.",
    "C-0078": "Condiciones: 30 días. Entregar en Álava (CP 01047). Grupo de tarifa: profesional.",
    "C-0079": "Condiciones: transferencia 15 días. Entregar en Las Palmas (CP 35626). Grupo de tarifa: mayorista.",
    "C-0080": "Condiciones: transferencia 15 días. Entregar en Córdoba (CP 14040). Grupo de tarifa: mayorista.",
    "C-0081": "Condiciones: contado. Entregar en Tarragona (CP 43185). Grupo de tarifa: profesional.",
    "C-0082": "Condiciones: 30 días. Entregar en León (CP 24892). Grupo de tarifa: profesional.",
    "C-0083": "Condiciones: contado. Entregar en Castellón (CP 12431). Grupo de tarifa: profesional.",
    "C-0084": "Condiciones: contado. Entregar en Salamanca (CP 37657). Grupo de tarifa: profesional.",
    "C-0085": "Condiciones: transferencia 15 días. Entregar en Zaragoza (CP 50829). Grupo de tarifa: mayorista.",
    "C-0086": "Condiciones: transferencia 15 días. Entregar en Ávila (CP 05014). Grupo de tarifa: mayorista.",
    "C-0087": "Condiciones: contado. Entregar en Zaragoza (CP 50422). Grupo de tarifa: general.",
    "C-0088": "Condiciones: contado. Entregar en Cáceres (CP 10641). Grupo de tarifa: profesional.",
    "C-0089": "Condiciones: contado. Entregar en Teruel (CP 44685). Grupo de tarifa: general.",
    "C-0090": "Condiciones: contado. Entregar en Burgos (CP 09483). Grupo de tarifa: profesional.",
    "C-0091": "Condiciones: 30 días. Entregar en Valladolid (CP 47762). Grupo de tarifa: general.",
    "C-0092": "Condiciones: contado. Entregar en Valladolid (CP 47777). Grupo de tarifa: profesional.",
    "C-0093": "Condiciones: 60 días. Entregar en Tarragona (CP 43954). Grupo de tarifa: general.",
    "C-0094": "Condiciones: contado. Entregar en Almería (CP 04015). Grupo de tarifa: profesional.",
    "C-0095": "Condiciones: 30 días. Entregar en Valladolid (CP 47614). Grupo de tarifa: profesional.",
    "C-0096": "Condiciones: transferencia 15 días. Entregar en León (CP 24971). Grupo de tarifa: profesional.",
    "C-0097": "Condiciones: 60 días. Entregar en Melilla (CP 52119). Grupo de tarifa: mayorista.",
    "C-0098": "Condiciones: transferencia 15 días. Entregar en Navarra (CP 31394). Grupo de tarifa: profesional.",
    "C-0099": "Condiciones: 60 días. Entregar en Granada (CP 18062). Grupo de tarifa: mayorista.",
    "C-0100": "Condiciones: 60 días. Entregar en Cantabria (CP 39852). Grupo de tarifa: mayorista.",
    "C-0101": "Condiciones: transferencia 15 días. Entregar en Lleida (CP 25396). Grupo de tarifa: mayorista.",
    "C-0102": "Condiciones: contado. Entregar en Guadalajara (CP 19705). Grupo de tarifa: profesional.",
    "C-0103": "Condiciones: contado. Entregar en Madrid (CP 28161). Grupo de tarifa: profesional.",
    "C-0104": "Condiciones: transferencia 15 días. Entregar en Granada (CP 18998). Grupo de tarifa: profesional.",
    "C-0105": "Condiciones: 30 días. Entregar en Melilla (CP 52390). Grupo de tarifa: mayorista.",
    "C-0106": "Condiciones: transferencia 15 días. Entregar en Cantabria (CP 39058). Grupo de tarifa: profesional.",
    "C-0107": "Condiciones: contado. Entregar en Santa Cruz de Tenerife (CP 38769). Grupo de tarifa: profesional.",
    "C-0108": "Condiciones: 60 días. Entregar en Las Palmas (CP 35825). Grupo de tarifa: general.",
    "C-0109": "Condiciones: 60 días. Entregar en Santa Cruz de Tenerife (CP 38533). Grupo de tarifa: mayorista.",
    "C-0110": "Condiciones: 30 días. Entregar en Asturias (CP 33603). Grupo de tarifa: general.",
    "C-0111": "Condiciones: 60 días. Entregar en Badajoz (CP 06185). Grupo de tarifa: profesional.",
    "C-0112": "Condiciones: 30 días. Entregar en Zaragoza (CP 50529). Grupo de tarifa: general.",
    "C-0113": "Condiciones: contado. Entregar en León (CP 24887). Grupo de tarifa: profesional.",
    "C-0114": "Condiciones: contado. Entregar en Cáceres (CP 10323). Grupo de tarifa: profesional.",
    "C-0115": "Condiciones: contado. Entregar en Cantabria (CP 39021). Grupo de tarifa: general.",
    "C-0116": "Condiciones: 30 días. Entregar en Santa Cruz de Tenerife (CP 38580). Grupo de tarifa: profesional.",
    "C-0117": "Condiciones: transferencia 15 días. Entregar en Baleares (CP 07969). Grupo de tarifa: mayorista.",
    "C-0118": "Condiciones: 30 días. Entregar en Alicante (CP 03346). Grupo de tarifa: general.",
    "C-0119": "Condiciones: contado. Entregar en Albacete (CP 02052). Grupo de tarifa: mayorista.",
    "C-0120": "Condiciones: contado. Entregar en Ourense (CP 32969). Grupo de tarifa: mayorista.",
}


def ficha_cliente(codigo):
    c = CLIENTES[codigo]
    return "\n".join([cabecera(c.nombre), f"Código: {c.codigo}", f"Provincia: {c.provincia}", PIES.get(codigo, "")])

