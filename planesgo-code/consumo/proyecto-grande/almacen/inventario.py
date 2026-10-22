"""Inventario del almacén: catálogo de productos, existencias por almacén, reservas y movimientos.

Las existencias se llevan por SKU (y, si se quiere, por almacén). Una reserva aparta unidades para un pedido
confirmado: no salen del almacén hasta que se preparan, pero dejan de estar disponibles para otros pedidos.
"""

from dataclasses import dataclass, field

# Unidades de cada producto que nunca se reservan: cubren roturas y errores de recuento del inventario.
MARGEN_SEGURIDAD = 7

ALMACENES = ('MAD', 'BCN', 'VLC', 'SEV', 'BIL')


@dataclass(frozen=True)
class Producto:
    sku: str
    nombre: str
    familia: str
    peso_kg: float
    precio: float
    ubicacion: str
    stock_minimo: int = 10


# Catálogo: SKU -> Producto (nombre, familia, peso en kg, precio de tarifa sin IVA, ubicación pasillo-estantería-balda
# y stock mínimo antes de pedir al proveedor).
CATALOGO = {
    "AL-0001": Producto("AL-0001", 'Tornillo hexagonal M4', "tornilleria", 0.032, 0.31, "A-03-05", 5),
    "AL-0002": Producto("AL-0002", 'Tornillo hexagonal M5', "tornilleria", 0.035, 0.14, "D-02-01", 25),
    "AL-0003": Producto("AL-0003", 'Tornillo hexagonal M6', "tornilleria", 0.04, 0.48, "G-02-05", 5),
    "AL-0004": Producto("AL-0004", 'Tornillo hexagonal M8', "tornilleria", 0.085, 1.21, "A-19-05", 25),
    "AL-0005": Producto("AL-0005", 'Tornillo hexagonal M10', "tornilleria", 0.008, 0.44, "C-10-04", 10),
    "AL-0006": Producto("AL-0006", 'Tornillo alomado M4', "tornilleria", 0.05, 1.1, "C-04-05", 50),
    "AL-0007": Producto("AL-0007", 'Tornillo alomado M5', "tornilleria", 0.059, 0.73, "B-19-01", 50),
    "AL-0008": Producto("AL-0008", 'Tornillo alomado M6', "tornilleria", 0.022, 1.3, "G-11-04", 50),
    "AL-0009": Producto("AL-0009", 'Tornillo alomado M8', "tornilleria", 0.083, 0.71, "D-06-06", 10),
    "AL-0010": Producto("AL-0010", 'Tornillo alomado M10', "tornilleria", 0.011, 0.59, "H-11-06", 25),
    "AL-0011": Producto("AL-0011", 'Tuerca autoblocante M4', "tornilleria", 0.029, 1.86, "B-17-04", 10),
    "AL-0012": Producto("AL-0012", 'Tuerca autoblocante M5', "tornilleria", 0.069, 0.31, "H-14-01", 100),
    "AL-0013": Producto("AL-0013", 'Tuerca autoblocante M6', "tornilleria", 0.011, 1.07, "F-11-06", 20),
    "AL-0014": Producto("AL-0014", 'Tuerca autoblocante M8', "tornilleria", 0.055, 1.11, "H-03-01", 20),
    "AL-0015": Producto("AL-0015", 'Tuerca autoblocante M10', "tornilleria", 0.045, 1.27, "A-24-06", 20),
    "AL-0016": Producto("AL-0016", 'Arandela plana M4', "tornilleria", 0.06, 1.89, "H-10-06", 25),
    "AL-0017": Producto("AL-0017", 'Arandela plana M5', "tornilleria", 0.08, 0.68, "H-12-02", 50),
    "AL-0018": Producto("AL-0018", 'Arandela plana M6', "tornilleria", 0.014, 0.14, "E-05-06", 10),
    "AL-0019": Producto("AL-0019", 'Arandela plana M8', "tornilleria", 0.038, 1.74, "H-03-02", 25),
    "AL-0020": Producto("AL-0020", 'Arandela plana M10', "tornilleria", 0.039, 0.55, "C-14-05", 20),
    "AL-0021": Producto("AL-0021", 'Varilla roscada M4', "tornilleria", 0.065, 1.87, "G-08-02", 5),
    "AL-0022": Producto("AL-0022", 'Varilla roscada M5', "tornilleria", 0.019, 0.46, "D-01-04", 50),
    "AL-0023": Producto("AL-0023", 'Varilla roscada M6', "tornilleria", 0.02, 0.56, "C-14-05", 20),
    "AL-0024": Producto("AL-0024", 'Varilla roscada M8', "tornilleria", 0.056, 0.63, "C-23-05", 50),
    "AL-0025": Producto("AL-0025", 'Varilla roscada M10', "tornilleria", 0.06, 1.41, "H-22-05", 25),
    "AL-0026": Producto("AL-0026", 'Taco de nailon M4', "tornilleria", 0.038, 0.77, "H-21-04", 5),
    "AL-0027": Producto("AL-0027", 'Taco de nailon M5', "tornilleria", 0.02, 1.87, "H-06-01", 20),
    "AL-0028": Producto("AL-0028", 'Taco de nailon M6', "tornilleria", 0.056, 0.22, "C-18-01", 20),
    "AL-0029": Producto("AL-0029", 'Taco de nailon M8', "tornilleria", 0.057, 0.16, "D-20-04", 10),
    "AL-0030": Producto("AL-0030", 'Taco de nailon M10', "tornilleria", 0.059, 1.82, "F-16-01", 5),
    "AL-0031": Producto("AL-0031", 'Tirafondo M4', "tornilleria", 0.077, 1.89, "H-16-04", 20),
    "AL-0032": Producto("AL-0032", 'Tirafondo M5', "tornilleria", 0.011, 0.22, "F-24-03", 25),
    "AL-0033": Producto("AL-0033", 'Tirafondo M6', "tornilleria", 0.075, 0.33, "A-07-05", 20),
    "AL-0034": Producto("AL-0034", 'Tirafondo M8', "tornilleria", 0.017, 1.05, "A-17-03", 100),
    "AL-0035": Producto("AL-0035", 'Tirafondo M10', "tornilleria", 0.078, 1.33, "E-17-03", 10),
    "AL-0036": Producto("AL-0036", 'Remache ciego M4', "tornilleria", 0.035, 0.45, "F-21-02", 50),
    "AL-0037": Producto("AL-0037", 'Remache ciego M5', "tornilleria", 0.074, 1.87, "D-08-04", 100),
    "AL-0038": Producto("AL-0038", 'Remache ciego M6', "tornilleria", 0.073, 0.4, "H-12-06", 5),
    "AL-0039": Producto("AL-0039", 'Remache ciego M8', "tornilleria", 0.089, 1.51, "H-09-02", 100),
    "AL-0040": Producto("AL-0040", 'Remache ciego M10', "tornilleria", 0.056, 0.67, "F-12-01", 10),
    "AL-0041": Producto("AL-0041", 'Cable flexible 1,5 mm²', "electricidad", 0.29, 30.4, "F-07-04", 50),
    "AL-0042": Producto("AL-0042", 'Cable flexible 2,5 mm²', "electricidad", 2.365, 39.29, "A-16-06", 20),
    "AL-0043": Producto("AL-0043", 'Cable flexible 4 mm²', "electricidad", 1.929, 5.97, "B-13-06", 10),
    "AL-0044": Producto("AL-0044", 'Cable flexible 16 A', "electricidad", 1.173, 11.92, "F-03-06", 25),
    "AL-0045": Producto("AL-0045", 'Cable flexible 25 A', "electricidad", 1.138, 47.73, "B-24-02", 10),
    "AL-0046": Producto("AL-0046", 'Regleta 1,5 mm²', "electricidad", 2.384, 2.35, "H-21-02", 50),
    "AL-0047": Producto("AL-0047", 'Regleta 2,5 mm²', "electricidad", 1.992, 62.75, "F-05-05", 50),
    "AL-0048": Producto("AL-0048", 'Regleta 4 mm²', "electricidad", 0.358, 1.5, "B-17-06", 10),
    "AL-0049": Producto("AL-0049", 'Regleta 16 A', "electricidad", 1.069, 55.87, "D-01-03", 10),
    "AL-0050": Producto("AL-0050", 'Regleta 25 A', "electricidad", 0.738, 15.85, "F-09-05", 25),
    "AL-0051": Producto("AL-0051", 'Interruptor 1,5 mm²', "electricidad", 2.01, 4.46, "F-15-06", 50),
    "AL-0052": Producto("AL-0052", 'Interruptor 2,5 mm²', "electricidad", 1.965, 33.36, "C-18-02", 50),
    "AL-0053": Producto("AL-0053", 'Interruptor 4 mm²', "electricidad", 1.25, 55.94, "C-20-01", 10),
    "AL-0054": Producto("AL-0054", 'Interruptor 16 A', "electricidad", 0.455, 30.62, "B-18-01", 20),
    "AL-0055": Producto("AL-0055", 'Interruptor 25 A', "electricidad", 1.653, 34.25, "H-04-05", 5),
    "AL-0056": Producto("AL-0056", 'Base de enchufe 1,5 mm²', "electricidad", 0.634, 18.16, "B-17-04", 50),
    "AL-0057": Producto("AL-0057", 'Base de enchufe 2,5 mm²', "electricidad", 0.115, 57.28, "B-15-03", 50),
    "AL-0058": Producto("AL-0058", 'Base de enchufe 4 mm²', "electricidad", 2.337, 39.03, "D-23-03", 25),
    "AL-0059": Producto("AL-0059", 'Base de enchufe 16 A', "electricidad", 1.244, 51.79, "D-23-05", 20),
    "AL-0060": Producto("AL-0060", 'Base de enchufe 25 A', "electricidad", 2.219, 57.2, "D-15-02", 25),
    "AL-0061": Producto("AL-0061", 'Caja de empalme 1,5 mm²', "electricidad", 0.336, 28.63, "B-22-02", 25),
    "AL-0062": Producto("AL-0062", 'Caja de empalme 2,5 mm²', "electricidad", 0.222, 43.04, "B-05-06", 100),
    "AL-0063": Producto("AL-0063", 'Caja de empalme 4 mm²', "electricidad", 1.602, 9.66, "C-15-02", 100),
    "AL-0064": Producto("AL-0064", 'Caja de empalme 16 A', "electricidad", 2.288, 25.85, "H-06-06", 10),
    "AL-0065": Producto("AL-0065", 'Caja de empalme 25 A', "electricidad", 0.429, 27.96, "G-11-04", 10),
    "AL-0066": Producto("AL-0066", 'Diferencial 1,5 mm²', "electricidad", 0.888, 6.45, "F-01-03", 50),
    "AL-0067": Producto("AL-0067", 'Diferencial 2,5 mm²', "electricidad", 1.128, 45.18, "G-11-05", 50),
    "AL-0068": Producto("AL-0068", 'Diferencial 4 mm²', "electricidad", 0.744, 61.51, "B-08-01", 5),
    "AL-0069": Producto("AL-0069", 'Diferencial 16 A', "electricidad", 0.674, 3.11, "C-09-02", 25),
    "AL-0070": Producto("AL-0070", 'Diferencial 25 A', "electricidad", 2.047, 43.46, "E-13-02", 50),
    "AL-0071": Producto("AL-0071", 'Magnetotérmico 1,5 mm²', "electricidad", 2.21, 36.78, "F-03-03", 5),
    "AL-0072": Producto("AL-0072", 'Magnetotérmico 2,5 mm²', "electricidad", 1.929, 12.22, "B-09-01", 100),
    "AL-0073": Producto("AL-0073", 'Magnetotérmico 4 mm²', "electricidad", 0.258, 17.12, "D-03-03", 5),
    "AL-0074": Producto("AL-0074", 'Magnetotérmico 16 A', "electricidad", 1.116, 22.1, "G-09-05", 10),
    "AL-0075": Producto("AL-0075", 'Magnetotérmico 25 A', "electricidad", 0.152, 45.58, "B-06-03", 5),
    "AL-0076": Producto("AL-0076", 'Manguera 1,5 mm²', "electricidad", 0.476, 59.7, "E-17-02", 20),
    "AL-0077": Producto("AL-0077", 'Manguera 2,5 mm²', "electricidad", 1.097, 43.21, "E-12-01", 20),
    "AL-0078": Producto("AL-0078", 'Manguera 4 mm²', "electricidad", 0.137, 1.77, "D-17-04", 10),
    "AL-0079": Producto("AL-0079", 'Manguera 16 A', "electricidad", 2.246, 7.34, "G-22-04", 50),
    "AL-0080": Producto("AL-0080", 'Manguera 25 A', "electricidad", 2.011, 25.52, "E-23-02", 10),
    "AL-0081": Producto("AL-0081", 'Codo de cobre 1/2"', "fontaneria", 0.568, 31.69, "C-13-03", 5),
    "AL-0082": Producto("AL-0082", 'Codo de cobre 3/4"', "fontaneria", 1.344, 0.94, "E-14-02", 5),
    "AL-0083": Producto("AL-0083", 'Codo de cobre 1"', "fontaneria", 0.163, 32.03, "E-20-02", 100),
    "AL-0084": Producto("AL-0084", 'Codo de cobre 16 mm', "fontaneria", 0.49, 17.68, "C-09-04", 5),
    "AL-0085": Producto("AL-0085", 'Codo de cobre 20 mm', "fontaneria", 0.443, 36.56, "F-08-01", 20),
    "AL-0086": Producto("AL-0086", 'Manguito 1/2"', "fontaneria", 0.372, 7.28, "F-13-01", 25),
    "AL-0087": Producto("AL-0087", 'Manguito 3/4"', "fontaneria", 0.468, 25.07, "D-17-01", 5),
    "AL-0088": Producto("AL-0088", 'Manguito 1"', "fontaneria", 0.445, 3.77, "G-19-01", 25),
    "AL-0089": Producto("AL-0089", 'Manguito 16 mm', "fontaneria", 0.065, 11.84, "D-03-05", 50),
    "AL-0090": Producto("AL-0090", 'Manguito 20 mm', "fontaneria", 1.37, 6.24, "G-11-06", 25),
    "AL-0091": Producto("AL-0091", 'Llave de paso 1/2"', "fontaneria", 0.265, 27.63, "C-02-06", 50),
    "AL-0092": Producto("AL-0092", 'Llave de paso 3/4"', "fontaneria", 1.015, 27.99, "C-17-05", 50),
    "AL-0093": Producto("AL-0093", 'Llave de paso 1"', "fontaneria", 1.341, 30.66, "D-03-01", 5),
    "AL-0094": Producto("AL-0094", 'Llave de paso 16 mm', "fontaneria", 0.239, 13.96, "B-13-04", 50),
    "AL-0095": Producto("AL-0095", 'Llave de paso 20 mm', "fontaneria", 0.11, 1.11, "D-16-03", 5),
    "AL-0096": Producto("AL-0096", 'Tubo multicapa 1/2"', "fontaneria", 0.747, 3.04, "B-22-05", 5),
    "AL-0097": Producto("AL-0097", 'Tubo multicapa 3/4"', "fontaneria", 1.201, 18.22, "B-09-02", 100),
    "AL-0098": Producto("AL-0098", 'Tubo multicapa 1"', "fontaneria", 1.218, 9.08, "H-16-04", 5),
    "AL-0099": Producto("AL-0099", 'Tubo multicapa 16 mm', "fontaneria", 0.782, 26.11, "A-20-06", 100),
    "AL-0100": Producto("AL-0100", 'Tubo multicapa 20 mm', "fontaneria", 0.341, 22.95, "F-09-06", 100),
    "AL-0101": Producto("AL-0101", 'Sifón 1/2"', "fontaneria", 1.118, 23.76, "C-01-04", 5),
    "AL-0102": Producto("AL-0102", 'Sifón 3/4"', "fontaneria", 0.793, 36.97, "B-23-02", 100),
    "AL-0103": Producto("AL-0103", 'Sifón 1"', "fontaneria", 0.799, 27.05, "E-15-04", 25),
    "AL-0104": Producto("AL-0104", 'Sifón 16 mm', "fontaneria", 1.234, 37.75, "D-10-01", 25),
    "AL-0105": Producto("AL-0105", 'Sifón 20 mm', "fontaneria", 0.057, 17.66, "H-09-04", 10),
    "AL-0106": Producto("AL-0106", 'Latiguillo 1/2"', "fontaneria", 1.469, 35.39, "B-19-01", 10),
    "AL-0107": Producto("AL-0107", 'Latiguillo 3/4"', "fontaneria", 1.204, 10.24, "F-05-05", 100),
    "AL-0108": Producto("AL-0108", 'Latiguillo 1"', "fontaneria", 0.829, 33.75, "F-08-04", 25),
    "AL-0109": Producto("AL-0109", 'Latiguillo 16 mm', "fontaneria", 0.649, 6.38, "H-22-04", 25),
    "AL-0110": Producto("AL-0110", 'Latiguillo 20 mm', "fontaneria", 0.504, 5.69, "F-13-03", 5),
    "AL-0111": Producto("AL-0111", 'Te de latón 1/2"', "fontaneria", 1.349, 0.47, "F-13-01", 10),
    "AL-0112": Producto("AL-0112", 'Te de latón 3/4"', "fontaneria", 1.149, 34.3, "E-09-03", 5),
    "AL-0113": Producto("AL-0113", 'Te de latón 1"', "fontaneria", 0.647, 37.95, "B-12-04", 20),
    "AL-0114": Producto("AL-0114", 'Te de latón 16 mm', "fontaneria", 1.371, 10.95, "A-22-03", 100),
    "AL-0115": Producto("AL-0115", 'Te de latón 20 mm', "fontaneria", 1.499, 9.77, "E-14-05", 20),
    "AL-0116": Producto("AL-0116", 'Válvula antirretorno 1/2"', "fontaneria", 0.328, 14.44, "G-01-06", 25),
    "AL-0117": Producto("AL-0117", 'Válvula antirretorno 3/4"', "fontaneria", 1.464, 35.77, "D-24-01", 5),
    "AL-0118": Producto("AL-0118", 'Válvula antirretorno 1"', "fontaneria", 1.496, 15.85, "C-21-03", 25),
    "AL-0119": Producto("AL-0119", 'Válvula antirretorno 16 mm', "fontaneria", 0.107, 35.25, "C-06-04", 25),
    "AL-0120": Producto("AL-0120", 'Válvula antirretorno 20 mm', "fontaneria", 0.57, 11.6, "E-13-06", 10),
    "AL-0121": Producto("AL-0121", 'Pintura plástica 750 ml', "pintura", 6.387, 50.13, "G-04-02", 100),
    "AL-0122": Producto("AL-0122", 'Pintura plástica 4 l', "pintura", 3.479, 19.45, "H-18-02", 25),
    "AL-0123": Producto("AL-0123", 'Pintura plástica 15 l', "pintura", 19.041, 88.69, "H-14-02", 50),
    "AL-0124": Producto("AL-0124", 'Pintura plástica 30 mm', "pintura", 4.121, 9.16, "F-18-01", 20),
    "AL-0125": Producto("AL-0125", 'Pintura plástica 50 mm', "pintura", 5.098, 23.88, "D-01-06", 25),
    "AL-0126": Producto("AL-0126", 'Esmalte sintético 750 ml', "pintura", 8.101, 66.68, "D-13-03", 20),
    "AL-0127": Producto("AL-0127", 'Esmalte sintético 4 l', "pintura", 15.819, 44.94, "F-05-06", 50),
    "AL-0128": Producto("AL-0128", 'Esmalte sintético 15 l', "pintura", 11.161, 70.59, "D-03-03", 10),
    "AL-0129": Producto("AL-0129", 'Esmalte sintético 30 mm', "pintura", 8.137, 57.9, "G-10-01", 10),
    "AL-0130": Producto("AL-0130", 'Esmalte sintético 50 mm', "pintura", 0.774, 63.5, "H-19-04", 5),
    "AL-0131": Producto("AL-0131", 'Imprimación 750 ml', "pintura", 1.629, 82.87, "H-15-02", 5),
    "AL-0132": Producto("AL-0132", 'Imprimación 4 l', "pintura", 4.777, 14.55, "B-24-06", 100),
    "AL-0133": Producto("AL-0133", 'Imprimación 15 l', "pintura", 17.792, 79.77, "B-18-01", 5),
    "AL-0134": Producto("AL-0134", 'Imprimación 30 mm', "pintura", 16.45, 21.62, "A-21-06", 20),
    "AL-0135": Producto("AL-0135", 'Imprimación 50 mm', "pintura", 20.215, 56.2, "G-23-01", 5),
    "AL-0136": Producto("AL-0136", 'Barniz 750 ml', "pintura", 1.57, 47.25, "D-13-03", 10),
    "AL-0137": Producto("AL-0137", 'Barniz 4 l', "pintura", 16.621, 1.3, "E-15-03", 20),
    "AL-0138": Producto("AL-0138", 'Barniz 15 l', "pintura", 13.572, 78.8, "H-17-02", 50),
    "AL-0139": Producto("AL-0139", 'Barniz 30 mm', "pintura", 5.264, 85.54, "E-02-01", 10),
    "AL-0140": Producto("AL-0140", 'Barniz 50 mm', "pintura", 10.515, 60.42, "G-03-03", 10),
    "AL-0141": Producto("AL-0141", 'Rodillo 750 ml', "pintura", 14.048, 82.43, "D-16-01", 100),
    "AL-0142": Producto("AL-0142", 'Rodillo 4 l', "pintura", 7.165, 38.12, "G-07-01", 20),
    "AL-0143": Producto("AL-0143", 'Rodillo 15 l', "pintura", 15.548, 45.53, "D-16-02", 20),
    "AL-0144": Producto("AL-0144", 'Rodillo 30 mm', "pintura", 16.106, 18.23, "H-08-03", 20),
    "AL-0145": Producto("AL-0145", 'Rodillo 50 mm', "pintura", 2.378, 55.95, "C-08-04", 25),
    "AL-0146": Producto("AL-0146", 'Brocha 750 ml', "pintura", 19.127, 6.15, "C-13-01", 10),
    "AL-0147": Producto("AL-0147", 'Brocha 4 l', "pintura", 0.594, 53.54, "G-02-06", 5),
    "AL-0148": Producto("AL-0148", 'Brocha 15 l', "pintura", 3.948, 40.68, "F-24-01", 5),
    "AL-0149": Producto("AL-0149", 'Brocha 30 mm', "pintura", 19.57, 30.11, "C-21-05", 100),
    "AL-0150": Producto("AL-0150", 'Brocha 50 mm', "pintura", 9.873, 28.58, "G-12-03", 25),
    "AL-0151": Producto("AL-0151", 'Cinta de carrocero 750 ml', "pintura", 3.638, 1.45, "E-03-03", 25),
    "AL-0152": Producto("AL-0152", 'Cinta de carrocero 4 l', "pintura", 20.07, 12.06, "D-13-03", 20),
    "AL-0153": Producto("AL-0153", 'Cinta de carrocero 15 l', "pintura", 17.28, 39.17, "A-23-04", 10),
    "AL-0154": Producto("AL-0154", 'Cinta de carrocero 30 mm', "pintura", 7.89, 81.93, "D-11-03", 100),
    "AL-0155": Producto("AL-0155", 'Cinta de carrocero 50 mm', "pintura", 18.847, 3.86, "G-08-06", 25),
    "AL-0156": Producto("AL-0156", 'Disolvente 750 ml', "pintura", 0.95, 4.26, "B-02-03", 10),
    "AL-0157": Producto("AL-0157", 'Disolvente 4 l', "pintura", 15.718, 80.09, "F-12-03", 20),
    "AL-0158": Producto("AL-0158", 'Disolvente 15 l', "pintura", 20.116, 55.37, "E-24-06", 100),
    "AL-0159": Producto("AL-0159", 'Disolvente 30 mm', "pintura", 6.715, 25.4, "A-24-05", 100),
    "AL-0160": Producto("AL-0160", 'Disolvente 50 mm', "pintura", 19.882, 6.94, "D-04-04", 100),
    "AL-0161": Producto("AL-0161", 'Martillo de carpintero pequeño', "herramienta", 1.726, 44.0, "G-09-04", 25),
    "AL-0162": Producto("AL-0162", 'Martillo de carpintero mediano', "herramienta", 0.308, 24.1, "A-24-03", 100),
    "AL-0163": Producto("AL-0163", 'Martillo de carpintero grande', "herramienta", 1.409, 28.92, "F-11-04", 20),
    "AL-0164": Producto("AL-0164", 'Martillo de carpintero PZ2', "herramienta", 1.428, 28.41, "D-13-02", 10),
    "AL-0165": Producto("AL-0165", 'Martillo de carpintero 8"', "herramienta", 0.781, 30.76, "H-18-05", 20),
    "AL-0166": Producto("AL-0166", 'Destornillador pequeño', "herramienta", 0.356, 21.06, "B-03-03", 50),
    "AL-0167": Producto("AL-0167", 'Destornillador mediano', "herramienta", 0.225, 6.69, "H-23-04", 10),
    "AL-0168": Producto("AL-0168", 'Destornillador grande', "herramienta", 0.483, 20.63, "D-24-05", 100),
    "AL-0169": Producto("AL-0169", 'Destornillador PZ2', "herramienta", 1.386, 36.42, "E-10-03", 50),
    "AL-0170": Producto("AL-0170", 'Destornillador 8"', "herramienta", 0.54, 13.55, "E-07-04", 10),
    "AL-0171": Producto("AL-0171", 'Alicate universal pequeño', "herramienta", 0.399, 12.74, "E-19-02", 20),
    "AL-0172": Producto("AL-0172", 'Alicate universal mediano', "herramienta", 0.191, 13.45, "D-17-05", 10),
    "AL-0173": Producto("AL-0173", 'Alicate universal grande', "herramienta", 1.197, 6.87, "H-02-01", 5),
    "AL-0174": Producto("AL-0174", 'Alicate universal PZ2', "herramienta", 0.897, 38.13, "H-12-01", 20),
    "AL-0175": Producto("AL-0175", 'Alicate universal 8"', "herramienta", 0.481, 4.69, "D-03-03", 50),
    "AL-0176": Producto("AL-0176", 'Llave inglesa pequeño', "herramienta", 1.57, 22.04, "E-22-01", 5),
    "AL-0177": Producto("AL-0177", 'Llave inglesa mediano', "herramienta", 1.176, 33.37, "F-07-01", 20),
    "AL-0178": Producto("AL-0178", 'Llave inglesa grande', "herramienta", 0.665, 4.42, "E-02-05", 100),
    "AL-0179": Producto("AL-0179", 'Llave inglesa PZ2', "herramienta", 1.201, 11.35, "A-11-04", 100),
    "AL-0180": Producto("AL-0180", 'Llave inglesa 8"', "herramienta", 0.72, 29.51, "B-07-01", 25),
    "AL-0181": Producto("AL-0181", 'Nivel de burbuja pequeño', "herramienta", 1.023, 5.25, "B-13-06", 50),
    "AL-0182": Producto("AL-0182", 'Nivel de burbuja mediano', "herramienta", 0.346, 25.73, "C-13-06", 20),
    "AL-0183": Producto("AL-0183", 'Nivel de burbuja grande', "herramienta", 0.785, 14.82, "E-14-01", 20),
    "AL-0184": Producto("AL-0184", 'Nivel de burbuja PZ2', "herramienta", 1.362, 40.94, "G-14-01", 20),
    "AL-0185": Producto("AL-0185", 'Nivel de burbuja 8"', "herramienta", 1.189, 19.5, "G-07-01", 25),
    "AL-0186": Producto("AL-0186", 'Flexómetro pequeño', "herramienta", 1.631, 20.93, "B-13-05", 20),
    "AL-0187": Producto("AL-0187", 'Flexómetro mediano', "herramienta", 0.873, 9.57, "A-02-05", 10),
    "AL-0188": Producto("AL-0188", 'Flexómetro grande', "herramienta", 1.182, 42.08, "B-19-05", 20),
    "AL-0189": Producto("AL-0189", 'Flexómetro PZ2', "herramienta", 1.348, 9.97, "F-10-02", 50),
    "AL-0190": Producto("AL-0190", 'Flexómetro 8"', "herramienta", 0.375, 5.42, "G-16-02", 20),
    "AL-0191": Producto("AL-0191", 'Sierra de costilla pequeño', "herramienta", 0.298, 43.52, "H-11-01", 50),
    "AL-0192": Producto("AL-0192", 'Sierra de costilla mediano', "herramienta", 1.673, 19.37, "C-21-02", 50),
    "AL-0193": Producto("AL-0193", 'Sierra de costilla grande', "herramienta", 0.776, 39.32, "H-06-05", 10),
    "AL-0194": Producto("AL-0194", 'Sierra de costilla PZ2', "herramienta", 0.152, 43.33, "C-13-03", 5),
    "AL-0195": Producto("AL-0195", 'Sierra de costilla 8"', "herramienta", 0.337, 44.73, "D-02-05", 100),
    "AL-0196": Producto("AL-0196", 'Juego de brocas pequeño', "herramienta", 0.146, 38.96, "B-13-05", 25),
    "AL-0197": Producto("AL-0197", 'Juego de brocas mediano', "herramienta", 1.026, 29.78, "E-21-04", 20),
    "AL-0198": Producto("AL-0198", 'Juego de brocas grande', "herramienta", 1.082, 21.02, "F-15-05", 25),
    "AL-0199": Producto("AL-0199", 'Juego de brocas PZ2', "herramienta", 0.387, 2.65, "H-15-02", 25),
    "AL-0200": Producto("AL-0200", 'Juego de brocas 8"', "herramienta", 1.393, 36.43, "H-06-04", 25),
    "AL-0201": Producto("AL-0201", 'Manguera de riego 15 m', "jardin", 2.963, 10.82, "G-12-01", 25),
    "AL-0202": Producto("AL-0202", 'Manguera de riego 25 m', "jardin", 13.212, 47.93, "A-21-02", 5),
    "AL-0203": Producto("AL-0203", 'Manguera de riego 50 l', "jardin", 23.991, 23.82, "B-02-05", 25),
    "AL-0204": Producto("AL-0204", 'Manguera de riego 1 kg', "jardin", 17.041, 56.85, "A-03-05", 100),
    "AL-0205": Producto("AL-0205", 'Manguera de riego 5 kg', "jardin", 18.068, 9.49, "C-16-03", 10),
    "AL-0206": Producto("AL-0206", 'Aspersor 15 m', "jardin", 17.902, 52.42, "D-03-03", 50),
    "AL-0207": Producto("AL-0207", 'Aspersor 25 m', "jardin", 19.709, 12.95, "E-15-02", 20),
    "AL-0208": Producto("AL-0208", 'Aspersor 50 l', "jardin", 13.157, 66.38, "D-19-03", 50),
    "AL-0209": Producto("AL-0209", 'Aspersor 1 kg', "jardin", 13.255, 24.2, "A-07-02", 25),
    "AL-0210": Producto("AL-0210", 'Aspersor 5 kg', "jardin", 4.36, 67.54, "F-13-02", 20),
    "AL-0211": Producto("AL-0211", 'Tijera de podar 15 m', "jardin", 3.169, 39.06, "F-15-05", 50),
    "AL-0212": Producto("AL-0212", 'Tijera de podar 25 m', "jardin", 15.165, 63.75, "B-09-05", 100),
    "AL-0213": Producto("AL-0213", 'Tijera de podar 50 l', "jardin", 22.3, 53.6, "F-09-04", 20),
    "AL-0214": Producto("AL-0214", 'Tijera de podar 1 kg', "jardin", 15.096, 27.09, "B-15-02", 10),
    "AL-0215": Producto("AL-0215", 'Tijera de podar 5 kg', "jardin", 16.077, 69.05, "E-17-03", 20),
    "AL-0216": Producto("AL-0216", 'Saco de sustrato 15 m', "jardin", 16.692, 70.88, "F-24-01", 100),
    "AL-0217": Producto("AL-0217", 'Saco de sustrato 25 m', "jardin", 1.072, 12.29, "G-14-05", 20),
    "AL-0218": Producto("AL-0218", 'Saco de sustrato 50 l', "jardin", 23.305, 11.07, "D-20-06", 5),
    "AL-0219": Producto("AL-0219", 'Saco de sustrato 1 kg', "jardin", 0.775, 1.98, "F-10-01", 50),
    "AL-0220": Producto("AL-0220", 'Saco de sustrato 5 kg', "jardin", 9.415, 17.54, "E-19-02", 10),
    "AL-0221": Producto("AL-0221", 'Programador de riego 15 m', "jardin", 9.649, 59.96, "C-05-01", 10),
    "AL-0222": Producto("AL-0222", 'Programador de riego 25 m', "jardin", 18.453, 33.45, "B-21-02", 100),
    "AL-0223": Producto("AL-0223", 'Programador de riego 50 l', "jardin", 20.38, 30.02, "E-01-01", 100),
    "AL-0224": Producto("AL-0224", 'Programador de riego 1 kg', "jardin", 21.379, 64.47, "H-20-05", 100),
    "AL-0225": Producto("AL-0225", 'Programador de riego 5 kg', "jardin", 12.916, 13.39, "A-02-01", 50),
    "AL-0226": Producto("AL-0226", 'Pistola de riego 15 m', "jardin", 0.851, 14.83, "C-02-01", 5),
    "AL-0227": Producto("AL-0227", 'Pistola de riego 25 m', "jardin", 16.006, 47.91, "D-05-04", 10),
    "AL-0228": Producto("AL-0228", 'Pistola de riego 50 l', "jardin", 13.571, 46.92, "G-20-02", 50),
    "AL-0229": Producto("AL-0229", 'Pistola de riego 1 kg', "jardin", 8.182, 22.88, "A-24-04", 100),
    "AL-0230": Producto("AL-0230", 'Pistola de riego 5 kg', "jardin", 14.091, 28.14, "G-24-04", 5),
    "AL-0231": Producto("AL-0231", 'Malla de sombreo 15 m', "jardin", 19.337, 33.56, "D-04-03", 10),
    "AL-0232": Producto("AL-0232", 'Malla de sombreo 25 m', "jardin", 16.816, 10.45, "E-23-01", 20),
    "AL-0233": Producto("AL-0233", 'Malla de sombreo 50 l', "jardin", 16.605, 49.48, "E-10-06", 10),
    "AL-0234": Producto("AL-0234", 'Malla de sombreo 1 kg', "jardin", 2.404, 37.42, "C-09-02", 100),
    "AL-0235": Producto("AL-0235", 'Malla de sombreo 5 kg', "jardin", 5.432, 12.97, "F-07-04", 20),
    "AL-0236": Producto("AL-0236", 'Abono granulado 15 m', "jardin", 15.712, 28.44, "H-16-05", 100),
    "AL-0237": Producto("AL-0237", 'Abono granulado 25 m', "jardin", 0.365, 3.66, "D-19-03", 10),
    "AL-0238": Producto("AL-0238", 'Abono granulado 50 l', "jardin", 10.302, 42.89, "C-05-01", 5),
    "AL-0239": Producto("AL-0239", 'Abono granulado 1 kg', "jardin", 3.087, 45.46, "C-12-02", 100),
    "AL-0240": Producto("AL-0240", 'Abono granulado 5 kg', "jardin", 0.941, 4.72, "A-23-01", 100),
}


