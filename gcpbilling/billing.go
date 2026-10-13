package gcpbilling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var spanishMonths = [...]string{
	"", // 1-indexado
	"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
	"Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
}

// TableConfig contiene la configuración resuelta para la consulta en BigQuery
type TableConfig struct {
	FullTable    string
	QueryProject string
	Location     string
}

// DefaultBillingExportTable es la tabla o dataset de exportación por defecto
const DefaultBillingExportTable = "planes-dev.gcp_billing_export"

// ResolveTableConfig analiza y resuelve la configuración de tabla y proyecto desde el entorno
func ResolveTableConfig() (TableConfig, error) {
	tableEnv := strings.TrimSpace(os.Getenv("GCP_BILLING_EXPORT_TABLE"))
	tableEnv = strings.Trim(tableEnv, "`")
	if tableEnv == "" {
		tableEnv = DefaultBillingExportTable
	}

	parts := strings.Split(tableEnv, ".")
	var fullTable string
	var defaultProj string

	if len(parts) == 2 {
		// proyecto.dataset -> usar comodín de exportación estándar
		fullTable = fmt.Sprintf("%s.%s.gcp_billing_export_v1_*", parts[0], parts[1])
		defaultProj = parts[0]
	} else if len(parts) == 3 {
		fullTable = fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
		defaultProj = parts[0]
	} else {
		return TableConfig{}, fmt.Errorf("formato inválido en GCP_BILLING_EXPORT_TABLE (%s): se requiere 'proyecto.dataset.tabla' o 'proyecto.dataset'", tableEnv)
	}

	queryProject := strings.TrimSpace(os.Getenv("GCP_BILLING_QUERY_PROJECT"))
	if queryProject == "" {
		queryProject = defaultProj
	}

	location := strings.TrimSpace(os.Getenv("GCP_BILLING_LOCATION"))

	return TableConfig{
		FullTable:    fullTable,
		QueryProject: queryProject,
		Location:     location,
	}, nil
}

// EstimateIssueDate calcula la fecha de emisión estimada (día 1 del mes siguiente),
// la etiqueta en español y el mes formateado 'YYYY-MM' a partir de 'YYYYMM' o 'YYYY-MM'.
func EstimateIssueDate(invoiceMonthRaw string) (invoiceMonthFormatted string, label string, issueDate string, year int, err error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(invoiceMonthRaw), "-", "")
	if len(cleaned) != 6 {
		return "", "", "", 0, fmt.Errorf("mes de factura inválido (%s): se esperaban 6 dígitos YYYYMM", invoiceMonthRaw)
	}

	y, err := strconv.Atoi(cleaned[:4])
	if err != nil {
		return "", "", "", 0, fmt.Errorf("año inválido en mes de factura: %w", err)
	}
	m, err := strconv.Atoi(cleaned[4:])
	if err != nil || m < 1 || m > 12 {
		return "", "", "", 0, fmt.Errorf("mes inválido (%s): debe estar entre 01 y 12", cleaned[4:])
	}

	invoiceMonthFormatted = fmt.Sprintf("%04d-%02d", y, m)
	label = fmt.Sprintf("%s %04d", spanishMonths[m], y)

	// Fecha estimada de emisión: día 1 del mes siguiente
	t := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	issueDate = t.Format("2006-01-02")
	year = y

	return invoiceMonthFormatted, label, issueDate, year, nil
}

func round2(val float64) float64 {
	return math.Round(val*100) / 100
}

func getStringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return fmt.Sprintf("%.6f", val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprint(v)
	}
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return val
}

func parseInt(s string) int {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return val
}

