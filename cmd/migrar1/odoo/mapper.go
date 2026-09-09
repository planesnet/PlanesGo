package odoo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"pasigo/cmd/migrar1/db"
	"regexp"
	"strings"
	"sync"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

// CleanVAT normaliza un CIF/NIF/NIE (quita espacios, guiones, puntos y mayúsculas)
func CleanVAT(vat string) string {
	cleaned := strings.ToUpper(strings.TrimSpace(vat))
	cleaned = nonAlphanumericRegex.ReplaceAllString(cleaned, "")
	// Si empieza por prefijo ES o similar
	return cleaned
}

func NormalizeString(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "á", "a")
	s = strings.ReplaceAll(s, "é", "e")
	s = strings.ReplaceAll(s, "í", "i")
	s = strings.ReplaceAll(s, "ó", "o")
	s = strings.ReplaceAll(s, "ú", "u")
	s = strings.ReplaceAll(s, "ü", "u")
	return s
}

type ExternalMappingsFile struct {
	Partners map[string]int `json:"partners"` // source_id_or_key -> target_id
	Users    map[string]int `json:"users"`
	Projects map[string]int `json:"projects"`
}

type MasterRegistry struct {
	mu sync.RWMutex
	db *db.DB

	// Cachés de destino
	targetPartnersByVAT   map[string]Partner
	targetPartnersByEmail map[string]Partner
	targetPartnersByName  map[string]Partner
	targetPartnersByID    map[int]Partner

	targetUsersByLogin map[string]User
	targetUsersByEmail map[string]User
	targetUsersByName  map[string]User
	targetUsersByID    map[int]User

	targetEmployeesByEmail map[string]Employee
	targetEmployeesByName  map[string]Employee
	targetEmployeesByID    map[int]Employee

	targetProjectsByName map[string]Project
	targetProjectsByID   map[int]Project

	// Mapeos manuales desde fichero
	fileOverrides ExternalMappingsFile
}

func NewMasterRegistry(database *db.DB) *MasterRegistry {
	return &MasterRegistry{
		db:                     database,
		targetPartnersByVAT:   make(map[string]Partner),
		targetPartnersByEmail: make(map[string]Partner),
		targetPartnersByName:  make(map[string]Partner),
		targetPartnersByID:    make(map[int]Partner),

		targetUsersByLogin: make(map[string]User),
		targetUsersByEmail: make(map[string]User),
		targetUsersByName:  make(map[string]User),
		targetUsersByID:    make(map[int]User),

		targetEmployeesByEmail: make(map[string]Employee),
		targetEmployeesByName:  make(map[string]Employee),
		targetEmployeesByID:    make(map[int]Employee),

		targetProjectsByName: make(map[string]Project),
		targetProjectsByID:   make(map[int]Project),

		fileOverrides: ExternalMappingsFile{
			Partners: make(map[string]int),
			Users:    make(map[string]int),
			Projects: make(map[string]int),
		},
	}
}

// LoadTargetMasters carga en memoria todos los maestros de destino para resolución ultrarrápida
func (m *MasterRegistry) LoadTargetMasters(ctx context.Context, targetClient *Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Cargar Partners de Destino
	partnerRaw, err := targetClient.SearchRead(ctx, "res.partner", nil, []string{"id", "name", "vat", "email", "phone", "is_company", "active"}, 0, 0, "id asc")
	if err != nil {
		return fmt.Errorf("error cargando partners de destino: %w", err)
	}
	var partners []Partner
	if err := json.Unmarshal(partnerRaw, &partners); err == nil {
		for _, p := range partners {
			m.targetPartnersByID[p.ID] = p
			if p.VAT != "" {
				m.targetPartnersByVAT[CleanVAT(p.VAT)] = p
			}
			if p.Email != "" {
				m.targetPartnersByEmail[strings.ToLower(strings.TrimSpace(p.Email))] = p
			}
			if p.Name != "" {
				m.targetPartnersByName[NormalizeString(p.Name)] = p
			}
		}
	}

	// 2. Cargar Usuarios de Destino
	userRaw, err := targetClient.SearchRead(ctx, "res.users", nil, []string{"id", "name", "login", "email", "active"}, 0, 0, "id asc")
	if err != nil {
		return fmt.Errorf("error cargando usuarios de destino: %w", err)
	}
	var users []User
	if err := json.Unmarshal(userRaw, &users); err == nil {
		for _, u := range users {
			m.targetUsersByID[u.ID] = u
			if u.Login != "" {
				m.targetUsersByLogin[strings.ToLower(strings.TrimSpace(u.Login))] = u
			}
			if u.Email != "" {
				m.targetUsersByEmail[strings.ToLower(strings.TrimSpace(u.Email))] = u
			}
			if u.Name != "" {
				m.targetUsersByName[NormalizeString(u.Name)] = u
			}
		}
	}

	// 3. Cargar Empleados de Destino (si existe hr.employee)
	empRaw, err := targetClient.SearchRead(ctx, "hr.employee", nil, []string{"id", "name", "work_email", "user_id", "active"}, 0, 0, "id asc")
	if err == nil {
		var emps []Employee
		if err := json.Unmarshal(empRaw, &emps); err == nil {
			for _, e := range emps {
				m.targetEmployeesByID[e.ID] = e
				if e.WorkEmail != "" {
					m.targetEmployeesByEmail[strings.ToLower(strings.TrimSpace(e.WorkEmail))] = e
				}
				if e.Name != "" {
					m.targetEmployeesByName[NormalizeString(e.Name)] = e
				}
			}
		}
	}

	// 4. Cargar Proyectos de Destino
	projRaw, err := targetClient.SearchRead(ctx, "project.project", nil, []string{"id", "name", "display_name", "active"}, 0, 0, "id asc")
	if err != nil {
		return fmt.Errorf("error cargando proyectos de destino: %w", err)
	}
	var projs []Project
	if err := json.Unmarshal(projRaw, &projs); err == nil {
		for _, pr := range projs {
			m.targetProjectsByID[pr.ID] = pr
			if pr.Name != "" {
				m.targetProjectsByName[NormalizeString(pr.Name)] = pr
			}
			if pr.DisplayName != "" {
				m.targetProjectsByName[NormalizeString(pr.DisplayName)] = pr
			}
		}
	}

	return nil
}

