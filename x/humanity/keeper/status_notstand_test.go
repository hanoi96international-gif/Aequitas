package keeper

import "testing"

// Die Notantwort von /api/status muss die Angaben tragen, an denen die App
// das Netz prueft -- sonst bricht eine Registrierung kurz nach einem Neustart
// nach der Gesichtspruefung ab (Befund 01.10.2026).
func TestStatusNotstand_TraegtNetzangaben(t *testing.T) {
	s := statusNotstand(42)
	if s["chain_evm_id"] != 1926 {
		t.Fatalf("chain_evm_id fehlt/falsch: %v", s["chain_evm_id"])
	}
	if k, _ := s["netz_kennung"].(string); k == "" || k != netzKennung() {
		t.Fatalf("netz_kennung fehlt/falsch: %v", s["netz_kennung"])
	}
	if v, _ := s["register_vertrag"].(string); v != vertragVersion() {
		t.Fatalf("register_vertrag fehlt/falsch: %v", s["register_vertrag"])
	}
	if s["height"] != int64(42) || s["stand_veraltet"] != true {
		t.Fatalf("Hoehe/Vermerk falsch: %v", s)
	}
}
