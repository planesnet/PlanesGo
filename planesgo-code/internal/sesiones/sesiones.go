// Package sesiones guarda en memoria las sesiones del arnés (una por usuario y navegador).
//
// La cookie del navegador lleva solo un token aleatorio; la sesión de Odoo y los datos de conexión (p. ej. el token
// de ver de Studio TV) se quedan en el servidor.
package sesiones

import (
	"crypto/rand"
	"encoding/hex"
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
