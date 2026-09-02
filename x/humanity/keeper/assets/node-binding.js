// Register a validator, end to end, in the browser.
//
// WHAT THIS REPLACES
//
// The page used to produce one signature and print two environment variables
// for the operator to paste onto their node. That covered the wallet binding
// and nothing else: the actual chain registration needs three proofs, two of
// which only exist on the machine holding the keys.
//
// The result was measurable. Of three witness keys on this network, exactly
// one was in the chain registry; the other two sat in a hand-maintained
// PERSONHOOD_PUBLIC_KEYS list on every box — the very permission the registry
// was built to remove. And one of the two block producers was not in the
// validator registry at all.
//
// The same step on the coordinator side failed three times in a row, silently,
// before it moved into the browser. This one had never been walked.
//
// Now the node assembles what only it can (its own signing-key proof, and the
// witness proof fetched from the matching service), the wallet signs the
// sentence the node hands back, and the page registers it.
//
// Same DOM discipline as before: everything from the wallet or the network is
// written with textContent, never as markup.

function zeile(text, art) {
  const out = document.getElementById('out');
  out.style.display = 'block';
  const d = document.createElement('div');
  d.style.marginBottom = '4px';
  const marke = document.createElement('span');
  marke.textContent = art === 'ok' ? '✓ ' : art === 'warn' ? '! ' : '· ';
  marke.style.color = art === 'ok' ? '#22C55E' : art === 'warn' ? '#f87171' : '#6B7A99';
  marke.style.fontWeight = 'bold';
  d.appendChild(marke);
  const t = document.createElement('span');
  t.textContent = text;
  d.appendChild(t);
  out.appendChild(d);
}

function fehler(text) {
  const e = document.getElementById('err');
  e.textContent = text;
  e.style.display = 'block';
}

async function alsJSON(antwort) {
  const roh = await antwort.text();
  try {
    return JSON.parse(roh);
  } catch (e) {
    // Ein Proxy oder eine Fehlerseite antwortet mit HTML. Als JSON gelesen
    // ergibt das eine Meldung ueber ein unerwartetes Zeichen, die nichts
    // ueber die Ursache sagt.
    return { error: 'unexpected answer (HTTP ' + antwort.status + '): ' + roh.slice(0, 160) };
  }
}

async function eintragen() {
  const knopf = document.getElementById('connectBtn');
  document.getElementById('err').style.display = 'none';
  document.getElementById('out').textContent = '';

  const matching = document.getElementById('signingAddr').value.trim().replace(/\/+$/, '');
  // Eine 0x-Adresse hier ist der haeufigste Fehlgriff, und die allgemeine
  // https://-Meldung half dabei nicht: sie sagt, was falsch ist, aber nicht,
  // warum jemand gerade DAS eingetragen hat. Wer eine Wallet-Adresse
  // hineinschreibt, sucht das Feld, in das seine Identitaet gehoert -- und
  // dieses Feld gibt es nicht, die Wallet kommt ueber den Knopf.
  if (/^0x[0-9a-fA-F]{40}$/.test(matching)) {
    fehler('That is a wallet address, and this field does not want one. Your wallet is ' +
      'connected by the button below — you never type an address on this page. This field ' +
      'wants the web address your own matching service answers on, for example ' +
      'https://verifier.example.org. If you do not run a matching service, leave it empty: ' +
      'your node still registers, it just does not count as a witness.');
    return;
  }
  if (matching && !/^https:\/\/[^\s/]+/.test(matching)) {
    fehler('The matching service address must start with https:// — or leave the field empty if this ' +
      'node does not run one. Without it the node registers, but does not count as a witness.');
    return;
  }
  if (!window.ethereum) {
    fehler('No wallet found. Install MetaMask or another browser wallet extension.');
    return;
  }

  knopf.disabled = true;
  try {
    const konten = await window.ethereum.request({ method: 'eth_requestAccounts' });
    const wallet = (konten[0] || '').toLowerCase();
    zeile('Wallet connected: ' + wallet, 'ok');

    zeile('Asking this node for its own proofs…');
    let url = '/api/validator-selfproof?wallet=' + encodeURIComponent(wallet);
    if (matching) url += '&matching_url=' + encodeURIComponent(matching);
    const r1 = await fetch(url);
    const d1 = await alsJSON(r1);
    if (!r1.ok || !d1.signing_address) {
      fehler(d1.error || 'This node could not prove its own signing key.');
      return;
    }
    zeile('Node signing address: ' + d1.signing_address, 'ok');
    if (d1.personhood_key) {
      zeile('Witness key: ' + d1.personhood_key, 'ok');
    } else if (matching) {
      // Benannt, nicht verschwiegen: ohne diesen Nachweis wird der Knoten
      // eingetragen, zaehlt aber nicht als Bezeuger -- und genau dieser
      // Unterschied ist der Grund, warum zwei Schluessel noch in einer Datei
      // stehen.
      zeile('No witness proof: ' + (d1.personhood_error || 'unknown') +
        ' — registering without it; this node will not count as a witness.', 'warn');
    } else {
      zeile('No matching service given — registering without a witness key.', 'warn');
    }

    zeile('Waiting for your signature…');
    const signatur = await window.ethereum.request({
      method: 'personal_sign',
      params: [d1.message, wallet],
    });
    zeile('Signed.', 'ok');

    zeile('Registering on the chain…');
    const koerper = {
      signing_address: d1.signing_address,
      human_wallet: wallet,
      human_signature: signatur,
      signing_key_signature: d1.signing_key_signature,
    };
    if (d1.personhood_key) {
      koerper.personhood_key = d1.personhood_key;
      koerper.personhood_signature = d1.personhood_signature;
      koerper.matching_url = d1.matching_url;
    }
    const r2 = await fetch('/api/register-validator-key', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(koerper),
    });
    const d2 = await alsJSON(r2);
    if (!r2.ok || d2.error) {
      fehler(d2.error || 'The node refused the registration.');
      return;
    }
    zeile('Registered on this node.', 'ok');
    zeile('The registry is per-node and is not replicated — open this page on each ' +
      'node that should honour it, or ask its operator to.', 'warn');
    zeile('Done. Nothing further to run.', 'ok');
  } catch (e) {
    // Ablehnen in der Wallet ist ein Nein, kein Fehler.
    fehler(e && e.code === 4001
      ? 'Rejected in the wallet.'
      : 'Failed: ' + (e && (e.message || e.shortMessage) ? (e.message || e.shortMessage) : e));
  } finally {
    knopf.disabled = false;
  }
}

document.getElementById('connectBtn').addEventListener('click', eintragen);
