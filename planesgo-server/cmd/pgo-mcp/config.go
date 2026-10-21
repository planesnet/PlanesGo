package main

// Configuración del proyecto (.planesgo.json), vinculación por sesión y credenciales.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config representa el archivo .planesgo.json encontrado en el proyecto o la vinculación de sesión.
// Los campos de ticket ya no se escriben, pero se respetan si un .planesgo.json antiguo los trae.
type Config struct {
	OdooProjectID            int    `json:"odoo_project_id"`
	OdooProjectName          string `json:"odoo_project_name"`
	OdooTicketID             int    `json:"odoo_ticket_id,omitempty"`
	OdooTicketRef            string `json:"odoo_ticket_ref,omitempty"`
	OdooTaskID               int    `json:"odoo_task_id,omitempty"`
	PlanesGoURL              string `json:"planesgo_url"`
	ProjectPath              string `json:"project_path,omitempty"`
	SessionID                string `json:"session_id,omitempty"`
	AutoCreateTasks          bool   `json:"auto_create_tasks,omitempty"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds,omitempty"`
	IdleTimeoutMinutes       int    `json:"idle_timeout_minutes,omitempty"`
}

// AuthConfig representa el archivo ~/.planesgo_auth.json
type AuthConfig struct {
	AntigravityToken string `json:"antigravity_token"`
	PlanesGoURL      string `json:"planesgo_url"`
	UserEmail        string `json:"user_email"`
}

// errMissingProjectPath indica que el arnés no ha pasado la raíz del proyecto de su sesión.
var errMissingProjectPath = fmt.Errorf("falta project_path: el MCP se ejecuta fuera de un proyecto; indica la raíz del proyecto de esta sesión en project_path (o --path)")

func sessionCwd() string {
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}
	return os.Getenv("PWD")
}

// isBackgroundDir indica si dir no identifica un proyecto (HOME o ~/.gemini, donde los arneses lanzan el MCP en segundo plano).
func isBackgroundDir(dir string) bool {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return false
	}
	homeDir, _ = filepath.Abs(homeDir)
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	geminiDir := filepath.Join(homeDir, ".gemini")
	return abs == homeDir || abs == geminiDir || strings.HasPrefix(abs, geminiDir+string(filepath.Separator))
}

// findConfig busca .planesgo.json estrictamente en el directorio principal del proyecto
// (prohibido buscar en directorios padres o hijos).
func findConfig(customDir ...string) (*Config, string, error) {
	var startDirs []string
	for _, d := range customDir {
		if strings.TrimSpace(d) != "" {
			startDirs = append(startDirs, strings.TrimSpace(d))
		}
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		homeDir, _ = filepath.Abs(homeDir)
	}

	if len(startDirs) == 0 {
		// Sin project_path explícito solo vale el cwd cuando el MCP se lanzó dentro de un proyecto.
		// Si corre en segundo plano (HOME o ~/.gemini) el proyecto debe venir de la sesión del arnés.
		if cwd := sessionCwd(); cwd != "" && !isBackgroundDir(cwd) {
			startDirs = append(startDirs, cwd)
		}
	}

	for _, dir := range startDirs {
		curr, err := filepath.Abs(dir)
		if err != nil {
			curr = dir
		}

		// Si curr apunta a un archivo, obtener su directorio contenedor
		if info, err := os.Stat(curr); err == nil && !info.IsDir() {
			curr = filepath.Dir(curr)
		}

		// La ubicación debe ser estrictamente la del directorio principal del proyecto.
		// En el home (~/) existe un .planesgo.json personal para antigravity cli;
		// ignorarlo si no fue solicitado explícitamente como proyecto único.
		if homeDir != "" && curr == homeDir && len(customDir) == 0 {
			continue
		}

		candidate := filepath.Join(curr, ".planesgo.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			data, err := os.ReadFile(candidate)
			if err != nil {
				return nil, candidate, err
			}
			var cfg Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, candidate, fmt.Errorf("error al parsear .planesgo.json: %w", err)
			}
			return &cfg, candidate, nil
		}
	}

	if len(startDirs) == 0 {
		return nil, "", errMissingProjectPath
	}
	return nil, "", fmt.Errorf("no se encontró .planesgo.json estrictamente en el directorio principal del proyecto")
}

const defaultSessionsDir = "/tmp/planesgo-sessions"

func sessionsDir() string {
	if d := strings.TrimSpace(os.Getenv("PLANESGO_SESSIONS_DIR")); d != "" {
		return d
	}
	return defaultSessionsDir
}

func sanitizeSessionID(sessionID string) string {
	s := strings.TrimSpace(sessionID)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := strings.Trim(b.String(), ".")
	if len(res) > 120 {
		res = res[:120]
	}
	return res
}