// LoadFileOverrides carga o importa mappings.json si existe
func (m *MasterRegistry) LoadFileOverrides(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if path == "" {
		path = "mappings.json"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var fileMappings ExternalMappingsFile
	if err := json.Unmarshal(data, &fileMappings); err != nil {
		return fmt.Errorf("error parseando %s: %w", path, err)
	}

	m.fileOverrides = fileMappings
	return nil
}

// SaveFileOverrides guarda los mapeos actuales a mappings.json
func (m *MasterRegistry) SaveFileOverrides(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if path == "" {
		path = "mappings.json"
	}

	data, err := json.MarshalIndent(m.fileOverrides, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ResolvePartner busca o mapea un Partner estrictamente sin crear ninguno en destino
func (m *MasterRegistry) ResolvePartner(srcPartner Partner) (int, string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if srcPartner.ID == 0 {
		return 0, "", "none", nil
	}

	// 1. Comprobar override en fichero
	srcIDStr := fmt.Sprintf("%d", srcPartner.ID)
	if targetID, ok := m.fileOverrides.Partners[srcIDStr]; ok && targetID > 0 {
		if target, exists := m.targetPartnersByID[targetID]; exists {
			return targetID, target.Name, "file_override", nil
		}
	}

	// 2. Comprobar base de datos local SQLite
	if existing, err := m.db.GetMasterMapping("res.partner", srcPartner.ID); err == nil && existing != nil {
		if existing.Status == "resolved" && existing.TargetID > 0 {
			return existing.TargetID, existing.TargetName, existing.MatchCriterion, nil
		}
	}

	// 3. Cascada de coincidencia automática
	// A) Coincidencia por CIF/NIF/VAT
	if srcPartner.VAT != "" {
		cleaned := CleanVAT(srcPartner.VAT)
		if cleaned != "" {
			if target, ok := m.targetPartnersByVAT[cleaned]; ok {
				_ = m.db.SaveMasterMapping(&db.MasterMapping{
					EntityType:     "res.partner",
					SourceID:       srcPartner.ID,
					SourceName:     srcPartner.Name,
					SourceKey:      srcPartner.VAT,
					TargetID:       target.ID,
					TargetName:     target.Name,
					MatchCriterion: "vat",
					Status:         "resolved",
				})
				return target.ID, target.Name, "vat", nil
			}
		}
	}

	// B) Coincidencia por Email
	if srcPartner.Email != "" {
		cleanedEmail := strings.ToLower(strings.TrimSpace(srcPartner.Email))
		if target, ok := m.targetPartnersByEmail[cleanedEmail]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "res.partner",
				SourceID:       srcPartner.ID,
				SourceName:     srcPartner.Name,
				SourceKey:      srcPartner.Email,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "email",
				Status:         "resolved",
			})
			return target.ID, target.Name, "email", nil
		}
	}

	// C) Coincidencia por Nombre Exacto / Normalizado
	if srcPartner.Name != "" {
		normName := NormalizeString(srcPartner.Name)
		if target, ok := m.targetPartnersByName[normName]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "res.partner",
				SourceID:       srcPartner.ID,
				SourceName:     srcPartner.Name,
				SourceKey:      srcPartner.Name,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "name",
				Status:         "resolved",
			})
			return target.ID, target.Name, "name", nil
		}
	}

	// No encontrado -> NO CREAR. Registrar como pendiente
	_ = m.db.SaveMasterMapping(&db.MasterMapping{
		EntityType:     "res.partner",
		SourceID:       srcPartner.ID,
		SourceName:     srcPartner.Name,
		SourceKey:      fmt.Sprintf("VAT:%s | Email:%s", srcPartner.VAT, srcPartner.Email),
		TargetID:       0,
		TargetName:     "",
		MatchCriterion: "unresolved",
		Status:         "pending",
	})

	return 0, "", "unresolved", fmt.Errorf("partner '%s' (ID %d) no encontrado en destino", srcPartner.Name, srcPartner.ID)
}

