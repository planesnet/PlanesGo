//go:build windows

package proceso

func grupoVivo(int) bool { return false }

func GrupoVivo(int) bool   { return false }
func SenalGrupo(int, bool) {}