// ParseQueryRows procesa las filas crudas devueltas por jobs.query de BigQuery
func ParseQueryRows(rows []BQRow) ([]RawInvoiceRow, error) {
	var result []RawInvoiceRow
	for i, r := range rows {
		if len(r.F) < 7 {
			return nil, fmt.Errorf("fila %d con número insuficiente de columnas (%d, esperadas 7)", i, len(r.F))
		}
		billingAcc := getStringVal(r.F[0].V)
		monthRaw := getStringVal(r.F[1].V)
		currency := getStringVal(r.F[2].V)
		cost := round2(parseFloat(getStringVal(r.F[3].V)))
		credits := round2(parseFloat(getStringVal(r.F[4].V)))
		tax := round2(parseFloat(getStringVal(r.F[5].V)))
		projects := parseInt(getStringVal(r.F[6].V))
		total := round2(cost + credits)

		result = append(result, RawInvoiceRow{
			BillingAccountID: billingAcc,
			InvoiceMonthRaw:  monthRaw,
			Currency:         currency,
			Cost:             cost,
			Credits:          credits,
			Tax:              tax,
			Total:            total,
			Projects:         projects,
		})
	}
	return result, nil
}

// GroupInvoices agrupa las filas de facturas por año y mes en orden descendente
func GroupInvoices(rows []RawInvoiceRow) ([]YearData, error) {
	type monthGroupKey struct {
		year         int
		invoiceMonth string
	}

	monthMap := make(map[monthGroupKey]*MonthData)
	yearMap := make(map[int]*YearData)

	for _, row := range rows {
		invMonth, label, issueDate, year, err := EstimateIssueDate(row.InvoiceMonthRaw)
		if err != nil {
			return nil, err
		}

		key := monthGroupKey{year: year, invoiceMonth: invMonth}
		mGroup, ok := monthMap[key]
		if !ok {
			mGroup = &MonthData{
				InvoiceMonth: invMonth,
				Label:        label,
				IssueDate:    issueDate,
				Invoices:     []InvoiceItem{},
			}
			monthMap[key] = mGroup
		}

		item := InvoiceItem{
			BillingAccountID: row.BillingAccountID,
			Currency:         row.Currency,
			Cost:             row.Cost,
			Credits:          row.Credits,
			Tax:              row.Tax,
			Total:            row.Total,
			Projects:         row.Projects,
		}
		mGroup.Invoices = append(mGroup.Invoices, item)

		yGroup, ok := yearMap[year]
		if !ok {
			yGroup = &YearData{
				Year:            year,
				TotalByCurrency: make(map[string]float64),
				Months:          []MonthData{},
			}
			yearMap[year] = yGroup
		}
		yGroup.TotalByCurrency[row.Currency] = round2(yGroup.TotalByCurrency[row.Currency] + row.Total)
	}

	// Ordenar facturas de cada mes por cuenta de facturación
	for _, mGroup := range monthMap {
		sort.Slice(mGroup.Invoices, func(i, j int) bool {
			return mGroup.Invoices[i].BillingAccountID < mGroup.Invoices[j].BillingAccountID
		})
	}

	// Asociar meses a sus respectivos años
	for key, mGroup := range monthMap {
		yGroup := yearMap[key.year]
		yGroup.Months = append(yGroup.Months, *mGroup)
	}

	// Obtener lista de años ordenada de forma DESCENDENTE
	var yearsList []int
	for y := range yearMap {
		yearsList = append(yearsList, y)
	}
	sort.Slice(yearsList, func(i, j int) bool {
		return yearsList[i] > yearsList[j]
	})

	var result []YearData
	for _, y := range yearsList {
		yGroup := yearMap[y]
		// Ordenar meses dentro del año de forma DESCENDENTE por fecha de emisión / invoice_month
		sort.Slice(yGroup.Months, func(i, j int) bool {
			return yGroup.Months[i].InvoiceMonth > yGroup.Months[j].InvoiceMonth
		})
		result = append(result, *yGroup)
	}

	if result == nil {
		result = []YearData{}
	}

	return result, nil
}

// BuildQuerySQL genera la sentencia SQL paramétrica con la tabla y condiciones requeridas
func BuildQuerySQL(fullTable string) string {
	return fmt.Sprintf(`SELECT
  billing_account_id,
  invoice.month AS invoice_month,
  currency,
  SUM(cost) AS cost,
  SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS credits,
  SUM(IF(cost_type = 'tax', cost, 0)) AS tax,
  COUNT(DISTINCT project.id) AS projects
FROM `+"`%s`"+`
WHERE invoice.month IS NOT NULL
  AND invoice.month < FORMAT_DATE('%%Y%%m', CURRENT_DATE('America/Los_Angeles'))
GROUP BY billing_account_id, invoice_month, currency
ORDER BY invoice_month DESC, billing_account_id`, fullTable)
}

