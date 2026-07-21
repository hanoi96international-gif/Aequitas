#!/usr/bin/env python3
"""MetaMask UX: persistent wallet across tabs, disconnect button, mobile deep link."""
import sys

FILE = r'C:\Users\aequitas-chain\x\humanity\keeper\api_html.go'

with open(FILE, 'r', encoding='utf-8') as f:
    src = f.read()

replacements = []

# ──────────────────────────────────────────────────────────────
# 1. Register card: add disconnect button after Connect button
# ──────────────────────────────────────────────────────────────
replacements.append((
    '    <button class="rbtn bc" id="btn-conn" onclick="connectWallet()" data-i18n="btn-conn">🦊 CONNECT METAMASK</button>',
    '    <button class="rbtn bc" id="btn-conn" onclick="connectWallet()" data-i18n="btn-conn">🦊 CONNECT METAMASK</button>\n    <button id="btn-disconnect" onclick="disconnectWallet()" style="display:none;margin-top:6px;padding:8px 16px;font-size:0.6rem;letter-spacing:1px;border:1px solid rgba(248,113,113,0.4);background:rgba(248,113,113,0.08);color:var(--red);border-radius:6px;cursor:pointer;width:100%">⊘ DISCONNECT WALLET</button>'
))

# ──────────────────────────────────────────────────────────────
# 2. Swap card: add disconnect button after Connect button
# ──────────────────────────────────────────────────────────────
replacements.append((
    '    <button class="rbtn bc" id="swap-btn-conn" onclick="connectSwapWallet()" data-i18n="btn-conn">🦊 CONNECT METAMASK</button>',
    '    <button class="rbtn bc" id="swap-btn-conn" onclick="connectSwapWallet()" data-i18n="btn-conn">🦊 CONNECT METAMASK</button>\n    <button id="swap-btn-disconnect" onclick="disconnectWallet()" style="display:none;margin-top:6px;padding:8px 16px;font-size:0.6rem;letter-spacing:1px;border:1px solid rgba(248,113,113,0.4);background:rgba(248,113,113,0.08);color:var(--red);border-radius:6px;cursor:pointer;width:100%">⊘ DISCONNECT WALLET</button>'
))

# ──────────────────────────────────────────────────────────────
# 3. connectWallet(): add localStorage + cross-tab sync + disconnect btn
# ──────────────────────────────────────────────────────────────
replacements.append((
    """    waddr = accounts[0];
    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    try {
      const br = await fetch('/api/balance?wallet=' + waddr);
      const bd = await br.json();
      if (bd.is_human) {
        addLog('Already registered! Balance: ' + bd.balance + ' AEQ', 'ok');
        document.getElementById('btn-reg').disabled = true;
        document.getElementById('btn-reg').textContent = 'ALREADY REGISTERED';
      } else if (proofData) {
        document.getElementById('btn-reg').disabled = false;
        document.getElementById('btn-reg').textContent = 'PROOF READY — CLICK TO REGISTER';
      } else {
        document.getElementById('btn-reg').disabled = true;
      }
    } catch (e) {
      document.getElementById('btn-reg').disabled = !proofData;
    }
  } catch (e) {
    addLog('Connection failed: ' + e.message, 'err');
  }
}""",
    """    waddr = accounts[0];
    swapWaddr = waddr;
    localStorage.setItem('aeq_wallet', waddr);
    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const dBtn = document.getElementById('btn-disconnect');
    if (dBtn) dBtn.style.display = 'block';
    // Sync swap tab wallet display
    const swapBox = document.getElementById('swap-wbox');
    const swapAdr = document.getElementById('swap-wadr');
    const swapBtn = document.getElementById('swap-btn-conn');
    const swapDBtn = document.getElementById('swap-btn-disconnect');
    if (swapBox) swapBox.style.display = 'block';
    if (swapAdr) swapAdr.textContent = waddr;
    if (swapBtn) { swapBtn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4); swapBtn.style.background = 'var(--green)'; swapBtn.style.color = '#050A14'; }
    if (swapDBtn) swapDBtn.style.display = 'block';
    try {
      const br = await fetch('/api/balance?wallet=' + waddr);
      const bd = await br.json();
      if (bd.is_human) {
        addLog('Already registered! Balance: ' + bd.balance + ' AEQ', 'ok');
        document.getElementById('btn-reg').disabled = true;
        document.getElementById('btn-reg').textContent = 'ALREADY REGISTERED';
      } else if (proofData) {
        document.getElementById('btn-reg').disabled = false;
        document.getElementById('btn-reg').textContent = 'PROOF READY — CLICK TO REGISTER';
      } else {
        document.getElementById('btn-reg').disabled = true;
      }
    } catch (e) {
      document.getElementById('btn-reg').disabled = !proofData;
    }
  } catch (e) {
    addLog('Connection failed: ' + e.message, 'err');
  }
}"""
))

