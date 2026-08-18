// Landing page live data.
//
// Everything drawn here comes from this node's own endpoints. Nothing is
// illustrative: the Lorenz curve is computed from the actual registered
// balances, and if that data cannot be loaded the curve is hidden rather than
// replaced with a plausible-looking shape. A page whose whole argument is
// "we measure our own inequality and publish it" cannot afford a decorative
// chart.

const fmt = (n, d = 0) => n.toLocaleString(undefined, {minimumFractionDigits: d, maximumFractionDigits: d});

function setText(id, value) {
  const el = document.getElementById(id);
  if (el) el.textContent = value;
}

// The GHOSTDAG badge in the header reports whether this page can actually
// reach the node, not whether someone remembered to write "healthy" into the
// markup. loadStats already fetches /api/status every cycle, so the badge
// rides on a request that has to happen anyway: it goes red the moment that
// request stops coming back, which is the only state worth showing.
function setHealthBadge(ok, note) {
  const el = document.getElementById('health-badge');
  if (!el) return;
  el.classList.toggle('badge-health-healthy', ok);
  el.classList.toggle('badge-health-unhealthy', !ok);
  el.textContent = ok ? '● GHOSTDAG' : '● NO NODE';
  el.title = note;
}

async function loadStats() {
  try {
    const d = await fetch('/api/status').then(r => r.json());
    setHealthBadge(true, 'Node answering — height ' + (d.height !== undefined ? d.height.toLocaleString() : '?'));

    if (d.total_humans !== undefined) setText('stat-humans', d.total_humans.toLocaleString());
    if (d.total_supply) setText('stat-supply', d.total_supply.replace(' AEQ', ''));
    if (d.height !== undefined) setText('stat-blocks', d.height.toLocaleString());

    if (typeof d.gini === 'number') {
      const g = d.gini.toFixed(4);
      setText('stat-gini', g);
      setText('gini-inline', g);
      // The static comparison bars in landing.go are sized as gini*100%
      // (Bitcoin 0.85 -> 85%). The live bar has to use the identical scale or
      // the chart misrepresents Aequitas against the countries beside it.
      const pct = Math.min(d.gini * 100, 100);
      const barAeq = document.getElementById('bar-aeq');
      if (barAeq) barAeq.style.width = pct + '%';
      setText('val-aeq', g);
    }

    // UBI pool + countdown to the next distribution.
    if (typeof d.pool_ubi === 'string' || typeof d.pool_ubi === 'number') {
      setText('mech-ubi-pool', fmt(parseFloat(d.pool_ubi), 4));
    }
    if (typeof d.ubi_next_payout_secs === 'number') {
      const s = Math.max(0, d.ubi_next_payout_secs);
      const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
      setText('mech-ubi-next', h > 0 ? `${h}h ${m}m` : `${m}m`);
    }
  } catch (e) {
    // The one state the badge exists for: the page is up, the node is not.
    setHealthBadge(false, 'Cannot reach /api/status from this page');
  }
}

async function loadWealthCap() {
  try {
    const d = await fetch('/api/wealth-cap').then(r => r.json());
    if (typeof d.cap_aeq === 'number') setText('mech-cap', fmt(d.cap_aeq));
    if (d.multiplier !== undefined && d.average_aeq !== undefined) {
      setText('mech-cap-formula', `${fmt(d.multiplier)}× × ${fmt(d.average_aeq)} AEQ average`);
    }
    // Fair share is the average balance: total supply divided equally.
    if (typeof d.average_aeq === 'number') setText('mech-fairshare', fmt(d.average_aeq));
  } catch (e) { /* leave placeholders */ }
}

// ── Lorenz curve ───────────────────────────────────────────────────────────
// Sort every wallet's total value ascending, then plot cumulative share of
// wealth against cumulative share of people. Perfect equality is the diagonal;
// the area between the diagonal and the curve is what the Gini coefficient
// summarises as a single number.
async function loadLorenz() {
  const path = document.getElementById('lorenz-path');
  const note = document.getElementById('lorenz-note');
  const wrap = document.querySelector('.lorenz-wrap');
  if (!path) return;

  try {
    // 2000 is the endpoint's maximum page size. Ask for all of it: the curve
    // must cover the same population the Gini beside it is computed over.
    const d = await fetch('/api/humans?limit=2000').then(r => r.json());
    const values = (d.humans || [])
      // No filtering. calcGiniFromBalances counts EVERY registered human,
      // including any whose wealth is 0 — and a human holding nothing is the
      // most important point on an inequality curve, not one to drop. Removing
      // them would flatten the curve while the Gini printed beside it still
      // counted them, reintroducing exactly the disagreement that state.go's
      // humanAEQWealthLocked comment was written about.
      .map(h => (typeof h.total_value_aeq === 'number' ? h.total_value_aeq : h.balance) || 0)
      .sort((a, b) => a - b);

    if (values.length < 2) {
      // Not enough wallets for a meaningful curve. Say so plainly instead of
      // drawing something that implies a distribution we cannot see.
      if (wrap) wrap.style.display = 'none';
      return;
    }

    // If the network ever outgrows one page, this curve would be a partial
    // sample sitting next to a whole-population Gini — two different things
    // presented as one. Hide it rather than mislead; a page whose argument is
    // "we publish our own inequality" cannot show a cropped distribution.
    if (typeof d.total === 'number' && d.total > values.length) {
      if (wrap) wrap.style.display = 'none';
      return;
    }

    const total = values.reduce((s, v) => s + v, 0);
    if (total <= 0) { if (wrap) wrap.style.display = 'none'; return; }

    // Chart box matches the axes drawn in the SVG.
    const X0 = 40, X1 = 300, Y0 = 180, Y1 = 20;
    let cum = 0;
    let pts = `M ${X0} ${Y0}`;
    values.forEach((v, i) => {
      cum += v;
      const x = X0 + ((i + 1) / values.length) * (X1 - X0);
      const y = Y0 - (cum / total) * (Y0 - Y1);
      pts += ` L ${x.toFixed(2)} ${y.toFixed(2)}`;
    });
    // Close along the baseline so the gap to the diagonal reads as an area.
    pts += ` L ${X1} ${Y0} Z`;
    path.setAttribute('d', pts);

    // Bottom 50% share — the single most legible number off a Lorenz curve.
    const half = Math.floor(values.length / 2);
    const bottomHalf = values.slice(0, half).reduce((s, v) => s + v, 0);
    const pctBottom = ((bottomHalf / total) * 100).toFixed(1);
    if (note) {
      note.textContent =
        `${values.length} wallets · the poorest 50% hold ${pctBottom}% of all AEQ ` +
        `(perfect equality would be 50%)`;
    }
  } catch (e) {
    if (wrap) wrap.style.display = 'none';
  }
}

