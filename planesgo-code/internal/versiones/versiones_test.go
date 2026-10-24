package versiones

import "testing"

func TestMajorAndCompatible(t *testing.T) {
	for v, want := range map[string]string{"0.7.0": "0.7", "0.7.12": "0.7", "v1.2.3": "1.2", "dev": "", "": "", "0.7": "", "0.x.1": ""} {
		if got := Major(v); got != want {
			t.Errorf("Major(%q) = %q, want %q", v, got, want)
		}
	}
	for _, c := range []struct {
		client, server string
		ok             bool
	}{
		{"0.7.0", "0.7.3", true}, // minor versions may differ
		{"0.7.9", "0.8.0", false},
		{"0.6.2", "0.7.0", false},
		{"dev", "0.7.0", true},
		{"0.7.0", "dev", true},
		{"", "0.7.0", true},
	} {
		if Compatible(c.client, c.server) != c.ok {
			t.Errorf("Compatible(%q, %q) != %v", c.client, c.server, c.ok)
		}
	}
}
