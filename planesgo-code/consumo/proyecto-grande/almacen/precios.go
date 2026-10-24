package almacen

// Precios: tarifas por grupo de cliente y familia, precios especiales y descuento por volumen.

import "math"

// Tramo es un tramo del descuento por volumen.
type Tramo struct {
	Minimo float64 // importe mínimo
	Pct    int     // % de descuento
}

// TramosVolumen es el descuento por volumen del importe de un pedido, de mayor a menor.
// Un pedido de 500 € justos ya tiene el 5 %.
var TramosVolumen = []Tramo{{1000, 10}, {500, 5}, {200, 2}}

// Coeficientes sobre el precio de tarifa por grupo de cliente y familia.
var Coeficientes = map[string]map[string]float64{
	"general": {
		"tornilleria":  0.99,
		"electricidad": 0.99,
		"fontaneria":   0.99,
		"pintura":      0.98,
		"herramienta":  1.0,
		"jardin":       0.97,
	},
	"profesional": {
		"tornilleria":  0.91,
		"electricidad": 0.91,
		"fontaneria":   0.89,
		"pintura":      0.89,
		"herramienta":  0.91,
		"jardin":       0.92,
	},
	"mayorista": {
		"tornilleria":  0.83,
		"electricidad": 0.85,
		"fontaneria":   0.83,
		"pintura":      0.82,
		"herramienta":  0.84,
		"jardin":       0.85,
	},
}

// ClaveEspecial es un cliente y un SKU con precio pactado.
type ClaveEspecial struct {
	Cliente string
	SKU     string
}

