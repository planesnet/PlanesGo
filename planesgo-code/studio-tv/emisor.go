package studiotv

import _ "embed"

// EmisorPython es el cliente que usan las pruebas para emitir (PCO_URL, PCO_TOKEN, PCO_CANAL, o
// ~/.planesgo/studio_tv.env). PlanesGo lo sirve en /install/pco_emisor.py.
//
//go:embed clients/python/pco_emisor.py
var EmisorPython []byte
