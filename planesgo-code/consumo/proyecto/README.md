# facturacion-demo

Pequeña librería de facturación en Go: clientes, productos, líneas, impuestos (IVA, IRPF, recargo de equivalencia),
totales, numeración y exportación.

- `facturacion/`: clientes, productos, líneas, redondeo, fechas, numeración, validación y configuración.
- `facturacion/impuestos/`: IVA, recargo de equivalencia y retenciones de IRPF.
- `facturacion/factura/`: totales, exportación a CSV e informes.

Pruebas: `go test ./...`
