package llm

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Un resultado de herramienta con una imagen (como una captura del navegador por MCP, anidada) llega a la
// herramienta como imagen, no como texto.
func TestImagenesClaude(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfalso"))
	l := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"mcp__navegador__computer","input":{"action":"screenshot"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"captura"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + png + `"}}]}]}}
{"type":"user","parent_tool_use_id":"p1","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":[{"type":"tool_result","content":[{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"` + png + `"}}]}]}]}}
`
	var hs []*Herramienta
	leeClaudeCode(strings.NewReader(l), func(e Evento) {
		if e.Herramienta != nil {
			hs = append(hs, e.Herramienta)
		}
	})
	if len(hs) != 3 || len(hs[1].Imagenes) != 1 || hs[1].Imagenes[0].Tipo != "image/png" || !strings.HasPrefix(string(hs[1].Imagenes[0].Datos), "\x89PNG") {
		t.Fatalf("principal: %+v", hs)
	}
	if strings.Contains(hs[1].Salida, png) {
		t.Error("la imagen no debe ir en la salida de texto")
	}
	if len(hs[2].Imagenes) != 1 || hs[2].Imagenes[0].Tipo != "image/jpeg" || hs[2].Padre != "p1" {
		t.Errorf("anidada en subagente: %+v", hs[2])
	}
}
