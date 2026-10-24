// Package almacen es la gestión de un almacén de ferretería: catálogo e inventario, pedidos y clientes, precios y
// descuentos, envíos, informes y devoluciones.
package almacen

// Inventario del almacén: catálogo de productos, existencias por almacén, reservas y movimientos.
//
// Las existencias se llevan por SKU (y, si se quiere, por almacén). Una reserva aparta unidades para un pedido
// confirmado: no salen del almacén hasta que se preparan, pero dejan de estar disponibles para otros pedidos.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// MargenSeguridad son las unidades de cada producto que nunca se reservan: cubren roturas y errores de recuento del
// inventario.
const MargenSeguridad = 7

// Almacenes son los almacenes de salida.
var Almacenes = []string{"MAD", "BCN", "VLC", "SEV", "BIL"}

// Producto es un artículo del catálogo.
type Producto struct {
	SKU         string
	Nombre      string
	Familia     string
	PesoKg      float64
	Precio      float64
	Ubicacion   string
	StockMinimo int
}

// Catalogo: SKU -> Producto (nombre, familia, peso en kg, precio de tarifa sin IVA, ubicación pasillo-estantería-balda
// y stock mínimo antes de pedir al proveedor).
var Catalogo = map[string]Producto{
	"AL-0001": {"AL-0001", "Tornillo hexagonal M4", "tornilleria", 0.032, 0.31, "A-03-05", 5},
	"AL-0002": {"AL-0002", "Tornillo hexagonal M5", "tornilleria", 0.035, 0.14, "D-02-01", 25},
	"AL-0003": {"AL-0003", "Tornillo hexagonal M6", "tornilleria", 0.04, 0.48, "G-02-05", 5},
	"AL-0004": {"AL-0004", "Tornillo hexagonal M8", "tornilleria", 0.085, 1.21, "A-19-05", 25},
	"AL-0005": {"AL-0005", "Tornillo hexagonal M10", "tornilleria", 0.008, 0.44, "C-10-04", 10},
	"AL-0006": {"AL-0006", "Tornillo alomado M4", "tornilleria", 0.05, 1.1, "C-04-05", 50},
	"AL-0007": {"AL-0007", "Tornillo alomado M5", "tornilleria", 0.059, 0.73, "B-19-01", 50},
	"AL-0008": {"AL-0008", "Tornillo alomado M6", "tornilleria", 0.022, 1.3, "G-11-04", 50},
	"AL-0009": {"AL-0009", "Tornillo alomado M8", "tornilleria", 0.083, 0.71, "D-06-06", 10},
	"AL-0010": {"AL-0010", "Tornillo alomado M10", "tornilleria", 0.011, 0.59, "H-11-06", 25},
	"AL-0011": {"AL-0011", "Tuerca autoblocante M4", "tornilleria", 0.029, 1.86, "B-17-04", 10},
	"AL-0012": {"AL-0012", "Tuerca autoblocante M5", "tornilleria", 0.069, 0.31, "H-14-01", 100},
	"AL-0013": {"AL-0013", "Tuerca autoblocante M6", "tornilleria", 0.011, 1.07, "F-11-06", 20},
	"AL-0014": {"AL-0014", "Tuerca autoblocante M8", "tornilleria", 0.055, 1.11, "H-03-01", 20},
	"AL-0015": {"AL-0015", "Tuerca autoblocante M10", "tornilleria", 0.045, 1.27, "A-24-06", 20},
	"AL-0016": {"AL-0016", "Arandela plana M4", "tornilleria", 0.06, 1.89, "H-10-06", 25},
	"AL-0017": {"AL-0017", "Arandela plana M5", "tornilleria", 0.08, 0.68, "H-12-02", 50},
	"AL-0018": {"AL-0018", "Arandela plana M6", "tornilleria", 0.014, 0.14, "E-05-06", 10},
	"AL-0019": {"AL-0019", "Arandela plana M8", "tornilleria", 0.038, 1.74, "H-03-02", 25},
	"AL-0020": {"AL-0020", "Arandela plana M10", "tornilleria", 0.039, 0.55, "C-14-05", 20},
	"AL-0021": {"AL-0021", "Varilla roscada M4", "tornilleria", 0.065, 1.87, "G-08-02", 5},
	"AL-0022": {"AL-0022", "Varilla roscada M5", "tornilleria", 0.019, 0.46, "D-01-04", 50},
	"AL-0023": {"AL-0023", "Varilla roscada M6", "tornilleria", 0.02, 0.56, "C-14-05", 20},
	"AL-0024": {"AL-0024", "Varilla roscada M8", "tornilleria", 0.056, 0.63, "C-23-05", 50},
	"AL-0025": {"AL-0025", "Varilla roscada M10", "tornilleria", 0.06, 1.41, "H-22-05", 25},
	"AL-0026": {"AL-0026", "Taco de nailon M4", "tornilleria", 0.038, 0.77, "H-21-04", 5},
	"AL-0027": {"AL-0027", "Taco de nailon M5", "tornilleria", 0.02, 1.87, "H-06-01", 20},
	"AL-0028": {"AL-0028", "Taco de nailon M6", "tornilleria", 0.056, 0.22, "C-18-01", 20},
	"AL-0029": {"AL-0029", "Taco de nailon M8", "tornilleria", 0.057, 0.16, "D-20-04", 10},
	"AL-0030": {"AL-0030", "Taco de nailon M10", "tornilleria", 0.059, 1.82, "F-16-01", 5},
	"AL-0031": {"AL-0031", "Tirafondo M4", "tornilleria", 0.077, 1.89, "H-16-04", 20},
	"AL-0032": {"AL-0032", "Tirafondo M5", "tornilleria", 0.011, 0.22, "F-24-03", 25},
	"AL-0033": {"AL-0033", "Tirafondo M6", "tornilleria", 0.075, 0.33, "A-07-05", 20},
	"AL-0034": {"AL-0034", "Tirafondo M8", "tornilleria", 0.017, 1.05, "A-17-03", 100},
	"AL-0035": {"AL-0035", "Tirafondo M10", "tornilleria", 0.078, 1.33, "E-17-03", 10},
	"AL-0036": {"AL-0036", "Remache ciego M4", "tornilleria", 0.035, 0.45, "F-21-02", 50},
	"AL-0037": {"AL-0037", "Remache ciego M5", "tornilleria", 0.074, 1.87, "D-08-04", 100},
	"AL-0038": {"AL-0038", "Remache ciego M6", "tornilleria", 0.073, 0.4, "H-12-06", 5},
	"AL-0039": {"AL-0039", "Remache ciego M8", "tornilleria", 0.089, 1.51, "H-09-02", 100},
	"AL-0040": {"AL-0040", "Remache ciego M10", "tornilleria", 0.056, 0.67, "F-12-01", 10},
	"AL-0041": {"AL-0041", "Cable flexible 1,5 mm²", "electricidad", 0.29, 30.4, "F-07-04", 50},
	"AL-0042": {"AL-0042", "Cable flexible 2,5 mm²", "electricidad", 2.365, 39.29, "A-16-06", 20},
	"AL-0043": {"AL-0043", "Cable flexible 4 mm²", "electricidad", 1.929, 5.97, "B-13-06", 10},
	"AL-0044": {"AL-0044", "Cable flexible 16 A", "electricidad", 1.173, 11.92, "F-03-06", 25},
	"AL-0045": {"AL-0045", "Cable flexible 25 A", "electricidad", 1.138, 47.73, "B-24-02", 10},
	"AL-0046": {"AL-0046", "Regleta 1,5 mm²", "electricidad", 2.384, 2.35, "H-21-02", 50},
	"AL-0047": {"AL-0047", "Regleta 2,5 mm²", "electricidad", 1.992, 62.75, "F-05-05", 50},
	"AL-0048": {"AL-0048", "Regleta 4 mm²", "electricidad", 0.358, 1.5, "B-17-06", 10},
	"AL-0049": {"AL-0049", "Regleta 16 A", "electricidad", 1.069, 55.87, "D-01-03", 10},
	"AL-0050": {"AL-0050", "Regleta 25 A", "electricidad", 0.738, 15.85, "F-09-05", 25},
	"AL-0051": {"AL-0051", "Interruptor 1,5 mm²", "electricidad", 2.01, 4.46, "F-15-06", 50},
	"AL-0052": {"AL-0052", "Interruptor 2,5 mm²", "electricidad", 1.965, 33.36, "C-18-02", 50},
	"AL-0053": {"AL-0053", "Interruptor 4 mm²", "electricidad", 1.25, 55.94, "C-20-01", 10},
	"AL-0054": {"AL-0054", "Interruptor 16 A", "electricidad", 0.455, 30.62, "B-18-01", 20},
	"AL-0055": {"AL-0055", "Interruptor 25 A", "electricidad", 1.653, 34.25, "H-04-05", 5},
	"AL-0056": {"AL-0056", "Base de enchufe 1,5 mm²", "electricidad", 0.634, 18.16, "B-17-04", 50},
	"AL-0057": {"AL-0057", "Base de enchufe 2,5 mm²", "electricidad", 0.115, 57.28, "B-15-03", 50},
	"AL-0058": {"AL-0058", "Base de enchufe 4 mm²", "electricidad", 2.337, 39.03, "D-23-03", 25},
	"AL-0059": {"AL-0059", "Base de enchufe 16 A", "electricidad", 1.244, 51.79, "D-23-05", 20},
	"AL-0060": {"AL-0060", "Base de enchufe 25 A", "electricidad", 2.219, 57.2, "D-15-02", 25},
	"AL-0061": {"AL-0061", "Caja de empalme 1,5 mm²", "electricidad", 0.336, 28.63, "B-22-02", 25},
	"AL-0062": {"AL-0062", "Caja de empalme 2,5 mm²", "electricidad", 0.222, 43.04, "B-05-06", 100},
	"AL-0063": {"AL-0063", "Caja de empalme 4 mm²", "electricidad", 1.602, 9.66, "C-15-02", 100},
	"AL-0064": {"AL-0064", "Caja de empalme 16 A", "electricidad", 2.288, 25.85, "H-06-06", 10},
	"AL-0065": {"AL-0065", "Caja de empalme 25 A", "electricidad", 0.429, 27.96, "G-11-04", 10},
	"AL-0066": {"AL-0066", "Diferencial 1,5 mm²", "electricidad", 0.888, 6.45, "F-01-03", 50},
	"AL-0067": {"AL-0067", "Diferencial 2,5 mm²", "electricidad", 1.128, 45.18, "G-11-05", 50},
	"AL-0068": {"AL-0068", "Diferencial 4 mm²", "electricidad", 0.744, 61.51, "B-08-01", 5},
	"AL-0069": {"AL-0069", "Diferencial 16 A", "electricidad", 0.674, 3.11, "C-09-02", 25},
	"AL-0070": {"AL-0070", "Diferencial 25 A", "electricidad", 2.047, 43.46, "E-13-02", 50},
	"AL-0071": {"AL-0071", "Magnetotérmico 1,5 mm²", "electricidad", 2.21, 36.78, "F-03-03", 5},
	"AL-0072": {"AL-0072", "Magnetotérmico 2,5 mm²", "electricidad", 1.929, 12.22, "B-09-01", 100},
	"AL-0073": {"AL-0073", "Magnetotérmico 4 mm²", "electricidad", 0.258, 17.12, "D-03-03", 5},
	"AL-0074": {"AL-0074", "Magnetotérmico 16 A", "electricidad", 1.116, 22.1, "G-09-05", 10},
	"AL-0075": {"AL-0075", "Magnetotérmico 25 A", "electricidad", 0.152, 45.58, "B-06-03", 5},
	"AL-0076": {"AL-0076", "Manguera 1,5 mm²", "electricidad", 0.476, 59.7, "E-17-02", 20},
	"AL-0077": {"AL-0077", "Manguera 2,5 mm²", "electricidad", 1.097, 43.21, "E-12-01", 20},
	"AL-0078": {"AL-0078", "Manguera 4 mm²", "electricidad", 0.137, 1.77, "D-17-04", 10},
	"AL-0079": {"AL-0079", "Manguera 16 A", "electricidad", 2.246, 7.34, "G-22-04", 50},
	"AL-0080": {"AL-0080", "Manguera 25 A", "electricidad", 2.011, 25.52, "E-23-02", 10},
	"AL-0081": {"AL-0081", "Codo de cobre 1/2\"", "fontaneria", 0.568, 31.69, "C-13-03", 5},
	"AL-0082": {"AL-0082", "Codo de cobre 3/4\"", "fontaneria", 1.344, 0.94, "E-14-02", 5},
	"AL-0083": {"AL-0083", "Codo de cobre 1\"", "fontaneria", 0.163, 32.03, "E-20-02", 100},
	"AL-0084": {"AL-0084", "Codo de cobre 16 mm", "fontaneria", 0.49, 17.68, "C-09-04", 5},
	"AL-0085": {"AL-0085", "Codo de cobre 20 mm", "fontaneria", 0.443, 36.56, "F-08-01", 20},
	"AL-0086": {"AL-0086", "Manguito 1/2\"", "fontaneria", 0.372, 7.28, "F-13-01", 25},
	"AL-0087": {"AL-0087", "Manguito 3/4\"", "fontaneria", 0.468, 25.07, "D-17-01", 5},
	"AL-0088": {"AL-0088", "Manguito 1\"", "fontaneria", 0.445, 3.77, "G-19-01", 25},
	"AL-0089": {"AL-0089", "Manguito 16 mm", "fontaneria", 0.065, 11.84, "D-03-05", 50},
	"AL-0090": {"AL-0090", "Manguito 20 mm", "fontaneria", 1.37, 6.24, "G-11-06", 25},
	"AL-0091": {"AL-0091", "Llave de paso 1/2\"", "fontaneria", 0.265, 27.63, "C-02-06", 50},
	"AL-0092": {"AL-0092", "Llave de paso 3/4\"", "fontaneria", 1.015, 27.99, "C-17-05", 50},
	"AL-0093": {"AL-0093", "Llave de paso 1\"", "fontaneria", 1.341, 30.66, "D-03-01", 5},
	"AL-0094": {"AL-0094", "Llave de paso 16 mm", "fontaneria", 0.239, 13.96, "B-13-04", 50},
	"AL-0095": {"AL-0095", "Llave de paso 20 mm", "fontaneria", 0.11, 1.11, "D-16-03", 5},
	"AL-0096": {"AL-0096", "Tubo multicapa 1/2\"", "fontaneria", 0.747, 3.04, "B-22-05", 5},
	"AL-0097": {"AL-0097", "Tubo multicapa 3/4\"", "fontaneria", 1.201, 18.22, "B-09-02", 100},
	"AL-0098": {"AL-0098", "Tubo multicapa 1\"", "fontaneria", 1.218, 9.08, "H-16-04", 5},
	"AL-0099": {"AL-0099", "Tubo multicapa 16 mm", "fontaneria", 0.782, 26.11, "A-20-06", 100},
	"AL-0100": {"AL-0100", "Tubo multicapa 20 mm", "fontaneria", 0.341, 22.95, "F-09-06", 100},
	"AL-0101": {"AL-0101", "Sifón 1/2\"", "fontaneria", 1.118, 23.76, "C-01-04", 5},
	"AL-0102": {"AL-0102", "Sifón 3/4\"", "fontaneria", 0.793, 36.97, "B-23-02", 100},
	"AL-0103": {"AL-0103", "Sifón 1\"", "fontaneria", 0.799, 27.05, "E-15-04", 25},
	"AL-0104": {"AL-0104", "Sifón 16 mm", "fontaneria", 1.234, 37.75, "D-10-01", 25},
	"AL-0105": {"AL-0105", "Sifón 20 mm", "fontaneria", 0.057, 17.66, "H-09-04", 10},
	"AL-0106": {"AL-0106", "Latiguillo 1/2\"", "fontaneria", 1.469, 35.39, "B-19-01", 10},
	"AL-0107": {"AL-0107", "Latiguillo 3/4\"", "fontaneria", 1.204, 10.24, "F-05-05", 100},
	"AL-0108": {"AL-0108", "Latiguillo 1\"", "fontaneria", 0.829, 33.75, "F-08-04", 25},
	"AL-0109": {"AL-0109", "Latiguillo 16 mm", "fontaneria", 0.649, 6.38, "H-22-04", 25},
	"AL-0110": {"AL-0110", "Latiguillo 20 mm", "fontaneria", 0.504, 5.69, "F-13-03", 5},
	"AL-0111": {"AL-0111", "Te de latón 1/2\"", "fontaneria", 1.349, 0.47, "F-13-01", 10},
	"AL-0112": {"AL-0112", "Te de latón 3/4\"", "fontaneria", 1.149, 34.3, "E-09-03", 5},
	"AL-0113": {"AL-0113", "Te de latón 1\"", "fontaneria", 0.647, 37.95, "B-12-04", 20},
	"AL-0114": {"AL-0114", "Te de latón 16 mm", "fontaneria", 1.371, 10.95, "A-22-03", 100},
	"AL-0115": {"AL-0115", "Te de latón 20 mm", "fontaneria", 1.499, 9.77, "E-14-05", 20},
	"AL-0116": {"AL-0116", "Válvula antirretorno 1/2\"", "fontaneria", 0.328, 14.44, "G-01-06", 25},
	"AL-0117": {"AL-0117", "Válvula antirretorno 3/4\"", "fontaneria", 1.464, 35.77, "D-24-01", 5},
	"AL-0118": {"AL-0118", "Válvula antirretorno 1\"", "fontaneria", 1.496, 15.85, "C-21-03", 25},
	"AL-0119": {"AL-0119", "Válvula antirretorno 16 mm", "fontaneria", 0.107, 35.25, "C-06-04", 25},
	"AL-0120": {"AL-0120", "Válvula antirretorno 20 mm", "fontaneria", 0.57, 11.6, "E-13-06", 10},
	"AL-0121": {"AL-0121", "Pintura plástica 750 ml", "pintura", 6.387, 50.13, "G-04-02", 100},
	"AL-0122": {"AL-0122", "Pintura plástica 4 l", "pintura", 3.479, 19.45, "H-18-02", 25},
	"AL-0123": {"AL-0123", "Pintura plástica 15 l", "pintura", 19.041, 88.69, "H-14-02", 50},
	"AL-0124": {"AL-0124", "Pintura plástica 30 mm", "pintura", 4.121, 9.16, "F-18-01", 20},
	"AL-0125": {"AL-0125", "Pintura plástica 50 mm", "pintura", 5.098, 23.88, "D-01-06", 25},
	"AL-0126": {"AL-0126", "Esmalte sintético 750 ml", "pintura", 8.101, 66.68, "D-13-03", 20},
	"AL-0127": {"AL-0127", "Esmalte sintético 4 l", "pintura", 15.819, 44.94, "F-05-06", 50},
	"AL-0128": {"AL-0128", "Esmalte sintético 15 l", "pintura", 11.161, 70.59, "D-03-03", 10},
	"AL-0129": {"AL-0129", "Esmalte sintético 30 mm", "pintura", 8.137, 57.9, "G-10-01", 10},
	"AL-0130": {"AL-0130", "Esmalte sintético 50 mm", "pintura", 0.774, 63.5, "H-19-04", 5},
	"AL-0131": {"AL-0131", "Imprimación 750 ml", "pintura", 1.629, 82.87, "H-15-02", 5},
	"AL-0132": {"AL-0132", "Imprimación 4 l", "pintura", 4.777, 14.55, "B-24-06", 100},
	"AL-0133": {"AL-0133", "Imprimación 15 l", "pintura", 17.792, 79.77, "B-18-01", 5},
	"AL-0134": {"AL-0134", "Imprimación 30 mm", "pintura", 16.45, 21.62, "A-21-06", 20},
	"AL-0135": {"AL-0135", "Imprimación 50 mm", "pintura", 20.215, 56.2, "G-23-01", 5},
	"AL-0136": {"AL-0136", "Barniz 750 ml", "pintura", 1.57, 47.25, "D-13-03", 10},
	"AL-0137": {"AL-0137", "Barniz 4 l", "pintura", 16.621, 1.3, "E-15-03", 20},
	"AL-0138": {"AL-0138", "Barniz 15 l", "pintura", 13.572, 78.8, "H-17-02", 50},
	"AL-0139": {"AL-0139", "Barniz 30 mm", "pintura", 5.264, 85.54, "E-02-01", 10},
	"AL-0140": {"AL-0140", "Barniz 50 mm", "pintura", 10.515, 60.42, "G-03-03", 10},
	"AL-0141": {"AL-0141", "Rodillo 750 ml", "pintura", 14.048, 82.43, "D-16-01", 100},
	"AL-0142": {"AL-0142", "Rodillo 4 l", "pintura", 7.165, 38.12, "G-07-01", 20},
	"AL-0143": {"AL-0143", "Rodillo 15 l", "pintura", 15.548, 45.53, "D-16-02", 20},
	"AL-0144": {"AL-0144", "Rodillo 30 mm", "pintura", 16.106, 18.23, "H-08-03", 20},
	"AL-0145": {"AL-0145", "Rodillo 50 mm", "pintura", 2.378, 55.95, "C-08-04", 25},
	"AL-0146": {"AL-0146", "Brocha 750 ml", "pintura", 19.127, 6.15, "C-13-01", 10},
	"AL-0147": {"AL-0147", "Brocha 4 l", "pintura", 0.594, 53.54, "G-02-06", 5},
	"AL-0148": {"AL-0148", "Brocha 15 l", "pintura", 3.948, 40.68, "F-24-01", 5},
	"AL-0149": {"AL-0149", "Brocha 30 mm", "pintura", 19.57, 30.11, "C-21-05", 100},
	"AL-0150": {"AL-0150", "Brocha 50 mm", "pintura", 9.873, 28.58, "G-12-03", 25},
	"AL-0151": {"AL-0151", "Cinta de carrocero 750 ml", "pintura", 3.638, 1.45, "E-03-03", 25},
	"AL-0152": {"AL-0152", "Cinta de carrocero 4 l", "pintura", 20.07, 12.06, "D-13-03", 20},
	"AL-0153": {"AL-0153", "Cinta de carrocero 15 l", "pintura", 17.28, 39.17, "A-23-04", 10},
	"AL-0154": {"AL-0154", "Cinta de carrocero 30 mm", "pintura", 7.89, 81.93, "D-11-03", 100},
	"AL-0155": {"AL-0155", "Cinta de carrocero 50 mm", "pintura", 18.847, 3.86, "G-08-06", 25},
	"AL-0156": {"AL-0156", "Disolvente 750 ml", "pintura", 0.95, 4.26, "B-02-03", 10},
	"AL-0157": {"AL-0157", "Disolvente 4 l", "pintura", 15.718, 80.09, "F-12-03", 20},
	"AL-0158": {"AL-0158", "Disolvente 15 l", "pintura", 20.116, 55.37, "E-24-06", 100},
	"AL-0159": {"AL-0159", "Disolvente 30 mm", "pintura", 6.715, 25.4, "A-24-05", 100},
	"AL-0160": {"AL-0160", "Disolvente 50 mm", "pintura", 19.882, 6.94, "D-04-04", 100},
	"AL-0161": {"AL-0161", "Martillo de carpintero pequeño", "herramienta", 1.726, 44.0, "G-09-04", 25},
	"AL-0162": {"AL-0162", "Martillo de carpintero mediano", "herramienta", 0.308, 24.1, "A-24-03", 100},
	"AL-0163": {"AL-0163", "Martillo de carpintero grande", "herramienta", 1.409, 28.92, "F-11-04", 20},
	"AL-0164": {"AL-0164", "Martillo de carpintero PZ2", "herramienta", 1.428, 28.41, "D-13-02", 10},
	"AL-0165": {"AL-0165", "Martillo de carpintero 8\"", "herramienta", 0.781, 30.76, "H-18-05", 20},
	"AL-0166": {"AL-0166", "Destornillador pequeño", "herramienta", 0.356, 21.06, "B-03-03", 50},
	"AL-0167": {"AL-0167", "Destornillador mediano", "herramienta", 0.225, 6.69, "H-23-04", 10},
	"AL-0168": {"AL-0168", "Destornillador grande", "herramienta", 0.483, 20.63, "D-24-05", 100},
	"AL-0169": {"AL-0169", "Destornillador PZ2", "herramienta", 1.386, 36.42, "E-10-03", 50},
	"AL-0170": {"AL-0170", "Destornillador 8\"", "herramienta", 0.54, 13.55, "E-07-04", 10},
	"AL-0171": {"AL-0171", "Alicate universal pequeño", "herramienta", 0.399, 12.74, "E-19-02", 20},
	"AL-0172": {"AL-0172", "Alicate universal mediano", "herramienta", 0.191, 13.45, "D-17-05", 10},
	"AL-0173": {"AL-0173", "Alicate universal grande", "herramienta", 1.197, 6.87, "H-02-01", 5},
	"AL-0174": {"AL-0174", "Alicate universal PZ2", "herramienta", 0.897, 38.13, "H-12-01", 20},
	"AL-0175": {"AL-0175", "Alicate universal 8\"", "herramienta", 0.481, 4.69, "D-03-03", 50},
	"AL-0176": {"AL-0176", "Llave inglesa pequeño", "herramienta", 1.57, 22.04, "E-22-01", 5},
	"AL-0177": {"AL-0177", "Llave inglesa mediano", "herramienta", 1.176, 33.37, "F-07-01", 20},
	"AL-0178": {"AL-0178", "Llave inglesa grande", "herramienta", 0.665, 4.42, "E-02-05", 100},
	"AL-0179": {"AL-0179", "Llave inglesa PZ2", "herramienta", 1.201, 11.35, "A-11-04", 100},
	"AL-0180": {"AL-0180", "Llave inglesa 8\"", "herramienta", 0.72, 29.51, "B-07-01", 25},
	"AL-0181": {"AL-0181", "Nivel de burbuja pequeño", "herramienta", 1.023, 5.25, "B-13-06", 50},
	"AL-0182": {"AL-0182", "Nivel de burbuja mediano", "herramienta", 0.346, 25.73, "C-13-06", 20},
	"AL-0183": {"AL-0183", "Nivel de burbuja grande", "herramienta", 0.785, 14.82, "E-14-01", 20},
	"AL-0184": {"AL-0184", "Nivel de burbuja PZ2", "herramienta", 1.362, 40.94, "G-14-01", 20},
	"AL-0185": {"AL-0185", "Nivel de burbuja 8\"", "herramienta", 1.189, 19.5, "G-07-01", 25},
	"AL-0186": {"AL-0186", "Flexómetro pequeño", "herramienta", 1.631, 20.93, "B-13-05", 20},
	"AL-0187": {"AL-0187", "Flexómetro mediano", "herramienta", 0.873, 9.57, "A-02-05", 10},
	"AL-0188": {"AL-0188", "Flexómetro grande", "herramienta", 1.182, 42.08, "B-19-05", 20},
	"AL-0189": {"AL-0189", "Flexómetro PZ2", "herramienta", 1.348, 9.97, "F-10-02", 50},
	"AL-0190": {"AL-0190", "Flexómetro 8\"", "herramienta", 0.375, 5.42, "G-16-02", 20},
	"AL-0191": {"AL-0191", "Sierra de costilla pequeño", "herramienta", 0.298, 43.52, "H-11-01", 50},
	"AL-0192": {"AL-0192", "Sierra de costilla mediano", "herramienta", 1.673, 19.37, "C-21-02", 50},
	"AL-0193": {"AL-0193", "Sierra de costilla grande", "herramienta", 0.776, 39.32, "H-06-05", 10},
	"AL-0194": {"AL-0194", "Sierra de costilla PZ2", "herramienta", 0.152, 43.33, "C-13-03", 5},
	"AL-0195": {"AL-0195", "Sierra de costilla 8\"", "herramienta", 0.337, 44.73, "D-02-05", 100},
	"AL-0196": {"AL-0196", "Juego de brocas pequeño", "herramienta", 0.146, 38.96, "B-13-05", 25},
	"AL-0197": {"AL-0197", "Juego de brocas mediano", "herramienta", 1.026, 29.78, "E-21-04", 20},
	"AL-0198": {"AL-0198", "Juego de brocas grande", "herramienta", 1.082, 21.02, "F-15-05", 25},
	"AL-0199": {"AL-0199", "Juego de brocas PZ2", "herramienta", 0.387, 2.65, "H-15-02", 25},
	"AL-0200": {"AL-0200", "Juego de brocas 8\"", "herramienta", 1.393, 36.43, "H-06-04", 25},
	"AL-0201": {"AL-0201", "Manguera de riego 15 m", "jardin", 2.963, 10.82, "G-12-01", 25},
	"AL-0202": {"AL-0202", "Manguera de riego 25 m", "jardin", 13.212, 47.93, "A-21-02", 5},
	"AL-0203": {"AL-0203", "Manguera de riego 50 l", "jardin", 23.991, 23.82, "B-02-05", 25},
	"AL-0204": {"AL-0204", "Manguera de riego 1 kg", "jardin", 17.041, 56.85, "A-03-05", 100},
	"AL-0205": {"AL-0205", "Manguera de riego 5 kg", "jardin", 18.068, 9.49, "C-16-03", 10},
	"AL-0206": {"AL-0206", "Aspersor 15 m", "jardin", 17.902, 52.42, "D-03-03", 50},
	"AL-0207": {"AL-0207", "Aspersor 25 m", "jardin", 19.709, 12.95, "E-15-02", 20},
	"AL-0208": {"AL-0208", "Aspersor 50 l", "jardin", 13.157, 66.38, "D-19-03", 50},
	"AL-0209": {"AL-0209", "Aspersor 1 kg", "jardin", 13.255, 24.2, "A-07-02", 25},
	"AL-0210": {"AL-0210", "Aspersor 5 kg", "jardin", 4.36, 67.54, "F-13-02", 20},
	"AL-0211": {"AL-0211", "Tijera de podar 15 m", "jardin", 3.169, 39.06, "F-15-05", 50},
	"AL-0212": {"AL-0212", "Tijera de podar 25 m", "jardin", 15.165, 63.75, "B-09-05", 100},
	"AL-0213": {"AL-0213", "Tijera de podar 50 l", "jardin", 22.3, 53.6, "F-09-04", 20},
	"AL-0214": {"AL-0214", "Tijera de podar 1 kg", "jardin", 15.096, 27.09, "B-15-02", 10},
	"AL-0215": {"AL-0215", "Tijera de podar 5 kg", "jardin", 16.077, 69.05, "E-17-03", 20},
	"AL-0216": {"AL-0216", "Saco de sustrato 15 m", "jardin", 16.692, 70.88, "F-24-01", 100},
	"AL-0217": {"AL-0217", "Saco de sustrato 25 m", "jardin", 1.072, 12.29, "G-14-05", 20},
	"AL-0218": {"AL-0218", "Saco de sustrato 50 l", "jardin", 23.305, 11.07, "D-20-06", 5},
	"AL-0219": {"AL-0219", "Saco de sustrato 1 kg", "jardin", 0.775, 1.98, "F-10-01", 50},
	"AL-0220": {"AL-0220", "Saco de sustrato 5 kg", "jardin", 9.415, 17.54, "E-19-02", 10},
	"AL-0221": {"AL-0221", "Programador de riego 15 m", "jardin", 9.649, 59.96, "C-05-01", 10},
	"AL-0222": {"AL-0222", "Programador de riego 25 m", "jardin", 18.453, 33.45, "B-21-02", 100},
	"AL-0223": {"AL-0223", "Programador de riego 50 l", "jardin", 20.38, 30.02, "E-01-01", 100},
	"AL-0224": {"AL-0224", "Programador de riego 1 kg", "jardin", 21.379, 64.47, "H-20-05", 100},
	"AL-0225": {"AL-0225", "Programador de riego 5 kg", "jardin", 12.916, 13.39, "A-02-01", 50},
	"AL-0226": {"AL-0226", "Pistola de riego 15 m", "jardin", 0.851, 14.83, "C-02-01", 5},
	"AL-0227": {"AL-0227", "Pistola de riego 25 m", "jardin", 16.006, 47.91, "D-05-04", 10},
	"AL-0228": {"AL-0228", "Pistola de riego 50 l", "jardin", 13.571, 46.92, "G-20-02", 50},
	"AL-0229": {"AL-0229", "Pistola de riego 1 kg", "jardin", 8.182, 22.88, "A-24-04", 100},
	"AL-0230": {"AL-0230", "Pistola de riego 5 kg", "jardin", 14.091, 28.14, "G-24-04", 5},
	"AL-0231": {"AL-0231", "Malla de sombreo 15 m", "jardin", 19.337, 33.56, "D-04-03", 10},
	"AL-0232": {"AL-0232", "Malla de sombreo 25 m", "jardin", 16.816, 10.45, "E-23-01", 20},
	"AL-0233": {"AL-0233", "Malla de sombreo 50 l", "jardin", 16.605, 49.48, "E-10-06", 10},
	"AL-0234": {"AL-0234", "Malla de sombreo 1 kg", "jardin", 2.404, 37.42, "C-09-02", 100},
	"AL-0235": {"AL-0235", "Malla de sombreo 5 kg", "jardin", 5.432, 12.97, "F-07-04", 20},
	"AL-0236": {"AL-0236", "Abono granulado 15 m", "jardin", 15.712, 28.44, "H-16-05", 100},
	"AL-0237": {"AL-0237", "Abono granulado 25 m", "jardin", 0.365, 3.66, "D-19-03", 10},
	"AL-0238": {"AL-0238", "Abono granulado 50 l", "jardin", 10.302, 42.89, "C-05-01", 5},
	"AL-0239": {"AL-0239", "Abono granulado 1 kg", "jardin", 3.087, 45.46, "C-12-02", 100},
	"AL-0240": {"AL-0240", "Abono granulado 5 kg", "jardin", 0.941, 4.72, "A-23-01", 100},
}