# ──────────────────────────────────────────────────────────────
# 4. connectSwapWallet(): add localStorage + cross-tab sync + disconnect btn
# ──────────────────────────────────────────────────────────────
replacements.append((
    """    swapWaddr = accounts[0];
    document.getElementById('swap-wbox').style.display = 'block';
    document.getElementById('swap-wadr').textContent = swapWaddr;
    const btn = document.getElementById('swap-btn-conn');
    btn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    await refreshSwapBalances();
    await loadLPPosition();
    document.getElementById('swap-btn-go').disabled = false;
    document.getElementById('swap-btn-faucet').disabled = false;
    document.getElementById('swap-btn-addliq').disabled = false;
    setSwapDirection('aeq_to_tusd');
  } catch (e) {
    swapLog('Connection failed: ' + e.message, 'err');
  }
}""",
    """    swapWaddr = accounts[0];
    waddr = swapWaddr;
    localStorage.setItem('aeq_wallet', swapWaddr);
    document.getElementById('swap-wbox').style.display = 'block';
    document.getElementById('swap-wadr').textContent = swapWaddr;
    const btn = document.getElementById('swap-btn-conn');
    btn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const swapDBtn = document.getElementById('swap-btn-disconnect');
    if (swapDBtn) swapDBtn.style.display = 'block';
    // Sync register tab wallet display
    const regBox = document.getElementById('wbox');
    const regAdr = document.getElementById('wadr');
    const regBtn = document.getElementById('btn-conn');
    const regDBtn = document.getElementById('btn-disconnect');
    if (regBox) regBox.style.display = 'block';
    if (regAdr) regAdr.textContent = swapWaddr;
    if (regBtn) { regBtn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4); regBtn.style.background = 'var(--green)'; regBtn.style.color = '#050A14'; }
    if (regDBtn) regDBtn.style.display = 'block';
    await refreshSwapBalances();
    await loadLPPosition();
    document.getElementById('swap-btn-go').disabled = false;
    document.getElementById('swap-btn-faucet').disabled = false;
    document.getElementById('swap-btn-addliq').disabled = false;
    setSwapDirection('aeq_to_tusd');
  } catch (e) {
    swapLog('Connection failed: ' + e.message, 'err');
  }
}"""
))

# ──────────────────────────────────────────────────────────────
# 5. connectWalletAndProve(): same sync treatment
# ──────────────────────────────────────────────────────────────
replacements.append((
    """    waddr = accounts[0];
    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';

    const br = await fetch('/api/balance?wallet=' + waddr);""",
    """    waddr = accounts[0];
    swapWaddr = waddr;
    localStorage.setItem('aeq_wallet', waddr);
    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const dBtn = document.getElementById('btn-disconnect');
    if (dBtn) dBtn.style.display = 'block';

    const br = await fetch('/api/balance?wallet=' + waddr);"""
))

# ──────────────────────────────────────────────────────────────
# 6. MetaMask not-found handlers — add mobile deep link
#    There are 3 plain-text "MetaMask not found" addLog/swapLog calls remaining
# ──────────────────────────────────────────────────────────────

# 6a. connectSwapWallet (swapLog)
replacements.append((
    "    swapLog('MetaMask not found. Please install MetaMask.', 'err');\n    return;",
    "    const _isMobS = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);\n    if (_isMobS) { const _dl = 'https://metamask.app.link/dapp/' + window.location.host; swapLog('\U0001f98a MetaMask nicht gefunden. Mobile: <a href=\"' + _dl + '\" style=\"color:var(--gold)\">In MetaMask App öffnen</a>', 'warn'); } else { swapLog('\U0001f98a MetaMask not found — <a href=\"https://metamask.io/download/\" target=\"_blank\" style=\"color:var(--gold)\">install MetaMask</a>', 'warn'); }\n    return;"
))

# 6b. connectWalletAndProve (addLog, first occurrence)
replacements.append((
    "    addLog('MetaMask not found. Please install MetaMask.', 'err');\n    return;\n  }\n  if (!pendingBioHash)",
    "    const _isMobC = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);\n    if (_isMobC) { const _dl = 'https://metamask.app.link/dapp/' + window.location.host; addLog('\U0001f98a Mobile: <a href=\"' + _dl + '\" style=\"color:var(--gold)\">In MetaMask App öffnen</a>', 'warn'); } else { addLog('\U0001f98a MetaMask not found — <a href=\"https://metamask.io/download/\" target=\"_blank\" style=\"color:var(--gold)\">install MetaMask</a>', 'warn'); }\n    return;\n  }\n  if (!pendingBioHash)"
))