// ResolveUser busca un Usuario/Trabajador en destino sin crearlo
func (m *MasterRegistry) ResolveUser(srcUser User) (int, string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if srcUser.ID == 0 {
		return 0, "", "none", nil
	}

	// 1. Override fichero
	srcIDStr := fmt.Sprintf("%d", srcUser.ID)
	if targetID, ok := m.fileOverrides.Users[srcIDStr]; ok && targetID > 0 {
		if target, exists := m.targetUsersByID[targetID]; exists {
			return targetID, target.Name, "file_override", nil
		}
	}

	// 2. Base de datos SQLite
	if existing, err := m.db.GetMasterMapping("res.users", srcUser.ID); err == nil && existing != nil {
		if existing.Status == "resolved" && existing.TargetID > 0 {
			return existing.TargetID, existing.TargetName, existing.MatchCriterion, nil
		}
	}

	// 3. Login match
	if srcUser.Login != "" {
		cleanedLogin := strings.ToLower(strings.TrimSpace(srcUser.Login))
		if target, ok := m.targetUsersByLogin[cleanedLogin]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "res.users",
				SourceID:       srcUser.ID,
				SourceName:     srcUser.Name,
				SourceKey:      srcUser.Login,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "login",
				Status:         "resolved",
			})
			return target.ID, target.Name, "login", nil
		}
	}

	// 4. Email match
	if srcUser.Email != "" {
		cleanedEmail := strings.ToLower(strings.TrimSpace(srcUser.Email))
		if target, ok := m.targetUsersByEmail[cleanedEmail]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "res.users",
				SourceID:       srcUser.ID,
				SourceName:     srcUser.Name,
				SourceKey:      srcUser.Email,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "email",
				Status:         "resolved",
			})
			return target.ID, target.Name, "email", nil
		}
	}

	// 5. Name match
	if srcUser.Name != "" {
		normName := NormalizeString(srcUser.Name)
		if target, ok := m.targetUsersByName[normName]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "res.users",
				SourceID:       srcUser.ID,
				SourceName:     srcUser.Name,
				SourceKey:      srcUser.Name,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "name",
				Status:         "resolved",
			})
			return target.ID, target.Name, "name", nil
		}
	}

	// No encontrado -> NO CREAR
	_ = m.db.SaveMasterMapping(&db.MasterMapping{
		EntityType:     "res.users",
		SourceID:       srcUser.ID,
		SourceName:     srcUser.Name,
		SourceKey:      srcUser.Login,
		TargetID:       0,
		TargetName:     "",
		MatchCriterion: "unresolved",
		Status:         "pending",
	})

	return 0, "", "unresolved", fmt.Errorf("usuario '%s' (ID %d) no encontrado en destino", srcUser.Name, srcUser.ID)
}

// ResolveProject busca un Proyecto existente en destino sin crear ninguno
func (m *MasterRegistry) ResolveProject(srcProject Project) (int, string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if srcProject.ID == 0 {
		return 0, "", "none", nil
	}

	// 1. Override fichero
	srcIDStr := fmt.Sprintf("%d", srcProject.ID)
	if targetID, ok := m.fileOverrides.Projects[srcIDStr]; ok && targetID > 0 {
		if target, exists := m.targetProjectsByID[targetID]; exists {
			return targetID, target.Name, "file_override", nil
		}
	}

	// 2. Base de datos SQLite
	if existing, err := m.db.GetMasterMapping("project.project", srcProject.ID); err == nil && existing != nil {
		if existing.Status == "resolved" && existing.TargetID > 0 {
			return existing.TargetID, existing.TargetName, existing.MatchCriterion, nil
		}
	}

	// 3. Name match
	if srcProject.Name != "" {
		normName := NormalizeString(srcProject.Name)
		if target, ok := m.targetProjectsByName[normName]; ok {
			_ = m.db.SaveMasterMapping(&db.MasterMapping{
				EntityType:     "project.project",
				SourceID:       srcProject.ID,
				SourceName:     srcProject.Name,
				SourceKey:      srcProject.Name,
				TargetID:       target.ID,
				TargetName:     target.Name,
				MatchCriterion: "name",
				Status:         "resolved",
			})
			return target.ID, target.Name, "name", nil
		}
	}

	// No encontrado -> NO CREAR
	_ = m.db.SaveMasterMapping(&db.MasterMapping{
		EntityType:     "project.project",
		SourceID:       srcProject.ID,
		SourceName:     srcProject.Name,
		SourceKey:      srcProject.Name,
		TargetID:       0,
		TargetName:     "",
		MatchCriterion: "unresolved",
		Status:         "pending",
	})

	return 0, "", "unresolved", fmt.Errorf("proyecto '%s' (ID %d) no encontrado en destino", srcProject.Name, srcProject.ID)
}
