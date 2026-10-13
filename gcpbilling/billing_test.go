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
	"os"
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

// 7. Test: ResolveTokenProvider con ruta de archivo en GCP_BILLING_SA_JSON
func TestResolveTokenProviderFilePath(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("error generando clave privada RSA: %v", err)
	}
	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	saKey := ServiceAccountKey{
		Type:         "service_account",
		ProjectID:    "mi-proyecto",
		PrivateKeyID: "clave123",
		PrivateKey:   string(pemBytes),
		ClientEmail:  "test-sa@mi-proyecto.iam.gserviceaccount.com",
		ClientID:     "123456789",
		TokenURI:     "https://oauth2.googleapis.com/token",
	}

	saBytes, err := json.Marshal(saKey)
	if err != nil {
		t.Fatalf("error serializando clave: %v", err)
	}

	tmpFile := t.TempDir() + "/sa_test.json"
	if err := os.WriteFile(tmpFile, saBytes, 0600); err != nil {
		t.Fatalf("error escribiendo archivo temporal: %v", err)
	}

	t.Setenv("GCP_BILLING_SA_JSON", tmpFile)
	provider, err := ResolveTokenProvider(http.DefaultClient)
	if err != nil {
		t.Fatalf("error resolviendo provider desde ruta de archivo: %v", err)
	}
	if provider == nil {
		t.Fatalf("provider no debería ser nil")
	}
}

// 8. Test: ExtractModel y ResolveOrganization
func TestExtractModelAndResolveOrganization(t *testing.T) {
	testsModel := []struct {
		sku      string
		expected string
	}{
		{"Generative AI text prompts - Gemini 3.8 Flash GA", "Gemini 3.8 Flash"},
		{"Gemini 2.5 Pro Multimodal Input Token", "Gemini 2.5 Pro"},
		{"Gemini 2.0 Flash Thought Token", "Gemini 2.0 Flash"},
		{"Gemini 1.5 Flash input tokens", "Gemini 1.5 Flash"},
		{"gemini-embedding-001 prompt tokens", "gemini-embedding-001"},
		{"Claude 3.5 Sonnet prompt", "Claude 3.5 Sonnet"},
		{"Duet AI Developer Subscription", "Duet AI"},
		{"N2 Instance Core running in Americas", ""},
		{"Standard Storage US Regional", ""},
	}

	for _, tt := range testsModel {
		got := ExtractModel(tt.sku)
		if got != tt.expected {
			t.Errorf("ExtractModel(%q) = %q; want %q", tt.sku, got, tt.expected)
		}
	}

	testsOrg := []struct {
		projectID   string
		projectName string
		expected    string
	}{
		{"maderasplanes-odoo-erp", "maderasplanes-odoo", "maderasplanes.com"},
		{"maderasplanes", "Maderas Planes", "maderasplanes.com"},
		{"planes-dev", "planes-dev", "planesnet.com"},
		{"autopyme", "AutoPyme Production", "planesnet.com"},
		{"mazana-health", "Mazana Health", "planesnet.com"},
	}

	for _, tt := range testsOrg {
		got := ResolveOrganization(tt.projectID, tt.projectName)
		if got != tt.expected {
			t.Errorf("ResolveOrganization(%q, %q) = %q; want %q", tt.projectID, tt.projectName, got, tt.expected)
		}
	}
}

type dummyTokenProvider struct {
	token string
}

func (d *dummyTokenProvider) GetToken(ctx context.Context) (string, error) {
	return d.token, nil
}

// 9. Test: GetInvoiceDetail con mock de BigQuery
func TestGetInvoiceDetailMock(t *testing.T) {
	mockBQ := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := BQQueryResponse{
			JobComplete: true,
			TotalRows:   "2",
			Rows: []BQRow{
				{
					F: []BQField{
						{V: "Vertex AI"},
						{V: "planes-dev"},
						{V: "Planes Dev"},
						{V: "Generative AI - Gemini 3.8 Flash GA input"},
						{V: "120.50"},
						{V: "-20.50"},
						{V: "USD"},
					},
				},
				{
					F: []BQField{
						{V: "Compute Engine"},
						{V: "maderasplanes-odoo-erp"},
						{V: "Maderas Odoo"},
						{V: "N2 Instance Core"},
						{V: "80.00"},
						{V: "0.00"},
						{V: "USD"},
					},
				},
				{
					F: []BQField{
						{V: "Cloud Storage"},
						{V: "planesnet-backup"},
						{V: "PlanesNet Backup"},
						{V: "Standard Storage US Regional"},
						{V: "0.00"},
						{V: "0.00"},
						{V: "USD"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockBQ.Close()

	svc := NewBillingServiceWithClient(&dummyTokenProvider{token: "mock-token"}, mockBQ.Client(), mockBQ.URL)
	detail, err := svc.GetInvoiceDetail(context.Background(), "2026-09", false)
	if err != nil {
		t.Fatalf("error inesperado en GetInvoiceDetail: %v", err)
	}

	if !detail.Success {
		t.Errorf("se esperaba success true")
	}
	if len(detail.Items) != 2 {
		t.Fatalf("se esperaban 2 items, obtenidos %d", len(detail.Items))
	}

	// Ordenado por ServiceDescription ASC: primero Compute Engine, luego Vertex AI
	item0 := detail.Items[0]
	if item0.ServiceDescription != "Compute Engine" {
		t.Errorf("servicio esperado Compute Engine, obtenido %s", item0.ServiceDescription)
	}
	if item0.Organization != "maderasplanes.com" {
		t.Errorf("organizacion esperada maderasplanes.com, obtenida %s", item0.Organization)
	}
	if item0.Model != "" {
		t.Errorf("modelo esperado vacio, obtenido %s", item0.Model)
	}
	if item0.NetCost != 80.00 {
		t.Errorf("net cost esperado 80.00, obtenido %f", item0.NetCost)
	}

	item1 := detail.Items[1]
	if item1.ServiceDescription != "Vertex AI" {
		t.Errorf("servicio esperado Vertex AI, obtenido %s", item1.ServiceDescription)
	}
	if item1.Organization != "planesnet.com" {
		t.Errorf("organizacion esperada planesnet.com, obtenida %s", item1.Organization)
	}
	if item1.Model != "Gemini 3.8 Flash" {
		t.Errorf("modelo esperado 'Gemini 3.8 Flash', obtenido %s", item1.Model)
	}
	if item1.NetCost != 100.00 {
		t.Errorf("net cost esperado 100.00, obtenido %f", item1.NetCost)
	}

	if detail.TotalCost != 200.50 {
		t.Errorf("total cost esperado 200.50, obtenido %f", detail.TotalCost)
	}
	if detail.TotalCredits != -20.50 {
		t.Errorf("total credits esperado -20.50, obtenido %f", detail.TotalCredits)
	}
	if detail.TotalNet != 180.00 {
		t.Errorf("total net esperado 180.00, obtenido %f", detail.TotalNet)
	}
}

// 10. Test: BuildDetailQuerySQL
func TestBuildDetailQuerySQL(t *testing.T) {
	sql := BuildDetailQuerySQL("my-project.my_dataset.my_table", "202609")
	if !strings.Contains(sql, "HAVING cost != 0") {
		t.Errorf("SQL debe contener 'HAVING cost != 0' para evitar error de aggregations of aggregations en BigQuery, obtenido:\n%s", sql)
	}
	if strings.Contains(sql, "HAVING SUM(cost)") {
		t.Errorf("SQL NO debe contener 'HAVING SUM(cost)' porque causa error 400 en BigQuery:\n%s", sql)
	}
}

