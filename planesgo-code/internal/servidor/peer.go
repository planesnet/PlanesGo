package servidor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"planesgo-code/internal/proceso"
)

// Defensa en profundidad: si quien se conecta es un agente lanzado por este pgocode (o un programa que lanzó un
// agente, o algo que desciende de ellos), solo se le atienden las órdenes del agente (/api/pgo/*), aunque traiga la
// cookie de la app o el servidor se abra al navegador (-navegador). Así un agente que encontrase la cookie de la
// interfaz no puede usar las rutas que hablan con Odoo. Se mira una vez por conexión (Linux; fuera, nada).

type peerKey struct{}

// peerInfo: lo que se sabe del otro extremo de la conexión (se calcula la primera vez que hace falta).
type peerInfo struct {
	once   sync.Once
	local  net.Addr
	remote net.Addr
	agent  bool
}

// ConnContext guarda las direcciones de la conexión para saber luego quién es (http.Server.ConnContext).
func (s *Servidor) ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, peerKey{}, &peerInfo{local: c.LocalAddr(), remote: c.RemoteAddr()})
}

// agentPrefixes: las marcas (PGO_EJECUCION) que pone este pgocode a sus agentes y a los programas que lanza.
func agentPrefixes() []string {
	return []string{fmt.Sprintf("%d-", os.Getpid()), fmt.Sprintf("prog-%d-", os.Getpid())}
}

// fromAgentProcess: la petición viene de un proceso de agente (o descendiente) de este pgocode.
func (s *Servidor) fromAgentProcess(r *http.Request) bool {
	if s.esProcesoAgente != nil {
		return s.esProcesoAgente(r)
	}
	pi, _ := r.Context().Value(peerKey{}).(*peerInfo)
	if pi == nil {
		return false
	}
	pi.once.Do(func() {
		if pid := proceso.PeerPid(pi.remote, pi.local); pid > 0 {
			pi.agent = proceso.MarkedLineage(pid, agentPrefixes())
		}
	})
	return pi.agent
}
