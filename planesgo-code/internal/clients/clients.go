// Package clients keeps the client apps paired with this pgocode server. A client (the desktop app) pairs once
// with the pairing secret, which the server leaves in a file only its system user can read; it gets a token of its
// own, and every request then carries it. The server keeps only a hash of each token, so the file of clients gives
// nobody access. Clients can be revoked.
package clients

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Client: one paired client app.
type Client struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"` // the computer and user it was paired from
	Hash     string    `json:"hash"` // sha256 of its token
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen"`
}

// Registry: the paired clients (in File) and the pairing secret (in SecretFile).
type Registry struct {
	File       string
	SecretFile string
	Now        func() time.Time // tests

	mu      sync.Mutex
	clients []Client
	loaded  bool
	secret  string
}

// PairPath: the server route where a client pairs; SecretName: the file, in the server's data folder, with the pairing
// secret. The agents' guard (internal/llm) blocks both: an agent must not pair itself as a client.
const (
	PairPath   = "/api/clientes/emparejar"
	SecretName = "emparejar"
)

var ErrBadSecret = errors.New("el secreto de emparejamiento no es válido")

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (r *Registry) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Init creates a new pairing secret (each time the server starts) and loads the paired clients.
func (r *Registry) Init() error {
	s, err := randomHex(32)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.SecretFile), 0o700); err != nil {
		return err
	}
	if err := writeFile(r.SecretFile, []byte(s+"\n")); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secret = s
	return r.loadLocked()
}

// ReadSecret: the pairing secret a local client reads (it is the same system user's file).
func ReadSecret(file string) (string, error) {
	b, err := os.ReadFile(file)
	return strings.TrimSpace(string(b)), err
}

func (r *Registry) loadLocked() error {
	if r.loaded {
		return nil
	}
	r.loaded = true
	b, err := os.ReadFile(r.File)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &r.clients)
}

func (r *Registry) saveLocked() error {
	b, _ := json.MarshalIndent(r.clients, "", " ")
	return writeFile(r.File, b)
}

// writeFile writes atomically with permissions 0600.
func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Pair registers a client that knows the pairing secret and returns its id and token (the token is shown only
// now: the server keeps its hash).
func (r *Registry) Pair(secret, name string) (id, token string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(r.secret)) != 1 {
		return "", "", ErrBadSecret
	}
	if err := r.loadLocked(); err != nil {
		return "", "", err
	}
	if id, err = randomHex(8); err != nil {
		return "", "", err
	}
	if token, err = randomHex(32); err != nil {
		return "", "", err
	}
	name = strings.TrimSpace(name)
	if len(name) > 120 {
		name = name[:120]
	}
	now := r.now()
	r.clients = append(r.clients, Client{ID: id, Name: name, Hash: hash(token), Created: now, LastSeen: now})
	return id, token, r.saveLocked()
}

// Check: the client a token belongs to (ok false if none). Every hash is compared in constant time.
func (r *Registry) Check(token string) (id string, ok bool) {
	if token == "" {
		return "", false
	}
	h := hash(token)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loadLocked() != nil {
		return "", false
	}
	for i, c := range r.clients {
		if subtle.ConstantTimeCompare([]byte(h), []byte(c.Hash)) == 1 {
			id, ok = c.ID, true
			// Last seen, saved at most once a minute (each request comes through here).
			if now := r.now(); now.Sub(c.LastSeen) > time.Minute {
				r.clients[i].LastSeen = now
				r.saveLocked()
			}
		}
	}
	return id, ok
}

// List: the paired clients, the latest seen first (without their hashes).
func (r *Registry) List() []Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadLocked()
	l := make([]Client, len(r.clients))
	for i, c := range r.clients {
		c.Hash = ""
		l[i] = c
	}
	sort.Slice(l, func(i, j int) bool { return l[i].LastSeen.After(l[j].LastSeen) })
	return l
}

// Revoke removes a client: its token stops working at once.
func (r *Registry) Revoke(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadLocked(); err != nil {
		return err
	}
	for i, c := range r.clients {
		if c.ID == id {
			r.clients = append(r.clients[:i], r.clients[i+1:]...)
			return r.saveLocked()
		}
	}
	return errors.New("no hay ese cliente")
}
