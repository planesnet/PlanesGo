package odoo

import (
	"os"
	"path/filepath"
	"testing"

	"pasigo/cmd/migrar1/db"
)

func TestCleanVATAndNormalize(t *testing.T) {
	if CleanVAT(" ES-B12345678 ") != "ESB12345678" {
		t.Errorf("CleanVAT falló: %s", CleanVAT(" ES-B12345678 "))
	}
	if NormalizeString("  Ángel Gómez Pérez  ") != "angel gomez perez" {
		t.Errorf("NormalizeString falló: %s", NormalizeString("  Ángel Gómez Pérez  "))
	}
}

func TestResolvePartnerCascade(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "mapper_test_*")
	defer os.RemoveAll(tempDir)
	database, _ := db.Open(filepath.Join(tempDir, "test.db"))
	defer database.Close()

	registry := NewMasterRegistry(database)
	// Simulamos partners de destino
	registry.targetPartnersByID[10] = Partner{ID: 10, Name: "Cliente Alfa S.L.", VAT: "B99887766", Email: "info@alfa.es"}
	registry.targetPartnersByVAT["B99887766"] = registry.targetPartnersByID[10]
	registry.targetPartnersByEmail["info@alfa.es"] = registry.targetPartnersByID[10]
	registry.targetPartnersByName["cliente alfa s.l."] = registry.targetPartnersByID[10]

	// 1. Coincidencia por CIF
	targetID, targetName, match, err := registry.ResolvePartner(Partner{
		ID:   1,
		Name: "Alfa Diferente",
		VAT:  "B-99.88.77-66",
	})
	if err != nil || targetID != 10 || match != "vat" {
		t.Errorf("Fallo en resolución por VAT: ID=%d, match=%s, err=%v", targetID, match, err)
	}
	_ = targetName

	// 2. Coincidencia por Email
	targetID2, _, match2, err2 := registry.ResolvePartner(Partner{
		ID:    2,
		Name:  "Sin CIF",
		Email: "info@alfa.es",
	})
	if err2 != nil || targetID2 != 10 || match2 != "email" {
		t.Errorf("Fallo en resolución por Email: ID=%d, match=%s, err=%v", targetID2, match2, err2)
	}

	// 3. No coincidencia (estrictamente no crea nada)
	targetID3, _, match3, err3 := registry.ResolvePartner(Partner{
		ID:    3,
		Name:  "Empresa Desconocida X",
		VAT:   "A00000000",
		Email: "noexiste@test.es",
	})
	if err3 == nil || targetID3 != 0 || match3 != "unresolved" {
		t.Errorf("Debería haber fallado sin crear nada: ID=%d, match=%s, err=%v", targetID3, match3, err3)
	}
}
