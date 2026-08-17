# Launch-Posts — X (@AequitasMoney) und Telegram

Fertige Texte zum Kopieren für den 18.08.2026. Alles hier ist an dem geprüft,
was die Chain heute wirklich tut (README, WHITEPAPER, `docs/LAUNCH_2026-08-18.md`) —
keine Zahl in diesen Posts steht ohne Deckung im Code.

**Reihenfolge am Launch-Tag:** erst die drei Verify-Checks aus
`docs/LAUNCH_2026-08-18.md` (Schritt 4), *dann* posten. Ein Launch-Thread, der
auf eine Domain ohne Zertifikat zeigt, ist der eine Fehler, der sich nicht
zurücknehmen lässt.

Die beiden Kanäle: X `@AequitasMoney`, Telegram `https://t.me/aequitasmoney`.

---

## 1. X-Profil (einmalig einrichten)

**Bio** (max. 160 Zeichen):

```text
Geld, das jedem Menschen gleich gehört. Proof-of-Humanity-Chain, Chain ID 1926. Jeder verifizierte Mensch: 1.000 AEQ. Kein Mining, kein Vorverkauf. Open Source.
```

**Englische Alternative**, falls das Profil auf EN laufen soll:

```text
Money that belongs to every human equally. Proof-of-Humanity chain, Chain ID 1926. Every verified human: 1,000 AEQ. No mining, no presale. Open source.
```

- **Website:** `https://aequitas.digital`
- **Standort:** leer lassen oder „On chain — Chain ID 1926"
- **Angeheftet:** zuerst der Vorstellungspost (Abschnitt 2), ab dem 18.08.
  Post 1/7 des Launch-Threads

---

## 2. Der erste Post — kurze Vorstellung (EN)

Der allererste Post auf dem Account, angepinnt bis der Launch-Thread ihn
ablöst. Kurz, ohne Hook-Rhetorik, sagt was die Sache ist — und schließt mit
dem Satz, der ohnehin im Footer der Landing Page und über dem README steht.
Der Account soll mit demselben Satz anfangen, mit dem die Seite aufhört.

**Achtung Link:** `aequitas.digital` löst erst nach dem DNS-Switch am 18.08.
auf (`docs/LAUNCH_2026-08-18.md`, Schritt 1–4). Wird der Post heute
abgesetzt, Fassung **A ohne Domain** nehmen — ein toter Link im ersten Post
eines Accounts ist teuer, und die Domain steht ohnehin im Profilfeld
„Website".

**A — heute, ohne Domain (272 Zeichen)**

```text
We are Aequitas, a blockchain with one unusual rule: new money is created by exactly one event, a verified human joining. Each one receives 1,000 AEQ. Once.

No mining, no staking, no presale, nothing to buy.

Money exists because people exist. Nothing more, nothing less.
```

**B — ab 18.08., mit Domain (280 Zeichen, X zählt jede URL als 23)**

Um den Link unterzubringen, ohne den Schlusssatz zu verlieren, fällt
„We are" und „no staking" weg. Der Schlusssatz bleibt der Schlusssatz.

```text
Aequitas is a blockchain with one unusual rule: new money is created by exactly one event, a verified human joining. Each one receives 1,000 AEQ. Once.

No mining, no presale, nothing to buy.

aequitas.digital

Money exists because people exist. Nothing more, nothing less.
```

---

## 3. Der zweite Post — das langfristige Ziel und der Alpha-Stand

Kommt nach der Vorstellung. Er sagt, wohin das Ganze soll — und im selben
Atemzug, wo es heute steht.

Das ist keine Bescheidenheitsgeste, sondern das, was das Whitepaper selbst
festhält. §3.2 („Was zum Start läuft", und dieser Abschnitt hat laut eigener
Ansage Vorrang vor allem darüber): Was die Einmaligkeit heute trägt, ist der
Nullifier auf der Kette. Der ist lückenlos — aber er beweist nur, dass
dieselbe *Identitätsquelle* nicht zweimal zählt. Ob diese Quelle ein Mensch
ist oder ein Gerät, entscheidet die Betriebsart: ohne erreichbaren
Koordinator bindet die Anmeldung an ein **Gerät**, und dieselbe Person kann
sich auf einem zweiten Telefon erneut anmelden.

Ein Post, der „ein Mensch, eine Wallet" heute als erledigt verkauft, ist
also am ersten Tag widerlegbar — von jemandem mit zwei Telefonen. Deshalb
steht das Ziel im Futur und der Stand im Präsens.