// Especiales son los precios especiales pactados: (cliente, sku) -> precio unitario sin IVA.
var Especiales = map[ClaveEspecial]float64{
	{"C-0016", "AL-0230"}: 21.67, // Pistola de riego 5 kg
	{"C-0051", "AL-0029"}: 0.14,  // Taco de nailon M8
	{"C-0115", "AL-0029"}: 0.12,  // Taco de nailon M8
	{"C-0114", "AL-0120"}: 9.77,  // Válvula antirretorno 20 mm
	{"C-0075", "AL-0044"}: 10.01, // Cable flexible 16 A
	{"C-0012", "AL-0003"}: 0.39,  // Tornillo hexagonal M6
	{"C-0125", "AL-0022"}: 0.41,  // Varilla roscada M5
	{"C-0085", "AL-0190"}: 4.56,  // Flexómetro 8"
	{"C-0028", "AL-0166"}: 17.32, // Destornillador pequeño
	{"C-0112", "AL-0126"}: 49.84, // Esmalte sintético 750 ml
	{"C-0140", "AL-0083"}: 22.49, // Codo de cobre 1"
	{"C-0024", "AL-0165"}: 23.73, // Martillo de carpintero 8"
	{"C-0065", "AL-0168"}: 15.71, // Destornillador grande
	{"C-0036", "AL-0192"}: 13.69, // Sierra de costilla mediano
	{"C-0102", "AL-0215"}: 50.84, // Tijera de podar 5 kg
	{"C-0095", "AL-0048"}: 1.41,  // Regleta 4 mm²
	{"C-0135", "AL-0217"}: 11.35, // Saco de sustrato 25 m
	{"C-0044", "AL-0027"}: 1.68,  // Taco de nailon M5
	{"C-0080", "AL-0191"}: 37.17, // Sierra de costilla pequeño
	{"C-0098", "AL-0048"}: 1.29,  // Regleta 4 mm²
	{"C-0092", "AL-0082"}: 0.71,  // Codo de cobre 3/4"
	{"C-0035", "AL-0142"}: 35.45, // Rodillo 4 l
	{"C-0065", "AL-0062"}: 30.75, // Caja de empalme 2,5 mm²
	{"C-0028", "AL-0146"}: 5.54,  // Brocha 750 ml
	{"C-0104", "AL-0232"}: 7.45,  // Malla de sombreo 25 m
	{"C-0056", "AL-0127"}: 36.21, // Esmalte sintético 4 l
	{"C-0041", "AL-0077"}: 36.76, // Manguera 2,5 mm²
	{"C-0021", "AL-0037"}: 1.63,  // Remache ciego M5
	{"C-0042", "AL-0036"}: 0.36,  // Remache ciego M4
	{"C-0103", "AL-0023"}: 0.53,  // Varilla roscada M6
	{"C-0113", "AL-0123"}: 66.31, // Pintura plástica 15 l
	{"C-0096", "AL-0001"}: 0.22,  // Tornillo hexagonal M4
	{"C-0131", "AL-0109"}: 4.69,  // Latiguillo 16 mm
	{"C-0019", "AL-0170"}: 9.67,  // Destornillador 8"
	{"C-0108", "AL-0228"}: 36.82, // Pistola de riego 50 l
	{"C-0113", "AL-0003"}: 0.42,  // Tornillo hexagonal M6
	{"C-0046", "AL-0232"}: 9.21,  // Malla de sombreo 25 m
	{"C-0097", "AL-0076"}: 41.85, // Manguera 1,5 mm²
	{"C-0145", "AL-0173"}: 5.41,  // Alicate universal grande
	{"C-0051", "AL-0121"}: 36.16, // Pintura plástica 750 ml
	{"C-0083", "AL-0133"}: 65.02, // Imprimación 15 l
	{"C-0137", "AL-0233"}: 42.38, // Malla de sombreo 50 l
	{"C-0040", "AL-0103"}: 25.44, // Sifón 1"
	{"C-0021", "AL-0208"}: 59.9,  // Aspersor 50 l
	{"C-0085", "AL-0156"}: 3.68,  // Disolvente 750 ml
	{"C-0145", "AL-0147"}: 43.11, // Brocha 4 l
	{"C-0095", "AL-0124"}: 7.92,  // Pintura plástica 30 mm
	{"C-0036", "AL-0077"}: 39.59, // Manguera 2,5 mm²
	{"C-0136", "AL-0227"}: 41.13, // Pistola de riego 25 m
	{"C-0049", "AL-0057"}: 49.81, // Base de enchufe 2,5 mm²
	{"C-0115", "AL-0177"}: 24.07, // Llave inglesa mediano
	{"C-0149", "AL-0096"}: 2.55,  // Tubo multicapa 1/2"
	{"C-0107", "AL-0093"}: 25.52, // Llave de paso 1"
	{"C-0145", "AL-0113"}: 30.33, // Te de latón 1"
	{"C-0030", "AL-0059"}: 38.59, // Base de enchufe 16 A
	{"C-0052", "AL-0141"}: 73.15, // Rodillo 750 ml
	{"C-0057", "AL-0221"}: 54.52, // Programador de riego 15 m
	{"C-0025", "AL-0049"}: 46.52, // Regleta 16 A
	{"C-0065", "AL-0182"}: 21.16, // Nivel de burbuja mediano
	{"C-0142", "AL-0118"}: 11.99, // Válvula antirretorno 1"
	{"C-0147", "AL-0179"}: 8.27,  // Llave inglesa PZ2
	{"C-0132", "AL-0233"}: 41.92, // Malla de sombreo 50 l
	{"C-0021", "AL-0218"}: 8.88,  // Saco de sustrato 50 l
	{"C-0019", "AL-0205"}: 7.69,  // Manguera de riego 5 kg
	{"C-0129", "AL-0141"}: 68.15, // Rodillo 750 ml
	{"C-0030", "AL-0161"}: 41.7,  // Martillo de carpintero pequeño
	{"C-0132", "AL-0027"}: 1.52,  // Taco de nailon M5
	{"C-0101", "AL-0140"}: 44.88, // Barniz 50 mm
	{"C-0050", "AL-0145"}: 45.81, // Rodillo 50 mm
	{"C-0024", "AL-0036"}: 0.36,  // Remache ciego M4
	{"C-0015", "AL-0104"}: 28.66, // Sifón 16 mm
	{"C-0096", "AL-0011"}: 1.31,  // Tuerca autoblocante M4
	{"C-0055", "AL-0118"}: 12.28, // Válvula antirretorno 1"
	{"C-0035", "AL-0110"}: 5.28,  // Latiguillo 20 mm
	{"C-0023", "AL-0160"}: 6.57,  // Disolvente 50 mm
	{"C-0052", "AL-0145"}: 40.77, // Rodillo 50 mm
	{"C-0091", "AL-0044"}: 9.44,  // Cable flexible 16 A
	{"C-0088", "AL-0206"}: 46.7,  // Aspersor 15 m
	{"C-0003", "AL-0212"}: 48.7,  // Tijera de podar 25 m
	{"C-0062", "AL-0096"}: 2.52,  // Tubo multicapa 1/2"
	{"C-0135", "AL-0092"}: 24.64, // Llave de paso 3/4"
	{"C-0012", "AL-0210"}: 57.47, // Aspersor 5 kg
	{"C-0026", "AL-0092"}: 23.43, // Llave de paso 3/4"
	{"C-0029", "AL-0009"}: 0.66,  // Tornillo alomado M8
	{"C-0063", "AL-0066"}: 5.09,  // Diferencial 1,5 mm²
	{"C-0115", "AL-0006"}: 1.0,   // Tornillo alomado M4
	{"C-0149", "AL-0113"}: 27.64, // Te de latón 1"
	{"C-0006", "AL-0125"}: 17.38, // Pintura plástica 50 mm
	{"C-0067", "AL-0048"}: 1.11,  // Regleta 4 mm²
	{"C-0075", "AL-0224"}: 56.2,  // Programador de riego 1 kg
	{"C-0098", "AL-0215"}: 50.82, // Tijera de podar 5 kg
	{"C-0065", "AL-0138"}: 74.81, // Barniz 15 l
	{"C-0069", "AL-0114"}: 7.7,   // Te de latón 16 mm
	{"C-0088", "AL-0039"}: 1.24,  // Remache ciego M8
	{"C-0124", "AL-0224"}: 45.64, // Programador de riego 1 kg
	{"C-0010", "AL-0020"}: 0.41,  // Arandela plana M10
	{"C-0101", "AL-0216"}: 58.05, // Saco de sustrato 15 m
	{"C-0041", "AL-0178"}: 4.03,  // Llave inglesa grande
	{"C-0101", "AL-0059"}: 47.56, // Base de enchufe 16 A
	{"C-0133", "AL-0020"}: 0.43,  // Arandela plana M10
	{"C-0136", "AL-0056"}: 14.13, // Base de enchufe 1,5 mm²
	{"C-0034", "AL-0151"}: 1.24,  // Cinta de carrocero 750 ml
	{"C-0055", "AL-0044"}: 10.79, // Cable flexible 16 A
	{"C-0120", "AL-0085"}: 30.87, // Codo de cobre 20 mm
	{"C-0100", "AL-0240"}: 3.72,  // Abono granulado 5 kg
	{"C-0002", "AL-0086"}: 6.15,  // Manguito 1/2"
	{"C-0086", "AL-0059"}: 36.52, // Base de enchufe 16 A
	{"C-0118", "AL-0225"}: 12.67, // Programador de riego 5 kg
	{"C-0012", "AL-0162"}: 17.75, // Martillo de carpintero mediano
	{"C-0037", "AL-0070"}: 34.6,  // Diferencial 25 A
	{"C-0017", "AL-0129"}: 54.88, // Esmalte sintético 30 mm
	{"C-0092", "AL-0146"}: 5.19,  // Brocha 750 ml
	{"C-0150", "AL-0036"}: 0.43,  // Remache ciego M4
	{"C-0009", "AL-0235"}: 10.9,  // Malla de sombreo 5 kg
	{"C-0025", "AL-0224"}: 48.34, // Programador de riego 1 kg
	{"C-0110", "AL-0163"}: 24.38, // Martillo de carpintero grande
	{"C-0026", "AL-0093"}: 27.53, // Llave de paso 1"
	{"C-0061", "AL-0224"}: 57.97, // Programador de riego 1 kg
	{"C-0037", "AL-0175"}: 3.37,  // Alicate universal 8"
	{"C-0088", "AL-0190"}: 4.29,  // Flexómetro 8"
	{"C-0063", "AL-0090"}: 5.73,  // Manguito 20 mm
	{"C-0104", "AL-0086"}: 5.21,  // Manguito 1/2"
	{"C-0087", "AL-0172"}: 10.5,  // Alicate universal mediano
	{"C-0124", "AL-0129"}: 45.85, // Esmalte sintético 30 mm
	{"C-0063", "AL-0208"}: 50.36, // Aspersor 50 l
	{"C-0090", "AL-0039"}: 1.11,  // Remache ciego M8
	{"C-0002", "AL-0228"}: 43.06, // Pistola de riego 50 l
	{"C-0117", "AL-0104"}: 30.63, // Sifón 16 mm
	{"C-0146", "AL-0198"}: 16.3,  // Juego de brocas grande
	{"C-0044", "AL-0151"}: 1.04,  // Cinta de carrocero 750 ml
	{"C-0078", "AL-0185"}: 15.15, // Nivel de burbuja 8"
	{"C-0147", "AL-0142"}: 32.96, // Rodillo 4 l
	{"C-0088", "AL-0019"}: 1.62,  // Arandela plana M8
	{"C-0150", "AL-0237"}: 2.64,  // Abono granulado 25 m
	{"C-0046", "AL-0078"}: 1.5,   // Manguera 4 mm²
	{"C-0120", "AL-0092"}: 26.38, // Llave de paso 3/4"
	{"C-0110", "AL-0185"}: 17.88, // Nivel de burbuja 8"
	{"C-0018", "AL-0215"}: 56.7,  // Tijera de podar 5 kg
	{"C-0045", "AL-0071"}: 34.0,  // Magnetotérmico 1,5 mm²
	{"C-0140", "AL-0006"}: 0.98,  // Tornillo alomado M4
	{"C-0069", "AL-0061"}: 25.08, // Caja de empalme 1,5 mm²
	{"C-0056", "AL-0013"}: 0.86,  // Tuerca autoblocante M6
	{"C-0052", "AL-0229"}: 19.46, // Pistola de riego 1 kg
	{"C-0129", "AL-0166"}: 15.27, // Destornillador pequeño
	{"C-0062", "AL-0188"}: 30.05, // Flexómetro grande
	{"C-0034", "AL-0154"}: 58.35, // Cinta de carrocero 30 mm
	{"C-0019", "AL-0208"}: 60.01, // Aspersor 50 l
	{"C-0148", "AL-0088"}: 3.32,  // Manguito 1"
	{"C-0002", "AL-0049"}: 42.89, // Regleta 16 A
	{"C-0004", "AL-0164"}: 22.18, // Martillo de carpintero PZ2
	{"C-0008", "AL-0055"}: 26.73, // Interruptor 25 A
	{"C-0007", "AL-0167"}: 5.5,   // Destornillador mediano
	{"C-0087", "AL-0045"}: 34.1,  // Cable flexible 25 A
	{"C-0107", "AL-0204"}: 40.44, // Manguera de riego 1 kg
	{"C-0086", "AL-0199"}: 2.18,  // Juego de brocas PZ2
	{"C-0103", "AL-0066"}: 6.03,  // Diferencial 1,5 mm²
	{"C-0004", "AL-0007"}: 0.68,  // Tornillo alomado M5
	{"C-0145", "AL-0168"}: 19.53, // Destornillador grande
	{"C-0015", "AL-0107"}: 8.74,  // Latiguillo 3/4"
	{"C-0085", "AL-0041"}: 21.99, // Cable flexible 1,5 mm²
	{"C-0040", "AL-0054"}: 22.53, // Interruptor 16 A
	{"C-0024", "AL-0092"}: 25.29, // Llave de paso 3/4"
	{"C-0109", "AL-0089"}: 9.88,  // Manguito 16 mm
	{"C-0143", "AL-0040"}: 0.58,  // Remache ciego M10
	{"C-0148", "AL-0085"}: 27.69, // Codo de cobre 20 mm
	{"C-0067", "AL-0209"}: 21.24, // Aspersor 1 kg
	{"C-0009", "AL-0199"}: 2.28,  // Juego de brocas PZ2
	{"C-0141", "AL-0181"}: 4.27,  // Nivel de burbuja pequeño
	{"C-0072", "AL-0093"}: 25.47, // Llave de paso 1"
	{"C-0071", "AL-0034"}: 0.8,   // Tirafondo M8
	{"C-0143", "AL-0122"}: 14.1,  // Pintura plástica 4 l
	{"C-0093", "AL-0039"}: 1.43,  // Remache ciego M8
	{"C-0059", "AL-0103"}: 24.05, // Sifón 1"
	{"C-0024", "AL-0240"}: 3.34,  // Abono granulado 5 kg
	{"C-0035", "AL-0032"}: 0.16,  // Tirafondo M5
	{"C-0129", "AL-0053"}: 46.92, // Interruptor 4 mm²
	{"C-0047", "AL-0067"}: 42.24, // Diferencial 2,5 mm²
	{"C-0094", "AL-0189"}: 7.35,  // Flexómetro PZ2
	{"C-0046", "AL-0223"}: 26.55, // Programador de riego 50 l
	{"C-0042", "AL-0136"}: 33.42, // Barniz 750 ml
	{"C-0063", "AL-0114"}: 10.35, // Te de latón 16 mm
	{"C-0128", "AL-0055"}: 29.42, // Interruptor 25 A
	{"C-0089", "AL-0231"}: 30.21, // Malla de sombreo 15 m
	{"C-0118", "AL-0055"}: 26.75, // Interruptor 25 A
	{"C-0007", "AL-0028"}: 0.19,  // Taco de nailon M6
	{"C-0004", "AL-0017"}: 0.61,  // Arandela plana M5
	{"C-0103", "AL-0173"}: 6.29,  // Alicate universal grande
	{"C-0016", "AL-0059"}: 43.56, // Base de enchufe 16 A
	{"C-0105", "AL-0233"}: 46.01, // Malla de sombreo 50 l
	{"C-0058", "AL-0008"}: 0.99,  // Tornillo alomado M6
	{"C-0068", "AL-0182"}: 20.8,  // Nivel de burbuja mediano
	{"C-0060", "AL-0091"}: 20.74, // Llave de paso 1/2"
	{"C-0109", "AL-0165"}: 23.68, // Martillo de carpintero 8"
	{"C-0128", "AL-0056"}: 17.17, // Base de enchufe 1,5 mm²
	{"C-0041", "AL-0123"}: 81.22, // Pintura plástica 15 l
	{"C-0069", "AL-0193"}: 28.87, // Sierra de costilla grande
	{"C-0077", "AL-0073"}: 12.36, // Magnetotérmico 4 mm²
	{"C-0002", "AL-0125"}: 21.92, // Pintura plástica 50 mm
	{"C-0064", "AL-0042"}: 30.64, // Cable flexible 2,5 mm²
	{"C-0116", "AL-0055"}: 28.93, // Interruptor 25 A
	{"C-0054", "AL-0218"}: 10.19, // Saco de sustrato 50 l
	{"C-0093", "AL-0012"}: 0.28,  // Tuerca autoblocante M5
	{"C-0113", "AL-0047"}: 50.75, // Regleta 2,5 mm²
	{"C-0036", "AL-0240"}: 3.66,  // Abono granulado 5 kg
	{"C-0007", "AL-0207"}: 9.43,  // Aspersor 25 m
	{"C-0003", "AL-0035"}: 1.23,  // Tirafondo M10
	{"C-0039", "AL-0129"}: 51.18, // Esmalte sintético 30 mm
	{"C-0025", "AL-0193"}: 29.18, // Sierra de costilla grande
	{"C-0102", "AL-0024"}: 0.51,  // Varilla roscada M8
	{"C-0102", "AL-0226"}: 11.63, // Pistola de riego 15 m
	{"C-0009", "AL-0150"}: 21.68, // Brocha 50 mm
	{"C-0004", "AL-0010"}: 0.43,  // Tornillo alomado M10
	{"C-0060", "AL-0148"}: 32.85, // Brocha 15 l
	{"C-0027", "AL-0187"}: 6.75,  // Flexómetro mediano
	{"C-0082", "AL-0017"}: 0.63,  // Arandela plana M5
	{"C-0031", "AL-0125"}: 22.51, // Pintura plástica 50 mm
	{"C-0135", "AL-0110"}: 3.99,  // Latiguillo 20 mm
	{"C-0058", "AL-0176"}: 18.41, // Llave inglesa pequeño
	{"C-0140", "AL-0129"}: 54.91, // Esmalte sintético 30 mm
	{"C-0136", "AL-0091"}: 25.14, // Llave de paso 1/2"
	{"C-0020", "AL-0090"}: 5.88,  // Manguito 20 mm
	{"C-0058", "AL-0188"}: 30.22, // Flexómetro grande
	{"C-0046", "AL-0004"}: 0.93,  // Tornillo hexagonal M8
	{"C-0018", "AL-0012"}: 0.23,  // Tuerca autoblocante M5
	{"C-0013", "AL-0105"}: 15.85, // Sifón 20 mm
	{"C-0093", "AL-0069"}: 2.19,  // Diferencial 16 A
	{"C-0011", "AL-0168"}: 16.78, // Destornillador grande
	{"C-0073", "AL-0141"}: 64.52, // Rodillo 750 ml
	{"C-0106", "AL-0224"}: 57.14, // Programador de riego 1 kg
	{"C-0069", "AL-0103"}: 21.79, // Sifón 1"
	{"C-0139", "AL-0108"}: 26.86, // Latiguillo 1"
	{"C-0039", "AL-0100"}: 20.43, // Tubo multicapa 20 mm
	{"C-0105", "AL-0206"}: 38.57, // Aspersor 15 m
	{"C-0002", "AL-0062"}: 36.67, // Caja de empalme 2,5 mm²
	{"C-0066", "AL-0178"}: 3.77,  // Llave inglesa grande
	{"C-0097", "AL-0062"}: 39.01, // Caja de empalme 2,5 mm²
	{"C-0030", "AL-0023"}: 0.51,  // Varilla roscada M6
	{"C-0009", "AL-0233"}: 43.5,  // Malla de sombreo 50 l
	{"C-0104", "AL-0178"}: 3.71,  // Llave inglesa grande
	{"C-0114", "AL-0141"}: 71.47, // Rodillo 750 ml
	{"C-0117", "AL-0148"}: 28.49, // Brocha 15 l
	{"C-0121", "AL-0131"}: 65.1,  // Imprimación 750 ml
	{"C-0140", "AL-0098"}: 6.89,  // Tubo multicapa 1"
	{"C-0097", "AL-0091"}: 24.26, // Llave de paso 1/2"
	{"C-0101", "AL-0135"}: 43.08, // Imprimación 50 mm
	{"C-0083", "AL-0019"}: 1.49,  // Arandela plana M8
	{"C-0140", "AL-0171"}: 9.63,  // Alicate universal pequeño
	{"C-0068", "AL-0068"}: 57.03, // Diferencial 4 mm²
	{"C-0122", "AL-0220"}: 15.44, // Saco de sustrato 5 kg
	{"C-0134", "AL-0151"}: 1.19,  // Cinta de carrocero 750 ml
	{"C-0057", "AL-0037"}: 1.34,  // Remache ciego M5
	{"C-0136", "AL-0094"}: 11.6,  // Llave de paso 16 mm
	{"C-0136", "AL-0044"}: 10.77, // Cable flexible 16 A
	{"C-0062", "AL-0173"}: 5.11,  // Alicate universal grande
	{"C-0118", "AL-0046"}: 2.02,  // Regleta 1,5 mm²
	{"C-0012", "AL-0083"}: 25.47, // Codo de cobre 1"
	{"C-0110", "AL-0032"}: 0.18,  // Tirafondo M5
	{"C-0065", "AL-0097"}: 13.22, // Tubo multicapa 3/4"
	{"C-0092", "AL-0170"}: 12.21, // Destornillador 8"
	{"C-0134", "AL-0078"}: 1.44,  // Manguera 4 mm²
	{"C-0023", "AL-0071"}: 29.38, // Magnetotérmico 1,5 mm²
	{"C-0115", "AL-0178"}: 3.22,  // Llave inglesa grande
	{"C-0123", "AL-0188"}: 37.85, // Flexómetro grande
	{"C-0133", "AL-0039"}: 1.06,  // Remache ciego M8
	{"C-0034", "AL-0094"}: 11.48, // Llave de paso 16 mm
	{"C-0061", "AL-0160"}: 5.5,   // Disolvente 50 mm
	{"C-0098", "AL-0065"}: 19.7,  // Caja de empalme 25 A
	{"C-0052", "AL-0001"}: 0.26,  // Tornillo hexagonal M4
	{"C-0015", "AL-0152"}: 8.98,  // Cinta de carrocero 4 l
	{"C-0140", "AL-0071"}: 34.18, // Magnetotérmico 1,5 mm²
	{"C-0066", "AL-0062"}: 32.98, // Caja de empalme 2,5 mm²
	{"C-0113", "AL-0024"}: 0.52,  // Varilla roscada M8
	{"C-0127", "AL-0220"}: 12.67, // Saco de sustrato 5 kg
	{"C-0033", "AL-0109"}: 5.99,  // Latiguillo 16 mm
	{"C-0075", "AL-0159"}: 22.74, // Disolvente 30 mm
	{"C-0012", "AL-0184"}: 33.19, // Nivel de burbuja PZ2
	{"C-0094", "AL-0011"}: 1.63,  // Tuerca autoblocante M4
	{"C-0076", "AL-0105"}: 14.26, // Sifón 20 mm
	{"C-0066", "AL-0091"}: 20.99, // Llave de paso 1/2"
	{"C-0149", "AL-0034"}: 0.98,  // Tirafondo M8
	{"C-0050", "AL-0219"}: 1.74,  // Saco de sustrato 1 kg
	{"C-0096", "AL-0017"}: 0.59,  // Arandela plana M5
	{"C-0085", "AL-0221"}: 43.03, // Programador de riego 15 m
	{"C-0115", "AL-0098"}: 7.25,  // Tubo multicapa 1"
	{"C-0107", "AL-0128"}: 65.93, // Esmalte sintético 15 l
	{"C-0145", "AL-0119"}: 32.92, // Válvula antirretorno 16 mm
	{"C-0112", "AL-0107"}: 9.72,  // Latiguillo 3/4"
	{"C-0046", "AL-0228"}: 33.61, // Pistola de riego 50 l
	{"C-0102", "AL-0126"}: 48.93, // Esmalte sintético 750 ml
	{"C-0003", "AL-0172"}: 10.2,  // Alicate universal mediano
	{"C-0052", "AL-0103"}: 22.6,  // Sifón 1"
	{"C-0076", "AL-0142"}: 29.83, // Rodillo 4 l
	{"C-0100", "AL-0198"}: 17.13, // Juego de brocas grande
	{"C-0024", "AL-0057"}: 52.24, // Base de enchufe 2,5 mm²
	{"C-0147", "AL-0210"}: 47.54, // Aspersor 5 kg
	{"C-0128", "AL-0023"}: 0.51,  // Varilla roscada M6
	{"C-0056", "AL-0145"}: 45.52, // Rodillo 50 mm
	{"C-0052", "AL-0183"}: 11.62, // Nivel de burbuja grande
	{"C-0015", "AL-0141"}: 71.94, // Rodillo 750 ml
	{"C-0107", "AL-0216"}: 59.96, // Saco de sustrato 15 m
	{"C-0105", "AL-0210"}: 48.12, // Aspersor 5 kg
	{"C-0038", "AL-0083"}: 25.1,  // Codo de cobre 1"
	{"C-0133", "AL-0002"}: 0.1,   // Tornillo hexagonal M5
	{"C-0138", "AL-0071"}: 30.53, // Magnetotérmico 1,5 mm²
	{"C-0023", "AL-0081"}: 25.22, // Codo de cobre 1/2"
	{"C-0077", "AL-0143"}: 36.36, // Rodillo 15 l
	{"C-0108", "AL-0175"}: 3.34,  // Alicate universal 8"
	{"C-0078", "AL-0064"}: 23.7,  // Caja de empalme 16 A
	{"C-0112", "AL-0220"}: 14.64, // Saco de sustrato 5 kg
	{"C-0079", "AL-0052"}: 24.45, // Interruptor 2,5 mm²
	{"C-0054", "AL-0138"}: 68.01, // Barniz 15 l
	{"C-0119", "AL-0169"}: 29.95, // Destornillador PZ2
	{"C-0150", "AL-0037"}: 1.48,  // Remache ciego M5
	{"C-0088", "AL-0052"}: 27.16, // Interruptor 2,5 mm²
	{"C-0143", "AL-0170"}: 9.66,  // Destornillador 8"
	{"C-0081", "AL-0003"}: 0.4,   // Tornillo hexagonal M6
	{"C-0105", "AL-0145"}: 50.68, // Rodillo 50 mm
	{"C-0010", "AL-0071"}: 27.77, // Magnetotérmico 1,5 mm²
	{"C-0113", "AL-0075"}: 34.19, // Magnetotérmico 25 A
	{"C-0054", "AL-0206"}: 49.45, // Aspersor 15 m
	{"C-0114", "AL-0053"}: 51.44, // Interruptor 4 mm²
	{"C-0015", "AL-0047"}: 50.73, // Regleta 2,5 mm²
	{"C-0032", "AL-0013"}: 0.79,  // Tuerca autoblocante M6
	{"C-0019", "AL-0209"}: 20.55, // Aspersor 1 kg
	{"C-0047", "AL-0004"}: 1.13,  // Tornillo hexagonal M8
	{"C-0144", "AL-0189"}: 8.97,  // Flexómetro PZ2
	{"C-0128", "AL-0057"}: 49.75, // Base de enchufe 2,5 mm²
	{"C-0076", "AL-0206"}: 39.46, // Aspersor 15 m
	{"C-0041", "AL-0038"}: 0.36,  // Remache ciego M6
	{"C-0053", "AL-0133"}: 57.85, // Imprimación 15 l
}

