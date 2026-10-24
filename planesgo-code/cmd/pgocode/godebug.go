// SQL Server makes its own certificate with a negative serial number, which Go refuses to parse since 1.23: without
// this, no connection to a SQL Server that has no certificate of its own (Sage's, the local container) gets past TLS.

//go:debug x509negativeserial=1
package main