// SinStock es el error de reservar o sacar más unidades de las disponibles.
type SinStock struct {
	SKU         string
	Pedidas     int
	Disponibles int
}

func (e *SinStock) Error() string {
	return fmt.Sprintf("%s: se piden %d y hay %d disponibles", e.SKU, e.Pedidas, e.Disponibles)
}

// Movimiento es una entrada (cantidad positiva) o una salida (negativa) de stock.
type Movimiento struct {
	SKU      string
	Cantidad int
	Motivo   string
	Almacen  string
}

// Inventario son las existencias, las reservas y los movimientos del almacén.
type Inventario struct {
	Existencias map[string]int // sku -> unidades en el almacén
	Reservado   map[string]int // sku -> unidades reservadas para pedidos
	Movimientos []Movimiento
}

// NuevoInventario es un inventario vacío.
func NuevoInventario() *Inventario {
	return &Inventario{Existencias: map[string]int{}, Reservado: map[string]int{}}
}

// Entrada registra una compra en el almacén de Madrid.
func (inv *Inventario) Entrada(sku string, cantidad int) error {
	return inv.EntradaEn(sku, cantidad, "compra", "MAD")
}

// EntradaEn registra una entrada con su motivo y almacén.
func (inv *Inventario) EntradaEn(sku string, cantidad int, motivo, almacen string) error {
	if cantidad <= 0 {
		return errors.New("la cantidad de una entrada ha de ser positiva")
	}
	inv.Existencias[sku] += cantidad
	inv.Movimientos = append(inv.Movimientos, Movimiento{sku, cantidad, motivo, almacen})
	return nil
}

