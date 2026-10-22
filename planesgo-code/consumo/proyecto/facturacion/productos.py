from dataclasses import dataclass

TIPOS_IVA = {"general": 21.0, "reducido": 10.0, "superreducido": 4.0, "exento": 0.0}


@dataclass
class Producto:
    codigo: str
    nombre: str
    precio: float
    tipo_iva: str = "general"

    def porcentaje_iva(self):
        return TIPOS_IVA[self.tipo_iva]