**Aber die Reihenfolge entscheidet.** Ein Post, der mit der Einschränkung
anfängt, klingt wie eine Entschuldigung für ein kleines Projekt. Das Ziel
ist nicht klein: acht Milliarden Menschen, jeder genau einmal gezählt, jeder
mit demselben Anfangsbetrag. Also zuerst die Größe der Sache, dann der Stand
— und der Schlusssatz trägt die Haltung, nicht die Einschränkung.

**Empfohlen (276 Zeichen)**

```text
Money has never been handed out one equal share per person. That is what we are building: every human on Earth registered exactly once, eight billion of us starting from the same 1,000 AEQ.

This is alpha: two phones can still mean two registrations.

We are early, not small.
```

**Variante, Ziel zuerst benannt (273 Zeichen)**

```text
We are building a currency that counts people, not capital: every human on Earth registered once, with the same 1,000 AEQ to start.

Eight billion, one count each. That is the target.

Today we are alpha — two phones can still mean two registrations. The goal is unchanged.
```

**Variante, nüchterner (275 Zeichen)**

```text
The goal, in full: every human on Earth counted exactly once. Eight billion people, one registration each, all starting from the same 1,000 AEQ.

Today we are alpha — two phones can still mean two registrations.

Big goal, early days. Both true, and we would rather say both.
```

**DE, falls der Account zweisprachig posten soll (279 Zeichen)**

```text
Geld wurde noch nie pro Kopf zu gleichen Teilen ausgegeben. Genau das bauen wir: jeder Mensch auf der Erde einmal registriert, acht Milliarden mit denselben 1.000 AEQ am Start.

Alpha ist es trotzdem: zwei Telefone können heute zwei Registrierungen sein.

Früh dran, nicht klein.
```

---

## 4. Der dritte Post — was läuft, was beweist, was gebaut wird

Drei Sachen sollen hier rein: die Vorteile, der Beweis-Stack von heute, und
woran gearbeitet wird. Das passt nicht in 280 Zeichen, ohne dass alles drei
zur Aufzählung verkommt — also ein kurzer Thread aus drei Posts, plus eine
Ein-Post-Fassung darunter, falls es doch nur einer werden soll.

**Was hier belegt ist und was nicht:** Groth16, circom/snarkjs, BN128,
`BioVerifier.sol` und der keccak256-Nullifier laufen heute — sie stehen in
den technischen Kenndaten des Whitepapers und im deployten Contract
(`0xc369D27b…`). Die MPC-Deduplizierung dagegen ist Code im Repo
(`x/humanity/mpc/`, 1406 Zeilen mit Tests), aber **noch nicht verdrahtet**:
kein Paket außerhalb von `x/humanity/mpc` importiert sie. Deshalb steht sie
im Post unter „arbeiten wir dran" und nicht unter „haben wir" — nachprüfbar
mit einem grep, und genau das wird jemand tun.

**3a — die Vorteile (274 Zeichen)**

```text
What the chain already does: ~1 second blocks, no gas fees, EVM so MetaMask just works, no premine and no team allocation, a wealth cap and demurrage that redistribute instead of burning — and a Gini index the network publishes about itself.

Chain ID 1926, all open source.
```

**3b — was eine Registrierung heute beweist (277 Zeichen)**

```text
What proves a registration today: a Groth16 zk-SNARK, circom and snarkjs, BN128 curve, verified on chain by BioVerifier.sol.

It carries two values: a commitment and a nullifier. The nullifier is claimed with the registration, in the same step. Replay it and the chain refuses.
```

**3c — woran gearbeitet wird (264 Zeichen)**

```text
What we are working on: de-duplication under multi-party computation. Templates are secret-shared across validators and compared by Hamming distance in the field — no single party ever holds a face.

Code and tests are in the repo. Not yet wired into registration.
```

**3d — optional, die Designregel dahinter (261 Zeichen)**

Lohnt sich, weil sie zeigt, dass die Fehlerrate mitgedacht ist statt
verschwiegen. Steht so auch im Code (`OutcomeReview` in `mpc/registry.go`).

```text
One design rule we hold to: a near-match never auto-rejects.

Biometric matching has a false-match rate that is never zero, and a wrongly rejected person would be locked out of a currency whose whole premise is that existing is enough. Close calls go to review.
```

**Ein-Post-Fassung (272 Zeichen)**

```text
Under the hood today: Groth16 zk-SNARKs verified on chain, a keccak256 nullifier that can be claimed exactly once, ~1 second blocks and no gas fees.

Next up: de-duplication under multi-party computation, so uniqueness can be checked without any party ever holding a face.
```

**Ein-Post-Fassung, DE (271 Zeichen)**