// InvoiceDetailItem representa una línea de desglose de facturación
type InvoiceDetailItem struct {
	Organization       string  `json:"organization"`
	ProjectID          string  `json:"project_id"`
	ProjectName        string  `json:"project_name"`
	ServiceDescription string  `json:"service_description"`
	SKUDescription     string  `json:"sku_description"`
	Model              string  `json:"model,omitempty"`
	Cost               float64 `json:"cost"`
	Credits            float64 `json:"credits"`
	NetCost            float64 `json:"net_cost"`
	Currency           string  `json:"currency"`
}

// InvoiceDetailResponse representa el desglose de costes de un mes
type InvoiceDetailResponse struct {
	Success      bool                `json:"success"`
	InvoiceMonth string              `json:"invoice_month"`
	Label        string              `json:"label"`
	Currency     string              `json:"currency"`
	TotalCost    float64             `json:"total_cost"`
	TotalCredits float64             `json:"total_credits"`
	TotalNet     float64             `json:"total_net"`
	Items        []InvoiceDetailItem `json:"items"`
}

type detailCacheEntry struct {
	data     *InvoiceDetailResponse
	cachedAt time.Time
}

var (
	geminiRegex    = regexp.MustCompile(`(?i)(gemini\s*[\d\.]+(?:\s*(?:flash|pro|ultra|lite|preview|ga))*)`)
	geminiEmbRegex = regexp.MustCompile(`(?i)(gemini-embedding-[^\s]+)`)
	geminiGenRegex = regexp.MustCompile(`(?i)(gemini\s*[\d\.\/]+(?:\s*[a-zA-Z]+)?)`)
	claudeRegex    = regexp.MustCompile(`(?i)(claude\s*[\d\.]+(?:\s*[a-zA-Z]+)?)`)
)

// ExtractModel extrae limpiamente el modelo de IA a partir de la descripción del SKU
func ExtractModel(sku string) string {
	if sku == "" {
		return ""
	}
	if m := geminiRegex.FindStringSubmatch(sku); len(m) > 1 {
		res := strings.TrimSpace(m[1])
		res = strings.TrimSuffix(res, " GA")
		res = strings.TrimSuffix(res, " preview")
		parts := strings.Fields(res)
		for i, p := range parts {
			low := strings.ToLower(p)
			if low == "gemini" || low == "flash" || low == "pro" || low == "ultra" || low == "lite" {
				parts[i] = strings.ToUpper(low[:1]) + low[1:]
			}
		}
		return strings.Join(parts, " ")
	}
	if m := geminiEmbRegex.FindStringSubmatch(sku); len(m) > 1 {
		return strings.ToLower(m[1])
	}
	if m := geminiGenRegex.FindStringSubmatch(sku); len(m) > 1 {
		parts := strings.Fields(m[1])
		for i, p := range parts {
			low := strings.ToLower(p)
			if low == "gemini" || low == "flash" || low == "pro" || low == "ultra" || low == "lite" {
				parts[i] = strings.ToUpper(low[:1]) + low[1:]
			}
		}
		return strings.Join(parts, " ")
	}
	if m := claudeRegex.FindStringSubmatch(sku); len(m) > 1 {
		parts := strings.Fields(m[1])
		for i, p := range parts {
			low := strings.ToLower(p)
			if low == "claude" || low == "sonnet" || low == "haiku" || low == "opus" {
				parts[i] = strings.ToUpper(low[:1]) + low[1:]
			}
		}
		return strings.Join(parts, " ")
	}
	if strings.Contains(strings.ToLower(sku), "duet ai") {
		return "Duet AI"
	}
	return ""
}

// ResolveOrganization mapea un proyecto a su organización corporativa correspondiente
func ResolveOrganization(projectID, projectName string) string {
	pid := strings.ToLower(projectID)
	pname := strings.ToLower(projectName)
	if strings.Contains(pid, "maderasplanes") || strings.Contains(pname, "maderasplanes") {
		return "maderasplanes.com"
	}
	return "planesnet.com"
}

