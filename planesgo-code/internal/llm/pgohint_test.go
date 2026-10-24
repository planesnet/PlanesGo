package llm

import (
	"strings"
	"testing"
)

func TestPgoHint(t *testing.T) {
	for _, c := range []struct {
		text string
		want bool
	}{
		{"arranca el servidor de pruebas con pgo ejecutar web npm run dev", true},
		{"ejecuta con pgo el make lento y dime qué sale", true},
		{"mira la salida de pgo del programa web", true},
		{"pgo parar web", true},
		{"/pgo imputar on", false},
		{"arranca el servidor con npm run dev", false},
		{"vincula el proyecto pgo con Odoo", false},
		{"el binario pgocode ejecuta los programas", false},
		{"busca en Drive el informe de ventas", true},
		{"lee https://docs.google.com/document/d/1AbCdEfGhIjK/edit y resúmelo", true},
		{"mira el documento de Google con los requisitos", true},
		{"compara con la hoja de cálculo de Google de gastos", true},
		{"pásalo a Google Sheets", true},
		{"actualiza los docs del proyecto", false},
		{"cambia la hoja de estilos (stylesheet)", false},
		{"/drive buscar x", false},
		{"mira en mi correo si Ana mandó la factura", true},
		{"busca los emails de la semana pasada de Odoo", true},
		{"¿qué hay nuevo en mi bandeja?", true},
		{"resume el hilo de Gmail con el cliente", true},
		{"lee el e-mail que me ha llegado del banco", true},
		{"añade el campo email al formulario de contacto", false},
		{"valida el correo electrónico del cliente", false},
		{"usa partner.email en la plantilla", false},
		{"el email: debe ser obligatorio", false},
		{"/gmail buscar x", false},
	} {
		if got := pgoHintFor(c.text) != ""; got != c.want {
			t.Errorf("%q: %v", c.text, got)
		}
	}
	if h := pgoHintFor("arranca con pgo ejecutar web npm run dev y lee el documento de Google"); !strings.Contains(h, "pgo ejecutar") || !strings.Contains(h, "pgo drive leer") {
		t.Errorf("los dos avisos: %q", h)
	}
	if h := pgoHintFor("mira mi correo y el documento de Google"); !strings.Contains(h, "pgo gmail leer") || !strings.Contains(h, "pgo drive leer") {
		t.Errorf("drive y gmail: %q", h)
	}
	if h := pgoHintFor("busca en Drive el informe"); strings.Contains(h, "gmail") {
		t.Errorf("solo drive: %q", h)
	}
	if !hasPgo([]string{"PGO_API=x", "PGO_TOKEN=y"}) || hasPgo([]string{"PGO_API=x"}) {
		t.Error("hasPgo")
	}
}

func TestSQLHint(t *testing.T) {
	for _, m := range []string{"saca los clientes de Sage", "haz una consulta SQL de las facturas", "¿qué tablas tiene la base de datos?", "mira en SQL Server"} {
		if !strings.Contains(pgoHintFor(m), "pgo sql") {
			t.Errorf("%q: sin la pista de pgo sql", m)
		}
	}
	for _, m := range []string{"arregla el test de fechas", "sagaz idea"} {
		if strings.Contains(pgoHintFor(m), "pgo sql") {
			t.Errorf("%q: pista de pgo sql de más", m)
		}
	}
}

func TestDesktopHint(t *testing.T) {
	for _, m := range []string{"entra por el escritorio remoto y abre Sage", "conéctate por RDP al servidor", "haz una captura de la pantalla de Windows", "en Windows, haz clic en Aceptar"} {
		if !strings.Contains(pgoHintFor(m), "pgo escritorio") {
			t.Errorf("%q: sin la pista del escritorio", m)
		}
	}
	for _, m := range []string{"compila para Windows", "arregla la captura de pantalla de la web", "/pgo escritorio"} {
		if strings.Contains(pgoHintFor(m), "pgo escritorio") {
			t.Errorf("%q: no debería llevar la pista del escritorio", m)
		}
	}
}