```text
Was heute unter der Haube läuft: Groth16-zk-SNARKs, on-chain verifiziert, ein keccak256-Nullifier, der genau einmal gilt, ~1 Sekunde Blockzeit, keine Gas-Gebühren.

Als Nächstes: Dedup per Multi-Party-Computation — Einmaligkeit prüfen, ohne dass jemand ein Gesicht sieht.
```

---

## 5. Launch-Thread (DE) — 18.08.2026

Sieben Posts, als Thread hintereinander. Jeder Post steht auch allein.

**1/7**

```text
Aequitas ist live: eine Blockchain, deren Geldmenge an nachgewiesene menschliche Existenz gebunden ist.

Jeder verifizierte Mensch bekommt 1.000 AEQ. Einmal. Kein Mining, kein Vorverkauf, keine Investition.

Chain ID 1926, EVM-kompatibel, Open Source.

aequitas.digital
```

**2/7**

```text
Das einzige Ereignis, das neues AEQ erzeugt: ein neuer verifizierter Mensch registriert sich.

Kein Mining. Kein Staking. Keine Protokoll-Emissionen. Keine Team-Allokation.

Kommt niemand dazu, entsteht kein Geld. Das ist die gesamte Geldpolitik.
```

**3/7**

```text
Wie die Einmaligkeit funktioniert:

Auf die Chain geht kein Bild und kein biometrischer Datensatz, sondern ein Commitment und ein Nullifier — ein Wert, der genau einmal gilt. Dieselbe Quelle ein zweites Mal: abgelehnt, egal über welchen Weg.

Kryptografie, kein Versprechen.
```

**4/7**

```text
Zwei Regeln halten das Geld verteilt:

Wealth Cap — keine Wallet über dem 25-fachen der Durchschnittsbalance.
Demurrage — 0,5% pro Monat auf alles über dem eigenen fairShare, nach 3 Monaten Karenz.

Beides vernichtet kein AEQ. Beides verteilt es um.
```

**5/7**

```text
Und das Netzwerk benotet sich selbst.

Aequitas misst seinen eigenen Gini-Koeffizienten on-chain und veröffentlicht ihn live. Ziel: unter 0,30 — skandinavisches Niveau.

Man muss das nicht glauben. Explorer aufmachen und die Zahl lesen.
```

**6/7** — der ehrliche Post. Nicht weglassen.

```text
Wo wir an Tag 1 wirklich stehen:

Eine laufende Chain (~1 Sekunde Blockzeit, keine Gas-Gebühren), zwei Validatoren, eine Android-App und 15 verifizierte Menschen.

Mehr nicht. Kein Listing, kein Preis, nichts zu kaufen. Nur Geld, das gleich anfängt.
```

**7/7**

```text
Alles offen einsehbar:

Website & Explorer → aequitas.digital
Code → github.com/hanoi96international-gif/Aequitas
Whitepaper → im Repo
Chat → https://t.me/aequitasmoney

Fragen sind willkommen. Die skeptischen zuerst.
```

---

## 6. Launch thread (EN) — same seven posts

**1/7**

```text
Aequitas is live: a blockchain where the money supply is tied to verified human existence.

Every verified human receives 1,000 AEQ. Once. No mining, no presale, no investment.

Chain ID 1926, EVM compatible, open source.

aequitas.digital
```

**2/7**

```text
The only event that creates new AEQ is a new verified human registering.

No mining. No staking. No protocol emissions. No team allocation.

If nobody joins, no money is created. That is the entire monetary policy.
```

**3/7**

```text
How uniqueness works:

The chain never sees an image or a biometric record. It stores a commitment and a nullifier — a value valid exactly once. Send the same identity source again, by any path, and the chain refuses it.

That part is cryptography, not a promise.
```

**4/7**

```text
Two rules keep the money spread out:

Wealth cap — no wallet above 25x the average balance.
Demurrage — 0.5% per month on whatever you hold above your fair share, after 3 months of grace.

Neither destroys AEQ. Both redistribute it.
```

**5/7**

```text
And the network grades itself.

Aequitas measures its own Gini coefficient on chain and publishes it live. Target: below 0.30 — Scandinavian territory.

You don't have to take the claim on trust. Open the explorer and read the number.
```

**6/7**

```text
Where we actually stand on day one:

A running chain (~1 second blocks, no gas fees), two validators, an Android app, and 15 verified humans.

That's all. No listing, no price, nothing to buy. Just money that starts out equal.
```

**7/7**

```text
Everything is public:

Site & explorer → aequitas.digital
Code → github.com/hanoi96international-gif/Aequitas
Whitepaper → in the repo
Chat → https://t.me/aequitasmoney

Questions welcome. The sceptical ones first.
```