// BuildDetailQuerySQL genera la consulta SQL para extraer el detalle desglosado de un mes
func BuildDetailQuerySQL(fullTable, invoiceMonthClean string) string {
	return fmt.Sprintf(`SELECT
  project.id AS project_id,
  IFNULL(project.name, '') AS project_name,
  service.description AS service_description,
  sku.description AS sku_description,
  SUM(cost) AS cost,
  SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS credits,
  currency
FROM `+"`%s`"+`
WHERE REPLACE(invoice.month, '-', '') = '%s'
GROUP BY project_id, project_name, service_description, sku_description, currency
ORDER BY cost DESC`, fullTable, invoiceMonthClean)
}

// BillingService administra las consultas a BigQuery y la caché en memoria de 15 minutos
type BillingService struct {
	mu            sync.RWMutex
	tokenProvider TokenProvider
	httpClient    *http.Client
	baseURL       string // Permite override en tests de integración
	cachedData    *InvoicesResponse
	cachedAt      time.Time
	detailCache   map[string]*detailCacheEntry
}

func NewBillingService() *BillingService {
	return &BillingService{
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		baseURL:     "https://bigquery.googleapis.com/bigquery/v2",
		detailCache: make(map[string]*detailCacheEntry),
	}
}

func NewBillingServiceWithClient(tokenProvider TokenProvider, httpClient *http.Client, baseURL string) *BillingService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://bigquery.googleapis.com/bigquery/v2"
	}
	return &BillingService{
		tokenProvider: tokenProvider,
		httpClient:    httpClient,
		baseURL:       baseURL,
		detailCache:   make(map[string]*detailCacheEntry),
	}
}

// GetInvoices obtiene las facturas desde BigQuery o de la caché en memoria (15 min)
func (s *BillingService) GetInvoices(ctx context.Context, forceRefresh bool) (*InvoicesResponse, error) {
	if !forceRefresh {
		s.mu.RLock()
		if s.cachedData != nil && time.Since(s.cachedAt) < 15*time.Minute {
			data := s.cachedData
			s.mu.RUnlock()
			return data, nil
		}
		s.mu.RUnlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Doble comprobación si no es forceRefresh
	if !forceRefresh && s.cachedData != nil && time.Since(s.cachedAt) < 15*time.Minute {
		return s.cachedData, nil
	}

	tblConfig, err := ResolveTableConfig()
	if err != nil {
		return nil, err
	}

	token, err := s.getToken(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.executeQuery(ctx, tblConfig, token)
	if err != nil {
		return nil, err
	}

	rawRows, err := ParseQueryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("error procesando filas de facturación: %w", err)
	}

	years, err := GroupInvoices(rawRows)
	if err != nil {
		return nil, fmt.Errorf("error agrupando facturas: %w", err)
	}

	resp := &InvoicesResponse{
		Success:     true,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      tblConfig.FullTable,
		Years:       years,
	}

	if len(years) > 0 {
		s.cachedData = resp
		s.cachedAt = time.Now()
	} else {
		s.cachedData = nil
	}

	return resp, nil
}

// GetInvoiceDetail obtiene el desglose detallado de costes por organización, proyecto, servicio, SKU y modelo para un mes
func (s *BillingService) GetInvoiceDetail(ctx context.Context, invoiceMonth string, forceRefresh bool) (*InvoiceDetailResponse, error) {
	monthClean := strings.ReplaceAll(strings.TrimSpace(invoiceMonth), "-", "")
	if len(monthClean) != 6 {
		return nil, fmt.Errorf("formato de mes inválido (%s): se espera YYYYMM o YYYY-MM", invoiceMonth)
	}

	if !forceRefresh {
		s.mu.RLock()
		if s.detailCache != nil {
			if entry, ok := s.detailCache[monthClean]; ok && entry != nil && time.Since(entry.cachedAt) < 15*time.Minute {
				data := entry.data
				s.mu.RUnlock()
				return data, nil
			}
		}
		s.mu.RUnlock()
	}

	tblConfig, err := ResolveTableConfig()
	if err != nil {
		return nil, err
	}

	token, err := s.getToken(ctx)
	if err != nil {
		return nil, err
	}

	detailSQL := BuildDetailQuerySQL(tblConfig.FullTable, monthClean)
	rows, err := s.executeQuerySQL(ctx, tblConfig, token, detailSQL)
	if err != nil {
		return nil, err
	}

	var items []InvoiceDetailItem
	var totalCost, totalCredits float64
	currency := "USD"

	for _, r := range rows {
		if len(r.F) < 7 {
			continue
		}
		projID := getStringVal(r.F[0].V)
		projName := getStringVal(r.F[1].V)
		servDesc := getStringVal(r.F[2].V)
		skuDesc := getStringVal(r.F[3].V)
		cost := round2(parseFloat(getStringVal(r.F[4].V)))
		credits := round2(parseFloat(getStringVal(r.F[5].V)))
		curr := getStringVal(r.F[6].V)
		if curr != "" {
			currency = curr
		}
		netCost := round2(cost + credits)

		org := ResolveOrganization(projID, projName)
		model := ExtractModel(skuDesc)

		items = append(items, InvoiceDetailItem{
			Organization:       org,
			ProjectID:          projID,
			ProjectName:        projName,
			ServiceDescription: servDesc,
			SKUDescription:     skuDesc,
			Model:              model,
			Cost:               cost,
			Credits:            credits,
			NetCost:            netCost,
			Currency:           currency,
		})

		totalCost = round2(totalCost + cost)
		totalCredits = round2(totalCredits + credits)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].NetCost > items[j].NetCost
	})

	_, label, _, _, _ := EstimateIssueDate(monthClean)

	resp := &InvoiceDetailResponse{
		Success:      true,
		InvoiceMonth: monthClean,
		Label:        label,
		Currency:     currency,
		TotalCost:    totalCost,
		TotalCredits: totalCredits,
		TotalNet:     round2(totalCost + totalCredits),
		Items:        items,
	}

	s.mu.Lock()
	if s.detailCache == nil {
		s.detailCache = make(map[string]*detailCacheEntry)
	}
	if len(items) > 0 {
		s.detailCache[monthClean] = &detailCacheEntry{
			data:     resp,
			cachedAt: time.Now(),
		}
	} else {
		delete(s.detailCache, monthClean)
	}
	s.mu.Unlock()

	return resp, nil
}

