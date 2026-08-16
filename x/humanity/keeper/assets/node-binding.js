async function signBinding() {
  const errEl = document.getElementById('err');
  const outEl = document.getElementById('out');
  errEl.style.display = 'none';
  outEl.style.display = 'none';
  const signingAddr = document.getElementById('signingAddr').value.trim().toLowerCase();
  if (!/^0x[0-9a-f]{40}$/.test(signingAddr)) {
    errEl.textContent = 'Enter a valid signing address (0x followed by 40 hex characters).';
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
    const message = 'Aequitas: authorize validator ' + signingAddr;
    const signature = await window.ethereum.request({
      method: 'personal_sign',
      params: [message, wallet],
    });
    // FIX (P2, security audit 2026-07-21): `wallet` comes straight from
    // eth_requestAccounts (wallet-provider-supplied, unvalidated) — building
    // this via string-concatenated innerHTML would let a malicious/compromised
    // wallet provider inject markup/script into this page. Build the DOM with
    // textContent instead so wallet/signature are always treated as plain text.
    outEl.textContent = '';
    outEl.appendChild(document.createTextNode('Wallet: '));
    const walletSpan = document.createElement('span');
    walletSpan.className = 'hl';
    walletSpan.textContent = wallet;
    outEl.appendChild(walletSpan);
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createTextNode('Set these on your node:'));
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createTextNode('NODE_OPERATOR_WALLET=' + wallet));
    outEl.appendChild(document.createElement('br'));
    outEl.appendChild(document.createTextNode('NODE_OPERATOR_BINDING_SIGNATURE=' + signature));
    outEl.style.display = 'block';
  } catch (e) {
    errEl.textContent = 'Signing failed or was rejected: ' + (e && e.message ? e.message : e);
    errEl.style.display = 'block';
  }
}

document.getElementById('connectBtn').addEventListener('click', signBinding);
