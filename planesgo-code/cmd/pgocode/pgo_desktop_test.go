package main

import (
	"encoding/json"
	"testing"
)

func TestDesktopArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string // the desktop action as JSON; "" = error
	}{
		{[]string{"captura"}, `{"action":"capture"}`},
		{[]string{"captura", "--escala", "0.6"}, `{"action":"capture","scale":0.6}`},
		{[]string{"captura", "--escala=0.5"}, `{"action":"capture","scale":0.5}`},
		{[]string{"captura", "--escala", "3"}, ""},
		{[]string{"captura", "f.png"}, ""},
		{[]string{"clic", "300", "150"}, `{"action":"click","x":300,"y":150}`},
		{[]string{"clic", "300", "150", "--doble", "--derecho"}, `{"action":"click","double":true,"right":true,"x":300,"y":150}`},
		{[]string{"clic", "300"}, ""},
		{[]string{"clic", "-3", "4"}, ""},
		{[]string{"mueve", "1", "2", "--doble"}, ""},
		{[]string{"arrastra", "1", "2", "3", "4"}, `{"action":"drag","x":1,"x2":3,"y":2,"y2":4}`},
		{[]string{"rueda", "abajo"}, `{"action":"wheel","up":false}`},
		{[]string{"rueda", "arriba", "5"}, `{"action":"wheel","count":5,"up":true}`},
		{[]string{"rueda", "izquierda"}, ""},
		{[]string{"escribe", "--hola ñ"}, `{"action":"type","text":"--hola ñ"}`},
		{[]string{"escribe", "a", "b"}, ""},
		{[]string{"tecla", "ctrl+s", "Return"}, `{"action":"key","keys":["ctrl+s","Return"]}`},
		{[]string{"tecla"}, ""},
		{[]string{"tamaño", "1920x1080"}, `{"action":"size","height":1080,"width":1920}`},
		{[]string{"tamaño", "grande"}, ""},
		{[]string{"graba", "inicio"}, `{"action":"record","record":"start"}`},
		{[]string{"graba", "fin"}, `{"action":"record","record":"stop"}`},
		{[]string{"graba"}, ""},
		{[]string{"estado"}, `{"action":"status"}`},
		{[]string{"inicia"}, ""},
		{nil, ""},
	} {
		body, err := desktopArgs(c.args, "/p")
		if c.want == "" {
			if err == nil {
				t.Errorf("%v: debería fallar", c.args)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		b, _ := json.Marshal(body["desktop"])
		if string(b) != c.want || body["carpeta"] != "/p" {
			t.Errorf("%v: %s", c.args, b)
		}
	}
}
