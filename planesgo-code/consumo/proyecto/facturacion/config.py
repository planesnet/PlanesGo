import json
import os

_RUTA = os.path.join(os.path.dirname(os.path.dirname(__file__)), "config.json")


def carga(ruta=_RUTA):
    with open(ruta, encoding="utf-8") as f:
        return json.load(f)


def decimales():
    return carga().get("decimales", 2)
