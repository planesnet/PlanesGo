// pgocode: arnés de desarrollo de Planes Software Factory (backend Go + interfaz web/Electron).
//
//	pgocode                         # http://127.0.0.1:7801
//	pgocode -addr 127.0.0.1:7801 -raices ~/cowork,~/proyectos
//
// Se entra con el usuario de Odoo (ODOO_URL, ODOO_DB). Agentes: claude, opencode y gemini en el PATH.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"planesgo-code/internal/actualizacion"
	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/llm"
	"planesgo-code/internal/odoo"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/selector"
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

// tokensEstudio lee (o crea la primera vez) los tokens fijos del estudio local, para que el emisor no tenga que
// reconfigurarse cada vez que se abre la app.
func tokensEstudio(fichero string) (emitir, ver string, err error) {
	var t struct{ Emitir, Ver string }
	if b, e := os.ReadFile(fichero); e == nil && json.Unmarshal(b, &t) == nil && t.Emitir != "" && t.Ver != "" {
		return t.Emitir, t.Ver, nil
	}
	var e1, e2 error
	t.Emitir, e1 = studiotv.TokenAleatorio()
	t.Ver, e2 = studiotv.TokenAleatorio()
	if err := errors.Join(e1, e2); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(fichero), 0o700); err != nil {
		return "", "", err
	}
	b, _ := json.Marshal(t)
	return t.Emitir, t.Ver, os.WriteFile(fichero, b, 0o600)
}

// configuraEmisor deja ~/.planesgo/studio_tv.env apuntando al estudio local (lo lee pco_emisor.py: las pruebas
// emiten sin exportar nada). No lo toca si apunta a otro estudio (p. ej. el de PlanesGo en producción).
func configuraEmisor(url, token string) (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	f := filepath.Join(h, ".planesgo", "studio_tv.env")
	canal := "auto" // el emisor usa qa-<repositorio>-<rama> de la carpeta de la prueba
	if b, err := os.ReadFile(f); err == nil {
		v := map[string]string{}
		for _, l := range strings.Split(string(b), "\n") {
			if k, x, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
				v[k] = x
			}
		}
		if v["PCO_URL"] != "" && v["PCO_URL"] != url {
			return "", nil // es de otro estudio: se respeta
		}
		if v["PCO_CANAL"] != "" {
			canal = v["PCO_CANAL"]
		}
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
		return "", err
	}
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, []byte("PCO_URL="+url+"\nPCO_TOKEN="+token+"\nPCO_CANAL="+canal+"\n"), 0o600); err != nil {
		return "", err
	}
	return f, os.Rename(tmp, f)
}

// ampliaPath añade al PATH las carpetas donde suelen instalarse los agentes y que una app abierta desde el
// escritorio no hereda (no lee .bashrc): Node de nvm (la versión más nueva primero), ~/.opencode/bin, bun, npm.
func ampliaPath() {
	casa, err := os.UserHomeDir()
	if err != nil {
		return
	}
	actual := filepath.SplitList(os.Getenv("PATH"))
	esta := map[string]bool{}
	for _, d := range actual {
		esta[d] = true
	}
	nvm, _ := filepath.Glob(filepath.Join(casa, ".nvm", "versions", "node", "*", "bin"))
	sort.Sort(sort.Reverse(sort.StringSlice(nvm)))
	var extra []string
	for _, d := range append([]string{filepath.Join(casa, ".local", "bin"), filepath.Join(casa, ".opencode", "bin"),
		filepath.Join(casa, ".bun", "bin"), filepath.Join(casa, ".npm-global", "bin")}, nvm...) {
		if st, err := os.Stat(d); err == nil && st.IsDir() && !esta[d] {
			esta[d] = true
			extra = append(extra, d)
		}
	}
	if len(extra) > 0 {
		os.Setenv("PATH", strings.Join(append(actual, extra...), string(os.PathListSeparator)))
	}
}

