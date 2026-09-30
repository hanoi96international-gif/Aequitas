package keeper

import (
	"database/sql"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Der V8-Deploy beim Start einer V8-Kette, gegen eine echte Datenbank und die
// echte EVM des Knotens (ensureV8Deployed). Geprueft wird, was ein Mensch
// danach braucht: der Vertrag steht an der Genesis-Adresse, die Registrare
// stehen im Speicher (DeployContract sichert Mappings nicht von selbst), und
// eine in Go unterschriebene Anfrage kommt im Vertrag bis zur Beweispruefung
// -- also ueber Registrar-, Frist- und Signaturpruefung hinweg.

func v8DBLeeren(t *testing.T) {
	t.Helper()
	truncateDistTestTables(t) // prueft das Opt-in und legt das Schema an
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`TRUNCATE evm_contracts, evm_storage`); err != nil {
		t.Fatalf("evm-Tabellen leeren: %v", err)
	}
}

func TestV8Deploy_RealDB(t *testing.T) {
	v8DBLeeren(t)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	_, reg := v8Schluessel(t, testKeyRegistrar)
	reg2 := common.HexToAddress("0x2000000000000000000000000000000000000002")
	zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
	defer zurueck()
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(reg.Hex()))

	cs := testKnoten(t, "unused-v8-deploy-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	evm, err := NewEVMEngine(cs)
	if err != nil {
		t.Fatal(err)
	}
	deployer := "0x00000000000000000000000000000000000d0e01"
	EnsureContractsDeployed(evm, cs, deployer)

	if got := cs.getConfigValueDB(registerVertragVersionKey); got != V8ContractVersion {
		t.Fatalf("Version nicht gesetzt: %q", got)
	}
	if err := pruefeV8Stand(evm, reg, v8NetzSalt()); err != nil {
		t.Fatalf("V8 steht nicht wie verlangt: %v", err)
	}
	if v := cs.getConfigValueDB("v7_contract_version"); v != "" {
		t.Fatalf("unter V8 darf kein V7-Stand entstehen: %q", v)
	}

	// Eine in Go unterschriebene Anfrage (echte Adresse, echtes Netz-Salt)
	// kommt bis zur Beweispruefung. BioVerifier ist der echte Verifier, der
	// Platzhalter-Beweis faellt dort durch -- und genau dieser Grund zeigt,
	// dass alles davor stimmt.
	v8Addr := common.HexToAddress(V7_CONTRACT_ADDR)
	c, n := big.NewInt(1001), big.NewInt(2001)
	deadline := int64(4_000_000_000) // ausserhalb von "jetzt + 1 Tag"
	_, err = evm.CallContract(reg, v8Addr, v8Anfrage(t, alice, c, n, deadline,
		v8Unterschrift(t, aliceKey, v8RegisterDigest(v8Addr, v8NetzSalt(), alice, c, n, big.NewInt(0), big.NewInt(deadline))), v8TestMarker), big.NewInt(0), false)
	if err == nil || !strings.Contains(err.Error(), "deadline too far") {
		t.Fatalf("Frist ueber einen Tag muss der Vertrag ablehnen: %v", err)
	}
	// Frist innerhalb eines Tages, nach der Uhr des Knotens.
	deadline = time.Now().Unix() + 600
	gute := v8Anfrage(t, alice, c, n, deadline,
		v8Unterschrift(t, aliceKey, v8RegisterDigest(v8Addr, v8NetzSalt(), alice, c, n, big.NewInt(0), big.NewInt(deadline))), v8TestMarker)
	bisZumBeweis := func(von common.Address) bool {
		_, err := evm.CallContract(von, v8Addr, gute, big.NewInt(0), false)
		return err != nil && strings.Contains(err.Error(), "V8: invalid proof")
	}
	keinRegistrar := func(von common.Address) bool {
		_, err := evm.CallContract(von, v8Addr, gute, big.NewInt(0), false)
		return err != nil && strings.Contains(err.Error(), "not a registrar")
	}
	if !bisZumBeweis(reg) {
		t.Fatal("eigener Relayer: erwartet Abbruch erst an der Beweispruefung")
	}
	if !keinRegistrar(reg2) || !keinRegistrar(alice) {
		t.Fatal("nur der eigene Relayer ist Registrar")
	}
	fremd := v8Anfrage(t, alice, c, n, deadline,
		v8Unterschrift(t, aliceKey, v8RegisterDigest(v8Addr, testSalt, alice, c, n, big.NewInt(0), big.NewInt(deadline))), v8TestMarker)
	if _, err := evm.CallContract(reg, v8Addr, fremd, big.NewInt(0), false); err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("Signatur mit fremdem Netz-Salt muss scheitern: %v", err)
	}

	// Ein zweiter Start aendert den Vertrag nicht.
	vorher, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
	EnsureContractsDeployed(evm, cs, deployer)
	nachher, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
	if string(vorher) != string(nachher) || !bisZumBeweis(reg) {
		t.Fatal("zweiter Start hat den Vertrag veraendert")
	}

	// Schluesselwechsel: der neue Relayer wird Registrar, der alte nicht mehr.
	// Genau das konnte die feste Genesis-Liste nie -- ein verlorener
	// Schluessel haette die Registrierung fuer immer gesperrt.
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(reg2.Hex()))
	EnsureContractsDeployed(evm, cs, deployer)
	if !bisZumBeweis(reg2) || !keinRegistrar(reg) {
		t.Fatal("nach dem Schluesselwechsel muss der neue Relayer Registrar sein und der alte nicht")
	}

	// Knoten ohne Relayer: niemand registriert ueber ihn.
	t.Setenv("RELAYER_ADDRESS", "")
	EnsureContractsDeployed(evm, cs, deployer)
	if !keinRegistrar(reg) || !keinRegistrar(reg2) {
		t.Fatal("ohne Relayer darf es keinen Registrar geben, der einreichen kann")
	}
}

func TestV8Deploy_BautKeineV7DatenbankUm_RealDB(t *testing.T) {
	v8DBLeeren(t)
	cs := testKnoten(t, "unused-v8-deploy-v7db-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	evm, err := NewEVMEngine(cs)
	if err != nil {
		t.Fatal(err)
	}
	// Erst eine V7-Kette ...
	func() {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV7})
		defer zurueck()
		EnsureContractsDeployed(evm, cs, "0x00000000000000000000000000000000000d0e01")
	}()
	v7Code, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
	if len(v7Code) == 0 || cs.getConfigValueDB("v7_contract_version") != V7ContractVersion {
		t.Fatal("V7 wurde nicht angelegt")
	}
	// ... dann dieselbe Datenbank unter einer V8-Genesis: nichts anfassen.
	zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
	defer zurueck()
	EnsureContractsDeployed(evm, cs, "0x00000000000000000000000000000000000d0e01")
	nachher, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
	if string(nachher) != string(v7Code) {
		t.Fatal("eine V7-Datenbank darf unter V8 nicht umgebaut werden")
	}
	if cs.getConfigValueDB(registerVertragVersionKey) != "" {
		t.Fatal("V8 darf nicht als angelegt gelten")
	}
}