// Salida registra una venta desde el almacén de Madrid.
func (inv *Inventario) Salida(sku string, cantidad int) error {
	return inv.SalidaDe(sku, cantidad, "venta", "MAD")
}

// SalidaDe registra una salida con su motivo y almacén.
func (inv *Inventario) SalidaDe(sku string, cantidad int, motivo, almacen string) error {
	if cantidad > inv.Existencias[sku] {
		return &SinStock{sku, cantidad, inv.Existencias[sku]}
	}
	inv.Existencias[sku] -= cantidad
	inv.Movimientos = append(inv.Movimientos, Movimiento{sku, -cantidad, motivo, almacen})
	return nil
}

// Disponible son las unidades que se pueden reservar: las existencias menos lo reservado y el margen de seguridad.
func (inv *Inventario) Disponible(sku string) int {
	return max(inv.Existencias[sku]-inv.Reservado[sku]-MargenSeguridad, 0)
}

// ReservaStock aparta unidades para un pedido. Devuelve SinStock si no hay bastantes disponibles.
func (inv *Inventario) ReservaStock(sku string, cantidad int) error {
	if cantidad <= 0 {
		return errors.New("la cantidad de una reserva ha de ser positiva")
	}
	libres := inv.Disponible(sku)
	if cantidad > libres {
		return &SinStock{sku, cantidad, libres}
	}
	inv.Reservado[sku] += cantidad
	return nil
}