// entornoShell trae las variables del shell interactivo del usuario (.bashrc: nvm, GOOGLE_CLOUD_PROJECT…), que
// una app abierta desde el escritorio no hereda; así los agentes se comportan como en su terminal. Solo añade las
// que faltan (y el PATH, que se une). Si el shell tarda o falla, se sigue sin ellas.
func entornoShell() []string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/bash"
	}
	ctx, cancela := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancela()
	cmd := exec.CommandContext(ctx, sh, "-ilc", "env -0")
	cmd.Stdin = nil
	b, err := cmd.Output()
	if err != nil && len(b) == 0 {
		log.Printf("entorno del shell: %v", err)
		return nil
	}
	var nuevas []string
	for _, kv := range strings.Split(string(b), "\x00") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" || strings.HasPrefix(k, "BASH_FUNC_") {
			continue
		}
		switch k {
		case "PWD", "OLDPWD", "SHLVL", "_", "PS1", "TERM", "PGO_TOKEN_ESCRITORIO":
			continue
		case "PATH":
			actual := filepath.SplitList(os.Getenv("PATH"))
			esta := map[string]bool{}
			for _, d := range actual {
				esta[d] = true
			}
			for _, d := range filepath.SplitList(v) {
				if d != "" && !esta[d] {
					esta[d] = true
					actual = append(actual, d)
				}
			}
			os.Setenv("PATH", strings.Join(actual, string(os.PathListSeparator)))
			continue
		}
		if _, ya := os.LookupEnv(k); !ya {
			os.Setenv(k, v)
			nuevas = append(nuevas, k)
		}
	}
	return nuevas
}

