from dataclasses import dataclass


@dataclass
class Cliente:
    nif: str
    nombre: str
    pais: str = "ES"
    recargo_equivalencia: bool = False
    retencion_irpf: float = 0.0

    def es_nacional(self):
        return self.pais == "ES"

    def es_intracomunitario(self):
        return self.pais in {"DE", "FR", "IT", "PT", "NL", "BE", "AT", "IE"}
