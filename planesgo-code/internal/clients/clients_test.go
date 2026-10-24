package clients

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	r := &Registry{File: filepath.Join(dir, "clientes.json"), SecretFile: filepath.Join(dir, "emparejar")}
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPairCheckRevoke(t *testing.T) {
	r := newRegistry(t)
	secret, err := ReadSecret(r.SecretFile)
	if err != nil || len(secret) != 64 {
		t.Fatalf("secret: %q %v", secret, err)
	}
	if st, _ := os.Stat(r.SecretFile); st.Mode().Perm() != 0o600 {
		t.Errorf("secret file mode %v", st.Mode().Perm())
	}
	if _, _, err := r.Pair("otro", "x"); err != ErrBadSecret {
		t.Errorf("a wrong secret pairs: %v", err)
	}
	id, token, err := r.Pair(secret, "pcluis · luis")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Check(token); !ok || got != id {
		t.Errorf("check: %q %v", got, ok)
	}
	if _, ok := r.Check("no"); ok {
		t.Error("an unknown token passes")
	}
	// Only the hash is kept, on disk and in the list.
	b, _ := os.ReadFile(r.File)
	if strings.Contains(string(b), token) {
		t.Error("the token is stored")
	}
	if l := r.List(); len(l) != 1 || l[0].Hash != "" || l[0].Name != "pcluis · luis" {
		t.Errorf("list: %+v", l)
	}
	// Another server start: new secret, same clients.
	r2 := &Registry{File: r.File, SecretFile: r.SecretFile}
	if err := r2.Init(); err != nil {
		t.Fatal(err)
	}
	if s2, _ := ReadSecret(r.SecretFile); s2 == secret {
		t.Error("the pairing secret must change on each start")
	}
	if _, ok := r2.Check(token); !ok {
		t.Error("a paired client keeps working after a restart")
	}
	if err := r2.Revoke(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.Check(token); ok {
		t.Error("a revoked client must not pass")
	}
}

func TestLastSeen(t *testing.T) {
	r := newRegistry(t)
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { return now }
	secret, _ := ReadSecret(r.SecretFile)
	_, token, _ := r.Pair(secret, "a")
	now = now.Add(2 * time.Minute)
	r.Check(token)
	if l := r.List(); !l[0].LastSeen.Equal(now) {
		t.Errorf("last seen %v", l[0].LastSeen)
	}
}
