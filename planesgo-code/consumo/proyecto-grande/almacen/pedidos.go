package almacen

// Pedidos de clientes: líneas, estados y transiciones, y los clientes del almacén.
//
// Un pedido nace en BORRADOR, se CONFIRMA, pasa a EN_PREPARACION cuando el almacén empieza a prepararlo, se
// ENVIA y se ENTREGA. Se puede CANCELAR mientras no se haya enviado.

import (
	"fmt"
	"math"
	"slices"
	"sort"
)

// MaxLineasPedido es el máximo de líneas de un pedido.
const MaxLineasPedido = 48

// Estados de un pedido.
var Estados = []string{"BORRADOR", "CONFIRMADO", "EN_PREPARACION", "ENVIADO", "ENTREGADO", "CANCELADO"}

// Transiciones: estado -> estados a los que puede pasar.
var Transiciones = map[string][]string{
	"BORRADOR":       {"CONFIRMADO", "CANCELADO"},
	"CONFIRMADO":     {"EN_PREPARACION", "CANCELADO"},
	"EN_PREPARACION": {"ENVIADO", "CANCELADO"},
	"ENVIADO":        {"ENTREGADO"},
	"ENTREGADO":      {},
	"CANCELADO":      {},
}

// Cliente es un cliente del almacén.
type Cliente struct {
	Codigo      string
	Nombre      string
	CP          string
	Provincia   string
	Condiciones string
	Grupo       string
}

