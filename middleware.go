package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

var (
	indexTmplOnce   sync.Once
	cachedIndexTmpl *template.Template
	cachedIndexErr  error

	expressStandaloneOnce sync.Once
	cachedExpressTmpl     *template.Template
	cachedExpressErr      error
)

func getIndexTemplate() (*template.Template, error) {
	if os.Getenv("ENV") == "development" {
		t, err := template.ParseFiles("templates/index.html")
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseGlob("templates/partials/*.html"); err != nil {
			log.Printf("[WARN] Error cargando plantillas parciales: %v", err)
		}
		return t, nil
	}

	indexTmplOnce.Do(func() {
		t, err := template.ParseFiles("templates/index.html")
		if err != nil {
			cachedIndexErr = err
			return
		}
		if _, err := t.ParseGlob("templates/partials/*.html"); err != nil {
			log.Printf("[WARN] Error cargando plantillas parciales: %v", err)
		}
		cachedIndexTmpl = t
	})
	return cachedIndexTmpl, cachedIndexErr
}

func getExpressStandaloneTemplate() (*template.Template, error) {
	if os.Getenv("ENV") == "development" {
		t, err := template.ParseFiles("templates/express_standalone.html")
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseGlob("templates/partials/*.html"); err != nil {
			log.Printf("[WARN] Error cargando plantillas parciales para express: %v", err)
		}
		return t, nil
	}

	expressStandaloneOnce.Do(func() {
		t, err := template.ParseFiles("templates/express_standalone.html")
		if err != nil {
			cachedExpressErr = err
			return
		}
		if _, err := t.ParseGlob("templates/partials/*.html"); err != nil {
			log.Printf("[WARN] Error cargando plantillas parciales para express: %v", err)
		}
		cachedExpressTmpl = t
	})
	return cachedExpressTmpl, cachedExpressErr
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *statusResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *statusResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func loggingAndRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			if rec := recover(); rec != nil {
				rw.statusCode = http.StatusInternalServerError
				stack := debug.Stack()
				log.Printf("[PANIC CRÍTICO] %s %s: %v\nStack:\n%s", r.Method, r.URL.Path, rec, string(stack))
				http.Error(rw, "500 Internal Server Error", http.StatusInternalServerError)
			}
			duration := time.Since(start)
			clientIP := r.Header.Get("X-Forwarded-For")
			if clientIP == "" {
				clientIP = r.RemoteAddr
			}
			log.Printf("[HTTP] %s %s %s | Status: %d (%d bytes) | Duración: %v | IP: %s | UA: %s",
				r.Method, r.URL.Path, r.Proto, rw.statusCode, rw.bytesWritten, duration, clientIP, r.UserAgent())
		}()

		next.ServeHTTP(rw, r)
	})
}