class SinStock(Exception):
    """No hay unidades disponibles suficientes para reservar."""

    def __init__(self, sku, pedidas, disponibles):
        super().__init__(f"{sku}: se piden {pedidas} y hay {disponibles} disponibles")
        self.sku, self.pedidas, self.disponibles = sku, pedidas, disponibles


@dataclass
class Movimiento:
    sku: str
    cantidad: int  # positiva: entrada; negativa: salida
    motivo: str
    almacen: str = "MAD"


@dataclass
class Inventario:
    existencias: dict = field(default_factory=dict)  # sku -> unidades en el almacén
    reservado: dict = field(default_factory=dict)  # sku -> unidades reservadas para pedidos
    movimientos: list = field(default_factory=list)

    def entrada(self, sku, cantidad, motivo="compra", almacen="MAD"):
        if cantidad <= 0:
            raise ValueError("la cantidad de una entrada ha de ser positiva")
        self.existencias[sku] = self.existencias.get(sku, 0) + cantidad
        self.movimientos.append(Movimiento(sku, cantidad, motivo, almacen))

    def salida(self, sku, cantidad, motivo="venta", almacen="MAD"):
        if cantidad > self.existencias.get(sku, 0):
            raise SinStock(sku, cantidad, self.existencias.get(sku, 0))
        self.existencias[sku] -= cantidad
        self.movimientos.append(Movimiento(sku, -cantidad, motivo, almacen))

    def disponible(self, sku):
        """Unidades que se pueden reservar: las existencias menos lo reservado y el margen de seguridad."""
        return max(self.existencias.get(sku, 0) - self.reservado.get(sku, 0) - MARGEN_SEGURIDAD, 0)

    def reserva_stock(self, sku, cantidad):
        """Aparta unidades para un pedido. Lanza SinStock si no hay bastantes disponibles."""
        if cantidad <= 0:
            raise ValueError("la cantidad de una reserva ha de ser positiva")
        libres = self.disponible(sku)
        if cantidad > libres:
            raise SinStock(sku, cantidad, libres)
        self.reservado[sku] = self.reservado.get(sku, 0) + cantidad

    def libera(self, sku, cantidad):
        """Deshace (parte de) una reserva."""
        self.reservado[sku] = max(self.reservado.get(sku, 0) - cantidad, 0)

    def bajo_minimo(self):
        """Los SKU del catálogo cuyas existencias están por debajo de su stock mínimo."""
        return [sku for sku, p in CATALOGO.items() if self.existencias.get(sku, 0) < p.stock_minimo]


