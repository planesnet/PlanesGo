package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// La cookie de sesión contiene credenciales de Odoo, por lo que se cifra y autentica con AES-GCM:
// un cliente no puede leerla ni fabricar una sesión de otro usuario.

const sessionSecretFile = "data/session_secret"

var (
	sessionAEAD     cipher.AEAD
	sessionAEADOnce sync.Once
	sessionAEADErr  error
)

// initSessionCipher configura la clave a partir de SESSION_SECRET o, si no existe, de un secreto
// aleatorio persistido en data/session_secret para que las sesiones sobrevivan a los reinicios.
func initSessionCipher(secret, secretPath string) error {
	sessionAEADOnce.Do(func() {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			secret, sessionAEADErr = loadOrCreateSecret(secretPath)
			if sessionAEADErr != nil {
				return
			}
		}
		sessionAEAD, sessionAEADErr = newSessionAEAD(secret)
	})
	return sessionAEADErr
}

func loadOrCreateSecret(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return strings.TrimSpace(string(data)), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("no se pudo generar el secreto de sesión: %w", err)
	}
	secret := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("no se pudo crear el directorio del secreto de sesión: %w", err)
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return "", fmt.Errorf("no se pudo guardar el secreto de sesión: %w", err)
	}
	log.Printf("[SEGURIDAD] Secreto de sesión generado en %s", path)
	return secret, nil
}

func newSessionAEAD(secret string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// getSessionAEAD devuelve el cifrador; si main no lo inicializó (p. ej. en tests) usa una clave efímera.
func getSessionAEAD() (cipher.AEAD, error) {
	sessionAEADOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			sessionAEADErr = err
			return
		}
		sessionAEAD, sessionAEADErr = newSessionAEAD(hex.EncodeToString(buf))
	})
	if sessionAEADErr != nil {
		return nil, sessionAEADErr
	}
	if sessionAEAD == nil {
		return nil, errors.New("cifrador de sesión no inicializado")
	}
	return sessionAEAD, nil
}

func sealSession(plain []byte) ([]byte, error) {
	aead, err := getSessionAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, []byte(sessionCookieName)), nil
}

func openSession(sealed []byte) ([]byte, error) {
	aead, err := getSessionAEAD()
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("cookie de sesión inválida")
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	return aead.Open(nil, nonce, ciphertext, []byte(sessionCookieName))
}
