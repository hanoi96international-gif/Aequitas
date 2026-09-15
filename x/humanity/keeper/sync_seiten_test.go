package keeper

import "testing"

// 15.09.2026: eine vom Server nach Bytes gekuerzte Seite (X-Blocks-Truncated)
// galt als "Spitze erreicht" -- bei Bloecken aus einem Lastlauf deckt eine
// 12-MB-Seite gerade den 20-Hoehen-Ueberlapp, der Zeiger kam nie vom Fleck:
// 149 GB in 8,8 Stunden Leerlauf, je Box. Eine gekuerzte Seite ist nie die
// letzte.
func TestSeiteWarDieLetzte_GekuerzteSeiteIstNieDieLetzte(t *testing.T) {
	const pageSize = 500
	if !seiteWarDieLetzte(37, pageSize, false, false, false) {
		t.Fatal("kurze, ungekuerzte Seite: Spitze erreicht")
	}
	if seiteWarDieLetzte(37, pageSize, false, false, true) {
		t.Fatal("gekuerzte Seite: es muss weitergeblaettert werden")
	}
	if seiteWarDieLetzte(pageSize, pageSize, false, false, false) {
		t.Fatal("volle Seite: nie die letzte")
	}
	if seiteWarDieLetzte(37, pageSize, true, false, false) {
		t.Fatal("Tiefenlauf: kurze Seite ist nicht die letzte")
	}
	if seiteWarDieLetzte(20, pageSize, false, true, false) {
		t.Fatal("Rueckfall auf kleine Seite: kurz, weil so bestellt")
	}
}

// Ein gestrippter Block, den dieser Knoten schon im Speicher hat, bekommt
// keinen Rumpf nachgeladen -- der Ueberlapp besteht fast nur aus solchen.
func TestSchonBekannt_SpeicherDAG(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{"0xabc": {Hash: "0xabc", Height: 5}}}
	if !dag.schonBekannt(&Block{Hash: "0xabc", Height: 5}) {
		t.Fatal("im Speicher-DAG: bekannt")
	}
	if dag.schonBekannt(&Block{Hash: "0xdef", Height: 5}) {
		t.Fatal("nicht im Speicher, kein Zustand: unbekannt -- der Rumpf muss geholt werden")
	}
	if dag.schonBekannt(nil) || dag.schonBekannt(&Block{}) {
		t.Fatal("nil oder ohne Hash: unbekannt")
	}
}

// 15.09.2026, zweiter Mechanismus: eine Seite voller BEKANNTER Bloecke
// (der Ueberlapp) beendete den Zyklus wie eine Seite voller Fremdlinge --
// der Zeiger blieb 13 Zyklen an derselben Hoehe stehen und fiel 376 Hoehen
// hinter die Spitze.
func TestZyklusStoppenNachSeite_BekanntIstFortschritt(t *testing.T) {
	if zyklusStoppenNachSeite(0, 37, 37, false) {
		t.Fatal("eine Seite, die nur Bekanntes traegt, darf den Zyklus nicht beenden")
	}
	if !zyklusStoppenNachSeite(0, 0, 37, false) {
		t.Fatal("eine Seite ohne Bekanntes und ohne Anfuegbares ist die Fork-Signatur: stoppen")
	}
	if !zyklusStoppenNachSeite(0, 20, 37, false) {
		t.Fatal("teils bekannt, Rest nicht anfuegbar: stoppen")
	}
	if zyklusStoppenNachSeite(1, 0, 37, false) {
		t.Fatal("etwas angefuegt: weiter")
	}
	if zyklusStoppenNachSeite(0, 0, 37, true) {
		t.Fatal("im Tiefenlauf wird nie wegen einer leeren Seite gestoppt")
	}
}