def productos_tornilleria():
    """Los productos de la familia tornilleria, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "tornilleria"]


def valor_stock_tornilleria(inventario):
    """Valor a precio de tarifa de las existencias de tornilleria."""
    total = 0.0
    for p in productos_tornilleria():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_tornilleria(inventario, contado):
    """Diferencias de un recuento físico de tornilleria: sku -> contado - registrado."""
    dif = {}
    for p in productos_tornilleria():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_tornilleria():
    """Pasillos donde hay productos de tornilleria."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_tornilleria()})


def productos_electricidad():
    """Los productos de la familia electricidad, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "electricidad"]


def valor_stock_electricidad(inventario):
    """Valor a precio de tarifa de las existencias de electricidad."""
    total = 0.0
    for p in productos_electricidad():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_electricidad(inventario, contado):
    """Diferencias de un recuento físico de electricidad: sku -> contado - registrado."""
    dif = {}
    for p in productos_electricidad():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_electricidad():
    """Pasillos donde hay productos de electricidad."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_electricidad()})


def productos_fontaneria():
    """Los productos de la familia fontaneria, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "fontaneria"]


def valor_stock_fontaneria(inventario):
    """Valor a precio de tarifa de las existencias de fontaneria."""
    total = 0.0
    for p in productos_fontaneria():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_fontaneria(inventario, contado):
    """Diferencias de un recuento físico de fontaneria: sku -> contado - registrado."""
    dif = {}
    for p in productos_fontaneria():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_fontaneria():
    """Pasillos donde hay productos de fontaneria."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_fontaneria()})


