from dataclasses import dataclass

from .productos import Producto
from .redondeo import redondea


@dataclass
class Linea:
    producto: Producto
    cantidad: float
    descuento: float = 0.0  # porcentaje (0-100)

    def bruto(self):
        return self.producto.precio * self.cantidad

    def subtotal(self):
        """Base imponible de la línea: el bruto menos el descuento (en porcentaje)."""
        return redondea(self.bruto() - self.descuento)

    def porcentaje_iva(self):
        return self.producto.porcentaje_iva()