func redondea(x float64) float64 {
	return math.Round((x+1e-9)*100) / 100
}

// PrecioCliente es el precio unitario de un SKU para un cliente: el especial si lo hay; si no, la tarifa por el
// coeficiente de su grupo.
func PrecioCliente(sku, cliente, grupo string) float64 {
	if v, ok := Especiales[ClaveEspecial{cliente, sku}]; ok {
		return v
	}
	p := Catalogo[sku]
	return redondea(p.Precio * Coeficientes[grupo][p.Familia])
}

// DescuentoVolumen es el % de descuento por volumen que corresponde a un importe.
func DescuentoVolumen(importe float64) int {
	for _, t := range TramosVolumen {
		if importe > t.Minimo {
			return t.Pct
		}
	}
	return 0
}

// AplicaDescuento es el importe con el descuento por volumen aplicado.
func AplicaDescuento(importe float64) float64 {
	return redondea(importe * (1 - float64(DescuentoVolumen(importe))/100))
}

// TarifaTornilleria es la tarifa de tornilleria para un grupo: sku -> precio.
func TarifaTornilleria(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "tornilleria" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["tornilleria"])
		}
	}
	return r
}

// EspecialesTornilleria son los precios especiales de tornilleria de un cliente.
func EspecialesTornilleria(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "tornilleria" {
			r[k.SKU] = v
		}
	}
	return r
}