func (s *BillingService) getToken(ctx context.Context) (string, error) {
	tokenProv := s.tokenProvider
	if tokenProv == nil {
		prov, err := ResolveTokenProvider(s.httpClient)
		if err != nil {
			return "", err
		}
		s.tokenProvider = prov
		tokenProv = prov
	}

	token, err := tokenProv.GetToken(ctx)
	if err != nil {
		return "", fmt.Errorf("error obteniendo token de acceso para BigQuery: %w", err)
	}
	return token, nil
}

func (s *BillingService) executeQuery(ctx context.Context, cfg TableConfig, token string) ([]BQRow, error) {
	return s.executeQuerySQL(ctx, cfg, token, BuildQuerySQL(cfg.FullTable))
}

func (s *BillingService) executeQuerySQL(ctx context.Context, cfg TableConfig, token, querySQL string) ([]BQRow, error) {
	type queryRequest struct {
		Query        string `json:"query"`
		UseLegacySQL bool   `json:"useLegacySql"`
		TimeoutMs    int    `json:"timeoutMs"`
		Location     string `json:"location,omitempty"`
	}

	reqBody := queryRequest{
		Query:        querySQL,
		UseLegacySQL: false,
		TimeoutMs:    30000,
		Location:     cfg.Location,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	queryURL := fmt.Sprintf("%s/projects/%s/queries", strings.TrimRight(s.baseURL, "/"), url.PathEscape(cfg.QueryProject))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queryURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error conectando con BigQuery API: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta de BigQuery: %w", err)
	}

	var bqResp BQQueryResponse
	if err := json.Unmarshal(respBytes, &bqResp); err != nil {
		return nil, fmt.Errorf("error decodificando respuesta JSON de BigQuery (HTTP %d): %w", httpResp.StatusCode, err)
	}

	if httpResp.StatusCode != http.StatusOK {
		rawMsg := ""
		code := httpResp.StatusCode
		if bqResp.Error != nil && bqResp.Error.Message != "" {
			rawMsg = bqResp.Error.Message
			if bqResp.Error.Code > 0 {
				code = bqResp.Error.Code
			}
		} else if len(bqResp.Errors) > 0 {
			rawMsg = bqResp.Errors[0].Message
		}

		if rawMsg != "" {
			lowerMsg := strings.ToLower(rawMsg)
			if strings.Contains(lowerMsg, "insufficient authentication scopes") {
				return nil, fmt.Errorf("error de BigQuery (%d): %s. Causa: la máquina virtual de Google Cloud donde ejecuta PlanesGo tiene restringidos los ámbitos de acceso (Access Scopes). Solución: configure una clave de cuenta de servicio en la variable GCP_BILLING_SA_JSON (o GOOGLE_APPLICATION_CREDENTIALS), o bien edite la VM en Google Cloud para habilitar el ámbito 'cloud-platform'", code, rawMsg)
			}
			if strings.Contains(lowerMsg, "does not match any table") || strings.Contains(lowerMsg, "not found: table") {
				// El dataset existe y la conexión es válida, pero Google Cloud aún no ha volcado la tabla de facturación
				return nil, nil
			}
			return nil, fmt.Errorf("error de BigQuery (%d): %s", code, rawMsg)
		}
		return nil, fmt.Errorf("BigQuery devolvió error HTTP %d: %s", httpResp.StatusCode, string(respBytes))
	}

	allRows := bqResp.Rows

	// Si jobComplete es false, consultar getQueryResults hasta completar
	for !bqResp.JobComplete {
		jobID := bqResp.JobReference.JobID
		projID := bqResp.JobReference.ProjectID
		loc := bqResp.JobReference.Location
		if loc == "" {
			loc = cfg.Location
		}

		time.Sleep(1 * time.Second)

		pollURL := fmt.Sprintf("%s/projects/%s/queries/%s?timeoutMs=10000",
			strings.TrimRight(s.baseURL, "/"), url.PathEscape(projID), url.PathEscape(jobID))
		if loc != "" {
			pollURL += "&location=" + url.QueryEscape(loc)
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, err
		}
		pollReq.Header.Set("Authorization", "Bearer "+token)

		pollResp, err := s.httpClient.Do(pollReq)
		if err != nil {
			return nil, fmt.Errorf("error consultando estado del trabajo de BigQuery: %w", err)
		}
		pollBytes, err := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(pollBytes, &bqResp); err != nil {
			return nil, fmt.Errorf("error analizando estado de trabajo BigQuery: %w", err)
		}

		if bqResp.JobComplete {
			allRows = bqResp.Rows
			break
		}
	}

	// Manejo de paginación con pageToken
	for bqResp.PageToken != "" {
		jobID := bqResp.JobReference.JobID
		projID := bqResp.JobReference.ProjectID
		loc := bqResp.JobReference.Location
		if loc == "" {
			loc = cfg.Location
		}

		pageURL := fmt.Sprintf("%s/projects/%s/queries/%s?pageToken=%s",
			strings.TrimRight(s.baseURL, "/"), url.PathEscape(projID), url.PathEscape(jobID), url.QueryEscape(bqResp.PageToken))
		if loc != "" {
			pageURL += "&location=" + url.QueryEscape(loc)
		}

		pageReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, err
		}
		pageReq.Header.Set("Authorization", "Bearer "+token)

		pageResp, err := s.httpClient.Do(pageReq)
		if err != nil {
			return nil, fmt.Errorf("error paginando resultados de BigQuery: %w", err)
		}
		pageBytes, err := io.ReadAll(pageResp.Body)
		pageResp.Body.Close()
		if err != nil {
			return nil, err
		}

		var pageBQResp BQQueryResponse
		if err := json.Unmarshal(pageBytes, &pageBQResp); err != nil {
			return nil, fmt.Errorf("error analizando página de resultados: %w", err)
		}

		allRows = append(allRows, pageBQResp.Rows...)
		bqResp.PageToken = pageBQResp.PageToken
	}

	return allRows, nil
}
