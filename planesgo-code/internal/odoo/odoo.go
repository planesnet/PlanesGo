// Package odoo habla con el Odoo de PlanesNet (planesnet.autopyme.com) con la sesión de cada usuario.
//
// No se guardan contraseñas: el usuario se autentica una vez (/web/session/authenticate) y se conserva solo la
// cookie de sesión de Odoo. Así cada consulta respeta los permisos que ese usuario tiene en Odoo.
package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrSesionCaducada indica que Odoo ya no acepta la sesión: hay que volver a entrar.
var ErrSesionCaducada = errors.New("la sesión de Odoo ha caducado")

// ErrCredenciales indica usuario o contraseña incorrectos.
var ErrCredenciales = errors.New("usuario o contraseña incorrectos")

// Cliente apunta a una instancia y base de datos de Odoo.
type Cliente struct {
	URL  string
	DB   string
	HTTP *http.Client
}

// Nuevo crea un cliente para la URL y base de datos indicadas.
func Nuevo(url, db string) *Cliente {
	return &Cliente{URL: strings.TrimRight(url, "/"), DB: db, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Usuario es lo que devuelve Odoo al autenticar.
type Usuario struct {
	UID    int
	Nombre string
	Login  string
	// Sesion es la cookie session_id de Odoo. Es secreta: no se envía nunca al navegador.
	Sesion string
}

type errorRPC struct {
	Message string `json:"message"`
	Data    struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"data"`
}

func (e *errorRPC) Error() string {
	if e.Data.Message != "" {
		return e.Data.Message
	}
	return e.Message
}

// rpc hace una llamada JSON-RPC a una ruta web de Odoo con la cookie de sesión (si la hay).
func (c *Cliente) rpc(ctx context.Context, ruta, sesion string, params any, res any) (*http.Response, error) {
	cuerpo, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "call", "params": params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+ruta, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sesion != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sesion})
	}
	r, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar con Odoo: %w", err)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Odoo respondió %s", r.Status)
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *errorRPC       `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("respuesta de Odoo no válida: %w", err)
	}
	if env.Error != nil {
		switch {
		case strings.Contains(env.Error.Data.Name, "SessionExpired"):
			return nil, ErrSesionCaducada
		case strings.Contains(env.Error.Data.Name, "AccessDenied"):
			return nil, ErrCredenciales
		}
		return nil, env.Error
	}
	if res != nil {
		if err := json.Unmarshal(env.Result, res); err != nil {
			return nil, fmt.Errorf("respuesta de Odoo no válida: %w", err)
		}
	}
	return r, nil
}

// Autentica comprueba usuario y contraseña y devuelve la sesión de Odoo.
func (c *Cliente) Autentica(ctx context.Context, login, password string) (*Usuario, error) {
	var res struct {
		UID  json.RawMessage `json:"uid"`
		Name string          `json:"name"`
		User string          `json:"username"`
	}
	r, err := c.rpc(ctx, "/web/session/authenticate", "", map[string]any{"db": c.DB, "login": login, "password": password}, &res)
	if err != nil {
		return nil, err
	}
	var uid int
	if json.Unmarshal(res.UID, &uid) != nil || uid == 0 {
		return nil, ErrCredenciales
	}
	for _, ck := range r.Cookies() {
		if ck.Name == "session_id" && ck.Value != "" {
			return &Usuario{UID: uid, Nombre: res.Name, Login: res.User, Sesion: ck.Value}, nil
		}
	}
	return nil, errors.New("Odoo no devolvió la cookie de sesión")
}

// CallKW llama a un método de un modelo con la sesión del usuario (respeta sus permisos).
func (c *Cliente) CallKW(ctx context.Context, sesion, modelo, metodo string, args []any, kwargs map[string]any, res any) error {
	if kwargs == nil {
		kwargs = map[string]any{}
	}
	if args == nil {
		args = []any{}
	}
	_, err := c.rpc(ctx, "/web/dataset/call_kw/"+modelo+"/"+metodo, sesion,
		map[string]any{"model": modelo, "method": metodo, "args": args, "kwargs": kwargs}, res)
	return err
}

// Cierra termina la sesión en Odoo.
func (c *Cliente) Cierra(ctx context.Context, sesion string) error {
	_, err := c.rpc(ctx, "/web/session/destroy", sesion, map[string]any{}, nil)
	return err
}

// Many2One es un campo many2one de Odoo: [id, "nombre"] o false.
type Many2One struct {
	ID     int    `json:"id"`
	Nombre string `json:"nombre"`
}

func (m *Many2One) UnmarshalJSON(b []byte) error {
	var v []any
	if json.Unmarshal(b, &v) != nil || len(v) < 2 {
		*m = Many2One{}
		return nil
	}
	if id, ok := v[0].(float64); ok {
		m.ID = int(id)
	}
	m.Nombre, _ = v[1].(string)
	return nil
}

// Proyecto es un project.project de Odoo.
type Proyecto struct {
	ID          int      `json:"id"`
	Nombre      string   `json:"nombre"`
	Cliente     Many2One `json:"cliente"`
	Responsable Many2One `json:"responsable"`
}

type proyectoOdoo struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	PartnerID Many2One `json:"partner_id"`
	UserID    Many2One `json:"user_id"`
}

// Proyectos devuelve los proyectos activos que el usuario puede ver (o solo el indicado, si id > 0).
func (c *Cliente) Proyectos(ctx context.Context, sesion string, id int) ([]Proyecto, error) {
	dominio := []any{[]any{"active", "=", true}}
	if id > 0 {
		dominio = append(dominio, []any{"id", "=", id})
	}
	var filas []proyectoOdoo
	err := c.CallKW(ctx, sesion, "project.project", "search_read", []any{dominio},
		map[string]any{"fields": []string{"id", "name", "partner_id", "user_id"}, "order": "name asc"}, &filas)
	if err != nil {
		return nil, err
	}
	l := make([]Proyecto, len(filas))
	for i, f := range filas {
		l[i] = Proyecto{ID: f.ID, Nombre: f.Name, Cliente: f.PartnerID, Responsable: f.UserID}
	}
	return l, nil
}

// Avatar devuelve la imagen del usuario (128 px) con su propia sesión.
func (c *Cliente) Avatar(ctx context.Context, sesion string, uid int) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/web/image/res.users/%d/image_128", c.URL, uid), nil)
	if err != nil {
		return nil, "", err
	}
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sesion})
	r, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer r.Body.Close()
	tipo := r.Header.Get("Content-Type")
	if r.StatusCode != http.StatusOK || !strings.HasPrefix(tipo, "image/") {
		return nil, "", fmt.Errorf("sin avatar (%s)", r.Status)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	return b, tipo, err
}
