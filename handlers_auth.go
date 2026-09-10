package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pasigo/auth"
	"pasigo/config"
	"pasigo/odoo"
	"pasigo/store"
)

// handleGoogleAuth inicia el flujo de autenticación con Google OAuth 2.0
func (state *AppState) handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	clientID, clientSecret := state.cfg.GetGoogleAuthCredentials()
	if clientID == "" || clientSecret == "" {
		http.Error(w, "Google OAuth no configurado. Faltan las credenciales GOOGLE_CLIENT_ID y GOOGLE_CLIENT_SECRET.", http.StatusBadRequest)
		return
	}

	googleCfg := state.cfg.GoogleAuth
	googleCfg.ClientID = clientID
	googleCfg.ClientSecret = clientSecret
	googleService := auth.NewGoogleOAuthService(googleCfg)

	redirectURI := auth.CalculateRedirectURI(r)
	stateToken := auth.GenerateStateToken()

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    stateToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})

	authURL := googleService.GetAuthURL(stateToken, redirectURI)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleGoogleCallback procesa el retorno del consentimiento de Google OAuth 2.0
func (state *AppState) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Estado de seguridad no encontrado o sesión expirada."), http.StatusSeeOther)
		return
	}

	queryState := r.URL.Query().Get("state")
	if queryState == "" || queryState != stateCookie.Value {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Validación de seguridad OAuth inválida (CSRF detectado)."), http.StatusSeeOther)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("No se recibió código de autorización de Google."), http.StatusSeeOther)
		return
	}

	clientID, clientSecret := state.cfg.GetGoogleAuthCredentials()
	googleCfg := state.cfg.GoogleAuth
	googleCfg.ClientID = clientID
	googleCfg.ClientSecret = clientSecret
	googleService := auth.NewGoogleOAuthService(googleCfg)

	redirectURI := auth.CalculateRedirectURI(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tokenResp, err := googleService.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		log.Printf("[OAUTH] Error al intercambiar código: %v", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("No se pudo intercambiar el token con Google: "+err.Error()), http.StatusSeeOther)
		return
	}

	googleUser, err := googleService.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		log.Printf("[OAUTH] Error al obtener info de usuario: %v", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("No se pudo obtener información de Google: "+err.Error()), http.StatusSeeOther)
		return
	}

	allowedDomain := state.cfg.GoogleAuth.AllowedDomain
	if allowedDomain != "" && !strings.HasSuffix(strings.ToLower(googleUser.Email), "@"+strings.ToLower(allowedDomain)) {
		log.Printf("[OAUTH] Dominio no autorizado: %s (requerido: %s)", googleUser.Email, allowedDomain)
		http.Redirect(w, r, "/login?error="+url.QueryEscape(fmt.Sprintf("Acceso restringido a cuentas del dominio @%s", allowedDomain)), http.StatusSeeOther)
		return
	}

	log.Printf("[OAUTH] Autenticación Google exitosa para: %s (%s)", googleUser.Email, googleUser.Name)

	// Recuperar ajustes persistentes previos del usuario si existen
	var savedToken string
	var savedDB string
	if state.userStore != nil {
		if uSettings, ok := state.userStore.GetSettings(googleUser.Email); ok {
			savedToken = uSettings.OdooToken
			if !strings.EqualFold(strings.TrimSpace(uSettings.OdooDB), "pasi") {
				savedDB = uSettings.OdooDB
			}
		}
		if savedDB == "" {
			savedDB = state.userStore.GetSharedOdooDB()
		}
	}
	if savedDB == "" && state.cfg.Odoo.DB != "" && !strings.EqualFold(strings.TrimSpace(state.cfg.Odoo.DB), "pasi") {
		savedDB = state.cfg.Odoo.DB
	}
	if strings.EqualFold(strings.TrimSpace(savedDB), "pasi") || savedDB == "" {
		savedDB = DefaultOdooDB
	}

	sess := SessionData{
		URL:         DefaultOdooURL,
		DB:          savedDB,
		Username:    googleUser.Email,
		Password:    savedToken,
		AuthMethod:  "google",
		UserEmail:   googleUser.Email,
		UserName:    googleUser.Name,
		UserPicture: googleUser.Picture,
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encodeSession(sess),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400 * 30,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogin muestra y procesa el inicio de sesión manual en Odoo o acceso vía Google
func (state *AppState) handleLogin(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/login.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error cargando login.html: %v", err), http.StatusInternalServerError)
		return
	}

	state.mu.RLock()
	defaultCfg := state.cfg
	state.mu.RUnlock()

	isGoogleConfigured := defaultCfg.IsGoogleConfigured()
	var googleConfigError string
	if !isGoogleConfigured {
		googleConfigError = "Configuración OAuth no válida. No se han encontrado las variables de entorno GOOGLE_CLIENT_ID ni GOOGLE_CLIENT_SECRET en el sistema ni en .env."
	}

	loginDB := ""
	if state.userStore != nil {
		loginDB = state.userStore.GetSharedOdooDB()
	}
	if loginDB == "" && defaultCfg.Odoo.DB != "" && !strings.EqualFold(strings.TrimSpace(defaultCfg.Odoo.DB), "pasi") {
		loginDB = defaultCfg.Odoo.DB
	}
	if strings.EqualFold(strings.TrimSpace(loginDB), "pasi") || loginDB == "" {
		loginDB = DefaultOdooDB
	}

	if r.Method == http.MethodGet {
		errorQuery := r.URL.Query().Get("error")
		data := LoginPageData{
			Version:           Version,
			URL:               DefaultOdooURL,
			DB:                loginDB,
			Username:          defaultCfg.Odoo.Username,
			Password:          "",
			GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
			GoogleConfigured:  isGoogleConfigured,
			GoogleConfigError: googleConfigError,
			Error:             errorQuery,
		}
		tmpl.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		usernameInput := strings.TrimSpace(r.FormValue("username"))
		passwordInput := r.FormValue("password")

		userDB := loginDB
		if state.userStore != nil {
			if uSettings, ok := state.userStore.GetSettings(usernameInput); ok && uSettings.OdooDB != "" && !strings.EqualFold(strings.TrimSpace(uSettings.OdooDB), "pasi") {
				userDB = uSettings.OdooDB
			}
		}
		if strings.EqualFold(strings.TrimSpace(userDB), "pasi") {
			userDB = ""
		}

		if usernameInput == "" || passwordInput == "" {
			data := LoginPageData{
				Version:           Version,
				URL:               DefaultOdooURL,
				DB:                userDB,
				Username:          usernameInput,
				Password:          passwordInput,
				GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
				GoogleConfigured:  isGoogleConfigured,
				GoogleConfigError: googleConfigError,
				Error:             "Por favor, introduce usuario y contraseña de Odoo.",
			}
			tmpl.Execute(w, data)
			return
		}

		testCfg := config.OdooConfig{
			URL:      DefaultOdooURL,
			DB:       userDB,
			Username: usernameInput,
			Password: passwordInput,
			Limit:    200,
		}

		client := odoo.NewClient(testCfg)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		uid, authErr := client.Authenticate(ctx)
		if authErr != nil {
			log.Printf("[AUTH] Error de login para %s: %v", usernameInput, authErr)
			data := LoginPageData{
				Version:           Version,
				URL:               DefaultOdooURL,
				DB:                userDB,
				Username:          usernameInput,
				Password:          passwordInput,
				GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
				GoogleConfigured:  isGoogleConfigured,
				GoogleConfigError: googleConfigError,
				Error:             fmt.Sprintf("No se pudo iniciar sesión en Odoo: %v", authErr),
			}
			tmpl.Execute(w, data)
			return
		}

		log.Printf("[AUTH] Inicio de sesión exitoso en Odoo para %s (UID: %d)", usernameInput, uid)

		// Guardar también en almacén de usuario persistente
		if state.userStore != nil {
			existing, _ := state.userStore.GetSettings(usernameInput)
			dbToSave := userDB
			if existing.OdooDB != "" && !strings.EqualFold(strings.TrimSpace(existing.OdooDB), "pasi") {
				dbToSave = existing.OdooDB
			}
			if strings.EqualFold(strings.TrimSpace(dbToSave), "pasi") {
				dbToSave = ""
			}
			_ = state.userStore.SaveSettings(store.UserSettings{
				Email:     usernameInput,
				OdooUser:  usernameInput,
				OdooToken: passwordInput,
				OdooURL:   DefaultOdooURL,
				OdooDB:    dbToSave,
				PageLimit: 200,
			})
		}

		sess := SessionData{
			URL:        DefaultOdooURL,
			DB:         userDB,
			Username:   usernameInput,
			Password:   passwordInput,
			AuthMethod: "odoo",
			UserName:   usernameInput,
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    encodeSession(sess),
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 30,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// handleLogout destruye la cookie de sesión y redirige al login
func (state *AppState) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
