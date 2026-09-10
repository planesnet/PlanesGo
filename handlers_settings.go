package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pasigo/config"
	"pasigo/odoo"
	"pasigo/store"
)

// handleSettings gestiona la vista y guardado de ajustes de usuario (URL, BD, token Odoo, etc.)
func (state *AppState) handleSettings(w http.ResponseWriter, r *http.Request) {
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	isAnonymous := false
	if session == nil {
		isAnonymous = true
		session = &SessionData{
			Username:   "Configuración",
			UserEmail:  "",
			URL:        DefaultOdooURL,
			DB:         "",
			AuthMethod: "local",
		}
	}

	userEmail := session.Username
	if session.UserEmail != "" {
		userEmail = session.UserEmail
	}
	if isAnonymous {
		userEmail = "default"
	}

	state.mu.RLock()
	defaultCfg := state.cfg
	state.mu.RUnlock()

	// Obtener o inicializar ajustes persistentes del usuario
	var userSettings store.UserSettings
	if state.userStore != nil {
		if s, ok := state.userStore.GetSettings(userEmail); ok {
			userSettings = s
		} else {
			// Cargar configuración de servidor previamente guardada en el almacén de ajustes
			if sharedURL := state.userStore.GetSharedOdooURL(); sharedURL != "" {
				userSettings.OdooURL = sharedURL
			}
			if sharedDB := state.userStore.GetSharedOdooDB(); sharedDB != "" {
				userSettings.OdooDB = sharedDB
			}
		}
	}

	if userSettings.Email == "" {
		userSettings.Email = userEmail
	}
	if userSettings.OdooUser == "" && !isAnonymous {
		userSettings.OdooUser = userEmail
	}
	if userSettings.OdooURL == "" || userSettings.OdooURL == "https://www.planesnet.com" {
		if defaultCfg.Odoo.URL != "" && defaultCfg.Odoo.URL != "https://www.planesnet.com" {
			userSettings.OdooURL = defaultCfg.Odoo.URL
		} else {
			userSettings.OdooURL = DefaultOdooURL
		}
	}
	if userSettings.OdooDB == "" {
		if defaultCfg.Odoo.DB != "" && !strings.EqualFold(strings.TrimSpace(defaultCfg.Odoo.DB), "pasi") {
			userSettings.OdooDB = defaultCfg.Odoo.DB
		} else if state.userStore != nil {
			userSettings.OdooDB = state.userStore.GetSharedOdooDB()
		}
	}
	if strings.EqualFold(strings.TrimSpace(userSettings.OdooDB), "pasi") || userSettings.OdooDB == "" {
		userSettings.OdooDB = DefaultOdooDB
	}
	if userSettings.PageLimit <= 0 {
		userSettings.PageLimit = 200
	}
	if userSettings.OdooToken == "" && session.Password != "" {
		userSettings.OdooToken = session.Password
	}
	if userSettings.OdooUser == "" && session.Username != "" {
		userSettings.OdooUser = session.Username
	}

	// Si el store estaba vacío pero la sesión tiene token, auto-guardar para persistir en disco
	if state.userStore != nil && userEmail != "" && userEmail != "default" && userSettings.OdooToken != "" {
		if _, exists := state.userStore.GetSettings(userEmail); !exists {
			_ = state.userStore.SaveSettings(userSettings)
			log.Printf("[SETTINGS] Ajustes re-persistidos automáticamente para %s desde la sesión", userEmail)
		}
	}

	tmpl, err := template.ParseFiles("templates/settings.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al cargar plantilla settings.html: %v", err), http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		data := SettingsPageData{
			Version:  Version,
			Config:   defaultCfg,
			Session:  session,
			Settings: userSettings,
		}
		tmpl.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		odooUser := strings.TrimSpace(r.FormValue("odoo_user"))
		odooToken := strings.TrimSpace(r.FormValue("odoo_token"))
		odooURL := strings.TrimSpace(r.FormValue("odoo_url"))
		odooDB := strings.TrimSpace(r.FormValue("odoo_db"))
		limitStr := strings.TrimSpace(r.FormValue("page_limit"))

		if odooUser == "" && !isAnonymous {
			odooUser = userEmail
		}
		if odooURL == "" || odooURL == "https://www.planesnet.com" {
			odooURL = DefaultOdooURL
		}
		if strings.EqualFold(strings.TrimSpace(odooDB), "pasi") {
			odooDB = ""
		}
		if odooDB == "" {
			if userSettings.OdooDB != "" && !strings.EqualFold(strings.TrimSpace(userSettings.OdooDB), "pasi") {
				odooDB = userSettings.OdooDB
			} else if state.userStore != nil {
				odooDB = state.userStore.GetSharedOdooDB()
			}
		}
		if strings.EqualFold(strings.TrimSpace(odooDB), "pasi") {
			odooDB = ""
		}
		limit := userSettings.PageLimit
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}

		// Actualizar en memoria la configuración por defecto de Odoo
		state.mu.Lock()
		state.cfg.Odoo.URL = odooURL
		state.cfg.Odoo.DB = odooDB
		state.mu.Unlock()

		targetEmail := userEmail
		if targetEmail == "" || targetEmail == "default" {
			if odooUser != "" {
				targetEmail = odooUser
			} else {
				targetEmail = "default"
			}
		}

		updatedSettings := store.UserSettings{
			Email:     targetEmail,
			OdooUser:  odooUser,
			OdooToken: odooToken,
			OdooURL:   odooURL,
			OdooDB:    odooDB,
			PageLimit: limit,
		}

		var errMsg string
		var successMsg string

		if state.userStore != nil {
			if err := state.userStore.SaveSettings(updatedSettings); err != nil {
				errMsg = fmt.Sprintf("Error guardando ajustes: %v", err)
			} else {
				successMsg = "Ajustes de servidor Odoo guardados correctamente."
				log.Printf("[SETTINGS] Ajustes actualizados de forma persistente para %s (%s / %s)", targetEmail, odooURL, odooDB)
			}
		} else {
			successMsg = "Ajustes actualizados en la sesión actual."
		}

		// Si el usuario proporcionó usuario y token, actualizar la sesión activa
		if odooUser != "" || !isAnonymous {
			session.Password = odooToken
			if odooUser != "" {
				session.Username = odooUser
				session.UserEmail = odooUser
			}
			session.URL = odooURL
			session.DB = odooDB
			session.AuthMethod = "local"
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    encodeSession(*session),
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 30,
				SameSite: http.SameSiteLaxMode,
			})
		}

		state.mu.RLock()
		currentCfg := state.cfg
		state.mu.RUnlock()

		data := SettingsPageData{
			Version:        Version,
			Config:         currentCfg,
			Session:        session,
			Settings:       updatedSettings,
			SuccessMessage: successMsg,
			ErrorMessage:   errMsg,
		}
		tmpl.Execute(w, data)
	}
}