def productos_pintura():
    """Los productos de la familia pintura, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "pintura"]


def valor_stock_pintura(inventario):
    """Valor a precio de tarifa de las existencias de pintura."""
    total = 0.0
    for p in productos_pintura():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_pintura(inventario, contado):
    """Diferencias de un recuento físico de pintura: sku -> contado - registrado."""
    dif = {}
    for p in productos_pintura():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_pintura():
    """Pasillos donde hay productos de pintura."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_pintura()})


def productos_herramienta():
    """Los productos de la familia herramienta, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "herramienta"]


def valor_stock_herramienta(inventario):
    """Valor a precio de tarifa de las existencias de herramienta."""
    total = 0.0
    for p in productos_herramienta():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_herramienta(inventario, contado):
    """Diferencias de un recuento físico de herramienta: sku -> contado - registrado."""
    dif = {}
    for p in productos_herramienta():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_herramienta():
    """Pasillos donde hay productos de herramienta."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_herramienta()})


def productos_jardin():
    """Los productos de la familia jardin, por SKU."""
    return [p for p in CATALOGO.values() if p.familia == "jardin"]


def valor_stock_jardin(inventario):
    """Valor a precio de tarifa de las existencias de jardin."""
    total = 0.0
    for p in productos_jardin():
        total += inventario.existencias.get(p.sku, 0) * p.precio
    return round(total, 2)


def recuento_jardin(inventario, contado):
    """Diferencias de un recuento físico de jardin: sku -> contado - registrado."""
    dif = {}
    for p in productos_jardin():
        d = contado.get(p.sku, 0) - inventario.existencias.get(p.sku, 0)
        if d:
            dif[p.sku] = d
    return dif


def pasillos_jardin():
    """Pasillos donde hay productos de jardin."""
    return sorted({p.ubicacion.split("-")[0] for p in productos_jardin()})