// Clientes: código -> Cliente (nombre, código postal, provincia, condiciones de pago y grupo de tarifa).
var Clientes = map[string]Cliente{
	"C-0001": {"C-0001", "Ferretería Sánchez", "38780", "Santa Cruz de Tenerife", "60 días", "general"},
	"C-0002": {"C-0002", "Suministros Castillo", "07252", "Baleares", "30 días", "general"},
	"C-0003": {"C-0003", "Suministros Martín", "03972", "Alicante", "contado", "mayorista"},
	"C-0004": {"C-0004", "Instalaciones Méndez", "07135", "Baleares", "contado", "mayorista"},
	"C-0005": {"C-0005", "Almacenes Marín", "21344", "Huelva", "transferencia 15 días", "profesional"},
	"C-0006": {"C-0006", "Ferretería Garrido", "17952", "Girona", "60 días", "general"},
	"C-0007": {"C-0007", "Reformas Iglesias", "50987", "Zaragoza", "transferencia 15 días", "profesional"},
	"C-0008": {"C-0008", "Ferretería Lozano", "02446", "Albacete", "contado", "profesional"},
	"C-0009": {"C-0009", "Maderas López", "35579", "Las Palmas", "30 días", "mayorista"},
	"C-0010": {"C-0010", "Suministros Marín", "11446", "Cádiz", "contado", "mayorista"},
	"C-0011": {"C-0011", "Almacenes Marín", "49768", "Zamora", "contado", "general"},
	"C-0012": {"C-0012", "Reformas Cruz", "07503", "Baleares", "30 días", "profesional"},
	"C-0013": {"C-0013", "Reformas Calvo", "17591", "Girona", "30 días", "profesional"},
	"C-0014": {"C-0014", "Almacenes Delgado", "32169", "Ourense", "contado", "mayorista"},
	"C-0015": {"C-0015", "Suministros Cruz", "51713", "Ceuta", "contado", "mayorista"},
	"C-0016": {"C-0016", "Reformas Garrido", "07410", "Baleares", "transferencia 15 días", "mayorista"},
	"C-0017": {"C-0017", "Suministros Guerrero", "42025", "Soria", "60 días", "general"},
	"C-0018": {"C-0018", "Instalaciones Ortiz", "28922", "Madrid", "30 días", "profesional"},
	"C-0019": {"C-0019", "Almacenes Prieto", "09544", "Burgos", "contado", "profesional"},
	"C-0020": {"C-0020", "Reformas Gallego", "10888", "Cáceres", "transferencia 15 días", "mayorista"},
	"C-0021": {"C-0021", "Reformas Gil", "30449", "Murcia", "60 días", "mayorista"},
	"C-0022": {"C-0022", "Almacenes Torres", "22473", "Huesca", "30 días", "mayorista"},
	"C-0023": {"C-0023", "Almacenes Rubio", "20772", "Gipuzkoa", "30 días", "mayorista"},
	"C-0024": {"C-0024", "Bricolaje Castro", "47334", "Valladolid", "60 días", "general"},
	"C-0025": {"C-0025", "Almacenes Iglesias", "13264", "Ciudad Real", "contado", "general"},
	"C-0026": {"C-0026", "Suministros Molina", "25154", "Lleida", "30 días", "profesional"},
	"C-0027": {"C-0027", "Instalaciones Guerrero", "18200", "Granada", "contado", "mayorista"},
	"C-0028": {"C-0028", "Suministros Rubio", "14906", "Córdoba", "transferencia 15 días", "profesional"},
	"C-0029": {"C-0029", "Ferretería Ruiz", "26874", "La Rioja", "transferencia 15 días", "mayorista"},
	"C-0030": {"C-0030", "Almacenes Calvo", "41303", "Sevilla", "transferencia 15 días", "general"},
	"C-0031": {"C-0031", "Bricolaje Ortiz", "39755", "Cantabria", "transferencia 15 días", "general"},
	"C-0032": {"C-0032", "Almacenes Guerrero", "45587", "Toledo", "transferencia 15 días", "general"},
	"C-0033": {"C-0033", "Almacenes Serrano", "42127", "Soria", "transferencia 15 días", "profesional"},
	"C-0034": {"C-0034", "Reformas Ortiz", "41717", "Sevilla", "contado", "profesional"},
	"C-0035": {"C-0035", "Almacenes Santos", "46729", "Valencia", "30 días", "profesional"},
	"C-0036": {"C-0036", "Construcciones Méndez", "30020", "Murcia", "transferencia 15 días", "mayorista"},
	"C-0037": {"C-0037", "Bricolaje Iglesias", "50010", "Zaragoza", "transferencia 15 días", "profesional"},
	"C-0038": {"C-0038", "Suministros Martín", "17556", "Girona", "30 días", "general"},
	"C-0039": {"C-0039", "Almacenes Gallego", "23103", "Jaén", "transferencia 15 días", "mayorista"},
	"C-0040": {"C-0040", "Almacenes Méndez", "33016", "Asturias", "60 días", "mayorista"},
	"C-0041": {"C-0041", "Reformas Lozano", "48970", "Bizkaia", "transferencia 15 días", "general"},
	"C-0042": {"C-0042", "Bricolaje Santos", "33781", "Asturias", "contado", "mayorista"},
	"C-0043": {"C-0043", "Reformas López", "17280", "Girona", "transferencia 15 días", "profesional"},
	"C-0044": {"C-0044", "Ferretería Ruiz", "05428", "Ávila", "transferencia 15 días", "mayorista"},
	"C-0045": {"C-0045", "Reformas Ortiz", "07229", "Baleares", "60 días", "mayorista"},
	"C-0046": {"C-0046", "Construcciones Gallego", "15820", "A Coruña", "transferencia 15 días", "profesional"},
	"C-0047": {"C-0047", "Almacenes Gil", "09951", "Burgos", "contado", "mayorista"},
	"C-0048": {"C-0048", "Almacenes Méndez", "42575", "Soria", "30 días", "general"},
	"C-0049": {"C-0049", "Reformas Lozano", "30301", "Murcia", "30 días", "profesional"},
	"C-0050": {"C-0050", "Reformas Delgado", "18721", "Granada", "transferencia 15 días", "mayorista"},
	"C-0051": {"C-0051", "Instalaciones Guerrero", "44190", "Teruel", "transferencia 15 días", "general"},
	"C-0052": {"C-0052", "Instalaciones Garrido", "16670", "Cuenca", "60 días", "profesional"},
	"C-0053": {"C-0053", "Maderas Cruz", "28638", "Madrid", "contado", "mayorista"},
	"C-0054": {"C-0054", "Reformas Ramos", "20874", "Gipuzkoa", "transferencia 15 días", "general"},
	"C-0055": {"C-0055", "Suministros Iglesias", "51965", "Ceuta", "30 días", "mayorista"},
	"C-0056": {"C-0056", "Reformas Ruiz", "43011", "Tarragona", "30 días", "general"},
	"C-0057": {"C-0057", "Instalaciones Ortiz", "39103", "Cantabria", "30 días", "general"},
	"C-0058": {"C-0058", "Bricolaje Cano", "23803", "Jaén", "30 días", "general"},
	"C-0059": {"C-0059", "Construcciones Gil", "40912", "Segovia", "contado", "mayorista"},
	"C-0060": {"C-0060", "Instalaciones Molina", "32709", "Ourense", "30 días", "mayorista"},
	"C-0061": {"C-0061", "Suministros Cano", "43903", "Tarragona", "contado", "mayorista"},
	"C-0062": {"C-0062", "Suministros Ortiz", "27239", "Lugo", "30 días", "profesional"},
	"C-0063": {"C-0063", "Maderas López", "31478", "Navarra", "30 días", "mayorista"},
	"C-0064": {"C-0064", "Maderas Castro", "32168", "Ourense", "contado", "general"},
	"C-0065": {"C-0065", "Reformas Prieto", "45576", "Toledo", "transferencia 15 días", "mayorista"},
	"C-0066": {"C-0066", "Instalaciones Prieto", "24436", "León", "transferencia 15 días", "mayorista"},
	"C-0067": {"C-0067", "Suministros Serrano", "41369", "Sevilla", "contado", "general"},
	"C-0068": {"C-0068", "Ferretería Medina", "52096", "Melilla", "transferencia 15 días", "profesional"},
	"C-0069": {"C-0069", "Bricolaje Martín", "14735", "Córdoba", "transferencia 15 días", "mayorista"},
	"C-0070": {"C-0070", "Bricolaje Medina", "07882", "Baleares", "60 días", "profesional"},
	"C-0071": {"C-0071", "Maderas Gallego", "36789", "Pontevedra", "30 días", "profesional"},
	"C-0072": {"C-0072", "Construcciones Medina", "28257", "Madrid", "contado", "profesional"},
	"C-0073": {"C-0073", "Instalaciones Garrido", "32413", "Ourense", "60 días", "mayorista"},
	"C-0074": {"C-0074", "Instalaciones Calvo", "23998", "Jaén", "30 días", "mayorista"},
	"C-0075": {"C-0075", "Maderas Navarro", "22196", "Huesca", "60 días", "mayorista"},
	"C-0076": {"C-0076", "Instalaciones Torres", "38996", "Santa Cruz de Tenerife", "contado", "general"},
	"C-0077": {"C-0077", "Construcciones Santos", "35587", "Las Palmas", "contado", "profesional"},
	"C-0078": {"C-0078", "Instalaciones Gómez", "01047", "Álava", "30 días", "profesional"},
	"C-0079": {"C-0079", "Ferretería Calvo", "35626", "Las Palmas", "transferencia 15 días", "mayorista"},
	"C-0080": {"C-0080", "Bricolaje Pérez", "14040", "Córdoba", "transferencia 15 días", "mayorista"},
	"C-0081": {"C-0081", "Bricolaje Gómez", "43185", "Tarragona", "contado", "profesional"},
	"C-0082": {"C-0082", "Suministros Ruiz", "24892", "León", "30 días", "profesional"},
	"C-0083": {"C-0083", "Instalaciones Sanz", "12431", "Castellón", "contado", "profesional"},
	"C-0084": {"C-0084", "Ferretería Guerrero", "37657", "Salamanca", "contado", "profesional"},
	"C-0085": {"C-0085", "Ferretería Navarro", "50829", "Zaragoza", "transferencia 15 días", "mayorista"},
	"C-0086": {"C-0086", "Construcciones Cano", "05014", "Ávila", "transferencia 15 días", "mayorista"},
	"C-0087": {"C-0087", "Bricolaje Méndez", "50422", "Zaragoza", "contado", "general"},
	"C-0088": {"C-0088", "Maderas Ortega", "10641", "Cáceres", "contado", "profesional"},
	"C-0089": {"C-0089", "Ferretería Ruiz", "44685", "Teruel", "contado", "general"},
	"C-0090": {"C-0090", "Almacenes Navarro", "09483", "Burgos", "contado", "profesional"},
	"C-0091": {"C-0091", "Almacenes Cano", "47762", "Valladolid", "30 días", "general"},
	"C-0092": {"C-0092", "Reformas Ramos", "47777", "Valladolid", "contado", "profesional"},
	"C-0093": {"C-0093", "Maderas Prieto", "43954", "Tarragona", "60 días", "general"},
	"C-0094": {"C-0094", "Ferretería Ruiz", "04015", "Almería", "contado", "profesional"},
	"C-0095": {"C-0095", "Instalaciones Sanz", "47614", "Valladolid", "30 días", "profesional"},
	"C-0096": {"C-0096", "Ferretería Iglesias", "24971", "León", "transferencia 15 días", "profesional"},
	"C-0097": {"C-0097", "Bricolaje Ramos", "52119", "Melilla", "60 días", "mayorista"},
	"C-0098": {"C-0098", "Bricolaje Lozano", "31394", "Navarra", "transferencia 15 días", "profesional"},
	"C-0099": {"C-0099", "Reformas Marín", "18062", "Granada", "60 días", "mayorista"},
	"C-0100": {"C-0100", "Ferretería Ramos", "39852", "Cantabria", "60 días", "mayorista"},
	"C-0101": {"C-0101", "Construcciones Castro", "25396", "Lleida", "transferencia 15 días", "mayorista"},
	"C-0102": {"C-0102", "Almacenes Cano", "19705", "Guadalajara", "contado", "profesional"},
	"C-0103": {"C-0103", "Instalaciones Rubio", "28161", "Madrid", "contado", "profesional"},
	"C-0104": {"C-0104", "Bricolaje Ramos", "18998", "Granada", "transferencia 15 días", "profesional"},
	"C-0105": {"C-0105", "Suministros Cruz", "52390", "Melilla", "30 días", "mayorista"},
	"C-0106": {"C-0106", "Almacenes Sanz", "39058", "Cantabria", "transferencia 15 días", "profesional"},
	"C-0107": {"C-0107", "Almacenes Ortiz", "38769", "Santa Cruz de Tenerife", "contado", "profesional"},
	"C-0108": {"C-0108", "Maderas Pérez", "35825", "Las Palmas", "60 días", "general"},
	"C-0109": {"C-0109", "Almacenes Santos", "38533", "Santa Cruz de Tenerife", "60 días", "mayorista"},
	"C-0110": {"C-0110", "Reformas Méndez", "33603", "Asturias", "30 días", "general"},
	"C-0111": {"C-0111", "Almacenes Molina", "06185", "Badajoz", "60 días", "profesional"},
	"C-0112": {"C-0112", "Reformas Santos", "50529", "Zaragoza", "30 días", "general"},
	"C-0113": {"C-0113", "Ferretería Cruz", "24887", "León", "contado", "profesional"},
	"C-0114": {"C-0114", "Maderas Pérez", "10323", "Cáceres", "contado", "profesional"},
	"C-0115": {"C-0115", "Instalaciones Gallego", "39021", "Cantabria", "contado", "general"},
	"C-0116": {"C-0116", "Almacenes Cruz", "38580", "Santa Cruz de Tenerife", "30 días", "profesional"},
	"C-0117": {"C-0117", "Instalaciones Guerrero", "07969", "Baleares", "transferencia 15 días", "mayorista"},
	"C-0118": {"C-0118", "Bricolaje Ortiz", "03346", "Alicante", "30 días", "general"},
	"C-0119": {"C-0119", "Construcciones Pérez", "02052", "Albacete", "contado", "mayorista"},
	"C-0120": {"C-0120", "Reformas Prieto", "32969", "Ourense", "contado", "mayorista"},
	"C-0121": {"C-0121", "Construcciones Navarro", "46982", "Valencia", "contado", "profesional"},
	"C-0122": {"C-0122", "Reformas Delgado", "42091", "Soria", "transferencia 15 días", "general"},
	"C-0123": {"C-0123", "Maderas Gil", "24988", "León", "30 días", "mayorista"},
	"C-0124": {"C-0124", "Almacenes Serrano", "03964", "Alicante", "60 días", "profesional"},
	"C-0125": {"C-0125", "Ferretería García", "04264", "Almería", "transferencia 15 días", "general"},
	"C-0126": {"C-0126", "Suministros Ramos", "21773", "Huelva", "contado", "general"},
	"C-0127": {"C-0127", "Instalaciones Cano", "49668", "Zamora", "contado", "profesional"},
	"C-0128": {"C-0128", "Reformas Cortés", "17399", "Girona", "contado", "profesional"},
	"C-0129": {"C-0129", "Maderas Castillo", "11451", "Cádiz", "30 días", "general"},
	"C-0130": {"C-0130", "Ferretería Prieto", "46934", "Valencia", "30 días", "general"},
	"C-0131": {"C-0131", "Bricolaje Delgado", "05956", "Ávila", "60 días", "mayorista"},
	"C-0132": {"C-0132", "Bricolaje Cano", "07948", "Baleares", "transferencia 15 días", "general"},
	"C-0133": {"C-0133", "Suministros Cano", "22330", "Huesca", "30 días", "profesional"},
	"C-0134": {"C-0134", "Suministros Cortés", "10339", "Cáceres", "30 días", "mayorista"},
	"C-0135": {"C-0135", "Ferretería Serrano", "46462", "Valencia", "30 días", "profesional"},
	"C-0136": {"C-0136", "Bricolaje Rubio", "27421", "Lugo", "30 días", "general"},
	"C-0137": {"C-0137", "Ferretería Rubio", "37859", "Salamanca", "60 días", "profesional"},
	"C-0138": {"C-0138", "Bricolaje Ortiz", "32111", "Ourense", "60 días", "profesional"},
	"C-0139": {"C-0139", "Maderas Navarro", "10525", "Cáceres", "contado", "mayorista"},
	"C-0140": {"C-0140", "Almacenes Méndez", "19122", "Guadalajara", "60 días", "general"},
	"C-0141": {"C-0141", "Reformas Guerrero", "17244", "Girona", "30 días", "general"},
	"C-0142": {"C-0142", "Construcciones Marín", "27917", "Lugo", "30 días", "general"},
	"C-0143": {"C-0143", "Instalaciones Ramos", "41016", "Sevilla", "transferencia 15 días", "mayorista"},
	"C-0144": {"C-0144", "Reformas Calvo", "09453", "Burgos", "contado", "mayorista"},
	"C-0145": {"C-0145", "Instalaciones Serrano", "24445", "León", "contado", "profesional"},
	"C-0146": {"C-0146", "Almacenes Rubio", "37185", "Salamanca", "30 días", "general"},
	"C-0147": {"C-0147", "Almacenes Serrano", "13615", "Ciudad Real", "contado", "general"},
	"C-0148": {"C-0148", "Maderas Rubio", "12210", "Castellón", "30 días", "mayorista"},
	"C-0149": {"C-0149", "Almacenes Sanz", "13010", "Ciudad Real", "contado", "mayorista"},
	"C-0150": {"C-0150", "Construcciones López", "34830", "Palencia", "60 días", "profesional"},
}

