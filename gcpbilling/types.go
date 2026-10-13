package gcpbilling

// InvoicesResponse representa la respuesta JSON estructurada de facturación
type InvoicesResponse struct {
	Success     bool       `json:"success"`
	Error       string     `json:"error,omitempty"`
	GeneratedAt string     `json:"generated_at,omitempty"`
	Source      string     `json:"source,omitempty"`
	Years       []YearData `json:"years"`
}

// YearData agrupa las facturas y totales de un año natural
type YearData struct {
	Year            int                `json:"year"`
	TotalByCurrency map[string]float64 `json:"total_by_currency"`
	Months          []MonthData        `json:"months"`
}

// MonthData agrupa las facturas de un mes cerrado
type MonthData struct {
	InvoiceMonth string        `json:"invoice_month"` // Formato 'YYYY-MM', ej. "2026-09"
	Label        string        `json:"label"`         // Formato legible, ej. "Septiembre 2026"
	IssueDate    string        `json:"issue_date"`    // Fecha estimada de emisión 'YYYY-MM-DD', ej. "2026-10-01"
	Invoices     []InvoiceItem `json:"invoices"`
}

// InvoiceItem representa la factura o extracto de una cuenta de facturación en un mes
type InvoiceItem struct {
	BillingAccountID string  `json:"billing_account_id"`
	Currency         string  `json:"currency"`
	Cost             float64 `json:"cost"`
	Credits          float64 `json:"credits"`
	Tax              float64 `json:"tax"`
	Total            float64 `json:"total"`
	Projects         int     `json:"projects"`
}

// RawInvoiceRow representa una fila procesada de BigQuery
type RawInvoiceRow struct {
	BillingAccountID string
	InvoiceMonthRaw  string // 'YYYYMM'
	Currency         string
	Cost             float64
	Credits          float64
	Tax              float64
	Total            float64
	Projects         int
}

// BQQueryResponse representa la respuesta del endpoint jobs.query de BigQuery REST API
type BQQueryResponse struct {
	Kind         string         `json:"kind"`
	JobReference BQJobReference `json:"jobReference"`
	TotalRows    string         `json:"totalRows"`
	Rows         []BQRow        `json:"rows"`
	JobComplete  bool           `json:"jobComplete"`
	PageToken    string         `json:"pageToken"`
	Errors       []BQErrorProto `json:"errors,omitempty"`
	Error        *BQErrorDetail `json:"error,omitempty"`
}

type BQErrorDetail struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Errors  []BQErrorProto `json:"errors,omitempty"`
}

type BQJobReference struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	Location  string `json:"location"`
}

type BQRow struct {
	F []BQField `json:"f"`
}

type BQField struct {
	V interface{} `json:"v"`
}

type BQErrorProto struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}