func main() {
	// Enlazado como «pgo»: la orden de los agentes para ejecutar programas del proyecto.
	if filepath.Base(os.Args[0]) == "pgo" {
		os.Exit(pgo(os.Args[1:]))
	}
	ampliaPath()
	nuevasShell := entornoShell()
	addr := flag.String("addr", entorno("PGO_ADDR", "127.0.0.1:7801"), "dirección de escucha (solo local por defecto)")
	odooURL := flag.String("odoo", entorno("ODOO_URL", "https://planesnet.autopyme.com"), "URL de Odoo")
	odooDB := flag.String("db", entorno("ODOO_DB", "ap113"), "base de datos de Odoo")
	raices := flag.String("raices", entorno("PGO_RAICES", "~/cowork"), "carpetas donde buscar proyectos (.planesgo.json), separadas por comas")
	studioAddr := flag.String("studio", entorno("PGO_STUDIO_ADDR", "127.0.0.1:7802"), "dirección del Studio TV local (vacío para no arrancarlo)")
	urlAct := flag.String("actualizaciones", entorno("PGO_ACTUALIZACIONES", "https://planesgo.autopyme.com/install/pgocode"), "dónde se publican las actualizaciones (vacío: sin autoactualización)")
	dirGrab := flag.String("grabaciones", entorno("PGO_GRABACIONES", "~/.local/share/pgocode/grabaciones"), "carpeta de las grabaciones del Studio TV local")
	navegador := flag.Bool("navegador", os.Getenv("PGO_NAVEGADOR") == "1", "permite abrir Planes Go Code también en el navegador (por defecto, solo desde la app de escritorio)")
	verVersion := flag.Bool("version", false, "muestra la versión y sale")
	validaTiempos := flag.String("valida-tiempos", "", "valida un registro de tiempos (…/pgocode/tiempos/AAAA-MM-DD.jsonl), muestra el informe y sale")
	flag.Parse()
	if *validaTiempos != "" {
		f, err := os.Open(*validaTiempos)
		if err != nil {
			log.Fatal(err)
		}
		inf, err := imputacion.Valida(f, strings.HasPrefix(filepath.Base(*validaTiempos), time.Now().Format("2006-01-02")))
		if err != nil {
			log.Fatal(err)
		}
		b, _ := json.MarshalIndent(inf, "", "  ")
		fmt.Println(string(b))
		if inf.Errores > 0 {
			os.Exit(1)
		}
		return
	}
	*dirGrab = expande(*dirGrab)
	if *verVersion {
		fmt.Println("pgocode", version)
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
	var estudio *studiotv.Studio
	var srvStudio *http.Server
	if *studioAddr != "" {
		te, tv, err := tokensEstudio(filepath.Join(filepath.Dir(*dirGrab), "studio.json"))
		if err != nil {
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
			estudio = studiotv.Nuevo(te, tv)
			// Grabación disponible (la activa quien mira, canal a canal); por defecto solo directo.
			if err := estudio.GrabarEn(*dirGrab); err != nil {
				log.Printf("Studio TV local: %v", err)
			}
			srvStudio = &http.Server{Handler: estudio.Handler(), ReadHeaderTimeout: 10 * time.Second}
			go srvStudio.Serve(ln)
			log.Printf("Studio TV local en %s", studio.URL) // el token de emitir no se escribe en el registro
			if f, err := configuraEmisor(studio.URL, te); err != nil {
				log.Printf("emisor: %v", err)
			} else if f != "" {
				log.Printf("Emisor configurado en %s: las pruebas de este equipo emiten al estudio local", f)
			}
		}
	}

	s := &servidor.Servidor{
		Odoo:        odoo.Nuevo(*odooURL, *odooDB),
		Sesiones:    sesiones.NuevoAlmacen(),
		Indice:      proyectos.NuevoIndice(rs),
		Proveedores: llm.NuevoRegistro(llm.Agentes()...),
		Web:         web.Archivos(),
		Studio:      studio,
		Version:     version,
	}
	host, puerto, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("dirección no válida %q: %v", *addr, err)
	}
	// Solo en el propio equipo: la interfaz y la API no tienen más defensa que estar en local.
	if ip := net.ParseIP(host); !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		log.Fatalf("pgocode solo escucha en local (127.0.0.1, ::1 o localhost), no en %q", *addr)
	}
	s.Puerto = puerto
	// La app de escritorio pasa su secreto por entorno (no por argumento: se vería en la lista de procesos).
	s.TokenEscritorio = os.Getenv("PGO_TOKEN_ESCRITORIO")
	s.PermitirNavegador = *navegador

	// Registro de depuración (~/.local/share/pgocode/depuracion.log): agentes, órdenes, salidas y errores.
	if err := depuracion.Activa(filepath.Join(filepath.Dir(*dirGrab), "depuracion.log")); err != nil {
		log.Printf("depuración: %v", err)
	}
	// El log del backend (inicios de sesión, programas lanzados, actualizaciones…) también al registro: en la app
	// de escritorio su salida se pierde.
	log.SetOutput(io.MultiWriter(os.Stderr, depuracion.Escritor("backend")))
	depuracion.Log("inicio", "pgocode %s; variables del shell añadidas: %s; PATH=%s", version, strings.Join(nuevasShell, ","), os.Getenv("PATH"))

	// Programas del proyecto en local y la orden «pgo» para los agentes: ~/.local/share/pgocode/bin/pgo enlaza
	// con este ejecutable; los agentes la reciben en su PATH con un token propio de esta ejecución.
	s.Indice.Registro = filepath.Join(filepath.Dir(*dirGrab), "carpetas-proyectos.json")
	s.Selector = &selector.Selector{Fichero: filepath.Join(filepath.Dir(*dirGrab), "skills-embeddings.json")}
	go s.Selector.Precarga(context.Background()) // el modelo local tarda unos segundos en cargarse la primera vez
	// Imputación directa en Odoo. PlanesGo se duerme solo en los agentes que lanza pgocode (PLANESGO_GESTOR);
	// las sesiones de los agentes fuera de pgocode siguen imputando por su cuenta en el mismo equipo.
	s.Imputacion = &imputacion.Gestor{Odoo: s.Odoo, Aviso: func() { s.Eventos.Emite(map[string]string{"tipo": "imputacion"}) },
		FicheroEstado: filepath.Join(filepath.Dir(*dirGrab), "imputacion-dia.json"), Diario: filepath.Join(filepath.Dir(*dirGrab), "imputacion.log"),
		// Registro detallado de tiempos (un JSONL por día) para validar la imputación: Imputación → Validar.
		Tiempos: &imputacion.Registro{Dir: filepath.Join(filepath.Dir(*dirGrab), "tiempos"), Version: version}}
	// Recupera los partes de una ejecución anterior (cierre inesperado) y pone en marcha el cierre por espera.
	go s.Imputacion.Arranca(context.Background())
	// Adjuntos viejos (de sesiones ya terminadas): fuera, cuando el índice de carpetas esté listo.
	go func() {
		if n := s.LimpiaAdjuntos(); n > 0 {
			log.Printf("adjuntos: %d carpetas de más de 30 días borradas", n)
		}
	}()
	s.Procesos = procesos.Nuevo()
	s.AvisaProcesos()
	s.TokenAgente = aleatorio()
	if exe, err := os.Executable(); err == nil {
		bin := filepath.Join(filepath.Dir(*dirGrab), "bin")
		enlace := filepath.Join(bin, "pgo")
		if os.MkdirAll(bin, 0o755) == nil {
			os.Remove(enlace)
			if err := os.Symlink(exe, enlace); err == nil {
				s.DirBin = bin
			} else {
				log.Printf("orden pgo: %v", err)
			}
		}
	}

	// Autoactualización: la app instalada (paquete) vive junto al ejecutable; las versiones nuevas se preparan en
	// ~/.local/share/pgocode/actualizacion y se aplican cuando no hay nada en curso.
	datos := filepath.Dir(*dirGrab) // ~/.local/share/pgocode
	ficheroSesiones := filepath.Join(datos, "sesiones-reinicio.json")
	if n, err := s.Sesiones.Carga(ficheroSesiones, 5*time.Minute); err != nil {
		log.Printf("sesiones tras la actualización: %v", err)
	} else if n > 0 {
		log.Printf("sesiones recuperadas tras la actualización: %d", n)
	}
	exe, _ := os.Executable()
	if v := s.Version; v == "dev" || v == "" {
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "VERSION")); err == nil {
			s.Version = strings.TrimSpace(string(b))
		}
	}
	act := actualizacion.Nuevo(*urlAct, filepath.Dir(exe), filepath.Join(datos, "actualizacion"), s.Version)
	act.Electron = os.Getenv("PGO_ELECTRON")
	act.Ocupado = s.Ocupado
	if estudio != nil {
		s.EstudioOcupado = estudio.Ocupado
	}
	var salirParaActualizar atomic.Bool
	act.AlAplicar = func() {
		log.Printf("aplicando la actualización %s: se guarda la sesión y se reinicia", act.Estado().Nueva)
		if err := s.Sesiones.Guarda(ficheroSesiones); err != nil {
			log.Printf("no se pudo guardar la sesión: %v", err)
		}
		salirParaActualizar.Store(true)
		stop() // cierre limpio; al terminar se sale con actualizacion.SalidaActualizar
	}
	s.Actualizador = act
	s.DirRamas = filepath.Join(datos, "ramas")
	if e := act.Estado(); e.Estado == "desactivada" {
		log.Printf("autoactualización desactivada: %s", e.Error)
	} else {
		go act.Funciona(ctx)
	}
	if s.TokenEscritorio == "" && !s.PermitirNavegador {
		log.Print("aviso: sin PGO_TOKEN_ESCRITORIO ni -navegador nadie podrá usar la interfaz; ábrelo con la app de escritorio")
	}
	// Las peticiones heredan base: al apagar se cancelan todas, y con ellas los agentes en marcha
	// (exec.CommandContext), los chats y los directos. Sin esto, un agente podía seguir editando
	// ficheros con el arnés ya cerrado.
	base, cancelaBase := context.WithCancel(context.Background())
	srv := &http.Server{Addr: *addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return base }}
	apagado := make(chan struct{})
	go func() {
		defer close(apagado)
		<-ctx.Done()
		log.Print("cerrando Planes Go Code…")
		cancelaBase()
		s.Procesos.ParaTodos() // los programas lanzados se cierran con la app
		// Al actualizarse, los cronómetros siguen en Odoo y la versión nueva los retoma; al salir de verdad, se cierran.
		if salirParaActualizar.Load() {
			s.Imputacion.Suspende()
		} else {
			s.Imputacion.CierraTodo()
		}
		if estudio != nil {
			estudio.Cierra() // cierra bien los vídeos que se estén grabando
		}
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, x := range []*http.Server{srv, srvStudio} {
			if x != nil && x.Shutdown(c) != nil {
				x.Close()
			}
		}
	}()
	log.Printf("Planes Go Code %s en http://%s  (Odoo %s, base %s; proyectos en %s)", version, *addr, *odooURL, *odooDB, strings.Join(rs, ", "))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-apagado // espera a que los agentes y conexiones hayan terminado
	if salirParaActualizar.Load() {
		os.Exit(actualizacion.SalidaActualizar) // la app de escritorio cambia los ficheros y se vuelve a abrir
	}
}
