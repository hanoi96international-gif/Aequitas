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
    outEl.innerHTML = 'Wallet: <span class="hl">' + wallet + '</span><br><br>' +
      'Set these on your node:<br><br>' +
      'NODE_OPERATOR_WALLET=' + wallet + '<br>' +
      'NODE_OPERATOR_BINDING_SIGNATURE=' + signature;
    outEl.style.display = 'block';
  } catch (e) {
    errEl.textContent = 'Signing failed or was rejected: ' + (e && e.message ? e.message : e);
    errEl.style.display = 'block';
  }
}

document.getElementById('connectBtn').addEventListener('click', signBinding);
