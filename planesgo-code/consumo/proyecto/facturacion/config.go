package facturacion

import (
	"encoding/json"
	"os"
)

// RutaConfig es la configuración del proyecto, relativa a la raíz del módulo.
const RutaConfig = "config.json"

// Config es el contenido de config.json.
type Config struct {
	Nombre   string `json:"nombre"`
	Servidor struct {
		Host   string `json:"host"`
		Puerto int    `json:"puerto"`
	} `json:"servidor"`
	Moneda          string `json:"moneda"`
	Decimales       *int   `json:"decimales"`
	SeriePorDefecto string `json:"serie_por_defecto"`
}

// Carga lee la configuración de una ruta.
func Carga(ruta string) (Config, error) {
	var c Config
	b, err := os.ReadFile(ruta)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// Decimales son los decimales de los importes según la configuración (2 si no lo dice).
func Decimales() int {
	c, err := Carga(RutaConfig)
	if err != nil || c.Decimales == nil {
		return 2
	}
	return *c.Decimales
}
