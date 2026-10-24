package almacen

// Devoluciones: motivos, plazos por familia, reembolsos e incidencias.

import (
	"fmt"
	"strconv"
)

// Motivos de devolución admitidos.
var Motivos = map[string]string{
	"desistimiento": "El cliente desiste de la compra (14 días naturales)",
	"defectuoso":    "El producto llega roto o no funciona",
	"error_envio":   "Se envió otro producto o en otra cantidad",
	"garantia":      "Avería dentro del periodo de garantía",
}

// PlazosDevolucion son los días para devolver por familia y motivo.
var PlazosDevolucion = map[string]map[string]int{
	"tornilleria":  {"desistimiento": 14, "defectuoso": 15, "error_envio": 30, "garantia": 1095},
	"electricidad": {"desistimiento": 14, "defectuoso": 15, "error_envio": 30, "garantia": 1095},
	"fontaneria":   {"desistimiento": 14, "defectuoso": 15, "error_envio": 30, "garantia": 1095},
	"pintura":      {"desistimiento": 14, "defectuoso": 15, "error_envio": 30, "garantia": 1095},
	"herramienta":  {"desistimiento": 14, "defectuoso": 30, "error_envio": 30, "garantia": 730},
	"jardin":       {"desistimiento": 14, "defectuoso": 30, "error_envio": 30, "garantia": 1095},
}

