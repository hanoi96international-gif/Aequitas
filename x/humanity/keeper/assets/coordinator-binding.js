// Authorize a coordinator, end to end, in the browser.
//
// WHAT THIS REPLACES
//
// The page used to ask for the coordinator's public key, produce a signature,
// and print a shell command for the operator to run. On 2026-08-27 that last
// step silently failed three times in a row: signed, never registered, and
// nothing on screen said so. A step that needs a terminal is, for most
// people, not a step at all.
//
// Now the page does the whole thing. The node fetches the coordinator's own
// possession proof (GET /api/coordinator-proof), the wallet signs the exact
// sentence the node hands back, and the page posts the result. The node
// registers it and passes the same payload to its peers, so one signature
// reaches every node.
//
// Nothing about the checks changed: both proofs are still verified, in the
// same place, by every node that stores anything.
//
// Same DOM discipline as node-binding.js: values from the wallet and from the
// network are written with textContent, never as markup.

const ZUSTAND = { schritte: [], nachricht: null, pub: null, keySig: null, wallet: null };

function zeigeFehler(text) {
  const e = document.getElementById('err');
  e.textContent = text;
  e.style.display = 'block';
}

function schritt(text, art) {
  const zeile = document.createElement('div');
  zeile.style.marginBottom = '4px';
  const marke = document.createElement('span');
  marke.textContent = art === 'ok' ? '✓ ' : art === 'warn' ? '! ' : '· ';
  marke.style.color = art === 'ok' ? '#22C55E' : art === 'warn' ? '#f87171' : '#6B7A99';
  marke.style.fontWeight = 'bold';
  zeile.appendChild(marke);
  const t = document.createElement('span');
  t.textContent = text;
  zeile.appendChild(t);
  const out = document.getElementById('out');
  out.style.display = 'block';
  out.appendChild(zeile);
  return zeile;
}

async function alsJSON(antwort) {
  const roh = await antwort.text();
  try {
    return JSON.parse(roh);
  } catch (e) {
    // Ein Proxy oder eine Fehlerseite antwortet mit HTML. Das als JSON zu
    // lesen ergibt eine Meldung ueber ein unerwartetes Zeichen, die nichts
    // ueber die Ursache sagt -- deshalb hier der rohe Anfang.
    return { error: 'unexpected answer (HTTP ' + antwort.status + '): ' + roh.slice(0, 160) };
  }
}

async function eintragen() {
  const knopf = document.getElementById('connectBtn');
  const errEl = document.getElementById('err');
  errEl.style.display = 'none';
  document.getElementById('out').textContent = '';

  let adresse = document.getElementById('pubKey').value.trim().replace(/\/+$/, '');
  if (!/^https:\/\/[^\s/]+/.test(adresse)) {
    zeigeFehler("Enter your coordinator's public address, for example " +
      'https://verifier.example.org — not a key. The address must be https://, because ' +
      'that is how the nodes reach it.');
    return;
  }
  if (!window.ethereum) {
    zeigeFehler('No wallet found. Install MetaMask or another browser wallet extension.');
    return;
  }

  knopf.disabled = true;
  try {
    const konten = await window.ethereum.request({ method: 'eth_requestAccounts' });
    const wallet = (konten[0] || '').toLowerCase();
    ZUSTAND.wallet = wallet;
    schritt('Wallet connected: ' + wallet, 'ok');

    // 1. Der Knoten holt den Besitznachweis beim Coordinator ab.
    schritt('Asking your coordinator for its key…');
    const r1 = await fetch('/api/coordinator-proof?url=' + encodeURIComponent(adresse) +
      '&wallet=' + encodeURIComponent(wallet));
    const d1 = await alsJSON(r1);
    if (!r1.ok || !d1.public_key) {
      zeigeFehler(d1.error || 'Your coordinator did not answer with a proof.');
      return;
    }
    ZUSTAND.pub = d1.public_key;
    ZUSTAND.keySig = d1.key_signature;
    ZUSTAND.nachricht = d1.message;
    schritt('Coordinator key: ' + d1.public_key, 'ok');

    // 2. Die Wallet unterschreibt genau den Satz, den der Knoten nennt.
    schritt('Waiting for your signature…');
    const signatur = await window.ethereum.request({
      method: 'personal_sign',
      params: [d1.message, wallet],
    });
    schritt('Signed.', 'ok');

    // 3. Eintragen. Der Knoten reicht an seine Nachbarn weiter.
    schritt('Registering on the chain…');
    const r2 = await fetch('/api/register-coordinator-key', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        public_key: d1.public_key,
        human_wallet: wallet,
        human_signature: signatur,
        key_signature: d1.key_signature,
        url: adresse,
      }),
    });
    const d2 = await alsJSON(r2);
    if (!r2.ok || !d2.success) {
      zeigeFehler(d2.error || 'The node refused the registration.');
      return;
    }

    schritt('Registered on this node.', 'ok');
    const weiter = d2.forwarded_to || [];
    for (const n of weiter) {
      if (n.ok) {
        schritt('Also registered on ' + n.node, 'ok');
      } else {
        // Ein Nachbar, der gerade nicht antwortet, macht die Eintragung nicht
        // ungueltig -- er kennt sie nur noch nicht. Das gehoert benannt, sonst
        // sieht "fertig" nach mehr aus, als es ist.
        schritt('Not yet on ' + n.node + ' — that node was unreachable. ' +
          'Your coordinator works everywhere else; try again later to include it.', 'warn');
      }
    }
    if (weiter.length === 0) {
      schritt('This node knows no peers to pass it on to.', 'warn');
    }
    schritt('Done. Nothing further to run.', 'ok');
  } catch (e) {
    // Ablehnen in der Wallet ist ein Nein, kein Fehler.
    zeigeFehler(e && e.code === 4001
      ? 'Rejected in the wallet.'
      : 'Failed: ' + (e && (e.message || e.shortMessage) ? (e.message || e.shortMessage) : e));
  } finally {
    knopf.disabled = false;
  }
}

document.getElementById('connectBtn').addEventListener('click', eintragen);