loadStats();
loadWealthCap();
loadLorenz();

// Smooth scroll for anchor links
document.querySelectorAll('a[href^="#"]').forEach(a => {
  a.addEventListener('click', e => {
    e.preventDefault();
    document.querySelector(a.getAttribute('href'))?.scrollIntoView({behavior: 'smooth'});
  });
});

// ─── i18n ───────────────────────────────────────────────────────────────────
// English is deliberately NOT in this table: it lives in the markup, so it can
// never drift from what the page actually says. captureEnglish() snapshots it
// once on load, and switching back to English restores from that snapshot.
//
// A missing key falls back to English for that string alone — the same rule
// explorer.js uses — so a half-finished language degrades one line at a time
// instead of blanking the page.
const T = {
 "de": {
  "proof-of-humanity": "MENSCHLICHKEITSNACHWEIS",
  "live": "<span class=\"pulse\"></span>LIVE",
  "overview": "🏠 Übersicht",
  "register": "🔐 Registrieren",
  "explorer": "🔍 Explorer",
  "equality": "⚖️ Gleichheit",
  "network": "🌐 Netzwerk",
  "exchange": "🔄 Börse",
  "social": "💬 Kanäle",
  "money-that-belongsto-every": "Geld, das <span>jedem Menschen</span><br>gleich gehört",
  "aequitas-is-the-first": "Aequitas ist die erste Blockchain, deren Geldmenge mathematisch an nachgewiesene menschliche Existenz gebunden ist. Jeder Mensch erhält 1.000 AEQ — kein Mining, keine Investition, kein Startvorteil.",
  "verified-humans": "Verifizierte Menschen",
  "aeq-in-circulation": "AEQ im Umlauf",
  "gini-coefficient": "Gini-Koeffizient",
  "blocks-produced": "Erzeugte Blöcke",
  "how-it-works": "So funktioniert es",
  "three-steps-to-financial": "Drei Schritte zur finanziellen Teilhabe",
  "no-bank-account-no": "Kein Bankkonto, keine Krypto-Vorkenntnisse, keine Investition. Ein Smartphone genügt.",
  "biometric-capture": "Biometrische Erfassung",
  "your-device-captures-the": "Dein Gerät erfasst die biometrischen Signale und reduziert sie auf einen Einweg-Hash. Die Rohdaten verlassen das Gerät nie.",
  "zeroknowledge-proof": "Zero-Knowledge-Beweis",
  "a-groth16-proof-bn128": "Aus diesem Hash entsteht ein Groth16-Beweis (BN128). Er belegt, dass du ein <strong>einzigartiger Mensch</strong> bist, ohne preiszugeben, wer du bist.",
  "1000-aeq-granted": "1.000 AEQ gutgeschrieben",
  "your-wallet-is-permanently": "Deine Wallet wird binnen einer Sekunde dauerhaft on-chain registriert. Du erhältst 1.000 AEQ — einmalig, für immer.",
  "why-aequitas": "Warum Aequitas",
  "bitcoins-gini-is-085": "Bitcoins Gini liegt bei 0,85 — höher als in jedem Land",
  "the-cryptocurrency-that-was": "Die Kryptowährung, die Finanzen demokratisieren sollte, hat die extremste Vermögenskonzentration der Geschichte hervorgebracht. Aequitas beginnt am anderen Ende.",
  "radical-equality-by-design": "Radikale Gleichheit von Anfang an",
  "total-supply-verified-humans": "Gesamtmenge = verifizierte Menschen × 1.000 AEQ. Kein Pre-Mine, keine Gründerzuteilung, kein Vorteil für Frühe. Wer heute kommt und wer in zehn Jahren kommt, bekommt dasselbe.",
  "privacyfirst-verification": "Verifikation ohne Datenpreisgabe",
  "zeroknowledge-proofs-ensure-one": "Zero-Knowledge-Beweise sichern: ein Mensch, eine Wallet — ohne dass irgendwo biometrische Daten gespeichert werden.",
  "transparent-inequality-tracking": "Ungleichheit öffentlich gemessen",
  "for-everyone-on-earth": "Für alle Menschen",
  "no-bank-account-no-2": "Kein Bankkonto, keine Kreditkarte, kein Ausweis. Ein Android-Telefon reicht.",
  "wealth-equality": "Vermögensgleichheit",
  "the-fairest-currency-ever": "Die gerechteste Währung, die je gebaut wurde",
  "lower-gini-more-equality": "Niedrigerer Gini = mehr Gleichheit. Aequitas zielt auf unter 0,30 — skandinavisches Niveau.",
  "lorenz-curve-live": "Lorenz-Kurve — live",
  "drawn-from-every-registered": "Gezeichnet aus dem tatsächlichen Guthaben jeder registrierten Wallet. Die Gerade ist vollkommene Gleichheit; je weiter die Kurve durchhängt, desto ungleicher.",
  "loading-distribution": "Verteilung wird geladen…",
  "the-mechanisms": "Die Mechanismen",
  "four-rules-keep-it": "Vier Regeln halten es dauerhaft fair",
  "equality-is-not-a": "Gleichheit ist auf dieser Kette kein Versprechen, sondern Code, der ohne Abstimmung, ohne Admin-Schlüssel und ohne Ausnahme läuft.",
  "universal-basic-income": "Bedingungsloses Grundeinkommen",
  "every-verified-human-receives": "Jeder verifizierte Mensch erhält alle 24 Stunden einen gleichen Anteil am UBI-Pool — ohne Antrag, ohne Bedingung.",
  "the-pool-fills-from": "Der Pool speist sich aus vier Quellen",
  "20-of-every-transaction": "<strong>20 %</strong> jeder Transaktionsgebühr",
  "wealthcap-overflow-redistributed-t": "<strong>Überschuss über der Vermögensobergrenze</strong> — sofort umverteilt",
  "20-of-demurrage-charged": "<strong>20 % der Demurrage</strong> auf gehortete Guthaben",
  "abandoned-wallets-after-25": "<strong>Verwaiste Wallets</strong> — nach 2,5 Jahren in Treuhand, dann in den Pool",
  "wealth-cap": "Vermögensobergrenze",
  "no-wallet-may-hold": "Keine Wallet darf mehr als ein festes Vielfaches des Durchschnittsguthabens halten. Alles darüber fließt sofort in die Pools zurück.",
  "the-formula-in-full": "Die Formel im Ganzen",
  "n-is-the-number": "N ist die Zahl der registrierten Menschen. Der Faktor wächst mit jeder neuen Person um eins und bleibt ab dem 25. Menschen bei 25.",
  "demurrage": "Demurrage",
  "money-is-a-tool": "Geld ist ein Werkzeug, keine Trophäe. Wer weit über seinem fairen Anteil hält und nichts damit tut, zahlt dafür — und zwar an alle anderen.",
  "how-it-is-charged": "Wie sie erhoben wird",
  "only-after-three-months": "Erst nach <strong>drei Monaten Inaktivität</strong>, und nur auf den Anteil oberhalb deines fairen Anteils. Vernichtet wird nichts.",
  "escrow-amp-guardians": "Treuhand &amp; Guardians",
  "a-wallet-that-falls": "Eine Wallet, die jahrelang schweigt, sperrt ihren Wert nicht für immer vor allen anderen weg.",
  "what-happens-over-time": "Was mit der Zeit geschieht",
  "25-years-inactive-balance": "<strong>2,5 Jahre</strong> inaktiv → Guthaben geht in Treuhand, jederzeit rückholbar",
  "15-years-escrow-flows": "<strong>+1,5 Jahre</strong> → Treuhand fließt in den UBI-Pool für alle",
  "a-guardian-you-nominate": "Ein von dir benannter <strong>Guardian</strong> kann die Wallet wiederherstellen — nach einer Sperrfrist von 7 Tagen, die dir Zeit zum Widerspruch gibt",
  "recovery-stays-possible-the": "Wiederherstellung bleibt die ganze Zeit möglich, solange das Guthaben in Treuhand liegt",
  "tokenomics": "Tokenomics",
  "selfcorrecting-economic-mechanisms": "Sich selbst korrigierende Wirtschaftsmechanismen",
  "every-fee-is-automatically": "Jede Gebühr wird automatisch umverteilt. Kein manueller Eingriff, keine Governance-Abstimmung.",
  "validators-pool": "Validatoren-Pool",
  "node-operators-who-secure": "Node-Betreiber, die das Netz sichern, erhalten 40 % aller Swap-Gebühren. Tägliche Ausschüttung um 20:00 Berliner Zeit.",
  "liquidity-providers": "Liquiditätsgeber",
  "lp-pool-contributors-earn": "Wer Liquidität stellt, erhält 30 % anteilig zur Einlage. Tiefere Pools = geringerer Preiseinfluss für alle.",
  "ubi-pool": "UBI-Pool",
  "20-of-all-fees": "20 % aller Gebühren fließen in den UBI-Pool und werden alle 24 Stunden zu gleichen Teilen an alle verifizierten Menschen ausgeschüttet.",
  "treasury": "Treasury",
  "10-funds-protocol-development": "10 % finanzieren Protokollentwicklung, Sicherheitsaudits und Infrastruktur — vollständig on-chain nachvollziehbar.",
  "under-the-hood": "Unter der Haube",
  "built-to-be-verified": "Gebaut zum Nachprüfen, nicht zum Vertrauen",
  "every-claim-on-this": "Jede Behauptung auf dieser Seite ist an einem laufenden Node überprüfbar. Die Kette ist Open Source, EVM-kompatibel, und jede Zahl hier kommt aus derselben API, die jeder Node anbietet.",
  "consensus-amp-network": "Konsens &amp; Netzwerk",
  "consensus": "Konsens",
  "blockdag-with-ghostdag-ordering": "BlockDAG mit GHOSTDAG-Ordnung — Validatoren produzieren gleichzeitig, statt um einen Platz zu konkurrieren",
  "block-time": "Blockzeit",
  "1-second": "1 Sekunde",
  "validator-entry": "Zugang als Validator",
  "no-stake-required-a": "Kein Stake nötig. Ein Node-Betreiber muss ein registrierter Mensch sein — das ist die ganze Hürde",
  "peer-transport": "Peer-Transport",
  "libp2p-plus-an-independent": "libp2p, dazu ein unabhängiger HTTP-Sync-Pfad, damit auch ein Node hinter einer Firewall teilnimmt",
  "finality": "Finalität",
  "hard-checkpoints-equivocating-vali": "Harte Checkpoints; doppelt signierende Validatoren werden automatisch bestraft",
  "identity-amp-privacy": "Identität &amp; Privatsphäre",
  "proof-system": "Beweissystem",
  "groth16-over-bn128-circom": "Groth16 über BN128 (circom / snarkjs)",
  "what-the-chain-stores": "Was die Kette speichert",
  "a-nullifier-and-a": "Einen Nullifier und einen Einweg-Hash. Kein Bild, keine Vorlage, kein Name, kein Dokument",
  "what-it-proves": "Was er beweist",
  "that-this-human-has": "Dass dieser Mensch sich noch nie registriert hat — und sonst nichts",
  "duplicate-defence": "Schutz vor Doppelanmeldung",
  "recovery": "Wiederherstellung",
  "nominated-guardian-with-a": "Benannter Guardian mit 7-tägiger Sperrfrist",
  "chain-amp-compatibility": "Kette &amp; Kompatibilität",
  "chain-id": "Chain ID",
  "1926-add-it-to": "1926 — in MetaMask eintragbar wie jedes EVM-Netz",
  "evm": "EVM",
  "full-goethereum-execution-standard": "Vollständige go-ethereum-Ausführung; Standardwerkzeuge funktionieren unverändert",
  "precision": "Genauigkeit",
  "6-decimals-1-aeq": "6 Nachkommastellen (1 AEQ = 1.000.000 Mikro-AEQ)",
  "transaction-fee": "Transaktionsgebühr",
  "01-redistributed-never-burned": "0,1 %, umverteilt — nie vernichtet, nie von einem Betreiber einbehalten",
  "state": "State",
  "postgresql-per-node-reconstructabl": "PostgreSQL pro Node, allein aus der Kette rekonstruierbar",
  "no-premine-no-founder": "<strong>Kein Pre-Mine. Keine Gründerzuteilung. Kein Admin-Schlüssel.</strong>\n      Die Gesamtmenge entspricht den verifizierten Menschen mal 1.000 AEQ — mehr entsteht nie.",
  "social-media": "Social Media",
  "where-the-network-talks": "Wo das Netzwerk spricht",
  "announcements-the-state-of": "Ankündigungen, der Zustand der Kette und die unbequemen Fragen &mdash; öffentlich, auf beiden.",
  "aequitasmoney": "@AequitasMoney",
  "announcements-and-what-the": "Ankündigungen, und was die Kette tatsächlich tut. Kurzform.",
  "telegram": "Telegram",
  "tmeaequitasmoney": "t.me/aequitasmoney",
  "the-open-group-questions": "Die offene Gruppe: Fragen, Node-Betreiber und Hilfe beim Registrieren.",
  "get-started": "Loslegen",
  "join-the-fairest-currency": "Komm zur gerechtesten Währung der Welt",
  "download-the-aequitas-app": "Lade die Aequitas-App, erfasse deine Biometrie und erhalte binnen einer Sekunde 1.000 AEQ. Keine Gebühren, keine Investition, keine Voraussetzungen.",
  "download-aequitas-app-android": "📱 Aequitas-App laden (Android)",
  "view-on-github": "Auf GitHub ansehen",
  "register-2": "Registrieren",
  "block-explorer": "Block-Explorer",
  "equality-score": "Gleichheitsindex",
  "network-2": "Netzwerk",
  "exchange-2": "Börse",
  "node-guide-en": "Node-Anleitung (EN)",
  "node-guide-de": "Node-Anleitung (DE)",
  "github": "GitHub",
  "aequitas-chain-chain-id": "Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · gestartet Juni 2026",
  "money-exists-because-people": "\"<em>Geld existiert, weil Menschen existieren. Nicht mehr und nicht weniger.</em>\"",
  "live-on-chain-id": "<span class=\"pulse\"></span>\n    LIVE AUF CHAIN ID 1926",
  "download-aequitas-app": "📱 Aequitas-App laden",
  "open-explorer": "🌐 Explorer öffnen",
  "strengthaware-multisignal-matching": "Stärkebewusster Mehrsignal-Abgleich: ein starker biometrischer Treffer entscheidet allein, schwächere Signale müssen sich gegenseitig bestätigen"
 },
 "es": {
  "proof-of-humanity": "PRUEBA DE HUMANIDAD",
  "live": "<span class=\"pulse\"></span>EN VIVO",
  "overview": "🏠 Inicio",
  "register": "🔐 Registrarse",
  "explorer": "🔍 Explorador",
  "equality": "⚖️ Igualdad",
  "network": "🌐 Red",
  "exchange": "🔄 Intercambio",
  "social": "💬 Redes",
  "money-that-belongsto-every": "Dinero que pertenece<br>a <span>cada persona</span> por igual",
  "aequitas-is-the-first": "Aequitas es la primera cadena de bloques cuya oferta monetaria está ligada matemáticamente a la existencia humana verificada. Cada persona recibe 1.000 AEQ: sin minería, sin inversión, sin ventaja inicial.",
  "verified-humans": "Personas verificadas",
  "aeq-in-circulation": "AEQ en circulación",
  "gini-coefficient": "Coeficiente de Gini",
  "blocks-produced": "Bloques producidos",
  "how-it-works": "Cómo funciona",
  "three-steps-to-financial": "Tres pasos hacia la inclusión financiera",
  "no-bank-account-no": "Sin cuenta bancaria, sin conocimientos de cripto, sin inversión. Basta un teléfono.",
  "biometric-capture": "Captura biométrica",
  "your-device-captures-the": "Tu dispositivo capta las señales biométricas y las reduce a un hash de un solo sentido. Los datos brutos nunca salen del teléfono.",
  "zeroknowledge-proof": "Prueba de conocimiento cero",
  "a-groth16-proof-bn128": "A partir de ese hash se genera una prueba Groth16 (BN128). Demuestra que eres un <strong>ser humano único</strong> sin revelar quién eres.",
  "1000-aeq-granted": "1.000 AEQ concedidos",
  "your-wallet-is-permanently": "Tu monedero queda registrado en cadena de forma permanente en menos de un segundo. Recibes 1.000 AEQ, una sola vez, para siempre.",
  "why-aequitas": "Por qué Aequitas",
  "bitcoins-gini-is-085": "El Gini de Bitcoin es 0,85: más alto que el de cualquier país",
  "the-cryptocurrency-that-was": "La criptomoneda que iba a democratizar las finanzas creó la concentración de riqueza más extrema de la historia. Aequitas empieza por el otro extremo.",
  "radical-equality-by-design": "Igualdad radical por diseño",
  "total-supply-verified-humans": "Oferta total = personas verificadas × 1.000 AEQ. Sin preminado, sin asignación a fundadores, sin ventaja para los primeros. Quien llega hoy y quien llegue en diez años reciben lo mismo.",
  "privacyfirst-verification": "Verificación que respeta la privacidad",
  "zeroknowledge-proofs-ensure-one": "Las pruebas de conocimiento cero garantizan una persona, un monedero, sin almacenar ningún dato biométrico.",
  "transparent-inequality-tracking": "Desigualdad medida en público",
  "for-everyone-on-earth": "Para todo el mundo",
  "no-bank-account-no-2": "Sin cuenta bancaria, sin tarjeta, sin documento de identidad. Basta un teléfono Android.",
  "wealth-equality": "Igualdad patrimonial",
  "the-fairest-currency-ever": "La moneda más justa jamás creada",
  "lower-gini-more-equality": "Menor Gini = más igualdad. Aequitas apunta a menos de 0,30, nivel escandinavo.",
  "lorenz-curve-live": "Curva de Lorenz — en vivo",
  "drawn-from-every-registered": "Trazada con el saldo real de cada monedero registrado. La línea recta es la igualdad perfecta; cuanto más se hunde la curva, mayor es la desigualdad.",
  "loading-distribution": "Cargando distribución…",
  "the-mechanisms": "Los mecanismos",
  "four-rules-keep-it": "Cuatro reglas lo mantienen justo, de forma permanente",
  "equality-is-not-a": "En esta cadena la igualdad no es una promesa: es código que se ejecuta sin votación, sin clave de administrador y sin excepciones.",
  "universal-basic-income": "Renta básica universal",
  "every-verified-human-receives": "Cada persona verificada recibe una parte igual del fondo de RBU cada 24 horas: sin solicitud, sin condiciones.",
  "the-pool-fills-from": "El fondo se nutre de cuatro fuentes",
  "20-of-every-transaction": "<strong>20 %</strong> de cada comisión de transacción",
  "wealthcap-overflow-redistributed-t": "<strong>Excedente sobre el límite de riqueza</strong>: redistribuido en el acto",
  "20-of-demurrage-charged": "<strong>20 % de la demora</strong> cobrada sobre saldos acumulados",
  "abandoned-wallets-after-25": "<strong>Monederos abandonados</strong>: tras 2,5 años pasan a custodia y luego al fondo",
  "wealth-cap": "Límite de riqueza",
  "no-wallet-may-hold": "Ningún monedero puede tener más de un múltiplo fijo del saldo medio. Todo lo que sobrepase ese límite vuelve de inmediato a los fondos.",
  "the-formula-in-full": "La fórmula completa",
  "n-is-the-number": "N es el número de personas registradas. El multiplicador crece en uno con cada nueva persona y se fija en 25 a partir de la vigesimoquinta.",
  "demurrage": "Demora",
  "money-is-a-tool": "El dinero es una herramienta, no un trofeo. Quien acumula muy por encima de su parte justa sin hacer nada con ella, paga por ello, y paga a todos los demás.",
  "how-it-is-charged": "Cómo se cobra",
  "only-after-three-months": "Solo tras <strong>tres meses de inactividad</strong>, y solo sobre la parte que supera tu cuota justa. No se destruye nada.",
  "escrow-amp-guardians": "Custodia &amp; guardianes",
  "a-wallet-that-falls": "Un monedero que calla durante años no bloquea su valor para siempre frente a todos los demás.",
  "what-happens-over-time": "Qué ocurre con el tiempo",
  "25-years-inactive-balance": "<strong>2,5 años</strong> inactivo → el saldo pasa a custodia, recuperable en cualquier momento",
  "15-years-escrow-flows": "<strong>+1,5 años</strong> → la custodia pasa al fondo de RBU para todos",
  "a-guardian-you-nominate": "Un <strong>guardián</strong> designado por ti puede recuperar el monedero, tras un bloqueo de 7 días que te da tiempo a oponerte",
  "recovery-stays-possible-the": "La recuperación sigue siendo posible mientras el saldo permanezca en custodia",
  "tokenomics": "Tokenómica",
  "selfcorrecting-economic-mechanisms": "Mecanismos económicos que se autocorrigen",
  "every-fee-is-automatically": "Cada comisión se redistribuye automáticamente. Sin intervención manual, sin votación de gobernanza.",
  "validators-pool": "Fondo de validadores",
  "node-operators-who-secure": "Quienes operan nodos y aseguran la red ganan el 40 % de las comisiones de intercambio. Se distribuye a diario a las 20:00, hora de Berlín.",
  "liquidity-providers": "Proveedores de liquidez",
  "lp-pool-contributors-earn": "Quien aporta liquidez recibe el 30 % en proporción a su parte. Fondos más profundos = menor impacto en el precio para todos.",
  "ubi-pool": "Fondo de RBU",
  "20-of-all-fees": "El 20 % de todas las comisiones va al fondo de RBU y se reparte a partes iguales entre todas las personas verificadas cada 24 horas.",
  "treasury": "Tesorería",
  "10-funds-protocol-development": "El 10 % financia el desarrollo del protocolo, las auditorías de seguridad y la infraestructura, con total transparencia en cadena.",
  "under-the-hood": "Bajo el capó",
  "built-to-be-verified": "Hecho para verificarse, no para confiar",
  "every-claim-on-this": "Toda afirmación de esta página puede comprobarse contra un nodo en marcha. La cadena es de código abierto, compatible con EVM, y cada cifra proviene de la misma API que expone cualquier nodo.",
  "consensus-amp-network": "Consenso &amp; red",
  "consensus": "Consenso",
  "blockdag-with-ghostdag-ordering": "BlockDAG con ordenación GHOSTDAG: los validadores producen en paralelo en lugar de competir por un turno",
  "block-time": "Tiempo de bloque",
  "1-second": "1 segundo",
  "validator-entry": "Acceso como validador",
  "no-stake-required-a": "Sin stake. Quien opere un nodo debe ser una persona registrada: esa es toda la barrera",
  "peer-transport": "Transporte entre pares",
  "libp2p-plus-an-independent": "libp2p, más una vía de sincronización HTTP independiente para que un nodo tras un cortafuegos también participe",
  "finality": "Finalidad",
  "hard-checkpoints-equivocating-vali": "Puntos de control firmes; los validadores que firman doble son penalizados automáticamente",
  "identity-amp-privacy": "Identidad &amp; privacidad",
  "proof-system": "Sistema de pruebas",
  "groth16-over-bn128-circom": "Groth16 sobre BN128 (circom / snarkjs)",
  "what-the-chain-stores": "Qué guarda la cadena",
  "a-nullifier-and-a": "Un nulificador y un hash de un solo sentido. Sin imagen, sin plantilla, sin nombre, sin documento",
  "what-it-proves": "Qué demuestra",
  "that-this-human-has": "Que esta persona no se ha registrado antes, y nada más",
  "duplicate-defence": "Defensa contra duplicados",
  "recovery": "Recuperación",
  "nominated-guardian-with-a": "Guardián designado con bloqueo de 7 días",
  "chain-amp-compatibility": "Cadena &amp; compatibilidad",
  "chain-id": "Chain ID",
  "1926-add-it-to": "1926: se añade a MetaMask como cualquier red EVM",
  "evm": "EVM",
  "full-goethereum-execution-standard": "Ejecución completa de go-ethereum; las herramientas estándar funcionan sin cambios",
  "precision": "Precisión",
  "6-decimals-1-aeq": "6 decimales (1 AEQ = 1.000.000 micro-AEQ)",
  "transaction-fee": "Comisión de transacción",
  "01-redistributed-never-burned": "0,1 %, redistribuida: nunca quemada, nunca retenida por un operador",
  "state": "Estado",
  "postgresql-per-node-reconstructabl": "PostgreSQL en cada nodo, reconstruible únicamente a partir de la cadena",
  "no-premine-no-founder": "<strong>Sin preminado. Sin asignación a fundadores. Sin clave de administrador.</strong>\n      La oferta total equivale a las personas verificadas por 1.000 AEQ; nunca se crea más.",
  "social-media": "Redes sociales",
  "where-the-network-talks": "Donde habla la red",
  "announcements-the-state-of": "Anuncios, el estado de la cadena y las preguntas incómodas &mdash; en público, en ambas.",
  "aequitasmoney": "@AequitasMoney",
  "announcements-and-what-the": "Anuncios y lo que la cadena está haciendo realmente. Formato breve.",
  "telegram": "Telegram",
  "tmeaequitasmoney": "t.me/aequitasmoney",
  "the-open-group-questions": "El grupo abierto: preguntas, operadores de nodos y ayuda para registrarse.",
  "get-started": "Empezar",
  "join-the-fairest-currency": "Súmate a la moneda más justa del mundo",
  "download-the-aequitas-app": "Descarga la app de Aequitas, registra tu biometría y recibe 1.000 AEQ en un segundo. Sin comisiones, sin inversión, sin requisitos.",
  "download-aequitas-app-android": "📱 Descargar la app (Android)",
  "view-on-github": "Ver en GitHub",
  "register-2": "Registrarse",
  "block-explorer": "Explorador de bloques",
  "equality-score": "Índice de igualdad",
  "network-2": "Red",
  "exchange-2": "Intercambio",
  "node-guide-en": "Guía de nodo (EN)",
  "node-guide-de": "Guía de nodo (DE)",
  "github": "GitHub",
  "aequitas-chain-chain-id": "Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · lanzada en junio de 2026",
  "money-exists-because-people": "\"<em>El dinero existe porque existen las personas. Nada más y nada menos.</em>\"",
  "live-on-chain-id": "<span class=\"pulse\"></span>\n    EN VIVO EN LA CHAIN ID 1926",
  "download-aequitas-app": "📱 Descargar la app",
  "open-explorer": "🌐 Abrir el explorador",
  "strengthaware-multisignal-matching": "Cotejo multiseñal ponderado por fuerza: una coincidencia biométrica fuerte decide por sí sola; las señales débiles deben confirmarse entre sí"
 },
 "fr": {
  "proof-of-humanity": "PREUVE D'HUMANITÉ",
  "live": "<span class=\"pulse\"></span>EN DIRECT",
  "overview": "🏠 Accueil",
  "register": "🔐 S'inscrire",
  "explorer": "🔍 Explorateur",
  "equality": "⚖️ Égalité",
  "network": "🌐 Réseau",
  "exchange": "🔄 Échange",
  "social": "💬 Réseaux",
  "money-that-belongsto-every": "Une monnaie qui appartient<br>à <span>chaque être humain</span> à parts égales",
  "aequitas-is-the-first": "Aequitas est la première blockchain dont la masse monétaire est mathématiquement liée à l'existence humaine vérifiée. Chaque personne reçoit 1 000 AEQ : sans minage, sans investissement, sans avantage aux premiers arrivés.",
  "verified-humans": "Personnes vérifiées",
  "aeq-in-circulation": "AEQ en circulation",
  "gini-coefficient": "Coefficient de Gini",
  "blocks-produced": "Blocs produits",
  "how-it-works": "Comment ça marche",
  "three-steps-to-financial": "Trois étapes vers l'inclusion financière",
  "no-bank-account-no": "Pas de compte bancaire, pas de connaissances en crypto, pas d'investissement. Un smartphone suffit.",
  "biometric-capture": "Capture biométrique",
  "your-device-captures-the": "Votre appareil capte les signaux biométriques et les réduit à une empreinte à sens unique. Les données brutes ne quittent jamais le téléphone.",
  "zeroknowledge-proof": "Preuve à divulgation nulle",
  "a-groth16-proof-bn128": "Une preuve Groth16 (BN128) est générée à partir de cette empreinte. Elle atteste que vous êtes un <strong>être humain unique</strong> sans révéler qui vous êtes.",
  "1000-aeq-granted": "1 000 AEQ attribués",
  "your-wallet-is-permanently": "Votre portefeuille est enregistré définitivement sur la chaîne en moins d'une seconde. Vous recevez 1 000 AEQ, une seule fois, pour toujours.",
  "why-aequitas": "Pourquoi Aequitas",
  "bitcoins-gini-is-085": "Le Gini de Bitcoin est de 0,85 — plus élevé que celui de n'importe quel pays",
  "the-cryptocurrency-that-was": "La cryptomonnaie censée démocratiser la finance a produit la concentration de richesse la plus extrême de l'histoire. Aequitas commence par l'autre bout.",
  "radical-equality-by-design": "Une égalité radicale par conception",
  "total-supply-verified-humans": "Masse totale = personnes vérifiées × 1 000 AEQ. Pas de pré-minage, pas d'allocation aux fondateurs, aucun avantage aux premiers. Celui qui arrive aujourd'hui et celui qui arrivera dans dix ans reçoivent la même chose.",
  "privacyfirst-verification": "Une vérification qui protège la vie privée",
  "zeroknowledge-proofs-ensure-one": "Les preuves à divulgation nulle garantissent une personne, un portefeuille — sans stocker la moindre donnée biométrique.",
  "transparent-inequality-tracking": "Inégalité mesurée en public",
  "for-everyone-on-earth": "Pour tout le monde",
  "no-bank-account-no-2": "Pas de compte bancaire, pas de carte, pas de pièce d'identité. Un téléphone Android suffit.",
  "wealth-equality": "Égalité des patrimoines",
  "the-fairest-currency-ever": "La monnaie la plus juste jamais créée",
  "lower-gini-more-equality": "Gini plus bas = plus d'égalité. Aequitas vise moins de 0,30, niveau scandinave.",
  "lorenz-curve-live": "Courbe de Lorenz — en direct",
  "drawn-from-every-registered": "Tracée à partir du solde réel de chaque portefeuille enregistré. La droite représente l'égalité parfaite ; plus la courbe s'affaisse, plus l'inégalité est forte.",
  "loading-distribution": "Chargement de la répartition…",
  "the-mechanisms": "Les mécanismes",
  "four-rules-keep-it": "Quatre règles la gardent juste, durablement",
  "equality-is-not-a": "Sur cette chaîne, l'égalité n'est pas une promesse : c'est du code qui s'exécute sans vote, sans clé d'administration et sans exception.",
  "universal-basic-income": "Revenu de base universel",
  "every-verified-human-receives": "Chaque personne vérifiée reçoit une part égale du fonds RBU toutes les 24 heures — sans demande, sans condition.",
  "the-pool-fills-from": "Le fonds est alimenté par quatre sources",
  "20-of-every-transaction": "<strong>20 %</strong> de chaque frais de transaction",
  "wealthcap-overflow-redistributed-t": "<strong>Excédent au-dessus du plafond de fortune</strong> — redistribué à l'instant même",
  "20-of-demurrage-charged": "<strong>20 % de la démurrage</strong> prélevée sur les soldes thésaurisés",
  "abandoned-wallets-after-25": "<strong>Portefeuilles abandonnés</strong> — après 2,5 ans en séquestre, puis vers le fonds",
  "wealth-cap": "Plafond de fortune",
  "no-wallet-may-hold": "Aucun portefeuille ne peut détenir plus d'un multiple fixe du solde moyen. Tout ce qui dépasse retourne immédiatement dans les fonds.",
  "the-formula-in-full": "La formule complète",
  "n-is-the-number": "N est le nombre de personnes enregistrées. Le multiplicateur augmente de un à chaque nouvelle personne et se fixe à 25 à partir de la vingt-cinquième.",
  "demurrage": "Démurrage",
  "money-is-a-tool": "L'argent est un outil, pas un trophée. Détenir bien au-delà de sa part équitable sans rien en faire a un coût — et ce coût revient à tous les autres.",
  "how-it-is-charged": "Comment elle est prélevée",
  "only-after-three-months": "Seulement après <strong>trois mois d'inactivité</strong>, et uniquement sur la part dépassant votre juste part. Rien n'est détruit.",
  "escrow-amp-guardians": "Séquestre &amp; garants",
  "a-wallet-that-falls": "Un portefeuille silencieux pendant des années ne verrouille pas sa valeur pour toujours au détriment de tous les autres.",
  "what-happens-over-time": "Ce qui se passe avec le temps",
  "25-years-inactive-balance": "<strong>2,5 ans</strong> d'inactivité → le solde passe en séquestre, récupérable à tout moment",
  "15-years-escrow-flows": "<strong>+1,5 an</strong> → le séquestre alimente le fonds RBU pour tous",
  "a-guardian-you-nominate": "Un <strong>garant</strong> que vous désignez peut récupérer le portefeuille — après un délai de 7 jours qui vous laisse le temps de vous y opposer",
  "recovery-stays-possible-the": "La récupération reste possible tant que le solde est en séquestre",
  "tokenomics": "Tokenomique",
  "selfcorrecting-economic-mechanisms": "Des mécanismes économiques autocorrecteurs",
  "every-fee-is-automatically": "Chaque frais est redistribué automatiquement. Aucune intervention manuelle, aucun vote de gouvernance.",
  "validators-pool": "Fonds des validateurs",
  "node-operators-who-secure": "Les opérateurs de nœuds qui sécurisent le réseau perçoivent 40 % des frais d'échange. Distribution quotidienne à 20 h, heure de Berlin.",
  "liquidity-providers": "Fournisseurs de liquidité",
  "lp-pool-contributors-earn": "Ceux qui apportent de la liquidité reçoivent 30 % au prorata de leur part. Des fonds plus profonds = moins d'impact sur le prix pour tous.",
  "ubi-pool": "Fonds RBU",
  "20-of-all-fees": "20 % de tous les frais alimentent le fonds RBU, réparti à parts égales entre toutes les personnes vérifiées toutes les 24 heures.",
  "treasury": "Trésorerie",
  "10-funds-protocol-development": "10 % financent le développement du protocole, les audits de sécurité et l'infrastructure — en toute transparence sur la chaîne.",
  "under-the-hood": "Sous le capot",
  "built-to-be-verified": "Conçu pour être vérifié, pas pour qu'on lui fasse confiance",
  "every-claim-on-this": "Chaque affirmation de cette page est vérifiable auprès d'un nœud en fonctionnement. La chaîne est open source, compatible EVM, et chaque chiffre provient de la même API qu'expose n'importe quel nœud.",
  "consensus-amp-network": "Consensus &amp; réseau",
  "consensus": "Consensus",
  "blockdag-with-ghostdag-ordering": "BlockDAG avec ordonnancement GHOSTDAG — les validateurs produisent en parallèle au lieu de se disputer un créneau",
  "block-time": "Temps de bloc",
  "1-second": "1 seconde",
  "validator-entry": "Accès validateur",
  "no-stake-required-a": "Aucun stake requis. L'opérateur d'un nœud doit être une personne enregistrée — c'est toute la barrière",
  "peer-transport": "Transport entre pairs",
  "libp2p-plus-an-independent": "libp2p, plus une voie de synchronisation HTTP indépendante pour qu'un nœud derrière un pare-feu participe quand même",
  "finality": "Finalité",
  "hard-checkpoints-equivocating-vali": "Points de contrôle fermes ; les validateurs qui signent deux fois sont sanctionnés automatiquement",
  "identity-amp-privacy": "Identité &amp; vie privée",
  "proof-system": "Système de preuve",
  "groth16-over-bn128-circom": "Groth16 sur BN128 (circom / snarkjs)",
  "what-the-chain-stores": "Ce que la chaîne conserve",
  "a-nullifier-and-a": "Un nullificateur et une empreinte à sens unique. Pas d'image, pas de gabarit, pas de nom, pas de document",
  "what-it-proves": "Ce qu'elle prouve",
  "that-this-human-has": "Que cette personne ne s'est jamais inscrite auparavant — et rien d'autre",
  "duplicate-defence": "Défense contre les doublons",
  "recovery": "Récupération",
  "nominated-guardian-with-a": "Garant désigné, avec un délai de 7 jours",
  "chain-amp-compatibility": "Chaîne &amp; compatibilité",
  "chain-id": "Chain ID",
  "1926-add-it-to": "1926 — à ajouter dans MetaMask comme tout réseau EVM",
  "evm": "EVM",
  "full-goethereum-execution-standard": "Exécution go-ethereum complète ; les outils standards fonctionnent sans modification",
  "precision": "Précision",
  "6-decimals-1-aeq": "6 décimales (1 AEQ = 1 000 000 micro-AEQ)",
  "transaction-fee": "Frais de transaction",
  "01-redistributed-never-burned": "0,1 %, redistribués — jamais brûlés, jamais conservés par un opérateur",
  "state": "État",
  "postgresql-per-node-reconstructabl": "PostgreSQL sur chaque nœud, reconstructible à partir de la seule chaîne",
  "no-premine-no-founder": "<strong>Pas de pré-minage. Pas d'allocation aux fondateurs. Pas de clé d'administration.</strong>\n      La masse totale équivaut au nombre de personnes vérifiées multiplié par 1 000 AEQ — jamais davantage.",
  "social-media": "Réseaux sociaux",
  "where-the-network-talks": "Là où le réseau s'exprime",
  "announcements-the-state-of": "Annonces, état de la chaîne et questions qui dérangent &mdash; en public, sur les deux.",
  "aequitasmoney": "@AequitasMoney",
  "announcements-and-what-the": "Annonces, et ce que la chaîne fait réellement. Format court.",
  "telegram": "Telegram",
  "tmeaequitasmoney": "t.me/aequitasmoney",
  "the-open-group-questions": "Le groupe ouvert : questions, opérateurs de nœuds et aide à l'inscription.",
  "get-started": "Commencer",
  "join-the-fairest-currency": "Rejoignez la monnaie la plus juste du monde",
  "download-the-aequitas-app": "Téléchargez l'application Aequitas, enregistrez votre biométrie et recevez 1 000 AEQ en une seconde. Sans frais, sans investissement, sans prérequis.",
  "download-aequitas-app-android": "📱 Télécharger l'application (Android)",
  "view-on-github": "Voir sur GitHub",
  "register-2": "S'inscrire",
  "block-explorer": "Explorateur de blocs",
  "equality-score": "Indice d'égalité",
  "network-2": "Réseau",
  "exchange-2": "Échange",
  "node-guide-en": "Guide du nœud (EN)",
  "node-guide-de": "Guide du nœud (DE)",
  "github": "GitHub",
  "aequitas-chain-chain-id": "Aequitas Chain · Chain ID 1926 · <span>aequitas.digital</span> · lancée en juin 2026",
  "money-exists-because-people": "\"<em>L'argent existe parce que les gens existent. Rien de plus, rien de moins.</em>\"",
  "live-on-chain-id": "<span class=\"pulse\"></span>\n    EN DIRECT SUR LA CHAIN ID 1926",
  "download-aequitas-app": "📱 Télécharger l'application",
  "open-explorer": "🌐 Ouvrir l'explorateur",
  "strengthaware-multisignal-matching": "Appariement multi-signaux pondéré : une correspondance biométrique forte décide seule, les signaux plus faibles doivent se confirmer mutuellement"
 }
};