// TarifaElectricidad es la tarifa de electricidad para un grupo: sku -> precio.
func TarifaElectricidad(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "electricidad" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["electricidad"])
		}
	}
	return r
}

// EspecialesElectricidad son los precios especiales de electricidad de un cliente.
func EspecialesElectricidad(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "electricidad" {
			r[k.SKU] = v
		}
	}
	return r
}

// TarifaFontaneria es la tarifa de fontaneria para un grupo: sku -> precio.
func TarifaFontaneria(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "fontaneria" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["fontaneria"])
		}
	}
	return r
}

// EspecialesFontaneria son los precios especiales de fontaneria de un cliente.
func EspecialesFontaneria(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "fontaneria" {
			r[k.SKU] = v
		}
	}
	return r
}

// TarifaPintura es la tarifa de pintura para un grupo: sku -> precio.
func TarifaPintura(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "pintura" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["pintura"])
		}
	}
	return r
}

// EspecialesPintura son los precios especiales de pintura de un cliente.
func EspecialesPintura(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "pintura" {
			r[k.SKU] = v
		}
	}
	return r
}

// TarifaHerramienta es la tarifa de herramienta para un grupo: sku -> precio.
func TarifaHerramienta(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "herramienta" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["herramienta"])
		}
	}
	return r
}

// EspecialesHerramienta son los precios especiales de herramienta de un cliente.
func EspecialesHerramienta(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "herramienta" {
			r[k.SKU] = v
		}
	}
	return r
}

// TarifaJardin es la tarifa de jardin para un grupo: sku -> precio.
func TarifaJardin(grupo string) map[string]float64 {
	r := map[string]float64{}
	for sku, p := range Catalogo {
		if p.Familia == "jardin" {
			r[sku] = redondea(p.Precio * Coeficientes[grupo]["jardin"])
		}
	}
	return r
}

// EspecialesJardin son los precios especiales de jardin de un cliente.
func EspecialesJardin(cliente string) map[string]float64 {
	r := map[string]float64{}
	for k, v := range Especiales {
		if k.Cliente == cliente && Catalogo[k.SKU].Familia == "jardin" {
			r[k.SKU] = v
		}
	}
	return r
}