// TransicionInvalida es el error de un cambio que el pedido no admite en su estado.
type TransicionInvalida struct {
	Motivo string
}

func (e *TransicionInvalida) Error() string {
	return e.Motivo
}

// LineaPedido es una línea de un pedido.
type LineaPedido struct {
	SKU            string
	Cantidad       int
	PrecioUnitario float64
}

// Importe es la cantidad por el precio, redondeado a céntimos.
func (l LineaPedido) Importe() float64 {
	return redondea2(float64(l.Cantidad) * l.PrecioUnitario)
}

// Peso es el peso de la línea en kg.
func (l LineaPedido) Peso() float64 {
	return float64(l.Cantidad) * Catalogo[l.SKU].PesoKg
}

// Pedido es un pedido de un cliente.
type Pedido struct {
	Numero    string
	Cliente   string
	Lineas    []LineaPedido
	Estado    string
	Urgente   bool
	Historial [][2]string // {de, a}
}

// NuevoPedido es un pedido en BORRADOR.
func NuevoPedido(numero, cliente string) *Pedido {
	return &Pedido{Numero: numero, Cliente: cliente, Estado: "BORRADOR"}
}

// Anade añade una línea al borrador; con precio 0, el de tarifa del catálogo.
func (p *Pedido) Anade(sku string, cantidad int, precioUnitario float64) error {
	if p.Estado != "BORRADOR" {
		return &TransicionInvalida{"solo se pueden añadir líneas a un borrador"}
	}
	if len(p.Lineas) >= MaxLineasPedido {
		return fmt.Errorf("un pedido no puede tener más de %d líneas", MaxLineasPedido)
	}
	if precioUnitario == 0 {
		prod, ok := Catalogo[sku]
		if !ok {
			return fmt.Errorf("SKU desconocido: %s", sku)
		}
		precioUnitario = prod.Precio
	}
	p.Lineas = append(p.Lineas, LineaPedido{sku, cantidad, precioUnitario})
	return nil
}

