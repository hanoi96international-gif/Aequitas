// Authorize a coordinator's signing key with a human wallet.
//
// The parallel to node-binding.js, for the other half of the network. A
// coordinator issues the attestation the chain mints on, so its Ed25519 key
// has to be tied to a registered human before any matching service will
// accept what it signs — same rule as everywhere else: one human, one key.
//
// Two proofs are needed. This page produces the first (secp256k1/EIP-191,
// which only the human's wallet can make). The coordinator produces the
// second, the Ed25519 possession proof, on its own host — that key never
// leaves the machine and must never be pasted anywhere, including here.
//
// Same DOM-building discipline as node-binding.js: `wallet` and `signature`
// come from the wallet provider and are never trusted as markup. Everything
// is written with textContent.

function zeile(eltern, text, hervor) {
  const n = document.createElement(hervor ? 'span' : 'span');
  if (hervor) n.className = 'hl';
  n.textContent = text;
  eltern.appendChild(n);
  eltern.appendChild(document.createElement('br'));
}

async function signCoordinator() {
  const errEl = document.getElementById('err');
  const outEl = document.getElementById('out');
  errEl.style.display = 'none';
  outEl.style.display = 'none';

  const pub = document.getElementById('pubKey').value.trim().toLowerCase().replace(/^0x/, '');
  if (!/^[0-9a-f]{64}$/.test(pub)) {
    // Diese Meldung sah frueher jemand, der den Wert in seiner Wallet suchte.
    // Sie sagt deshalb zuerst, wo er NICHT ist.
    errEl.textContent = "Enter your coordinator's public key: 64 hex characters. " +
      'It is not in your wallet -- it belongs to your coordinator and is created on its ' +
      'own host. The setup script prints it on startup, and GET /inventory on your own ' +
      'coordinator reports it as attestation_public_key at any time.';
    errEl.style.display = 'block';
    return;
  }
  if (!window.ethereum) {
    errEl.textContent = 'No wallet found. Install MetaMask or another browser wallet extension.';
    errEl.style.display = 'block';
    return;
  }

  try {
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    const wallet = accounts[0];
    const message = 'Aequitas: authorize coordinator ' + pub;
    const signature = await window.ethereum.request({
      method: 'personal_sign',
      params: [message, wallet],
    });

    outEl.textContent = '';
    const w = document.createElement('span');
    w.appendChild(document.createTextNode('Wallet: '));
    const hl = document.createElement('span');
    hl.className = 'hl';
    hl.textContent = wallet;
    w.appendChild(hl);
    outEl.appendChild(w);
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createElement('br'));

    zeile(outEl, 'Run this on the host your coordinator runs on:');
    outEl.appendChild(document.createElement('br'));
    zeile(outEl, 'WALLET=' + wallet + ' ./coordinator-eintragen.sh ' + signature);
    outEl.appendChild(document.createElement('br'));
    zeile(outEl, 'It sends this to every node you list. The registry is per-node and is');
    zeile(outEl, 'not replicated — a node you skip will keep refusing your attestations.');

    outEl.style.display = 'block';
  } catch (e) {
    // A rejection in the wallet is a no, not a failure.
    errEl.textContent = (e && e.code === 4001)
      ? 'Rejected in the wallet.'
      : 'Signing failed: ' + (e && e.message ? e.message : e);
    errEl.style.display = 'block';
  }
}

document.getElementById('connectBtn').addEventListener('click', signCoordinator);