---

## 7. Einzelposts für die ersten Tage

Nicht alles am ersten Tag. Einer pro Tag reicht; jeder erklärt genau einen
Mechanismus und steht für sich.

**Wörgl / Demurrage (DE)**

```text
1932, Wörgl in Tirol: Die Gemeinde gab Geld aus, das an Wert verlor, wenn man es liegen ließ. Es wurde ausgegeben statt gehortet. Die Arbeitslosigkeit fiel in einem Jahr um 25%.

Aequitas macht daraus 0,5% pro Monat — und nur auf das, was über dem fairShare liegt.
```

**Wörgl / demurrage (EN)**

```text
In 1932 the town of Worgl, Austria issued money that lost value if you sat on it. People spent it instead of hoarding it. Unemployment fell 25% in a year.

Aequitas turns that into 0.5% per month — and only on what you hold above your fair share.
```

**Wealth Cap (DE)**

```text
Die Vermögensobergrenze in Aequitas ist keine Zahl, die jemand festlegt.

Sie ist das 25-fache der Durchschnittsbalance — steigt also, wenn alle reicher werden, und fällt, wenn nicht. Kein Admin-Key, kein Governance-Vote. Nur Arithmetik.
```

**Wealth cap (EN)**

```text
The wealth ceiling in Aequitas is not a number somebody picked.

It is 25x the average balance — so it rises when everyone gets richer and falls when they don't. No admin key, no governance vote. Just arithmetic.
```

**Woher das UBI kommt (DE)**

```text
Das Grundeinkommen in Aequitas kommt nicht aus Steuern und nicht aus einer Notenpresse.

Es kommt aus 0,1% Transaktionsgebühr, aus dem, was über der Vermögensobergrenze liegt, und aus der Demurrage. Vier Pools, feste Quoten, keine Abstimmung nötig.
```

**Where the UBI comes from (EN)**

```text
The basic income in Aequitas does not come from taxes, and it does not come from a printing press.

It comes from a 0.1% transaction fee, from balances above the wealth cap, and from demurrage. Four pools, fixed ratios, no vote required.
```

**Biometrie / Datenschutz (DE)**

```text
„Biometrie" klingt nach Überwachung. Was Aequitas wirklich macht:

Gesicht und Handfläche nimmt die Telefonkamera auf. Mehrere unabhängige Vergleichsdienste müssen mehrheitlich zustimmen, bevor eine Anmeldung zählt. Auf die Chain geht nur ein Commitment und ein Nullifier.
```

**Biometrics / privacy (EN)**

```text
"Biometrics" sounds like surveillance. What Aequitas actually does:

Face and palm are captured by your own phone's camera. Several independent matchers must agree by quorum before an enrolment counts. What reaches the chain is a commitment and a nullifier — nothing reversible.
```

**Node betreiben (DE)**

```text
Aequitas läuft auf zwei Validatoren. Das ist wenig, und wir schreiben es lieber hin, als es zu verschweigen.

Wer einen dritten stellen will: Der Node ist Go, die Anleitung liegt als PDF auf der Seite, der Code auf GitHub.

aequitas.digital
```

**Running a node (EN)**

```text
Aequitas runs on two validators. That is few, and we would rather write it down than leave it out.

If you want to run a third: the node is Go, the guide is a PDF on the site, the code is on GitHub.

aequitas.digital
```

---

## 8. Telegram

### Gruppenbeschreibung (max. 255 Zeichen)

```text
Aequitas — die Proof-of-Humanity-Chain. Jeder verifizierte Mensch erhaelt einmalig 1.000 AEQ. Kein Mining, kein Vorverkauf, nichts zu kaufen. Offizielle Gruppe fuer Fragen, Updates und Node-Betrieb. aequitas.digital
```

### Angepinnte Willkommensnachricht

