package gcpbilling

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 1. Test: Parseo de una respuesta de jobs.query de ejemplo
func TestParseQueryRows(t *testing.T) {
	sampleJSON := `{
		"kind": "bigquery#queryResponse",
		"schema": {
			"fields": [
				{"name": "billing_account_id", "type": "STRING"},
				{"name": "invoice_month", "type": "STRING"},
				{"name": "currency", "type": "STRING"},
				{"name": "cost", "type": "FLOAT"},
				{"name": "credits", "type": "FLOAT"},
				{"name": "tax", "type": "FLOAT"},
				{"name": "projects", "type": "INTEGER"}
			]
		},
		"jobComplete": true,
		"totalRows": "2",
		"rows": [
			{
				"f": [
					{"v": "012345-ABCDEF-678901"},
					{"v": "202609"},
					{"v": "EUR"},
					{"v": "100.50"},
					{"v": "-10.50"},
					{"v": "21.00"},
					{"v": "4"}
				]
			},
			{
				"f": [
					{"v": "987654-FEDCBA-109876"},
					{"v": "202608"},
					{"v": "USD"},
					{"v": "250.00"},
					{"v": "0.00"},
					{"v": "0.00"},
					{"v": "2"}
				]
			}
		]
	}`

	var bqResp BQQueryResponse
	if err := json.Unmarshal([]byte(sampleJSON), &bqResp); err != nil {
		t.Fatalf("error deserializando respuesta JSON de BigQuery: %v", err)
	}

	rows, err := ParseQueryRows(bqResp.Rows)
	if err != nil {
		t.Fatalf("error en ParseQueryRows: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("se esperaban 2 filas procesadas, obtenidas: %d", len(rows))
	}

	// Comprobar fila 1
	r0 := rows[0]
	if r0.BillingAccountID != "012345-ABCDEF-678901" {
		t.Errorf("billing_account_id inesperado: %s", r0.BillingAccountID)
	}
	if r0.InvoiceMonthRaw != "202609" {
		t.Errorf("invoice_month inesperado: %s", r0.InvoiceMonthRaw)
	}
	if r0.Currency != "EUR" {
		t.Errorf("currency inesperada: %s", r0.Currency)
	}
	if r0.Cost != 100.50 {
		t.Errorf("cost inesperado: %f", r0.Cost)
	}
	if r0.Credits != -10.50 {
		t.Errorf("credits inesperado: %f", r0.Credits)
	}
	if r0.Tax != 21.00 {
		t.Errorf("tax inesperado: %f", r0.Tax)
	}
	// Total = cost + credits = 100.50 + (-10.50) = 90.00
	if r0.Total != 90.00 {
		t.Errorf("total inesperado: %f (se esperaba 90.00)", r0.Total)
	}
	if r0.Projects != 4 {
		t.Errorf("projects inesperado: %d", r0.Projects)
	}
}

// 2. Test: Agrupación año/mes y orden descendente
func TestGroupInvoices(t *testing.T) {
	rows := []RawInvoiceRow{
		{
			BillingAccountID: "ACC-1",
			InvoiceMonthRaw:  "202511",
			Currency:         "EUR",
			Cost:             50.00,
			Credits:          0.00,
			Total:            50.00,
			Projects:         1,
		},
		{
			BillingAccountID: "ACC-1",
			InvoiceMonthRaw:  "202609",
			Currency:         "EUR",
			Cost:             100.00,
			Credits:          -10.00,
			Total:            90.00,
			Projects:         2,
		},
		{
			BillingAccountID: "ACC-2",
			InvoiceMonthRaw:  "202609",
			Currency:         "EUR",
			Cost:             200.00,
			Credits:          -20.00,
			Total:            180.00,
			Projects:         3,
		},
		{
			BillingAccountID: "ACC-1",
			InvoiceMonthRaw:  "202610",
			Currency:         "EUR",
			Cost:             300.00,
			Credits:          0.00,
			Total:            300.00,
			Projects:         5,
		},
		{
			BillingAccountID: "ACC-1",
			InvoiceMonthRaw:  "202512",
			Currency:         "EUR",
			Cost:             75.00,
			Credits:          0.00,
			Total:            75.00,
			Projects:         2,
		},
	}

	years, err := GroupInvoices(rows)
	if err != nil {
		t.Fatalf("error agrupando facturas: %v", err)
	}

	if len(years) != 2 {
		t.Fatalf("se esperaban 2 años, obtenidos: %d", len(years))
	}

	// Orden descendente por año: 2026 primero, luego 2025
	if years[0].Year != 2026 {
		t.Errorf("el primer año debería ser 2026, obtenido: %d", years[0].Year)
	}
	if years[1].Year != 2025 {
		t.Errorf("el segundo año debería ser 2025, obtenido: %d", years[1].Year)
	}

	// Totales de 2026: 90 + 180 + 300 = 570 EUR
	if years[0].TotalByCurrency["EUR"] != 570.00 {
		t.Errorf("total EUR de 2026 esperado 570.00, obtenido: %f", years[0].TotalByCurrency["EUR"])
	}

	// Meses en 2026 ordenados descendente: 2026-10 antes que 2026-09
	if len(years[0].Months) != 2 {
		t.Fatalf("se esperaban 2 meses en 2026, obtenidos: %d", len(years[0].Months))
	}
	if years[0].Months[0].InvoiceMonth != "2026-10" {
		t.Errorf("primer mes de 2026 debería ser 2026-10, obtenido: %s", years[0].Months[0].InvoiceMonth)
	}
	if years[0].Months[1].InvoiceMonth != "2026-09" {
		t.Errorf("segundo mes de 2026 debería ser 2026-09, obtenido: %s", years[0].Months[1].InvoiceMonth)
	}

	// El mes 2026-09 debe contener 2 facturas (ACC-1 y ACC-2) ordenadas
	sept := years[0].Months[1]
	if len(sept.Invoices) != 2 {
		t.Fatalf("se esperaban 2 facturas en 2026-09, obtenidas: %d", len(sept.Invoices))
	}
	if sept.Invoices[0].BillingAccountID != "ACC-1" || sept.Invoices[1].BillingAccountID != "ACC-2" {
		t.Errorf("facturas no ordenadas correctamente por billing_account_id: %v", sept.Invoices)
	}
}

// 3. Test: Cálculo de la fecha de emisión, incluido el paso de diciembre a enero
func TestEstimateIssueDate(t *testing.T) {
	tests := []struct {
		inputRaw      string
		expectedMonth string
		expectedLabel string
		expectedIssue string
		expectedYear  int
	}{
		{
			inputRaw:      "202609",
			expectedMonth: "2026-09",
			expectedLabel: "Septiembre 2026",
			expectedIssue: "2026-10-01",
			expectedYear:  2026,
		},
		{
			inputRaw:      "2026-09",
			expectedMonth: "2026-09",
			expectedLabel: "Septiembre 2026",
			expectedIssue: "2026-10-01",
			expectedYear:  2026,
		},
		{
			inputRaw:      "202612", // Paso crítico de Diciembre a Enero
			expectedMonth: "2026-12",
			expectedLabel: "Diciembre 2026",
			expectedIssue: "2027-01-01",
			expectedYear:  2026,
		},
		{
			inputRaw:      "202501",
			expectedMonth: "2025-01",
			expectedLabel: "Enero 2025",
			expectedIssue: "2025-02-01",
			expectedYear:  2025,
		},
	}

	for _, tc := range tests {
		mFormatted, label, issueDate, year, err := EstimateIssueDate(tc.inputRaw)
		if err != nil {
			t.Fatalf("error inesperado en EstimateIssueDate(%s): %v", tc.inputRaw, err)
		}
		if mFormatted != tc.expectedMonth {
			t.Errorf("para %s: mes formateado esperado '%s', obtenido '%s'", tc.inputRaw, tc.expectedMonth, mFormatted)
		}
		if label != tc.expectedLabel {
			t.Errorf("para %s: etiqueta esperada '%s', obtenida '%s'", tc.inputRaw, tc.expectedLabel, label)
		}
		if issueDate != tc.expectedIssue {
			t.Errorf("para %s: fecha emisión esperada '%s', obtenida '%s'", tc.inputRaw, tc.expectedIssue, issueDate)
		}
		if year != tc.expectedYear {
			t.Errorf("para %s: año esperado %d, obtenido %d", tc.inputRaw, tc.expectedYear, year)
		}
	}

	// Casos de error
	if _, _, _, _, err := EstimateIssueDate("2026"); err == nil {
		t.Errorf("se esperaba error para entrada de longitud menor a 6 dígitos")
	}
	if _, _, _, _, err := EstimateIssueDate("202613"); err == nil {
		t.Errorf("se esperaba error para mes 13")
	}
}

// 4. Test: JWT firmado contra un servidor httptest que haga de servidor de tokens, usando clave RSA generada
func TestJWTTokenExchangeWithHTTPServer(t *testing.T) {
	// Generar par de claves RSA en el test
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("error generando clave privada RSA para el test: %v", err)
	}

	const expectedClientEmail = "test-sa@mi-proyecto.iam.gserviceaccount.com"
	const expectedAccessToken = "ya29.test_simulated_access_token_12345"

	// Servidor simulador de OAuth2 token endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		grantType := r.FormValue("grant_type")
		if grantType != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, "grant_type inválido", http.StatusBadRequest)
			return
		}

		assertion := r.FormValue("assertion")
		if assertion == "" {
			http.Error(w, "assertion requerida", http.StatusBadRequest)
			return
		}

		// Validar el JWT recibido
		parts := strings.Split(assertion, ".")
		if len(parts) != 3 {
			http.Error(w, "JWT debe contener 3 partes", http.StatusBadRequest)
			return
		}

		// Verificar cabecera
		headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			http.Error(w, "error decodificando cabecera JWT", http.StatusBadRequest)
			return
		}
		var headerMap map[string]interface{}
		if err := json.Unmarshal(headerBytes, &headerMap); err != nil || headerMap["alg"] != "RS256" {
			http.Error(w, "cabecera JWT no válida o algoritmo distinto a RS256", http.StatusBadRequest)
			return
		}

		// Verificar claims
		claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "error decodificando claims JWT", http.StatusBadRequest)
			return
		}
		var claimsMap map[string]interface{}
		if err := json.Unmarshal(claimsBytes, &claimsMap); err != nil {
			http.Error(w, "error parseando claims JSON", http.StatusBadRequest)
			return
		}

		if claimsMap["iss"] != expectedClientEmail {
			http.Error(w, "iss no coincide", http.StatusBadRequest)
			return
		}
		if claimsMap["scope"] != defaultBigQueryScope {
			http.Error(w, "scope no coincide", http.StatusBadRequest)
			return
		}

		// Verificar firma RSA SHA256 con la clave pública generada
		signedContent := parts[0] + "." + parts[1]
		hasher := sha256.New()
		hasher.Write([]byte(signedContent))
		digest := hasher.Sum(nil)

		sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			http.Error(w, "error decodificando firma", http.StatusBadRequest)
			return
		}

		if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest, sigBytes); err != nil {
			http.Error(w, "firma RSA no verificada: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Responder con token válido
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": expectedAccessToken,
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	// Convertir clave privada a PEM para instanciar ServiceAccountKey
	privASN1 := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privASN1,
	}
	pemBytes := pem.EncodeToMemory(pemBlock)

	saKey := &ServiceAccountKey{
		Type:        "service_account",
		ClientEmail: expectedClientEmail,
		PrivateKey:  string(pemBytes),
		TokenURI:    server.URL,
	}

	provider, err := NewServiceAccountTokenProvider(saKey, server.Client())
	if err != nil {
		t.Fatalf("error inicializando ServiceAccountTokenProvider: %v", err)
	}

	cachedProvider := NewCachedTokenProvider(provider)

	// Primer intento: llamada al servidor
	ctx := context.Background()
	token1, err := cachedProvider.GetToken(ctx)
	if err != nil {
		t.Fatalf("error obteniendo token: %v", err)
	}
	if token1 != expectedAccessToken {
		t.Fatalf("token inesperado: esperado '%s', obtenido '%s'", expectedAccessToken, token1)
	}

	// Segundo intento: debe usar la caché en memoria sin volver a llamar
	token2, err := cachedProvider.GetToken(ctx)
	if err != nil {
		t.Fatalf("error en llamada subsecuente a GetToken: %v", err)
	}
	if token2 != expectedAccessToken {
		t.Fatalf("token en caché inconsistente: %s", token2)
	}
}
