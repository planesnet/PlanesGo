// firmar-version firma con ed25519 lo que anuncia version.json, para que las apps solo instalen versiones
// publicadas por quien tiene la clave privada (la pública va dentro de pgocode, en internal/actualizacion).
//
//	firmar-version -generar -clave <fichero>   crea la clave privada (PEM, 0600) y muestra la pública
//	firmar-version -clave <fichero> <version.json>   muestra la firma (base64) de ese version.json
//	firmar-version -publica -clave <fichero>   muestra la clave pública
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"planesgo-code/internal/actualizacion"
)

func main() {
	clave := flag.String("clave", "", "fichero de la clave privada (PEM PKCS#8)")
	generar := flag.Bool("generar", false, "crea la clave privada en -clave (no sobrescribe)")
	publica := flag.Bool("publica", false, "muestra la clave pública de -clave")
	flag.Parse()
	if err := ejecuta(*clave, *generar, *publica, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "firmar-version:", err)
		os.Exit(1)
	}
}

func ejecuta(clave string, generar, publica bool, args []string) error {
	if clave == "" {
		return errors.New("falta -clave")
	}
	if generar {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(clave), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(clave, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if err := pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		return nil
	}
	priv, err := lee(clave)
	if err != nil {
		return err
	}
	if publica {
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
		return nil
	}
	if len(args) != 1 {
		return errors.New("uso: firmar-version -clave <fichero> <version.json>")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	var p actualizacion.Publicada
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	if !priv.Public().(ed25519.PublicKey).Equal(actualizacion.ClavePublica()) {
		return errors.New("la clave no corresponde a la pública que lleva pgocode (internal/actualizacion): las apps rechazarían la firma")
	}
	fmt.Println(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, actualizacion.MensajeFirmado(p))))
	return nil
}

func lee(fichero string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(fichero)
	if err != nil {
		return nil, fmt.Errorf("no se puede leer la clave de firma: %w", err)
	}
	blq, _ := pem.Decode(b)
	if blq == nil || blq.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%s: no es una clave PEM «PRIVATE KEY»", fichero)
	}
	k, err := x509.ParsePKCS8PrivateKey(blq.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fichero, err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: no es una clave ed25519", fichero)
	}
	return priv, nil
}
