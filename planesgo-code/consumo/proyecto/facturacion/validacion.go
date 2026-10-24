package facturacion

import (
	"regexp"
	"strconv"
	"strings"
)

var patronNIF = regexp.MustCompile(`^[0-9]{8}[A-Z]$|^[A-Z][0-9]{7}[A-Z0-9]$`)

const letrasNIF = "TRWAGMYFPDXBNJZSQVHLCKE"

// NIFValido comprueba el formato de un NIF y, si es de persona física, su letra.
func NIFValido(nif string) bool {
	nif = strings.ToUpper(strings.TrimSpace(nif))
	if !patronNIF.MatchString(nif) {
		return false
	}
	if nif[0] >= '0' && nif[0] <= '9' {
		n, _ := strconv.Atoi(nif[:8])
		return letrasNIF[n%23] == nif[8]
	}
	return true
}