// Importe es la suma de las líneas.
func (p *Pedido) Importe() float64 {
	total := 0.0
	for _, l := range p.Lineas {
		total += l.Importe()
	}
	return redondea2(total)
}

// Peso es el peso del pedido en kg.
func (p *Pedido) Peso() float64 {
	total := 0.0
	for _, l := range p.Lineas {
		total += l.Peso()
	}
	return math.Round(total*1000) / 1000
}

// CambiaEstado pasa el pedido a otro estado, si la transición es válida.
func (p *Pedido) CambiaEstado(nuevo string, inv *Inventario) error {
	if !slices.Contains(Transiciones[p.Estado], nuevo) {
		return &TransicionInvalida{p.Estado + " -> " + nuevo}
	}
	p.Historial = append(p.Historial, [2]string{p.Estado, nuevo})
	p.Estado = nuevo
	return nil
}

// Habitual es una línea de un pedido habitual.
type Habitual struct {
	SKU      string
	Cantidad int
}

// Habituales son los pedidos habituales de algunos clientes: código de cliente -> líneas.
var Habituales = map[string][]Habitual{
	"C-0001": {{"AL-0216", 50}, {"AL-0024", 1}, {"AL-0105", 50}},
	"C-0002": {{"AL-0224", 12}, {"AL-0064", 5}},
	"C-0003": {{"AL-0213", 20}, {"AL-0010", 5}, {"AL-0180", 20}, {"AL-0148", 1}},
	"C-0004": {{"AL-0134", 50}, {"AL-0133", 2}, {"AL-0031", 20}},
	"C-0005": {{"AL-0063", 20}, {"AL-0200", 25}, {"AL-0148", 1}, {"AL-0075", 2}},
	"C-0006": {{"AL-0127", 50}, {"AL-0132", 1}, {"AL-0136", 5}, {"AL-0006", 10}},
	"C-0007": {{"AL-0058", 5}, {"AL-0043", 2}},
	"C-0008": {{"AL-0065", 1}, {"AL-0005", 2}, {"AL-0238", 10}},
	"C-0009": {{"AL-0005", 50}, {"AL-0134", 10}, {"AL-0180", 50}},
	"C-0010": {{"AL-0090", 2}, {"AL-0184", 5}},
	"C-0011": {{"AL-0070", 2}, {"AL-0120", 50}},
	"C-0012": {{"AL-0129", 12}, {"AL-0029", 2}, {"AL-0032", 25}, {"AL-0227", 5}},
	"C-0013": {{"AL-0152", 10}, {"AL-0221", 10}, {"AL-0038", 50}, {"AL-0192", 25}},
	"C-0014": {{"AL-0212", 1}, {"AL-0163", 25}},
	"C-0015": {{"AL-0108", 1}, {"AL-0102", 1}, {"AL-0199", 20}, {"AL-0087", 25}},
	"C-0016": {{"AL-0215", 20}, {"AL-0184", 25}},
	"C-0017": {{"AL-0206", 20}, {"AL-0209", 25}, {"AL-0217", 1}, {"AL-0084", 5}},
	"C-0018": {{"AL-0240", 20}, {"AL-0064", 25}, {"AL-0170", 1}, {"AL-0094", 2}},
	"C-0019": {{"AL-0048", 2}, {"AL-0084", 25}, {"AL-0052", 1}, {"AL-0058", 5}},
	"C-0020": {{"AL-0102", 50}, {"AL-0163", 1}, {"AL-0208", 1}},
	"C-0021": {{"AL-0222", 12}, {"AL-0235", 12}},
	"C-0022": {{"AL-0139", 1}, {"AL-0160", 2}, {"AL-0065", 2}, {"AL-0134", 1}},
	"C-0023": {{"AL-0061", 1}, {"AL-0074", 2}, {"AL-0079", 20}},
	"C-0024": {{"AL-0043", 2}, {"AL-0016", 12}, {"AL-0022", 50}, {"AL-0152", 5}},
	"C-0025": {{"AL-0032", 5}, {"AL-0227", 12}, {"AL-0235", 25}},
	"C-0026": {{"AL-0074", 12}, {"AL-0063", 2}, {"AL-0190", 12}, {"AL-0215", 50}},
	"C-0027": {{"AL-0178", 10}, {"AL-0167", 25}, {"AL-0052", 20}, {"AL-0118", 12}},
	"C-0028": {{"AL-0123", 50}, {"AL-0210", 12}, {"AL-0008", 10}, {"AL-0086", 10}},
	"C-0029": {{"AL-0132", 25}, {"AL-0150", 25}},
	"C-0030": {{"AL-0237", 20}, {"AL-0042", 10}},
	"C-0031": {{"AL-0143", 20}, {"AL-0126", 12}, {"AL-0073", 10}},
	"C-0032": {{"AL-0015", 1}, {"AL-0041", 2}, {"AL-0156", 20}},
	"C-0033": {{"AL-0169", 1}, {"AL-0133", 25}, {"AL-0214", 50}},
	"C-0034": {{"AL-0189", 2}, {"AL-0134", 10}, {"AL-0174", 5}},
	"C-0035": {{"AL-0087", 20}, {"AL-0036", 10}, {"AL-0158", 12}},
	"C-0036": {{"AL-0025", 50}, {"AL-0069", 5}, {"AL-0106", 2}, {"AL-0002", 25}},
	"C-0037": {{"AL-0150", 2}, {"AL-0128", 25}, {"AL-0147", 5}, {"AL-0107", 12}},
	"C-0038": {{"AL-0156", 2}, {"AL-0098", 50}, {"AL-0178", 50}, {"AL-0074", 20}},
	"C-0039": {{"AL-0091", 25}, {"AL-0135", 25}, {"AL-0166", 20}},
	"C-0040": {{"AL-0202", 50}, {"AL-0098", 50}},
	"C-0041": {{"AL-0048", 12}, {"AL-0206", 5}, {"AL-0112", 25}},
	"C-0042": {{"AL-0060", 2}, {"AL-0211", 20}, {"AL-0083", 10}, {"AL-0084", 10}},
	"C-0043": {{"AL-0229", 1}, {"AL-0007", 1}, {"AL-0066", 50}},
	"C-0044": {{"AL-0236", 12}, {"AL-0138", 25}, {"AL-0133", 25}},
	"C-0045": {{"AL-0119", 20}, {"AL-0011", 20}, {"AL-0116", 1}},
	"C-0046": {{"AL-0018", 10}, {"AL-0026", 25}, {"AL-0096", 25}, {"AL-0167", 5}},
	"C-0047": {{"AL-0108", 50}, {"AL-0103", 50}},
	"C-0048": {{"AL-0231", 20}, {"AL-0178", 2}, {"AL-0044", 20}, {"AL-0082", 20}},
	"C-0049": {{"AL-0212", 12}, {"AL-0132", 5}},
	"C-0050": {{"AL-0168", 12}, {"AL-0177", 20}},
	"C-0051": {{"AL-0228", 25}, {"AL-0162", 5}, {"AL-0135", 12}, {"AL-0209", 10}},
	"C-0052": {{"AL-0229", 10}, {"AL-0106", 5}, {"AL-0016", 2}, {"AL-0091", 1}},
	"C-0053": {{"AL-0106", 1}, {"AL-0202", 1}, {"AL-0079", 1}, {"AL-0235", 12}},
	"C-0054": {{"AL-0216", 2}, {"AL-0151", 1}, {"AL-0172", 1}},
	"C-0055": {{"AL-0045", 50}, {"AL-0197", 12}},
	"C-0056": {{"AL-0230", 5}, {"AL-0148", 10}, {"AL-0106", 2}, {"AL-0038", 5}},
	"C-0057": {{"AL-0195", 2}, {"AL-0008", 2}, {"AL-0020", 5}, {"AL-0134", 50}},
	"C-0058": {{"AL-0157", 25}, {"AL-0207", 1}, {"AL-0167", 1}},
	"C-0059": {{"AL-0198", 20}, {"AL-0037", 10}, {"AL-0091", 12}, {"AL-0044", 1}},
	"C-0060": {{"AL-0161", 2}, {"AL-0220", 2}, {"AL-0090", 10}},
	"C-0061": {{"AL-0160", 25}, {"AL-0006", 1}, {"AL-0057", 25}},
	"C-0062": {{"AL-0196", 1}, {"AL-0113", 1}, {"AL-0159", 10}, {"AL-0064", 10}},
	"C-0063": {{"AL-0041", 5}, {"AL-0081", 1}},
	"C-0064": {{"AL-0078", 25}, {"AL-0155", 12}, {"AL-0228", 50}},
	"C-0065": {{"AL-0063", 25}, {"AL-0173", 10}},
	"C-0066": {{"AL-0080", 25}, {"AL-0225", 50}, {"AL-0006", 10}},
	"C-0067": {{"AL-0045", 5}, {"AL-0092", 25}},
	"C-0068": {{"AL-0002", 12}, {"AL-0102", 20}},
	"C-0069": {{"AL-0086", 25}, {"AL-0086", 25}},
	"C-0070": {{"AL-0017", 2}, {"AL-0109", 20}, {"AL-0142", 10}, {"AL-0100", 10}},
	"C-0071": {{"AL-0073", 20}, {"AL-0061", 25}, {"AL-0009", 12}},
	"C-0072": {{"AL-0007", 20}, {"AL-0207", 5}, {"AL-0062", 5}, {"AL-0024", 10}},
	"C-0073": {{"AL-0140", 5}, {"AL-0143", 50}, {"AL-0120", 10}},
	"C-0074": {{"AL-0095", 20}, {"AL-0056", 25}},
	"C-0075": {{"AL-0162", 10}, {"AL-0077", 50}, {"AL-0130", 10}},
	"C-0076": {{"AL-0220", 50}, {"AL-0173", 5}},
	"C-0077": {{"AL-0067", 50}, {"AL-0151", 20}, {"AL-0137", 10}, {"AL-0104", 10}},
	"C-0078": {{"AL-0224", 2}, {"AL-0174", 2}},
	"C-0079": {{"AL-0219", 12}, {"AL-0189", 25}, {"AL-0008", 5}, {"AL-0080", 1}},
	"C-0080": {{"AL-0182", 2}, {"AL-0178", 5}, {"AL-0199", 10}},
	"C-0081": {{"AL-0049", 2}, {"AL-0018", 20}, {"AL-0207", 12}},
	"C-0082": {{"AL-0017", 12}, {"AL-0023", 10}},
	"C-0083": {{"AL-0033", 25}, {"AL-0073", 20}, {"AL-0104", 50}},
	"C-0084": {{"AL-0226", 5}, {"AL-0240", 12}, {"AL-0046", 1}, {"AL-0094", 20}},
	"C-0085": {{"AL-0007", 50}, {"AL-0064", 25}, {"AL-0091", 2}},
	"C-0086": {{"AL-0075", 2}, {"AL-0070", 10}},
	"C-0087": {{"AL-0174", 1}, {"AL-0104", 1}, {"AL-0156", 5}, {"AL-0111", 10}},
	"C-0088": {{"AL-0040", 25}, {"AL-0190", 1}, {"AL-0142", 12}},
	"C-0089": {{"AL-0164", 5}, {"AL-0145", 10}, {"AL-0146", 50}, {"AL-0184", 12}},
	"C-0090": {{"AL-0172", 20}, {"AL-0240", 1}, {"AL-0029", 12}},
	"C-0091": {{"AL-0225", 1}, {"AL-0063", 2}},
	"C-0092": {{"AL-0203", 20}, {"AL-0054", 20}},
	"C-0093": {{"AL-0234", 2}, {"AL-0107", 25}, {"AL-0192", 10}, {"AL-0072", 2}},
	"C-0094": {{"AL-0109", 50}, {"AL-0239", 20}, {"AL-0178", 50}},
	"C-0095": {{"AL-0014", 10}, {"AL-0110", 5}, {"AL-0126", 10}, {"AL-0012", 12}},
	"C-0096": {{"AL-0140", 5}, {"AL-0200", 10}},
	"C-0097": {{"AL-0067", 10}, {"AL-0016", 5}, {"AL-0092", 20}, {"AL-0106", 2}},
	"C-0098": {{"AL-0163", 12}, {"AL-0036", 5}},
	"C-0099": {{"AL-0181", 50}, {"AL-0172", 50}, {"AL-0061", 10}, {"AL-0002", 50}},
	"C-0100": {{"AL-0240", 20}, {"AL-0179", 12}},
}

