package main

// Tipos de tarea, normalización de nombres y utilidades comunes.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Version       = "2.0.0"
	DefaultServer = "https://planesgo.autopyme.com"

	TaskTypeAnalisisDiseno = "Análisis y diseño"
	TaskTypeDesarrollo     = "Desarrollo"
	TaskTypePruebas        = "Pruebas"

	// TASK_NAME_DEFAULT es el mismo nombre de tarea que usa el hook de Claude Code
	// (planesgo_claude_hook.py) para los latidos automáticos, así los minutos añadidos
	// a mano con pgo_time (o --time) caen en la misma tarea en vez de crear una aparte.
	TASK_NAME_DEFAULT = "Claude Code - Sesión IA"
)

func matchCanonicalType(t string) (string, bool) {
	norm := strings.ToLower(strings.TrimSpace(t))
	norm = strings.ReplaceAll(norm, "á", "a")
	norm = strings.ReplaceAll(norm, "é", "e")
	norm = strings.ReplaceAll(norm, "í", "i")
	norm = strings.ReplaceAll(norm, "ó", "o")
	norm = strings.ReplaceAll(norm, "ú", "u")
	norm = strings.ReplaceAll(norm, "ñ", "n")

	switch norm {
	case "analisis y diseno", "analisis y diseño", "análisis y diseño", "analisis", "análisis", "analysis", "investigacion", "auditoria", "diseno", "diseño", "design", "planificacion", "arquitectura":
		return TaskTypeAnalisisDiseno, true
	case "desarrollo", "dev", "development", "implementacion", "implementación", "impl", "ajuste", "ajustes", "fix", "fixes", "bugfix", "refactor", "servidor", "server", "infraestructura", "infra", "ops", "cliente", "client", "soporte", "support":
		return TaskTypeDesarrollo, true
	case "pruebas", "prueba", "test", "tests", "testing", "qa", "verificacion", "validacion":
		return TaskTypePruebas, true
	}
	return "", false
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func cleanAntigravityTaskName(taskName string) string {
	name := strings.TrimSpace(taskName)
	for {
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "[AGY]") {
			name = strings.TrimSpace(name[5:])
			continue
		}
		if strings.HasPrefix(upper, "[ANTIGRAVITY]") {
			name = strings.TrimSpace(name[14:])
			continue
		}
		break
	}
	return name
}

// formatTokens formatea un número entero con separador de miles (ej: 15420 -> "15.420")
func formatTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	in := strconv.Itoa(n)
	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// ActiveSessionMetadata contiene la información de la sesión activa de Antigravity registrada por los hooks
type ActiveSessionMetadata struct {
	TranscriptPath string  `json:"transcriptPath"`
	ConversationID string  `json:"conversationId"`
	ModelName      string  `json:"modelName"`
	UpdatedAt      float64 `json:"updatedAt"`
}

// estimateTokensFromActiveSession intenta estimar los tokens consumidos analizando el archivo de transcript de la sesión
func estimateTokensFromActiveSession() (int, int, int, string) {
	sessionFile := "/tmp/planesgo_active_session.json"
	var transcriptPath string
	var modelName string

	if data, err := os.ReadFile(sessionFile); err == nil {
		var meta ActiveSessionMetadata
		if err := json.Unmarshal(data, &meta); err == nil {
			transcriptPath = meta.TranscriptPath
			modelName = meta.ModelName
		}
	}

	if transcriptPath == "" || !fileExists(transcriptPath) {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			brainDir := filepath.Join(homeDir, ".gemini", "antigravity", "brain")
			var newestFile string
			var newestMod time.Time
			_ = filepath.Walk(brainDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if !info.IsDir() && (info.Name() == "transcript.jsonl" || info.Name() == "transcript_full.jsonl") {
					if info.ModTime().After(newestMod) {
						newestMod = info.ModTime()
						newestFile = path
					}
				}
				return nil
			})
			if newestFile != "" && time.Since(newestMod) < 2*time.Hour {
				transcriptPath = newestFile
			}
		}
	}

	if transcriptPath == "" || !fileExists(transcriptPath) {
		return 0, 0, 0, modelName
	}

	file, err := os.Open(transcriptPath)
	if err != nil {
		return 0, 0, 0, modelName
	}
	defer file.Close()

	var inputChars, outputChars int
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 5*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var step struct {
			Source    string          `json:"source"`
			Type      string          `json:"type"`
			Content   string          `json:"content"`
			Thinking  string          `json:"thinking"`
			ToolCalls json.RawMessage `json:"tool_calls"`
		}
		if err := json.Unmarshal(line, &step); err != nil {
			continue
		}

		if step.Source == "MODEL" {
			outputChars += len(step.Content) + len(step.Thinking) + len(step.ToolCalls)
		} else {
			inputChars += len(step.Content) + len(step.ToolCalls)
		}
	}

	tokensIn := int(float64(inputChars) / 3.8)
	tokensOut := int(float64(outputChars) / 3.8)
	tokensTotal := tokensIn + tokensOut

	return tokensTotal, tokensIn, tokensOut, modelName
}