// Respuestas tipo a las incidencias de devolución (código -> texto para el cliente).
var Respuestas = map[string]string{
	"R001": "Le enviaremos un producto de sustitución. Revisaremos el producto en un plazo de cinco días laborables. Disculpe las molestias ocasionadas.",
	"R002": "Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto. Hemos recibido su solicitud de devolución.",
	"R003": "El plazo para esta devolución ha vencido. Le enviaremos un producto de sustitución. Le rogamos que conserve el embalaje original.",
	"R004": "El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución.",
	"R005": "Le rogamos que conserve el embalaje original. Revisaremos el producto en un plazo de cinco días laborables. Disculpe las molestias ocasionadas.",
	"R006": "Revisaremos el producto en un plazo de cinco días laborables. Hemos recibido su solicitud de devolución. Disculpe las molestias ocasionadas.",
	"R007": "La garantía no cubre el desgaste por uso. Le enviaremos un producto de sustitución. Le rogamos que conserve el embalaje original.",
	"R008": "Revisaremos el producto en un plazo de cinco días laborables. Disculpe las molestias ocasionadas. Necesitamos una fotografía del defecto.",
	"R009": "El reembolso se hará con el mismo medio de pago. Le enviaremos un producto de sustitución. Necesitamos una fotografía del defecto.",
	"R010": "La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete. Revisaremos el producto en un plazo de cinco días laborables.",
	"R011": "Le enviaremos un producto de sustitución. La garantía no cubre el desgaste por uso. Disculpe las molestias ocasionadas.",
	"R012": "La garantía no cubre el desgaste por uso. Necesitamos una fotografía del defecto. Revisaremos el producto en un plazo de cinco días laborables.",
	"R013": "Un transportista pasará a recoger el paquete. El reembolso se hará con el mismo medio de pago. Necesitamos una fotografía del defecto.",
	"R014": "El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso. Le rogamos que conserve el embalaje original.",
	"R015": "Le enviaremos un producto de sustitución. Hemos recibido su solicitud de devolución. Le rogamos que conserve el embalaje original.",
	"R016": "Revisaremos el producto en un plazo de cinco días laborables. El plazo para esta devolución ha vencido. Necesitamos una fotografía del defecto.",
	"R017": "El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete.",
	"R018": "Revisaremos el producto en un plazo de cinco días laborables. La garantía no cubre el desgaste por uso. Le enviaremos un producto de sustitución.",
	"R019": "La garantía no cubre el desgaste por uso. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R020": "Un transportista pasará a recoger el paquete. Revisaremos el producto en un plazo de cinco días laborables. Necesitamos una fotografía del defecto.",
	"R021": "Hemos recibido su solicitud de devolución. Disculpe las molestias ocasionadas. Revisaremos el producto en un plazo de cinco días laborables.",
	"R022": "Le enviaremos un producto de sustitución. Disculpe las molestias ocasionadas. Revisaremos el producto en un plazo de cinco días laborables.",
	"R023": "El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido.",
	"R024": "Disculpe las molestias ocasionadas. La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete.",
	"R025": "Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original. El plazo para esta devolución ha vencido.",
	"R026": "Un transportista pasará a recoger el paquete. La garantía no cubre el desgaste por uso. El plazo para esta devolución ha vencido.",
	"R027": "Le enviaremos un producto de sustitución. La garantía no cubre el desgaste por uso. El plazo para esta devolución ha vencido.",
	"R028": "Necesitamos una fotografía del defecto. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
	"R029": "Le rogamos que conserve el embalaje original. Disculpe las molestias ocasionadas. La garantía no cubre el desgaste por uso.",
	"R030": "Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original. El reembolso se hará con el mismo medio de pago.",
	"R031": "Le enviaremos un producto de sustitución. Un transportista pasará a recoger el paquete. La garantía no cubre el desgaste por uso.",
	"R032": "Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución.",
	"R033": "Disculpe las molestias ocasionadas. El reembolso se hará con el mismo medio de pago. La garantía no cubre el desgaste por uso.",
	"R034": "La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago. Revisaremos el producto en un plazo de cinco días laborables.",
	"R035": "Le rogamos que conserve el embalaje original. Disculpe las molestias ocasionadas. El reembolso se hará con el mismo medio de pago.",
	"R036": "El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete.",
	"R037": "Revisaremos el producto en un plazo de cinco días laborables. Hemos recibido su solicitud de devolución. La garantía no cubre el desgaste por uso.",
	"R038": "Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original. La garantía no cubre el desgaste por uso.",
	"R039": "Disculpe las molestias ocasionadas. Revisaremos el producto en un plazo de cinco días laborables. Un transportista pasará a recoger el paquete.",
	"R040": "Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R041": "Disculpe las molestias ocasionadas. El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido.",
	"R042": "Revisaremos el producto en un plazo de cinco días laborables. Un transportista pasará a recoger el paquete. Le enviaremos un producto de sustitución.",
	"R043": "Disculpe las molestias ocasionadas. Un transportista pasará a recoger el paquete. Le enviaremos un producto de sustitución.",
	"R044": "Hemos recibido su solicitud de devolución. El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete.",
	"R045": "Un transportista pasará a recoger el paquete. Le enviaremos un producto de sustitución. El reembolso se hará con el mismo medio de pago.",
	"R046": "Hemos recibido su solicitud de devolución. Necesitamos una fotografía del defecto. Le rogamos que conserve el embalaje original.",
	"R047": "Necesitamos una fotografía del defecto. Revisaremos el producto en un plazo de cinco días laborables. Hemos recibido su solicitud de devolución.",
	"R048": "Necesitamos una fotografía del defecto. El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución.",
	"R049": "Hemos recibido su solicitud de devolución. El plazo para esta devolución ha vencido. Necesitamos una fotografía del defecto.",
	"R050": "Un transportista pasará a recoger el paquete. Necesitamos una fotografía del defecto. El reembolso se hará con el mismo medio de pago.",
	"R051": "Revisaremos el producto en un plazo de cinco días laborables. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
	"R052": "El plazo para esta devolución ha vencido. El reembolso se hará con el mismo medio de pago. Disculpe las molestias ocasionadas.",
	"R053": "El plazo para esta devolución ha vencido. El reembolso se hará con el mismo medio de pago. Revisaremos el producto en un plazo de cinco días laborables.",
	"R054": "Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución.",
	"R055": "El reembolso se hará con el mismo medio de pago. Le rogamos que conserve el embalaje original. Hemos recibido su solicitud de devolución.",
	"R056": "El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables. La garantía no cubre el desgaste por uso.",
	"R057": "Le enviaremos un producto de sustitución. Un transportista pasará a recoger el paquete. Revisaremos el producto en un plazo de cinco días laborables.",
	"R058": "Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas. La garantía no cubre el desgaste por uso.",
	"R059": "La garantía no cubre el desgaste por uso. El plazo para esta devolución ha vencido. Disculpe las molestias ocasionadas.",
	"R060": "El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución. Le enviaremos un producto de sustitución.",
	"R061": "El plazo para esta devolución ha vencido. Le enviaremos un producto de sustitución. Revisaremos el producto en un plazo de cinco días laborables.",
	"R062": "Un transportista pasará a recoger el paquete. Necesitamos una fotografía del defecto. Le rogamos que conserve el embalaje original.",
	"R063": "La garantía no cubre el desgaste por uso. Necesitamos una fotografía del defecto. El plazo para esta devolución ha vencido.",
	"R064": "El reembolso se hará con el mismo medio de pago. Le enviaremos un producto de sustitución. Disculpe las molestias ocasionadas.",
	"R065": "Un transportista pasará a recoger el paquete. La garantía no cubre el desgaste por uso. Le rogamos que conserve el embalaje original.",
	"R066": "Revisaremos el producto en un plazo de cinco días laborables. El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete.",
	"R067": "El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución. Necesitamos una fotografía del defecto.",
	"R068": "El reembolso se hará con el mismo medio de pago. Disculpe las molestias ocasionadas. Revisaremos el producto en un plazo de cinco días laborables.",
	"R069": "El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R070": "Hemos recibido su solicitud de devolución. Disculpe las molestias ocasionadas. Un transportista pasará a recoger el paquete.",
	"R071": "Le enviaremos un producto de sustitución. El reembolso se hará con el mismo medio de pago. La garantía no cubre el desgaste por uso.",
	"R072": "Hemos recibido su solicitud de devolución. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R073": "El plazo para esta devolución ha vencido. Le enviaremos un producto de sustitución. Le rogamos que conserve el embalaje original.",
	"R074": "Disculpe las molestias ocasionadas. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
	"R075": "Revisaremos el producto en un plazo de cinco días laborables. Un transportista pasará a recoger el paquete. Hemos recibido su solicitud de devolución.",
	"R076": "Le rogamos que conserve el embalaje original. Le enviaremos un producto de sustitución. La garantía no cubre el desgaste por uso.",
	"R077": "Hemos recibido su solicitud de devolución. Necesitamos una fotografía del defecto. Un transportista pasará a recoger el paquete.",
	"R078": "Le enviaremos un producto de sustitución. Un transportista pasará a recoger el paquete. Le rogamos que conserve el embalaje original.",
	"R079": "Le enviaremos un producto de sustitución. Necesitamos una fotografía del defecto. El plazo para esta devolución ha vencido.",
	"R080": "Un transportista pasará a recoger el paquete. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
	"R081": "Necesitamos una fotografía del defecto. Le rogamos que conserve el embalaje original. Un transportista pasará a recoger el paquete.",
	"R082": "El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables. La garantía no cubre el desgaste por uso.",
	"R083": "El reembolso se hará con el mismo medio de pago. Le enviaremos un producto de sustitución. Revisaremos el producto en un plazo de cinco días laborables.",
	"R084": "Hemos recibido su solicitud de devolución. El reembolso se hará con el mismo medio de pago. Revisaremos el producto en un plazo de cinco días laborables.",
	"R085": "El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso. Disculpe las molestias ocasionadas.",
	"R086": "Le rogamos que conserve el embalaje original. La garantía no cubre el desgaste por uso. Disculpe las molestias ocasionadas.",
	"R087": "Le rogamos que conserve el embalaje original. Hemos recibido su solicitud de devolución. Un transportista pasará a recoger el paquete.",
	"R088": "El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso.",
	"R089": "Hemos recibido su solicitud de devolución. Disculpe las molestias ocasionadas. Un transportista pasará a recoger el paquete.",
	"R090": "Necesitamos una fotografía del defecto. Hemos recibido su solicitud de devolución. El reembolso se hará con el mismo medio de pago.",
	"R091": "Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete.",
	"R092": "Le enviaremos un producto de sustitución. Disculpe las molestias ocasionadas. Necesitamos una fotografía del defecto.",
	"R093": "Necesitamos una fotografía del defecto. El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución.",
	"R094": "El reembolso se hará con el mismo medio de pago. Disculpe las molestias ocasionadas. Le enviaremos un producto de sustitución.",
	"R095": "La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido.",
	"R096": "Disculpe las molestias ocasionadas. Le rogamos que conserve el embalaje original. El reembolso se hará con el mismo medio de pago.",
	"R097": "Necesitamos una fotografía del defecto. Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido.",
	"R098": "Un transportista pasará a recoger el paquete. Hemos recibido su solicitud de devolución. Necesitamos una fotografía del defecto.",
	"R099": "Le rogamos que conserve el embalaje original. La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago.",
	"R100": "Necesitamos una fotografía del defecto. Disculpe las molestias ocasionadas. Le rogamos que conserve el embalaje original.",
	"R101": "Un transportista pasará a recoger el paquete. Necesitamos una fotografía del defecto. La garantía no cubre el desgaste por uso.",
	"R102": "Un transportista pasará a recoger el paquete. El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido.",
	"R103": "Hemos recibido su solicitud de devolución. La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago.",
	"R104": "Hemos recibido su solicitud de devolución. El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete.",
	"R105": "El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución. El plazo para esta devolución ha vencido.",
	"R106": "Necesitamos una fotografía del defecto. Hemos recibido su solicitud de devolución. La garantía no cubre el desgaste por uso.",
	"R107": "El reembolso se hará con el mismo medio de pago. Disculpe las molestias ocasionadas. Hemos recibido su solicitud de devolución.",
	"R108": "El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso. Revisaremos el producto en un plazo de cinco días laborables.",
	"R109": "Hemos recibido su solicitud de devolución. Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto.",
	"R110": "Hemos recibido su solicitud de devolución. Necesitamos una fotografía del defecto. Un transportista pasará a recoger el paquete.",
	"R111": "Un transportista pasará a recoger el paquete. Le rogamos que conserve el embalaje original. El plazo para esta devolución ha vencido.",
	"R112": "El plazo para esta devolución ha vencido. Le rogamos que conserve el embalaje original. Le enviaremos un producto de sustitución.",
	"R113": "Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso.",
	"R114": "Hemos recibido su solicitud de devolución. Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas.",
	"R115": "El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas.",
	"R116": "Hemos recibido su solicitud de devolución. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R117": "Necesitamos una fotografía del defecto. La garantía no cubre el desgaste por uso. Hemos recibido su solicitud de devolución.",
	"R118": "El plazo para esta devolución ha vencido. El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución.",
	"R119": "Le rogamos que conserve el embalaje original. El plazo para esta devolución ha vencido. Necesitamos una fotografía del defecto.",
	"R120": "El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas.",
	"R121": "La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido.",
	"R122": "El plazo para esta devolución ha vencido. Disculpe las molestias ocasionadas. Le enviaremos un producto de sustitución.",
	"R123": "Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas. El reembolso se hará con el mismo medio de pago.",
	"R124": "Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas. Le enviaremos un producto de sustitución.",
	"R125": "Revisaremos el producto en un plazo de cinco días laborables. Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido.",
	"R126": "Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto.",
	"R127": "Disculpe las molestias ocasionadas. Le enviaremos un producto de sustitución. El reembolso se hará con el mismo medio de pago.",
	"R128": "Hemos recibido su solicitud de devolución. Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido.",
	"R129": "Hemos recibido su solicitud de devolución. Un transportista pasará a recoger el paquete. El reembolso se hará con el mismo medio de pago.",
	"R130": "El plazo para esta devolución ha vencido. La garantía no cubre el desgaste por uso. Necesitamos una fotografía del defecto.",
	"R131": "La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete.",
	"R132": "Hemos recibido su solicitud de devolución. Disculpe las molestias ocasionadas. El plazo para esta devolución ha vencido.",
	"R133": "Le rogamos que conserve el embalaje original. La garantía no cubre el desgaste por uso. Hemos recibido su solicitud de devolución.",
	"R134": "Le rogamos que conserve el embalaje original. Revisaremos el producto en un plazo de cinco días laborables. Necesitamos una fotografía del defecto.",
	"R135": "Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original. Disculpe las molestias ocasionadas.",
	"R136": "Revisaremos el producto en un plazo de cinco días laborables. Le enviaremos un producto de sustitución. Hemos recibido su solicitud de devolución.",
	"R137": "Le enviaremos un producto de sustitución. La garantía no cubre el desgaste por uso. Un transportista pasará a recoger el paquete.",
	"R138": "Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto. Disculpe las molestias ocasionadas.",
	"R139": "Necesitamos una fotografía del defecto. Le enviaremos un producto de sustitución. Revisaremos el producto en un plazo de cinco días laborables.",
	"R140": "El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución. La garantía no cubre el desgaste por uso.",
	"R141": "El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución. Le enviaremos un producto de sustitución.",
	"R142": "El reembolso se hará con el mismo medio de pago. El plazo para esta devolución ha vencido. Le enviaremos un producto de sustitución.",
	"R143": "Le enviaremos un producto de sustitución. Hemos recibido su solicitud de devolución. El reembolso se hará con el mismo medio de pago.",
	"R144": "Le enviaremos un producto de sustitución. Un transportista pasará a recoger el paquete. Le rogamos que conserve el embalaje original.",
	"R145": "Un transportista pasará a recoger el paquete. Hemos recibido su solicitud de devolución. Le enviaremos un producto de sustitución.",
	"R146": "La garantía no cubre el desgaste por uso. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
	"R147": "Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido. Disculpe las molestias ocasionadas.",
	"R148": "Necesitamos una fotografía del defecto. Le rogamos que conserve el embalaje original. El reembolso se hará con el mismo medio de pago.",
	"R149": "El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución. El reembolso se hará con el mismo medio de pago.",
	"R150": "La garantía no cubre el desgaste por uso. El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete.",
	"R151": "El reembolso se hará con el mismo medio de pago. Disculpe las molestias ocasionadas. Revisaremos el producto en un plazo de cinco días laborables.",
	"R152": "Hemos recibido su solicitud de devolución. Revisaremos el producto en un plazo de cinco días laborables. La garantía no cubre el desgaste por uso.",
	"R153": "Un transportista pasará a recoger el paquete. Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto.",
	"R154": "Disculpe las molestias ocasionadas. Le rogamos que conserve el embalaje original. Revisaremos el producto en un plazo de cinco días laborables.",
	"R155": "La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago. Le enviaremos un producto de sustitución.",
	"R156": "Revisaremos el producto en un plazo de cinco días laborables. Hemos recibido su solicitud de devolución. Un transportista pasará a recoger el paquete.",
	"R157": "El reembolso se hará con el mismo medio de pago. Revisaremos el producto en un plazo de cinco días laborables. Le rogamos que conserve el embalaje original.",
	"R158": "Un transportista pasará a recoger el paquete. Disculpe las molestias ocasionadas. La garantía no cubre el desgaste por uso.",
	"R159": "Revisaremos el producto en un plazo de cinco días laborables. Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido.",
	"R160": "Un transportista pasará a recoger el paquete. El plazo para esta devolución ha vencido. Hemos recibido su solicitud de devolución.",
	"R161": "Un transportista pasará a recoger el paquete. Le enviaremos un producto de sustitución. Disculpe las molestias ocasionadas.",
	"R162": "Le rogamos que conserve el embalaje original. El plazo para esta devolución ha vencido. Un transportista pasará a recoger el paquete.",
	"R163": "Necesitamos una fotografía del defecto. El plazo para esta devolución ha vencido. Revisaremos el producto en un plazo de cinco días laborables.",
	"R164": "Necesitamos una fotografía del defecto. Le rogamos que conserve el embalaje original. Un transportista pasará a recoger el paquete.",
	"R165": "Revisaremos el producto en un plazo de cinco días laborables. Disculpe las molestias ocasionadas. La garantía no cubre el desgaste por uso.",
	"R166": "La garantía no cubre el desgaste por uso. Le rogamos que conserve el embalaje original. Necesitamos una fotografía del defecto.",
	"R167": "Un transportista pasará a recoger el paquete. Necesitamos una fotografía del defecto. Le enviaremos un producto de sustitución.",
	"R168": "Le enviaremos un producto de sustitución. El reembolso se hará con el mismo medio de pago. Hemos recibido su solicitud de devolución.",
	"R169": "La garantía no cubre el desgaste por uso. El reembolso se hará con el mismo medio de pago. Un transportista pasará a recoger el paquete.",
	"R170": "El reembolso se hará con el mismo medio de pago. Le enviaremos un producto de sustitución. El plazo para esta devolución ha vencido.",
}

