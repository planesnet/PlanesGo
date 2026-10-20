package studiotv

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"
)

// TokenAleatorio genera un token de 144 bits en hexadecimal.
func TokenAleatorio() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// autorizado compara en tiempo constante el token de la cabecera Authorization: Bearer o del parámetro ?t=.
func autorizado(r *http.Request, token string) bool {
	t := r.URL.Query().Get("t")
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		t = strings.TrimPrefix(a, "Bearer ")
	}
	return token != "" && t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1
}

// CertificadoPropio genera un certificado autofirmado (ECDSA P-256, un año) para los nombres e IP indicados.
func CertificadoPropio(hosts []string) (tls.Certificate, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return tls.Certificate{}, err
	}
	tpl := x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: "Studio TV"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if h != "" {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &k.PublicKey, k)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, nil
}
