package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// UserSettings representa los ajustes individuales de un usuario.
type UserSettings struct {
	Email       string    `json:"email"`
	OdooUser    string    `json:"odoo_user"`
	OdooToken   string    `json:"odoo_token"` // Contraseña o API Key de Odoo
	OdooURL     string    `json:"odoo_url"`
	OdooDB      string    `json:"odoo_db"`
	PageLimit   int       `json:"page_limit"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserSettingsStore maneja el almacenamiento persistente y concurrente de ajustes por usuario.
type UserSettingsStore struct {
	mu       sync.RWMutex
	filePath string
	settings map[string]UserSettings
}

// NewUserSettingsStore inicializa el almacén de configuración de usuarios desde un archivo JSON.
func NewUserSettingsStore(filePath string) (*UserSettingsStore, error) {
	if filePath == "" {
		filePath = filepath.Join("data", "user_settings.json")
	}

	store := &UserSettingsStore{
		filePath: filePath,
		settings: make(map[string]UserSettings),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *UserSettingsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("error leyendo almacén de usuarios %s: %w", s.filePath, err)
	}

	if len(data) == 0 {
		return nil
	}

	var loaded map[string]UserSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("error parseando JSON de usuarios: %w", err)
	}

	s.settings = loaded
	return nil
}

func (s *UserSettingsStore) save() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creando directorio de datos %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando ajustes: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf("error escribiendo archivo de ajustes %s: %w", s.filePath, err)
	}

	return nil
}

// GetSettings obtiene los ajustes correspondientes al email del usuario.
func (s *UserSettingsStore) GetSettings(email string) (UserSettings, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(email))
	settings, ok := s.settings[key]
	return settings, ok
}

// SaveSettings guarda o actualiza los ajustes de un usuario en memoria y en disco.
func (s *UserSettingsStore) SaveSettings(settings UserSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(settings.Email))
	if key == "" {
		return fmt.Errorf("el email del usuario no puede estar vacío")
	}

	settings.UpdatedAt = time.Now()
	s.settings[key] = settings

	return s.save()
}
