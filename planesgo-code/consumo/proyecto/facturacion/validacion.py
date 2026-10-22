import re

_NIF = re.compile(r"^[0-9]{8}[A-Z]$|^[A-Z][0-9]{7}[A-Z0-9]$")
_LETRAS = "TRWAGMYFPDXBNJZSQVHLCKE"


def nif_valido(nif):
    nif = (nif or "").upper().strip()
    if not _NIF.match(nif):
        return False
    if nif[0].isdigit():
        return _LETRAS[int(nif[:8]) % 23] == nif[8]
    return True
