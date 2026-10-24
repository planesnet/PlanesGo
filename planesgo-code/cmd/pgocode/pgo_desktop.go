package main

import (
	"fmt"
	"strconv"
	"strings"
)

// desktopHelp: «pgo escritorio» in the help of pgo.
const desktopHelp = `Escritorio del agente de la sesión (un Windows por RDP; lo inicia la persona en las propiedades de la sesión):
  pgo escritorio captura [--escala 0.6]     captura de la pantalla en .pgocode/salidas (te da la ruta: léela)
  pgo escritorio clic X Y [--doble|--derecho]
  pgo escritorio mueve X Y
  pgo escritorio arrastra X1 Y1 X2 Y2       las coordenadas son las de la última captura (con su escala)
  pgo escritorio rueda arriba|abajo [N]     en la posición del ratón (N pasos, 3 por defecto)
  pgo escritorio escribe "texto"            tal cual (acentos, ñ, €)
  pgo escritorio tecla ctrl+s [Return …]    teclas con nombres de xdotool (Return, Tab, Escape, F5, super…)
  pgo escritorio tamaño 1920x1080           cambia el tamaño de la pantalla remota
  pgo escritorio graba inicio|fin           vídeo de la sesión (evidencias) en la carpeta de intercambio
  pgo escritorio estado
`

// desktopActions: the Spanish orders of «pgo escritorio» and the backend's actions.
var desktopActions = map[string]string{"captura": "capture", "clic": "click", "mueve": "move", "arrastra": "drag",
	"rueda": "wheel", "escribe": "type", "tecla": "key", "tamaño": "size", "graba": "record", "estado": "status"}

// desktopArgs: the body of «pgo escritorio <orden> …» (the backend validates it again).
func desktopArgs(args []string, dir string) (map[string]any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("falta la orden (captura, clic, mueve, arrastra, rueda, escribe, tecla, tamaño, graba o estado)")
	}
	name, ok := desktopActions[args[0]]
	if !ok {
		return nil, fmt.Errorf("orden desconocida: %s", args[0])
	}
	a := map[string]any{"action": name}
	var pos []string
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch x := rest[i]; {
		case name == "click" && x == "--doble":
			a["double"] = true
		case name == "click" && x == "--derecho":
			a["right"] = true
		case name == "capture" && (x == "--escala" || strings.HasPrefix(x, "--escala=")):
			v, has := strings.CutPrefix(x, "--escala=")
			if !has {
				if i+1 >= len(rest) {
					return nil, fmt.Errorf("falta el valor de --escala")
				}
				i++
				v = rest[i]
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0.1 || f > 1 {
				return nil, fmt.Errorf("--escala va de 0.1 a 1")
			}
			a["scale"] = f
		case name != "type" && name != "key" && strings.HasPrefix(x, "--"):
			return nil, fmt.Errorf("opción desconocida %s", x)
		default:
			pos = append(pos, x)
		}
	}
	ints := func(n int, keys ...string) error {
		if len(pos) != n {
			return fmt.Errorf("hacen falta %d números", n)
		}
		for i, k := range keys {
			v, err := strconv.Atoi(pos[i])
			if err != nil || v < 0 {
				return fmt.Errorf("coordenada no válida: %s", pos[i])
			}
			a[k] = v
		}
		return nil
	}
	var err error
	switch name {
	case "capture", "status":
		if len(pos) > 0 {
			err = fmt.Errorf("sobra %s", pos[0])
		}
	case "click", "move":
		err = ints(2, "x", "y")
	case "drag":
		err = ints(4, "x", "y", "x2", "y2")
	case "wheel":
		if len(pos) == 0 || len(pos) > 2 || (pos[0] != "arriba" && pos[0] != "abajo") {
			return nil, fmt.Errorf("pgo escritorio rueda arriba|abajo [N]")
		}
		a["up"] = pos[0] == "arriba"
		if len(pos) == 2 {
			n, e := strconv.Atoi(pos[1])
			if e != nil || n <= 0 {
				return nil, fmt.Errorf("N tiene que ser un número")
			}
			a["count"] = n
		}
	case "type":
		if len(pos) != 1 {
			return nil, fmt.Errorf("el texto, entre comillas: pgo escritorio escribe \"texto\"")
		}
		a["text"] = pos[0]
	case "key":
		if len(pos) == 0 {
			return nil, fmt.Errorf("di qué teclas: pgo escritorio tecla ctrl+s")
		}
		a["keys"] = pos
	case "size":
		w, h, found := strings.Cut(strings.ToLower(strings.Join(pos, "")), "x")
		wi, e1 := strconv.Atoi(w)
		hi, e2 := strconv.Atoi(h)
		if len(pos) != 1 || !found || e1 != nil || e2 != nil {
			return nil, fmt.Errorf("el tamaño, como 1920x1080")
		}
		a["width"], a["height"] = wi, hi
	case "record":
		r := map[string]string{"inicio": "start", "fin": "stop"}
		if len(pos) != 1 || r[pos[0]] == "" {
			return nil, fmt.Errorf("pgo escritorio graba inicio|fin")
		}
		a["record"] = r[pos[0]]
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"desktop": a, "carpeta": dir}, nil
}
