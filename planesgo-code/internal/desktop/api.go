package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ClientCookie: the cookie that carries a client's token (the server's cookieEscritorio).
const ClientCookie = "pgo_escritorio"

// API: the pgocode server, as this client (its token). The helper only reports remote sessions; it never sends
// passwords.
type API struct {
	Base  string // http://127.0.0.1:7801
	Token string
	HTTP  *http.Client
}

// APIError: the server answered with an error (Msg: its explanation, in Spanish).
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string { return e.Msg }

// Gone: the server does not know the session (it restarted, or closed it after a silence).
func Gone(err error) bool {
	e, ok := err.(*APIError)
	return ok && (e.Status == http.StatusNotFound || e.Status == http.StatusGone)
}

func (a *API) post(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, a.Base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PGO", "1")
	req.AddCookie(&http.Cookie{Name: ClientCookie, Value: a.Token})
	c := a.HTTP
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("el servidor de pgocode no responde: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			e.Error = "el servidor de pgocode responde " + res.Status
		}
		return &APIError{Status: res.StatusCode, Msg: e.Error}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// SessionInfo: what the client reports when a remote session opens (the server's remoteRequest).
type SessionInfo struct {
	Kind      string `json:"kind"` // rdp, anydesk
	Direction string `json:"direction"`
	Project   int    `json:"project"`
	Name      string `json:"name,omitempty"`
	Host      string `json:"host,omitempty"`
	User      string `json:"user,omitempty"`
	Peer      string `json:"peer,omitempty"`
	PeerName  string `json:"peer_name,omitempty"`
}

// Create reports a new remote session and returns its id.
func (a *API) Create(s SessionInfo) (string, error) {
	var r struct {
		ID string `json:"id"`
	}
	err := a.post("/api/remote/sessions", s, &r)
	return r.ID, err
}

// Beat: the session is still open; active: someone used the keyboard or mouse lately.
func (a *API) Beat(id string, active bool) error {
	return a.post("/api/remote/sessions/"+url.PathEscape(id)+"/beat", map[string]bool{"active": active}, nil)
}

// End: the session ended (msg: why, explained; "" if normally).
func (a *API) End(id, msg string) error {
	return a.post("/api/remote/sessions/"+url.PathEscape(id)+"/end", map[string]string{"error": msg}, nil)
}
