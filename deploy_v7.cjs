const { ethers } = require('ethers');
const fs = require('fs');
const path = require('path');
const solc = require('solc');
const https = require('https');

const RPC_URL  = process.env.RPC_URL  || 'https://aequitas-production-9fba.up.railway.app/rpc';
const PK       = process.env.PK;
const VERIFIER = process.env.VERIFIER || '0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2';

if (!PK) { console.error('ERROR: PK not set'); process.exit(1); }

function rpcCall(method, params) {
  return new Promise((resolve, reject) => {
    const body = JSON.stringify({ jsonrpc:'2.0', id:1, method, params });
    const url = new URL(RPC_URL);
    const opts = { hostname: url.hostname, path: url.pathname, method: 'POST', headers: { 'Content-Type':'application/json', 'Content-Length': Buffer.byteLength(body) } };
    const req = https.request(opts, res => {
      let data = '';
      res.on('data', d => data += d);
      res.on('end', () => {
        try { const j = JSON.parse(data); resolve(j.result); } catch(e) { reject(e); }
      });
    });
    req.on('error', reject);
    req.write(body);
    req.end();
  });
}

function compile() {
  const filePath = path.join(__dirname, 'AequitasV7.sol');
  const source = fs.readFileSync(filePath, 'utf8');
  const input = {
    language: 'Solidity',
    sources: { 'AequitasV7.sol': { content: source } },
    settings: { optimizer: { enabled: true, runs: 200 }, outputSelection: { '*': { '*': ['abi','evm.bytecode'] } } }
  };
  const output = JSON.parse(solc.compile(JSON.stringify(input)));
  if (output.errors) {
    const errs = output.errors.filter(e => e.severity === 'error');
    if (errs.length) { errs.forEach(e => console.error(e.formattedMessage)); process.exit(1); }
  }
  const c = output.contracts['AequitasV7.sol']['AequitasV7'];
  console.log('Compiled. Bytecode:', (c.evm.bytecode.object.length/2).toLocaleString(), 'bytes');
  return { abi: c.abi, bytecode: '0x' + c.evm.bytecode.object };
}

async function deploy() {
  console.log('\n=== AEQUITAS V7 DEPLOYMENT ===');

  const wallet = new ethers.Wallet(PK);
  console.log('Deployer:', wallet.address);

  const balanceHex = await rpcCall('eth_getBalance', [wallet.address, 'latest']);
  console.log('Balance:', ethers.formatEther(BigInt(balanceHex)), 'AEQ');

  const nonceHex = await rpcCall('eth_getTransactionCount', [wallet.address, 'latest']);
  const nonce = parseInt(nonceHex, 16);
  console.log('Nonce:', nonce);

  const chainIdHex = await rpcCall('eth_chainId', []);
  const chainId = parseInt(chainIdHex, 16);
  console.log('Chain ID:', chainId);

  const { abi, bytecode } = compile();

  // Encode constructor
  const iface = new ethers.Interface(abi);
  const constructorData = iface.encodeDeploy([VERIFIER]);
  const deployData = bytecode + constructorData.slice(2);

  // Expected address
  const contractAddress = ethers.getCreateAddress({ from: wallet.address, nonce });
  console.log('Expected address:', contractAddress);

  // Build and sign transaction
  const tx = {
    nonce,
    gasLimit: 6_000_000n,
    gasPrice: 1_000_000_000n, // 1 gwei
    data: deployData,
    chainId,
    value: 0n,
  };

  const signed = await wallet.signTransaction(tx);
  console.log('\nSending signed transaction...');

  const txHash = await rpcCall('eth_sendRawTransaction', [signed]);
  console.log('TX Hash:', txHash);
  console.log('Waiting for contract...');

  // Poll for contract code
  for (let i = 0; i < 20; i++) {
    await new Promise(r => setTimeout(r, 3000));
    const code = await rpcCall('eth_getCode', [contractAddress, 'latest']);
    if (code && code !== '0x') {
      console.log('\n=== SUCCESS ===');
      console.log('Contract:', contractAddress);

      // Verify with ethers
      const provider = new ethers.JsonRpcProvider(RPC_URL);
      const deployed = new ethers.Contract(contractAddress, abi, provider);
      console.log('Name:', await deployed.name());
      console.log('Phase:', (await deployed.currentPhase()).toString());
      console.log('fairShare:', ethers.formatEther(await deployed.fairShare()), 'AEQ');

      const info = { address: contractAddress, verifier: VERIFIER, deployer: wallet.address, tx: txHash, chainId, date: new Date().toISOString() };
      fs.writeFileSync(path.join(__dirname, 'deployment_v7.json'), JSON.stringify(info, null, 2));
      console.log('\nSaved: deployment_v7.json');
      console.log('UPDATE api.go: contract_v7 =', contractAddress);
      return;
    }
    process.stdout.write('.');
  }
  console.log('\nTimeout — check manually:', contractAddress);
}

deploy().catch(e => { console.error('FAILED:', e.message); process.exit(1); });