// handleTestConnection prueba la conexión con Odoo y auto-guarda las credenciales si son correctas
func (state *AppState) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		OdooUser  string `json:"odoo_user"`
		OdooToken string `json:"odoo_token"`
		OdooURL   string `json:"odoo_url"`
		OdooDB    string `json:"odoo_db"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Formato JSON inválido"})
		return
	}

	if payload.OdooToken == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "El token de acceso no puede estar vacío."})
		return
	}

	if payload.OdooURL == "" || payload.OdooURL == "https://www.planesnet.com" {
		payload.OdooURL = DefaultOdooURL
	}
	if strings.EqualFold(strings.TrimSpace(payload.OdooDB), "pasi") {
		payload.OdooDB = ""
	}
	if payload.OdooDB == "" && state.userStore != nil {
		payload.OdooDB = state.userStore.GetSharedOdooDB()
	}
	if payload.OdooDB == "" {
		if !strings.EqualFold(strings.TrimSpace(state.cfg.Odoo.DB), "pasi") {
			payload.OdooDB = state.cfg.Odoo.DB
		}
	}
	if strings.EqualFold(strings.TrimSpace(payload.OdooDB), "pasi") {
		payload.OdooDB = ""
	}
	if payload.OdooDB == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "La base de datos de Odoo no puede estar vacía. Por favor, indícala en los ajustes."})
		return
	}

	testCfg := config.OdooConfig{
		URL:      payload.OdooURL,
		DB:       payload.OdooDB,
		Username: payload.OdooUser,
		Password: payload.OdooToken,
		Limit:    10,
	}

	client := odoo.NewClient(testCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	uid, err := client.Authenticate(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	serverVer, _ := client.GetServerVersion(ctx)

	// Si la autenticación tuvo éxito y hay sesión activa, auto-guardamos inmediatamente de forma persistente
	var session *SessionData
	cookie, cErr := r.Cookie(sessionCookieName)
	if cErr == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}
	if session != nil {
		userEmail := session.Username
		if session.UserEmail != "" {
			userEmail = session.UserEmail
		}
		if userEmail != "" && state.userStore != nil {
			_ = state.userStore.SaveSettings(store.UserSettings{
				Email:     userEmail,
				OdooUser:  payload.OdooUser,
				OdooToken: payload.OdooToken,
				OdooURL:   payload.OdooURL,
				OdooDB:    payload.OdooDB,
				PageLimit: 200,
			})
			log.Printf("[SETTINGS] Token auto-guardado tras prueba exitosa para %s (Odoo %s)", userEmail, serverVer)
		}
		session.Password = payload.OdooToken
		session.Username = payload.OdooUser
		session.URL = payload.OdooURL
		session.DB = payload.OdooDB
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    encodeSession(*session),
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 30,
			SameSite: http.SameSiteLaxMode,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"uid":            uid,
		"server_version": serverVer,
		"saved":          true,
	})
}
