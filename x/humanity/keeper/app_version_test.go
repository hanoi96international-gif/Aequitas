package keeper

import "testing"

// Nur eine Versionsnummer in strenger Form kommt durch -- nie ein beliebiger
// Text von aussen in die App.
func TestAppVersionAusAntwort(t *testing.T) {
	for roh, will := range map[string]string{
		`{"tag_name":"v1.16.0"}`:         "1.16.0",
		`{"tag_name":"1.2.3"}`:           "1.2.3",
		`{"tag_name":"v1.16.0-rc1"}`:     "",
		`{"tag_name":"<script>"}`:        "",
		`{"tag_name":"v99999.0.0"}`:      "",
		`{"tag_name":""}`:                "",
		`kaputt`:                         "",
		`{"tag_name":"v1.2.3 ; rm -rf"}`: "",
	} {
		if got := appVersionAusAntwort([]byte(roh)); got != will {
			t.Errorf("%s: %q, erwartet %q", roh, got, will)
		}
	}
}