# 6c. connectWallet (addLog, second occurrence) — use unique surrounding context
replacements.append((
    "    addLog('MetaMask not found. Please install MetaMask.', 'err');\n    return;\n  }\n  try {\n    await addToMetaMask();\n    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });\n    waddr = accounts[0];",
    "    const _isMobW = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);\n    if (_isMobW) { const _dl = 'https://metamask.app.link/dapp/' + window.location.host; addLog('\U0001f98a Mobile: <a href=\"' + _dl + '\" style=\"color:var(--gold)\">In MetaMask App öffnen</a>', 'warn'); } else { addLog('\U0001f98a MetaMask not found — <a href=\"https://metamask.io/download/\" target=\"_blank\" style=\"color:var(--gold)\">install MetaMask</a>', 'warn'); }\n    return;\n  }\n  try {\n    await addToMetaMask();\n    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });\n    waddr = accounts[0];"
))

# ──────────────────────────────────────────────────────────────
# 7. Add disconnectWallet() + restoreWalletFromStorage() functions
#    and call restore at startup — insert before checkProofParams();
# ──────────────────────────────────────────────────────────────
replacements.append((
    "checkProofParams();\nloadStatus();",
    """function disconnectWallet() {
  waddr = '';
  swapWaddr = '';
  localStorage.removeItem('aeq_wallet');
  // Reset register tab
  const wbox = document.getElementById('wbox');
  const wadr = document.getElementById('wadr');
  const bConn = document.getElementById('btn-conn');
  const bDisc = document.getElementById('btn-disconnect');
  const bReg = document.getElementById('btn-reg');
  if (wbox) wbox.style.display = 'none';
  if (wadr) wadr.textContent = '—';
  if (bConn) { bConn.textContent = '\U0001f98a CONNECT METAMASK'; bConn.style.background = ''; bConn.style.color = ''; }
  if (bDisc) bDisc.style.display = 'none';
  if (bReg) { bReg.disabled = true; bReg.textContent = 'REGISTER ON-CHAIN'; }
  // Reset swap tab
  const swapBox = document.getElementById('swap-wbox');
  const swapAdr = document.getElementById('swap-wadr');
  const swapConn = document.getElementById('swap-btn-conn');
  const swapDisc = document.getElementById('swap-btn-disconnect');
  const swapGo = document.getElementById('swap-btn-go');
  if (swapBox) swapBox.style.display = 'none';
  if (swapAdr) swapAdr.textContent = '—';
  if (swapConn) { swapConn.textContent = '\U0001f98a CONNECT METAMASK'; swapConn.style.background = ''; swapConn.style.color = ''; }
  if (swapDisc) swapDisc.style.display = 'none';
  if (swapGo) swapGo.disabled = true;
  addLog('✓ Wallet disconnected locally. To fully revoke, open MetaMask → Connected Sites.', 'info');
}

async function restoreWalletFromStorage() {
  const saved = localStorage.getItem('aeq_wallet');
  if (!saved || !window.ethereum) return;
  try {
    const accounts = await window.ethereum.request({ method: 'eth_accounts' });
    if (accounts && accounts[0] && accounts[0].toLowerCase() === saved.toLowerCase()) {
      waddr = accounts[0];
      swapWaddr = accounts[0];
      // Restore register tab UI
      const wbox = document.getElementById('wbox');
      const wadr = document.getElementById('wadr');
      const bConn = document.getElementById('btn-conn');
      const bDisc = document.getElementById('btn-disconnect');
      if (wbox) wbox.style.display = 'block';
      if (wadr) wadr.textContent = accounts[0];
      if (bConn) { bConn.textContent = accounts[0].slice(0,10)+'...'+accounts[0].slice(-4); bConn.style.background='var(--green)'; bConn.style.color='#050A14'; }
      if (bDisc) bDisc.style.display = 'block';
      // Restore swap tab UI
      const swapBox = document.getElementById('swap-wbox');
      const swapAdr = document.getElementById('swap-wadr');
      const swapConn = document.getElementById('swap-btn-conn');
      const swapDBtn = document.getElementById('swap-btn-disconnect');
      if (swapBox) swapBox.style.display = 'block';
      if (swapAdr) swapAdr.textContent = accounts[0];
      if (swapConn) { swapConn.textContent = accounts[0].slice(0,10)+'...'+accounts[0].slice(-4); swapConn.style.background='var(--green)'; swapConn.style.color='#050A14'; }
      if (swapDBtn) swapDBtn.style.display = 'block';
    } else {
      localStorage.removeItem('aeq_wallet');
    }
  } catch(e) {}
}

checkProofParams();
restoreWalletFromStorage();
loadStatus();"""
))

# ──────────────────────────────────────────────────────────────
# Apply
# ──────────────────────────────────────────────────────────────
errors = []
for i, (old, new) in enumerate(replacements):
    cnt = src.count(old)
    if cnt == 0:
        errors.append(f'[MISS #{i+1}] {old[:120]!r}')
    elif cnt > 1:
        errors.append(f'[DUP #{i+1}] found {cnt}x: {old[:120]!r}')
    else:
        src = src.replace(old, new)

if errors:
    print('ERRORS:')
    for e in errors:
        print(' ', e)
    sys.exit(1)

with open(FILE, 'w', encoding='utf-8') as f:
    f.write(src)

print(f'Done: {len(replacements)} replacements applied successfully.')
