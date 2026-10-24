// Checks of the token bench (cmd/consumo): each file is copied into a work copy of a bench project and run there
// with go test. Its own module so that planesgo-code's ./... leaves these files out.
module comprobaciones

go 1.22
