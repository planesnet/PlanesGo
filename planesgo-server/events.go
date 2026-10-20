package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"pasigo/odoo"
)

// SSEEvent representa un evento que se envía a los clientes conectados
type SSEEvent struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// SSEClient representa un cliente HTTP conectado vía EventSource
type SSEClient struct {
	userUID int
	ch      chan []byte
}

// SSEHub gestiona la distribución de eventos en tiempo real agrupados por usuario
type SSEHub struct {
	mu      sync.RWMutex
	clients map[int]map[*SSEClient]bool
}

// NewSSEHub inicializa un nuevo hub de eventos en tiempo real
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[int]map[*SSEClient]bool),
	}
}

// Register añade un cliente SSE al hub para un usuario concreto
func (h *SSEHub) Register(userUID int, client *SSEClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userUID] == nil {
		h.clients[userUID] = make(map[*SSEClient]bool)
	}
	h.clients[userUID][client] = true
	log.Printf("[SSE] Cliente conectado para UID %d (total clientes para UID: %d)", userUID, len(h.clients[userUID]))
}

// Unregister elimina un cliente SSE cuando se cierra la conexión
func (h *SSEHub) Unregister(userUID int, client *SSEClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.clients[userUID]; ok {
		delete(clients, client)
		close(client.ch)
		if len(clients) == 0 {
			delete(h.clients, userUID)
		}
	}
	log.Printf("[SSE] Cliente desconectado para UID %d", userUID)
}

// BroadcastToUser emite un evento SSE inmediatamente a todos los clientes del usuario (PC, móvil, popups)
func (h *SSEHub) BroadcastToUser(userUID int, eventType string, payload interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, ok := h.clients[userUID]
	if !ok || len(clients) == 0 {
		return
	}

	evt := SSEEvent{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}

	msg := []byte(fmt.Sprintf("event: message\ndata: %s\n\n", string(data)))
	for c := range clients {
		select {
		case c.ch <- msg:
		default:
			// Buffer lleno o cliente lento, no bloquear al resto
		}
	}
}

// broadcastUserEvent es el helper en AppState para notificar cambios
func (state *AppState) broadcastUserEvent(userUID int, eventType string, payload interface{}) {
	if state.sseHub != nil && userUID > 0 {
		state.sseHub.BroadcastToUser(userUID, eventType, payload)
	}
}

// handleAPIEvents maneja la conexión Server-Sent Events (SSE) nativa del navegador
func (state *AppState) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}
	if session == nil {
		http.Error(w, "No autenticado", http.StatusUnauthorized)
		return
	}

	odooCfg := state.resolveUserOdooConfig(session)
	client := odoo.GetClient(odooCfg)
	userUID := client.UID()
	if session.UserEmail != "" {
		if resolvedUID, err := client.ResolveUserUIDByEmail(r.Context(), session.UserEmail); err == nil && resolvedUID > 0 {
			userUID = resolvedUID
		}
	}
	if userUID <= 0 {
		http.Error(w, "Usuario no identificado", http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming SSE no soportado", http.StatusInternalServerError)
		return
	}

	// Desactivar write deadline para permitir conexión SSE persistente
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// Encabezados SSE estándar y anti-buffering para Nginx / Traefik
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sseClient := &SSEClient{
		userUID: userUID,
		ch:      make(chan []byte, 32),
	}

	state.sseHub.Register(userUID, sseClient)
	defer state.sseHub.Unregister(userUID, sseClient)

	// Saludo inicial para confirmar conexión lista
	initData, _ := json.Marshal(map[string]interface{}{
		"status":   "connected",
		"user_uid": userUID,
		"time":     time.Now().UnixMilli(),
	})
	w.Write([]byte(fmt.Sprintf("event: connected\ndata: %s\n\n", string(initData))))
	flusher.Flush()

	// Keep-alive ping cada 15 segundos para mantener vivas las conexiones NAT y móviles
	keepAliveTicker := time.NewTicker(15 * time.Second)
	defer keepAliveTicker.Stop()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case <-keepAliveTicker.C:
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		case msg, ok := <-sseClient.ch:
			if !ok {
				return
			}
			w.Write(msg)
			flusher.Flush()
		}
	}
}
