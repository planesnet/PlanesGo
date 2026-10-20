// Package google: acceso a Vertex AI con las credenciales por defecto de gcloud del usuario (el token se pide a
// «gcloud auth application-default print-access-token» y se guarda 45 minutos) y el proyecto de Google Cloud.
package google

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	mu    sync.Mutex
	token string
	hasta time.Time
)

// Token devuelve un token de acceso de Google.
func Token() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if token != "" && time.Now().Before(hasta) {
		return token, nil
	}
	ctx, cancela := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancela()
	b, err := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token").Output()
	if err != nil {
		return "", err
	}
	token, hasta = strings.TrimSpace(string(b)), time.Now().Add(45*time.Minute)
	return token, nil
}

// Proyecto: GOOGLE_CLOUD_PROJECT o planes-dev.
func Proyecto() string {
	if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		return p
	}
	return "planes-dev"
}