// codigosClientes son los códigos de los clientes, en orden.
func codigosClientes() []string {
	r := make([]string, 0, len(Clientes))
	for c := range Clientes {
		r = append(r, c)
	}
	sort.Strings(r)
	return r
}

// ClientesGeneral son los clientes del grupo de tarifa general.
func ClientesGeneral() []Cliente {
	var r []Cliente
	for _, c := range codigosClientes() {
		if Clientes[c].Grupo == "general" {
			r = append(r, Clientes[c])
		}
	}
	return r
}

// PedidoHabitualGeneral es un borrador con el pedido habitual de un cliente general (nil si no tiene).
func PedidoHabitualGeneral(codigo, numero string) *Pedido {
	lineas, ok := Habituales[codigo]
	if !ok || Clientes[codigo].Grupo != "general" {
		return nil
	}
	p := NuevoPedido(numero, codigo)
	for _, l := range lineas {
		p.Anade(l.SKU, l.Cantidad, 0)
	}
	return p
}

// ClientesProfesional son los clientes del grupo de tarifa profesional.
func ClientesProfesional() []Cliente {
	var r []Cliente
	for _, c := range codigosClientes() {
		if Clientes[c].Grupo == "profesional" {
			r = append(r, Clientes[c])
		}
	}
	return r
}

// PedidoHabitualProfesional es un borrador con el pedido habitual de un cliente profesional (nil si no tiene).
func PedidoHabitualProfesional(codigo, numero string) *Pedido {
	lineas, ok := Habituales[codigo]
	if !ok || Clientes[codigo].Grupo != "profesional" {
		return nil
	}
	p := NuevoPedido(numero, codigo)
	for _, l := range lineas {
		p.Anade(l.SKU, l.Cantidad, 0)
	}
	return p
}

// ClientesMayorista son los clientes del grupo de tarifa mayorista.
func ClientesMayorista() []Cliente {
	var r []Cliente
	for _, c := range codigosClientes() {
		if Clientes[c].Grupo == "mayorista" {
			r = append(r, Clientes[c])
		}
	}
	return r
}

// PedidoHabitualMayorista es un borrador con el pedido habitual de un cliente mayorista (nil si no tiene).
func PedidoHabitualMayorista(codigo, numero string) *Pedido {
	lineas, ok := Habituales[codigo]
	if !ok || Clientes[codigo].Grupo != "mayorista" {
		return nil
	}
	p := NuevoPedido(numero, codigo)
	for _, l := range lineas {
		p.Anade(l.SKU, l.Cantidad, 0)
	}
	return p
}
