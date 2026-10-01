package main

import (
	"log"
	"os"
	"time"

	// Base de zonas horarias embebida: no depende de que el sistema tenga tzdata
	_ "time/tzdata"
)

// defaultTimezone es la zona en la que trabajan los usuarios. Las fechas de los partes
// ("date" en Odoo es solo día) se calculan con time.Now(); en UTC, el trabajo hecho
// entre medianoche y las 2:00 hora peninsular se imputaba al día anterior.
const defaultTimezone = "Europe/Madrid"

func init() {
	// TZ explícito (entorno) tiene prioridad; si no, se usa la zona de los usuarios
	if os.Getenv("TZ") != "" {
		return
	}
	loc, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		log.Printf("[Zona horaria] No se pudo cargar %s, se mantiene %s: %v", defaultTimezone, time.Local, err)
		return
	}
	time.Local = loc
}