// PlazoDevolucion son los días para devolver un SKU por un motivo.
func PlazoDevolucion(sku, motivo string) int {
	return PlazosDevolucion[Catalogo[sku].Familia][motivo]
}

// DentroDePlazo dice si una devolución a los «dias» días está dentro de plazo.
func DentroDePlazo(sku, motivo string, dias int) bool {
	return dias <= PlazoDevolucion(sku, motivo)
}

// Reembolso es lo que se devuelve al cliente: los productos y el envío.
func Reembolso(importeProductos, costeEnvio float64, motivo string) (float64, error) {
	if _, ok := Motivos[motivo]; !ok {
		return 0, fmt.Errorf("motivo desconocido: %s", motivo)
	}
	return redondea2(importeProductos + costeEnvio), nil
}

// respuestasResto son las respuestas tipo cuyo número da ese resto entre 4.
func respuestasResto(resto int) map[string]string {
	r := map[string]string{}
	for k, v := range Respuestas {
		if n, err := strconv.Atoi(k[1:]); err == nil && n%4 == resto {
			r[k] = v
		}
	}
	return r
}

// RespuestasDesistimiento son las respuestas tipo que se usan con el motivo desistimiento.
func RespuestasDesistimiento() map[string]string {
	return respuestasResto(0)
}

// RespuestasDefectuoso son las respuestas tipo que se usan con el motivo defectuoso.
func RespuestasDefectuoso() map[string]string {
	return respuestasResto(1)
}

// RespuestasErrorEnvio son las respuestas tipo que se usan con el motivo error_envio.
func RespuestasErrorEnvio() map[string]string {
	return respuestasResto(2)
}

// RespuestasGarantia son las respuestas tipo que se usan con el motivo garantia.
func RespuestasGarantia() map[string]string {
	return respuestasResto(3)
}