```text
Willkommen bei Aequitas.

Aequitas ist eine Blockchain, in der neues Geld nur durch einen nachgewiesenen neuen Menschen entsteht. Jeder verifizierte Mensch bekommt einmalig 1.000 AEQ. Kein Mining, kein Staking, keine Team-Allokation.

MITMACHEN
1. App laden: aequitas.digital → Download (Android)
2. Identität in der App verifizieren — auf die Chain geht nur ein Commitment und ein Nullifier, kein Bild und kein biometrischer Datensatz
3. Wallet verbinden, registrieren, 1.000 AEQ erhalten

STAND: ALPHA
Die Kette läuft, aber das Projekt ist Alpha, und das langfristige Ziel — ein Mensch, eine Registrierung — trägt heute noch nicht vollständig. Der Nullifier auf der Kette verhindert lückenlos, dass dieselbe Identitätsquelle zweimal zählt. Ob hinter dieser Quelle ein Mensch oder ein Gerät steht, hängt von der Betriebsart ab: ohne erreichbaren Koordinator bindet die Anmeldung an ein Gerät, und dieselbe Person kann sich auf einem zweiten Telefon erneut anmelden. Nachzulesen im Whitepaper, Abschnitt 3.2. Wer hier mitmacht, macht bei einer Alpha mit.

LINKS
Website & Explorer: https://aequitas.digital
Code: https://github.com/hanoi96international-gif/Aequitas
Whitepaper: im Repo (WHITEPAPER.md)
X: https://x.com/AequitasMoney

WICHTIG, BITTE LESEN
AEQ ist nicht kaufbar. Es gibt keinen Vorverkauf, keine Investitionsrunde, kein Listing und keinen Preis. Wer dir hier oder per DM AEQ verkaufen will, betrügt dich.
Das Team schreibt dich nie zuerst per DM an.
Niemand aus dem Team fragt jemals nach deiner Seed-Phrase oder deinem privaten Schlüssel. Niemand. Nie.

REGELN
1. Keine Preis-, Kurs- oder Investment-Diskussionen — es gibt nichts zu handeln
2. Kein Spam, keine Referral-Links, keine anderen Projekte
3. Keine Seed-Phrasen, privaten Schlüssel oder Screenshots davon im Chat
4. Kritik und schwierige Fragen sind ausdrücklich erwünscht — Beleidigungen nicht
```

### Ankündigung am Launch-Tag (in die Gruppe, nach den Verify-Checks)

```text
Die Domain ist live: https://aequitas.digital

Ab jetzt laufen Website, Explorer und RPC unter dem eigenen Namen statt unter IP-Adressen. Die App zeigt auf dieselben drei Endpunkte.

Was ihr jetzt machen könnt:
- Explorer öffnen und die Chain-Höhe und den Gini-Index live nachlesen
- Die Android-App laden und euch registrieren
- Den Node-Guide lesen, wenn ihr einen dritten Validator stellen wollt

Fragen bitte hier in die Gruppe, nicht per DM.
```

---

## 9. Was wir nicht behaupten

Gilt für jeden Post, auch für spontane Antworten in Replies:

- **Kein Preis, keine Prognose, kein Renditeversprechen.** AEQ ist nicht
  handelbar, es gibt kein Listing. Jeder Satz, der wie eine Geldanlage klingt,
  ist falsch und zieht genau die falschen Leute an.
- **Keine Nutzerzahlen aufrunden.** Heute sind es 15 verifizierte Menschen und
  zwei Validatoren. Das ist der Stand; er wird geschrieben wie er ist.
- **Nichts als „fertig" verkaufen, was Roadmap ist.** iOS, Bridges und externe
  DEX-Integration existieren nicht. Der Android-Client und die Chain existieren.
- **Bei Biometrie exakt bleiben.** Gespeichert werden Commitment-Hash und
  Nullifier. Nicht „verschlüsselte Fingerabdrücke", nicht „biometrische Daten
  auf der Chain" — das wäre schlicht falsch beschrieben. Und die Erfassung
  läuft über die Telefonkamera (Gesicht und Handfläche), nicht über den
  Fingerabdrucksensor: Whitepaper §3.2 hat hier Vorrang vor der älteren
  Beschreibung im README.
- **„Ein Mensch, eine Wallet" nicht im Präsens behaupten.** Das ist das Ziel,
  nicht der heutige Stand. Was heute gilt: dieselbe Identitätsquelle zählt nie
  zweimal. Ohne erreichbaren Koordinator ist diese Quelle aber das Gerät, nicht
  der Mensch — zwei Telefone, zwei Registrierungen. Wer den Satz trotzdem im
  Präsens postet, wird am ersten Tag von jemandem mit zwei Telefonen widerlegt.
- **Alpha nennen, solange es Alpha ist.** Die Kette läuft echt, mit echten
  Guthaben — aber sie ist nicht fertig, und der Unterschied gehört in jeden
  Post, der neue Leute holt.
- **UBI-Zahlungen nicht ankündigen, solange die Pools leer sind.** Bei fünfzehn
  Wallets ohne Handel entstehen fast keine Gebühren, also wird fast nichts
  ausgeschüttet. Das ist erwartetes Verhalten einer ruhigen Chain — aber wer
  „täglich UBI" verspricht, produziert genau die Enttäuschung, die das Projekt
  nicht braucht.
