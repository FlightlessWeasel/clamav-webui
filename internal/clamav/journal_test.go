package clamav

import "testing"

func TestParseOnAccessFound(t *testing.T) {
	cases := []struct {
		line      string
		path, sig string
		ok        bool
	}{
		{"ScanOnAccess: /home/u/eicar.com: Win.Test.EICAR_HDB-1 FOUND", "/home/u/eicar.com", "Win.Test.EICAR_HDB-1", true},
		{"/srv/data/bad.bin: Some.Sig(abc:1) FOUND", "/srv/data/bad.bin", "Some.Sig(abc:1)", true},
		{"Tue Sep 20 08:15:11 2024 -> /x/y.txt: Eicar-Test-Signature FOUND", "/x/y.txt", "Eicar-Test-Signature", true},
		{"/srv/data/clean.txt: OK", "", "", false},
		{"SelfCheck: Database status OK.", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		p, s, ok := ParseOnAccessFound(c.line)
		if ok != c.ok || p != c.path || s != c.sig {
			t.Errorf("ParseOnAccessFound(%q) = (%q,%q,%v), want (%q,%q,%v)", c.line, p, s, ok, c.path, c.sig, c.ok)
		}
	}
}