const EN = {};
function captureEnglish() {
  document.querySelectorAll('[data-i18n]').forEach(function(el) {
    EN[el.getAttribute('data-i18n')] = el.innerHTML;
  });
}

function setLang(lang, remember) {
  const t = lang === 'en' ? EN : (T[lang] || {});
  document.documentElement.lang = lang;
  document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
  document.querySelectorAll('[data-i18n]').forEach(function(el) {
    const k = el.getAttribute('data-i18n');
    const v = (t[k] !== undefined) ? t[k] : EN[k];
    // Translations may carry safe inline markup (strong, em, span, br) — that
    // is why this is innerHTML. Nothing user-supplied ever reaches T or EN.
    if (v !== undefined) el.innerHTML = v;
  });
  const sel = document.getElementById('lang-sel');
  if (sel) sel.value = lang;
  // The explorer reads the same key, so the choice survives the jump between
  // the two pages — and, unlike the explorer alone, survives a reload.
  if (remember !== false) { try { localStorage.setItem('aeq_lang', lang); } catch (e) {} }
}

// The option list is built from T rather than written into the HTML. A
// hardcoded list drifts the moment a language is added or removed, and the
// failure is silent and ugly: the selector offers a language, the visitor
// picks it, and the page stays English. Built this way it can only ever offer
// what actually exists.
const LANG_NAMES = {en:'EN', de:'DE', es:'ES', fr:'FR', pt:'PT', it:'IT',
                    tr:'TR', id:'ID', ru:'RU', zh:'ZH', ar:'AR', hi:'HI'};

function buildLangOptions() {
  const sel = document.getElementById('lang-sel');
  if (!sel) return;
  const codes = ['en'].concat(Object.keys(T).sort());
  sel.innerHTML = codes.map(function(c) {
    return '<option value="' + c + '">\uD83C\uDF10 ' + (LANG_NAMES[c] || c.toUpperCase()) + '</option>';
  }).join('');
}

function initLang() {
  captureEnglish();
  buildLangOptions();
  let saved = null;
  try { saved = localStorage.getItem('aeq_lang'); } catch (e) {}
  if (!saved) {
    // Nothing stored yet: meet the visitor in their browser's language if we
    // speak it, rather than assuming English.
    const nav = (navigator.language || 'en').slice(0, 2).toLowerCase();
    if (T[nav]) saved = nav;
  }
  if (saved && saved !== 'en') setLang(saved, false);
  const sel = document.getElementById('lang-sel');
  if (sel) {
    if (saved) sel.value = saved;
    sel.addEventListener('change', function() { setLang(this.value); });
  }
}

document.addEventListener('DOMContentLoaded', initLang);
