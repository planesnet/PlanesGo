package studiotv

import _ "embed"

// EmisorPython es el cliente que usan las pruebas para emitir (PCO_URL, PCO_TOKEN, PCO_CANAL).
// Copia de psfcode/studio-tv/clients/python/pco_emisor.py.
//
//go:embed pco_emisor.py
var EmisorPython []byte
