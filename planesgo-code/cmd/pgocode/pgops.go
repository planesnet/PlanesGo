package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// «pgo ps»: remote PowerShell (WinRM) with the connection of the agent's session (internal/servidor/winrmconn.go).

// maxPsTransfer: the largest file «pgo ps sube» sends (internal/winrm.MaxTransfer).
const maxPsTransfer = 4 << 20

// psRequest: what «pgo ps …» asks the backend: the order, its body and, for a download, the local file.
type psRequest struct {
	endpoint string
	body     map[string]any
	local    string
}

// psArgs reads «pgo ps "<PowerShell>" | -f <fichero.ps1> [--escribir]», «pgo ps baja <remoto> <local>» and
// «pgo ps sube <local> <remoto> --escribir».
func psArgs(args []string) (psRequest, error) {
	write := false
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--escribir":
			write = true
		case a == "-f":
			if i+1 >= len(args) {
				return psRequest{}, errors.New("falta el fichero de -f")
			}
			i++
			b, err := os.ReadFile(args[i])
			if err != nil {
				return psRequest{}, err
			}
			rest = append(rest, string(b))
		case strings.HasPrefix(a, "--"):
			return psRequest{}, fmt.Errorf("opción desconocida %s", a)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 && (rest[0] == "baja" || rest[0] == "sube") {
		if len(rest) != 3 {
			return psRequest{}, fmt.Errorf("uso: pgo ps baja <remoto> <local> | pgo ps sube <local> <remoto> --escribir")
		}
		if rest[0] == "baja" {
			return psRequest{endpoint: "ps-download", body: map[string]any{"path": rest[1]}, local: rest[2]}, nil
		}
		data, err := readLimited(rest[1], maxPsTransfer)
		if err != nil {
			return psRequest{}, err
		}
		return psRequest{endpoint: "ps-upload", body: map[string]any{"path": rest[2], "data": base64.StdEncoding.EncodeToString(data), "escribir": write}}, nil
	}
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return psRequest{}, errors.New(`una sola orden (entre comillas) o -f <fichero.ps1>: pgo ps "Get-Service" o pgo ps -f script.ps1`)
	}
	return psRequest{endpoint: "ps", body: map[string]any{"script": rest[0], "escribir": write}}, nil
}

func readLimited(path string, max int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > max {
		return nil, fmt.Errorf("%s es demasiado grande (%d bytes; máximo %d)", path, info.Size(), max)
	}
	return os.ReadFile(path)
}

// psResult: the answer of /api/pgo/ps.
type psResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int `json:"exit_code"`
	Truncated bool
	ReadOnly  bool `json:"read_only"`
}

// psText: the output for the agent, compact: without trailing blank lines, the errors after it and a last line
// only when something is worth saying (exit code, truncated output).
func psText(r psResult) (stdout, stderr string) {
	out := strings.TrimRight(strings.ReplaceAll(r.Stdout, "\r\n", "\n"), " \t\n")
	if out != "" {
		out += "\n"
	}
	errText := strings.TrimRight(strings.ReplaceAll(r.Stderr, "\r\n", "\n"), " \t\n")
	if errText != "" {
		errText += "\n"
	}
	var notes []string
	if r.ExitCode != 0 {
		notes = append(notes, fmt.Sprintf("código de salida %d", r.ExitCode))
	}
	if r.Truncated {
		notes = append(notes, "salida recortada a 1 MiB: filtra la orden (Select-Object, -First…)")
	}
	if len(notes) > 0 {
		errText += "(" + strings.Join(notes, "; ") + ")\n"
	}
	return out, errText
}

// psOutput shows the answer of «pgo ps» (or saves the downloaded file) and returns the exit code of pgo.
func psOutput(req psRequest, raw []byte) int {
	switch req.endpoint {
	case "ps-download":
		var t struct {
			Data  string
			Bytes int64
		}
		json.Unmarshal(raw, &t)
		data, err := base64.StdEncoding.DecodeString(t.Data)
		if err == nil {
			err = os.WriteFile(req.local, data, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgo ps:", err)
			return 1
		}
		fmt.Printf("%s (%d bytes)\n", req.local, len(data))
	case "ps-upload":
		var t struct {
			Path  string
			Bytes int64
		}
		json.Unmarshal(raw, &t)
		fmt.Printf("%s (%d bytes)\n", t.Path, t.Bytes)
	default:
		var r psResult
		json.Unmarshal(raw, &r)
		out, errText := psText(r)
		fmt.Print(out)
		fmt.Fprint(os.Stderr, errText)
		if r.ExitCode != 0 {
			return 1
		}
	}
	return 0
}