func NormalizeTaskType(taskName, description, explicitType string) (string, string) {
	raw := cleanAntigravityTaskName(taskName)

	var detectedType string

	// 1. Si el nombre ya comienza por un corchete de tipo ej. [Análisis y diseño] o [Desarrollo] o [Pruebas]
	if strings.HasPrefix(raw, "[") {
		idx := strings.Index(raw, "]")
		if idx > 1 {
			bracketContent := raw[1:idx]
			if canon, ok := matchCanonicalType(bracketContent); ok {
				detectedType = canon
			}
		}
	}

	// 2. Si no se detectó en corchetes pero se pasó explicitType
	if detectedType == "" && explicitType != "" {
		if canon, ok := matchCanonicalType(explicitType); ok {
			detectedType = canon
		}
	}

	// 3. Inferencia automática por heurística semántica:
	// Priorizar análisis del título/nombre para evitar que palabras accesorias de la descripción sobreescriban el tipo.
	if detectedType == "" {
		rawNorm := strings.ToLower(raw)
		rawNorm = strings.ReplaceAll(rawNorm, "á", "a")
		rawNorm = strings.ReplaceAll(rawNorm, "é", "e")
		rawNorm = strings.ReplaceAll(rawNorm, "í", "i")
		rawNorm = strings.ReplaceAll(rawNorm, "ó", "o")
		rawNorm = strings.ReplaceAll(rawNorm, "ú", "u")
		rawNorm = strings.ReplaceAll(rawNorm, "ñ", "n")

		if containsAny(rawNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(rawNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "revis", "planificac", "explorac", "research", "benchmark", "inspecc", "arquitect") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(rawNorm, "desarroll", "implementac", "servidor", "server", "systemd", "nginx", "apache", "docker", "deploy", "despliegue", "ssh", "puerto", "backup", "cron", "proxy", "daemon", "demon", "firewall", "sysadmin", "cliente", "usuario", "soporte", "ticket", "reunion", "consulta", "duda", "demo", "capacitacion", "formacion", "funcional", "tarifa", "ajuste", "ajustes", "fix", "bug", "error", "correccion", "corregir", "refactor", "tweak", "patch", "parche", "limpieza", "lint", "linter", "estilo", "padding", "css", "tipografia") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 4. Si el título no arrojó coincidencias, inspeccionar la descripción
	if detectedType == "" && description != "" {
		descNorm := strings.ToLower(description)
		descNorm = strings.ReplaceAll(descNorm, "á", "a")
		descNorm = strings.ReplaceAll(descNorm, "é", "e")
		descNorm = strings.ReplaceAll(descNorm, "í", "i")
		descNorm = strings.ReplaceAll(descNorm, "ó", "o")
		descNorm = strings.ReplaceAll(descNorm, "ú", "u")
		descNorm = strings.ReplaceAll(descNorm, "ñ", "n")

		if containsAny(descNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(descNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "planificac") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(descNorm, "desarroll", "implementac", "servidor", "server", "systemd", "docker", "deploy", "cliente", "ajuste", "fix", "bug") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 5. Fallback por defecto: Desarrollo
	if detectedType == "" {
		detectedType = TaskTypeDesarrollo
	}

	// El nombre de la tarea es exclusivamente el nombre canónico sin prefijos [AGY]
	return detectedType, detectedType
}

// argTexto devuelve el argumento de texto nombre, sin espacios a los lados ("" si falta).
func argTexto(args map[string]interface{}, nombre string) string {
	v, _ := args[nombre].(string)
	return strings.TrimSpace(v)
}

// argNumero devuelve el argumento numérico nombre (JSON lo trae como float64; se aceptan también int y texto).
func argNumero(args map[string]interface{}, nombre string) float64 {
	switch v := args[nombre].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(v, ",", ".")), 64); err == nil {
			return f
		}
	}
	return 0
}

// argEntero es argNumero truncado a entero.
func argEntero(args map[string]interface{}, nombre string) int {
	return int(argNumero(args, nombre))
}