// Libera deshace (parte de) una reserva.
func (inv *Inventario) Libera(sku string, cantidad int) {
	inv.Reservado[sku] = max(inv.Reservado[sku]-cantidad, 0)
}

// BajoMinimo son los SKU del catálogo cuyas existencias están por debajo de su stock mínimo.
func (inv *Inventario) BajoMinimo() []string {
	var r []string
	for _, sku := range skus() {
		if inv.Existencias[sku] < Catalogo[sku].StockMinimo {
			r = append(r, sku)
		}
	}
	return r
}

// skus son los SKU del catálogo, en orden.
func skus() []string {
	r := make([]string, 0, len(Catalogo))
	for sku := range Catalogo {
		r = append(r, sku)
	}
	sort.Strings(r)
	return r
}

func redondea2(x float64) float64 {
	return math.Round(x*100) / 100
}

// ProductosTornilleria son los productos de la familia tornilleria, por SKU.
func ProductosTornilleria() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "tornilleria" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockTornilleria es el valor a precio de tarifa de las existencias de tornilleria.
func ValorStockTornilleria(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosTornilleria() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoTornilleria son las diferencias de un recuento físico de tornilleria: sku -> contado - registrado.
func RecuentoTornilleria(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosTornilleria() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosTornilleria son los pasillos donde hay productos de tornilleria.
func PasillosTornilleria() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosTornilleria() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}

// ProductosElectricidad son los productos de la familia electricidad, por SKU.
func ProductosElectricidad() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "electricidad" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockElectricidad es el valor a precio de tarifa de las existencias de electricidad.
func ValorStockElectricidad(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosElectricidad() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoElectricidad son las diferencias de un recuento físico de electricidad: sku -> contado - registrado.
func RecuentoElectricidad(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosElectricidad() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosElectricidad son los pasillos donde hay productos de electricidad.
func PasillosElectricidad() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosElectricidad() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}

// ProductosFontaneria son los productos de la familia fontaneria, por SKU.
func ProductosFontaneria() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "fontaneria" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockFontaneria es el valor a precio de tarifa de las existencias de fontaneria.
func ValorStockFontaneria(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosFontaneria() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoFontaneria son las diferencias de un recuento físico de fontaneria: sku -> contado - registrado.
func RecuentoFontaneria(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosFontaneria() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosFontaneria son los pasillos donde hay productos de fontaneria.
func PasillosFontaneria() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosFontaneria() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}

// ProductosPintura son los productos de la familia pintura, por SKU.
func ProductosPintura() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "pintura" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockPintura es el valor a precio de tarifa de las existencias de pintura.
func ValorStockPintura(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosPintura() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoPintura son las diferencias de un recuento físico de pintura: sku -> contado - registrado.
func RecuentoPintura(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosPintura() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosPintura son los pasillos donde hay productos de pintura.
func PasillosPintura() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosPintura() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}

// ProductosHerramienta son los productos de la familia herramienta, por SKU.
func ProductosHerramienta() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "herramienta" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockHerramienta es el valor a precio de tarifa de las existencias de herramienta.
func ValorStockHerramienta(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosHerramienta() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoHerramienta son las diferencias de un recuento físico de herramienta: sku -> contado - registrado.
func RecuentoHerramienta(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosHerramienta() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosHerramienta son los pasillos donde hay productos de herramienta.
func PasillosHerramienta() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosHerramienta() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}

// ProductosJardin son los productos de la familia jardin, por SKU.
func ProductosJardin() []Producto {
	var r []Producto
	for _, sku := range skus() {
		if p := Catalogo[sku]; p.Familia == "jardin" {
			r = append(r, p)
		}
	}
	return r
}

// ValorStockJardin es el valor a precio de tarifa de las existencias de jardin.
func ValorStockJardin(inv *Inventario) float64 {
	total := 0.0
	for _, p := range ProductosJardin() {
		total += float64(inv.Existencias[p.SKU]) * p.Precio
	}
	return redondea2(total)
}

// RecuentoJardin son las diferencias de un recuento físico de jardin: sku -> contado - registrado.
func RecuentoJardin(inv *Inventario, contado map[string]int) map[string]int {
	dif := map[string]int{}
	for _, p := range ProductosJardin() {
		if d := contado[p.SKU] - inv.Existencias[p.SKU]; d != 0 {
			dif[p.SKU] = d
		}
	}
	return dif
}

// PasillosJardin son los pasillos donde hay productos de jardin.
func PasillosJardin() []string {
	vistos := map[string]bool{}
	var r []string
	for _, p := range ProductosJardin() {
		if pasillo, _, _ := strings.Cut(p.Ubicacion, "-"); !vistos[pasillo] {
			vistos[pasillo] = true
			r = append(r, pasillo)
		}
	}
	sort.Strings(r)
	return r
}
