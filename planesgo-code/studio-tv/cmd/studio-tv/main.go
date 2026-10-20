// studio-tv: estudio de TV de PSFCode para ver en directo las pruebas automáticas.
//
//	studio-tv                                     # HTTP en 127.0.0.1:8080 (local o detrás de un proxy HTTPS)
//	studio-tv -addr :8443 -selfsigned             # HTTPS con certificado propio
//	studio-tv -addr :8443 -cert c.pem -key k.pem  # HTTPS con certificado propio de un dominio
//
// Tokens: -emit-token / STUDIO_TV_EMIT_TOKEN (emitir) y -view-token / STUDIO_TV_VIEW_TOKEN (ver). Si faltan,
// se generan y se muestran al arrancar. Mejor por variable de entorno: los argumentos se ven en la lista de procesos.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	studiotv "planesgo-code/studio-tv"
)

var version = "dev"

func token(v *string) {
	if *v != "" {
		return
	}
	t, err := studiotv.TokenAleatorio()
	if err != nil {
		log.Fatal(err)
	}
	*v = t
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "dirección de escucha")
	emitTok := flag.String("emit-token", os.Getenv("STUDIO_TV_EMIT_TOKEN"), "token del emisor (si falta, se genera)")
	viewTok := flag.String("view-token", os.Getenv("STUDIO_TV_VIEW_TOKEN"), "token de los espectadores (si falta, se genera)")
	cert := flag.String("cert", "", "certificado TLS (PEM)")
	key := flag.String("key", "", "clave TLS (PEM)")
	selfsigned := flag.Bool("selfsigned", false, "HTTPS con un certificado propio generado al arrancar")
	hosts := flag.String("hosts", "localhost,127.0.0.1", "nombres e IP del certificado propio, separados por comas")
	verVersion := flag.Bool("version", false, "muestra la versión y sale")
	flag.Parse()
	if *verVersion {
		fmt.Println("studio-tv", version)
		return
	}
	if (*cert == "") != (*key == "") {
		log.Fatal("-cert y -key van juntos")
	}
	token(emitTok)
	token(viewTok)
	if *emitTok == *viewTok {
		log.Fatal("el token de emitir y el de ver deben ser distintos")
	}

	srv := &http.Server{Addr: *addr, Handler: studiotv.Nuevo(*emitTok, *viewTok).Handler(), ReadHeaderTimeout: 10 * time.Second}
	esquema := "http"
	if *cert != "" || *selfsigned {
		esquema = "https"
	}
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	log.Printf("Studio TV %s en %s://%s", version, esquema, host)
	log.Printf("Ver:    %s://%s/?t=%s", esquema, host, *viewTok)
	log.Printf("Emitir: PCO_URL=%s://%s  PCO_TOKEN=%s  (PCO_CANAL=<nombre> opcional; por defecto %q)",
		esquema, host, *emitTok, studiotv.CanalPorDefecto)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Print("cerrando el estudio…")
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if srv.Shutdown(c) != nil {
			srv.Close() // los directos (MJPEG y SSE) no terminan solos
		}
	}()

	var err error
	switch {
	case *cert != "":
		err = srv.ListenAndServeTLS(*cert, *key)
	case *selfsigned:
		c, e := studiotv.CertificadoPropio(strings.Split(*hosts, ","))
		if e != nil {
			log.Fatal(e)
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{c}, MinVersion: tls.VersionTLS12}
		err = srv.ListenAndServeTLS("", "")
	default:
		err = srv.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
