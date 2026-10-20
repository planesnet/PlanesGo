// Package sesiones guarda en memoria las sesiones del arnés (una por usuario y navegador).
//
// La cookie del navegador lleva solo un token aleatorio; la sesión de Odoo y los datos de conexión (p. ej. el token
// de ver de Studio TV) se quedan en el servidor.
package sesiones

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Duracion es lo que dura una sesión sin uso.
const Duracion = 12 * time.Hour

// ConexionStudio es la conexión de un proyecto con un estudio de Studio TV.
type ConexionStudio struct {
	URL   string `json:"url"`
	Token string `json:"-"` // token de ver: no sale del servidor
	Local bool   `json:"local"`
}

// Sesion es el estado de un usuario identificado.
type Sesion struct {
	Token      string
	UID        int
	Nombre     string
	Login      string
	OdooSesion string

	mu        sync.Mutex
	ultimoUso time.Time
	// proyectos comprobados en Odoo y hasta cuándo vale la comprobación
	proyectos map[int]time.Time
	studio    map[int]ConexionStudio
}

// ProyectoComprobado dice si el proyecto se comprobó en Odoo hace poco.
func (s *Sesion) ProyectoComprobado(id int, ahora time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ahora.Before(s.proyectos[id])
}

// MarcaProyecto apunta que el usuario tiene acceso al proyecto durante el tiempo indicado.
func (s *Sesion) MarcaProyecto(id int, hasta time.Time) {
	s.mu.Lock()
	s.proyectos[id] = hasta
	s.mu.Unlock()
}

// Studio devuelve la conexión de Studio TV del proyecto.
func (s *Sesion) Studio(proyecto int) (ConexionStudio, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.studio[proyecto]
	return c, ok
}

// PonStudio guarda (o borra, con la URL vacía) la conexión de Studio TV del proyecto.
func (s *Sesion) PonStudio(proyecto int, c ConexionStudio) {
	s.mu.Lock()
	if c.URL == "" {
		delete(s.studio, proyecto)
	} else {
		s.studio[proyecto] = c
	}
	s.mu.Unlock()
}

// Almacen guarda las sesiones activas.
type Almacen struct {
	mu       sync.Mutex
	sesiones map[string]*Sesion
	ahora    func() time.Time
}

// NuevoAlmacen crea un almacén vacío.
func NuevoAlmacen() *Almacen {
	return &Almacen{sesiones: map[string]*Sesion{}, ahora: time.Now}
}

// Crea abre una sesión y devuelve su token.
func (a *Almacen) Crea(uid int, nombre, login, odooSesion string) (*Sesion, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	s := &Sesion{Token: hex.EncodeToString(b), UID: uid, Nombre: nombre, Login: login, OdooSesion: odooSesion,
		ultimoUso: a.ahora(), proyectos: map[int]time.Time{}, studio: map[int]ConexionStudio{}}
	a.mu.Lock()
	a.purgaLocked()
	a.sesiones[s.Token] = s
	a.mu.Unlock()
	return s, nil
}

// Busca devuelve la sesión del token si sigue viva, y renueva su caducidad.
func (a *Almacen) Busca(token string) (*Sesion, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sesiones[token]
	if !ok {
		return nil, false
	}
	ahora := a.ahora()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ahora.Sub(s.ultimoUso) > Duracion {
		delete(a.sesiones, token)
		return nil, false
	}
	s.ultimoUso = ahora
	return s, true
}

// Borra cierra la sesión.
func (a *Almacen) Borra(token string) {
	a.mu.Lock()
	delete(a.sesiones, token)
	a.mu.Unlock()
}

func (a *Almacen) purgaLocked() {
	ahora := a.ahora()
	for t, s := range a.sesiones {
		s.mu.Lock()
		if ahora.Sub(s.ultimoUso) > Duracion {
			delete(a.sesiones, t)
		}
		s.mu.Unlock()
	}
}

// Persistencia solo para el reinicio de una actualización: se guardan las sesiones abiertas (con la sesión de
// Odoo y las conexiones de Studio TV) en un fichero 0600, y al volver a arrancar se cargan y se borra el
// fichero. Así la app se actualiza y vuelve a abrirse sin pedir la contraseña.

type conexionGuardada struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Local bool   `json:"local"`
}

type sesionGuardada struct {
	Token      string                   `json:"token"`
	UID        int                      `json:"uid"`
	Nombre     string                   `json:"nombre"`
	Login      string                   `json:"login"`
	OdooSesion string                   `json:"odoo_sesion"`
	UltimoUso  time.Time                `json:"ultimo_uso"`
	Studio     map[int]conexionGuardada `json:"studio,omitempty"`
}

// Guarda escribe las sesiones vivas en fichero (0600).
func (a *Almacen) Guarda(fichero string) error {
	a.mu.Lock()
	var l []sesionGuardada
	for _, s := range a.sesiones {
		s.mu.Lock()
		g := sesionGuardada{Token: s.Token, UID: s.UID, Nombre: s.Nombre, Login: s.Login, OdooSesion: s.OdooSesion,
			UltimoUso: s.ultimoUso, Studio: map[int]conexionGuardada{}}
		for p, c := range s.studio {
			g.Studio[p] = conexionGuardada{URL: c.URL, Token: c.Token, Local: c.Local}
		}
		s.mu.Unlock()
		l = append(l, g)
	}
	a.mu.Unlock()
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := fichero + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, fichero)
}

// Carga recupera las sesiones guardadas si el fichero es reciente (maxEdad) y lo borra siempre.
func (a *Almacen) Carga(fichero string, maxEdad time.Duration) (int, error) {
	st, err := os.Stat(fichero)
	if err != nil {
		return 0, nil
	}
	defer os.Remove(fichero)
	if time.Since(st.ModTime()) > maxEdad {
		return 0, nil
	}
	b, err := os.ReadFile(fichero)
	if err != nil {
		return 0, err
	}
	var l []sesionGuardada
	if err := json.Unmarshal(b, &l); err != nil {
		return 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, g := range l {
		s := &Sesion{Token: g.Token, UID: g.UID, Nombre: g.Nombre, Login: g.Login, OdooSesion: g.OdooSesion,
			ultimoUso: g.UltimoUso, proyectos: map[int]time.Time{}, studio: map[int]ConexionStudio{}}
		for p, c := range g.Studio {
			s.studio[p] = ConexionStudio{URL: c.URL, Token: c.Token, Local: c.Local}
		}
		a.sesiones[s.Token] = s
	}
	return len(l), nil
}
