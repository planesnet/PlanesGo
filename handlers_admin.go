package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

// handleAdmin gestiona la página de administración de facturación de Google Cloud
func (state *AppState) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	session := state.getSession(r)
	if session == nil {
		http.Redirect(w, r, "/login?next=/admin", http.StatusFound)
		return
	}

	// Seguridad crítica: solo administradores autenticados mediante Google OAuth
	if session.IsAdmin() && session.AuthMethod != "google" {
		state.mu.RLock()
		googleAuthEnabled := state.cfg != nil && state.cfg.GoogleAuth.Enabled
		state.mu.RUnlock()

		if googleAuthEnabled {
			http.Redirect(w, r, "/auth/google?next=/admin", http.StatusFound)
			return
		}
		http.Error(w, "Acceso restringido: requiere autenticación con Google", http.StatusForbidden)
		return
	}

	if !session.IsAdmin() || session.AuthMethod != "google" {
		http.Error(w, "Acceso no autorizado", http.StatusForbidden)
		return
	}

	tmpl, err := template.ParseFiles("templates/admin.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error cargando plantilla admin.html: %v", err), http.StatusInternalServerError)
		return
	}

	data := struct {
		Version string
		Session *SessionData
	}{
		Version: Version,
		Session: session,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ADMIN] Error ejecutando plantilla admin.html: %v", err)
	}
}

// handleAPIAdminGCPInvoices devuelve las facturas mensuales de BigQuery en formato JSON
func (state *AppState) handleAPIAdminGCPInvoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Método no permitido",
		})
		return
	}

	session := state.getSession(r)
	if session == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Sesión no iniciada. Inicie sesión para acceder a la administración.",
		})
		return
	}

	if !session.IsAdmin() || session.AuthMethod != "google" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Acceso denegado: solo los administradores autenticados mediante Google tienen acceso a esta información.",
		})
		return
	}

	forceRefresh := r.URL.Query().Get("refresh") == "1"
	billingSvc := state.getBillingService()

	invoices, err := billingSvc.GetInvoices(r.Context(), forceRefresh)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(invoices)
}

// handleAPIAdminGCPInvoiceDetail devuelve el desglose detallado de costes de un mes de facturación
func (state *AppState) handleAPIAdminGCPInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Método no permitido",
		})
		return
	}

	session := state.getSession(r)
	if session == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Sesión no iniciada. Inicie sesión para acceder a la administración.",
		})
		return
	}

	if !session.IsAdmin() || session.AuthMethod != "google" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Acceso denegado: solo los administradores autenticados mediante Google tienen acceso a esta información.",
		})
		return
	}

	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Falta el parámetro 'month' (ejemplo: ?month=2026-09 o ?month=202609)",
		})
		return
	}

	forceRefresh := r.URL.Query().Get("refresh") == "1"
	billingSvc := state.getBillingService()

	detail, err := billingSvc.GetInvoiceDetail(r.Context(), month, forceRefresh)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(detail)
}