func sessionConfigPath(sessionID string) string {
	safe := sanitizeSessionID(sessionID)
	if safe == "" {
		return ""
	}
	return filepath.Join(sessionsDir(), safe+".json")
}

func resolveSessionID(args map[string]interface{}) string {
	if args != nil {
		if v, ok := args["session_id"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := args["ai_session_id"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if env := strings.TrimSpace(os.Getenv("PLANESGO_SESSION_ID")); env != "" {
		return env
	}
	return ""
}

// loadSessionConfig carga la configuración de proyecto específica de la sesión (Opción 1),
// permitiendo que múltiples sesiones en el mismo repositorio imputen a proyectos distintos.
func loadSessionConfig(sessionID string, projectPath string) (*Config, error) {
	p := sessionConfigPath(sessionID)
	if p == "" {
		return nil, fmt.Errorf("session_id vacío")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.OdooProjectID <= 0 && strings.TrimSpace(cfg.OdooProjectName) == "" {
		return nil, fmt.Errorf("configuración de sesión sin proyecto")
	}
	if projectPath != "" && cfg.ProjectPath != "" {
		absTarget, err1 := filepath.Abs(projectPath)
		absSaved, err2 := filepath.Abs(cfg.ProjectPath)
		if err1 == nil && err2 == nil && filepath.Clean(absTarget) != filepath.Clean(absSaved) {
			return nil, fmt.Errorf("la sesión está vinculada a otro directorio (%s)", cfg.ProjectPath)
		}
	}
	return &cfg, nil
}

// saveSessionConfig guarda la configuración de proyecto vinculada a una sesión específica (Opción 1).
func saveSessionConfig(sessionID string, projectPath string, cfg Config) error {
	p := sessionConfigPath(sessionID)
	if p == "" {
		return fmt.Errorf("session_id vacío")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	if projectPath != "" {
		if abs, err := filepath.Abs(projectPath); err == nil {
			cfg.ProjectPath = abs
		} else {
			cfg.ProjectPath = projectPath
		}
	}
	cfg.SessionID = strings.TrimSpace(sessionID)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0644)
}

// getAuth obtiene el token y la URL de PlanesGo
func getAuth(cfg *Config) (token string, apiURL string, err error) {
	apiURL = DefaultServer
	if envURL := strings.TrimSpace(os.Getenv("PLANESGO_URL")); envURL != "" {
		apiURL = envURL
	} else if cfg != nil && cfg.PlanesGoURL != "" {
		apiURL = cfg.PlanesGoURL
	}

	// PLANESGO_TOKEN es el nombre vigente; ANTIGRAVITY_TOKEN se mantiene por compatibilidad
	token = strings.TrimSpace(os.Getenv("PLANESGO_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ANTIGRAVITY_TOKEN"))
	}
	if token != "" {
		return token, cleanURL(apiURL), nil
	}

	home, err := os.UserHomeDir()
	if err == nil {
		authPath := filepath.Join(home, ".planesgo_auth.json")
		if data, err := os.ReadFile(authPath); err == nil {
			var auth AuthConfig
			if err := json.Unmarshal(data, &auth); err == nil {
				if auth.AntigravityToken != "" {
					token = auth.AntigravityToken
				}
				if auth.PlanesGoURL != "" && (cfg == nil || cfg.PlanesGoURL == "") {
					apiURL = auth.PlanesGoURL
				}
			}
		}
	}

	if token == "" {
		return "", cleanURL(apiURL), fmt.Errorf("token de autenticación no encontrado en $PLANESGO_TOKEN ni en ~/.planesgo_auth.json")
	}

	return token, cleanURL(apiURL), nil
}

func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.Contains(u, "localhost") || strings.Contains(u, "8089") {
		return DefaultServer
	}
	return strings.TrimRight(u, "/")
}

// saveConfigFile guarda la configuración estrictamente en .planesgo.json en la raíz del proyecto
func saveConfigFile(cfg Config, targetDir string) error {
	if targetDir == "" {
		cwd := sessionCwd()
		if cwd == "" || isBackgroundDir(cwd) {
			return errMissingProjectPath
		}
		targetDir = cwd
	}
	targetDir, _ = filepath.Abs(targetDir)

	data, mErr := json.MarshalIndent(cfg, "", "  ")
	if mErr != nil {
		return mErr
	}
	data = append(data, '\n')

	// La ubicación de .planesgo.json debe ser estrictamente la del directorio principal
	// asociada al proyecto. Prohibido escribir en directorios padres o hijos (custom, etc.).
	mainFilePath := filepath.Join(targetDir, ".planesgo.json")
	if err := os.WriteFile(mainFilePath, data, 0644); err != nil {
		return err
	}
	return nil
}
