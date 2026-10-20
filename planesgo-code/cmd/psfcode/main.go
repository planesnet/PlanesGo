// psfcode: arnés de desarrollo de Planes Software Factory (backend Go + interfaz web/Electron).
//
//	psfcode                         # http://127.0.0.1:7801
//	psfcode -addr 127.0.0.1:7801 -raices ~/cowork,~/proyectos
//
// Se entra con el usuario de Odoo (ODOO_URL, ODOO_DB). Proveedores: ANTHROPIC_API_KEY, OPENAI_API_KEY (y
// OPENAI_BASE_URL para servidores compatibles), GEMINI_API_KEY; agentes: claude, opencode, gemini en el PATH.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/odoo"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/servidor"
	"planesgo-code/internal/sesiones"
	studiotv "planesgo-code/studio-tv"
	"planesgo-code/web"
)

var version = "dev"

func entorno(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func expande(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func main() {
	addr := flag.String("addr", entorno("PSF_ADDR", "127.0.0.1:7801"), "dirección de escucha (solo local por defecto)")
	odooURL := flag.String("odoo", entorno("ODOO_URL", "https://planesnet.autopyme.com"), "URL de Odoo")
	odooDB := flag.String("db", entorno("ODOO_DB", "ap113"), "base de datos de Odoo")
	raices := flag.String("raices", entorno("PSF_RAICES", "~/cowork"), "carpetas donde buscar proyectos (.planesgo.json), separadas por comas")
	studioAddr := flag.String("studio", entorno("PSF_STUDIO_ADDR", "127.0.0.1:7802"), "dirección del Studio TV local (vacío para no arrancarlo)")
	verVersion := flag.Bool("version", false, "muestra la versión y sale")
	flag.Parse()
	if *verVersion {
		fmt.Println("psfcode", version)
		return
	}

	var rs []string
	for _, r := range strings.Split(*raices, ",") {
		if r = expande(r); r != "" {
			rs = append(rs, r)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var studio *servidor.StudioLocal
	var srvStudio *http.Server
	if *studioAddr != "" {
		te, err1 := studiotv.TokenAleatorio()
		tv, err2 := studiotv.TokenAleatorio()
		if err := errors.Join(err1, err2); err != nil {
			log.Fatal(err)
		}
		ln, err := net.Listen("tcp", *studioAddr)
		if err != nil {
			log.Printf("Studio TV local: %s ocupado, uso un puerto libre", *studioAddr)
			ln, err = net.Listen("tcp", "127.0.0.1:0")
		}
		if err != nil {
			log.Printf("Studio TV local desactivado: %v", err)
		} else {
			studio = &servidor.StudioLocal{URL: "http://" + ln.Addr().String(), TokenEmit: te, TokenVer: tv}
			srvStudio = &http.Server{Handler: studiotv.Nuevo(te, tv).Handler(), ReadHeaderTimeout: 10 * time.Second}
			go srvStudio.Serve(ln)
			log.Printf("Studio TV local en %s  (emitir: PCO_URL=%s PCO_TOKEN=%s)", studio.URL, studio.URL, te)
		}
	}

	s := &servidor.Servidor{
		Odoo:     odoo.Nuevo(*odooURL, *odooDB),
		Sesiones: sesiones.NuevoAlmacen(),
		Indice:   proyectos.NuevoIndice(rs),
		Proveedores: llm.NuevoRegistro(append([]llm.Proveedor{&llm.Claude{}, &llm.ChatGPT{}, &llm.Gemini{}},
			llm.Agentes()...)...),
		Web:     web.Archivos(),
		Studio:  studio,
		Version: version,
	}
	srv := &http.Server{Addr: *addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, x := range []*http.Server{srv, srvStudio} {
			if x != nil && x.Shutdown(c) != nil {
				x.Close()
			}
		}
	}()
	log.Printf("PSFCode %s en http://%s  (Odoo %s, base %s; proyectos en %s)", version, *addr, *odooURL, *odooDB, strings.Join(rs, ", "))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
