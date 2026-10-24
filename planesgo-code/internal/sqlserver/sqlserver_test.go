package sqlserver

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	ok := Config{Server: "192.168.125.229", User: "sa", Password: "x"}
	for _, c := range []Config{ok, {Server: `SAGE\SQLEXPRESS`, User: `MAZANA\luis`, Password: "x"}, {Server: "sql.local:1433", Database: "Sage 200", User: "u", Password: "x"}} {
		if err := c.Validate(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
	for _, c := range []Config{
		{Server: "host;Password=x", User: "u", Password: "x"},
		{Server: "-o", User: "u", Password: "x"},
		{Server: "h", User: "u;Encrypt=false", Password: "x"},
		{Server: "h", Database: "a;b", User: "u", Password: "x"},
		{Server: "h", User: "u"},
	} {
		if c.Validate() == nil {
			t.Errorf("%+v should be refused", c)
		}
	}
}

func TestDSN(t *testing.T) {
	c := Config{Server: `SAGE\SQL2019:1500`, Database: "Sage", User: `MAZANA\luis`, Password: "p@ss:w/rd?", TrustCertificate: true}
	u, err := url.Parse(c.dsn())
	if err != nil {
		t.Fatal(err)
	}
	pw, _ := u.User.Password()
	q := u.Query()
	if u.Host != "SAGE:1500" || u.Path != "/SQL2019" || u.User.Username() != `MAZANA\luis` || pw != "p@ss:w/rd?" ||
		q.Get("database") != "Sage" || q.Get("TrustServerCertificate") != "true" || q.Get("encrypt") != "true" {
		t.Errorf("dsn: %s", c.dsn())
	}
	if strings.Contains(c.String(), "p@ss") || !c.Windows() {
		t.Errorf("String/Windows: %s", c)
	}
}

func TestReadOnlyCheck(t *testing.T) {
	for _, b := range []string{"SELECT * FROM Clientes", "SELECT 'COMMIT' AS x -- ROLLBACK\n", "SELECT [commit] FROM t /* begin tran */", "UPDATE t SET a = 1"} {
		if why := ReadOnlyCheck(b); why != "" {
			t.Errorf("%q: %s", b, why)
		}
	}
	for _, b := range []string{"DELETE FROM t; COMMIT", "begin tran; update t set a=1", "SAVE TRANSACTION x", "rollback", "BEGIN DISTRIBUTED TRANSACTION"} {
		if ReadOnlyCheck(b) == "" {
			t.Errorf("%q must be refused when read only", b)
		}
	}
}

func TestPlain(t *testing.T) {
	when := time.Date(2026, 10, 24, 13, 5, 0, 0, time.UTC)
	cases := []struct {
		v    any
		typ  string
		want any
	}{
		{[]byte("12.50"), "DECIMAL", "12.50"},
		{[]byte("Mazana"), "VARCHAR", "Mazana"},
		{[]byte{0xff, 0x00}, "VARBINARY", "0xFF00"},
		{when, "DATETIME", "2026-10-24T13:05:00"},
		{int64(7), "INT", int64(7)},
		{nil, "INT", nil},
	}
	for _, c := range cases {
		if got := plain(c.v, c.typ); got != c.want {
			t.Errorf("plain(%v, %s) = %v, want %v", c.v, c.typ, got, c.want)
		}
	}
}
