import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import {
  type Address,
  type Hex,
  concatHex,
  encodeAbiParameters,
  encodeFunctionData,
  hexToBigInt,
  keccak256,
  numberToHex,
  padHex,
  sliceHex,
  toFunctionSelector,
  toHex,
} from "viem";

// AequitasV8 register: good path plus one abuse test per check
// (AGENTS.md "Missbrauchstest"). The verifier is MockGroth16Verifier, which
// accepts only pA[0] == VALID_MARKER while `accept` is true, so the proof
// check itself is exercised as well (valid vs. tampered vs. rejected).

const VALID_MARKER = 0xa11cen;
const SECP256K1_N =
  0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;
const SNARK_SCALAR_FIELD =
  21888242871839275222246405745257275088548364400416034343698204186575808495617n;

type Proof = {
  pA: readonly [bigint, bigint];
  pB: readonly [readonly [bigint, bigint], readonly [bigint, bigint]];
  pC: readonly [bigint, bigint];
};

const GOOD_PROOF: Proof = {
  pA: [VALID_MARKER, 1n],
  pB: [
    [2n, 3n],
    [4n, 5n],
  ],
  pC: [6n, 7n],
};
const BAD_PROOF: Proof = { ...GOOD_PROOF, pA: [VALID_MARKER + 1n, 1n] };

const REGISTER_TYPES = {
  Register: [
    { name: "human", type: "address" },
    { name: "commitment", type: "uint256" },
    { name: "nullifier", type: "uint256" },
    { name: "nonce", type: "uint256" },
    { name: "deadline", type: "uint256" },
  ],
} as const;

describe("AequitasV8 registerWithSig", async function () {
  const { viem, networkHelpers } = await network.create();
  const publicClient = await viem.getPublicClient();
  const chainId = BigInt(await publicClient.getChainId());
  const [registrar, alice, bob, mallory, registrar2] =
    await viem.getWalletClients();

  async function deployV8() {
    const verifier = await viem.deployContract("MockGroth16Verifier");
    const v8 = await viem.deployContract("AequitasV8", [
      verifier.address,
      [registrar.account.address, registrar2.account.address],
    ]);
    return { v8, verifier };
  }

  type V8 = Awaited<ReturnType<typeof deployV8>>["v8"];
  type Wallet = (typeof alice);

  async function sign(
    v8: V8,
    signer: Wallet,
    msg: {
      human: Address;
      commitment: bigint;
      nullifier: bigint;
      nonce?: bigint;
      deadline: bigint;
    },
    domainOverride: Partial<{
      name: string;
      version: string;
      chainId: bigint;
      verifyingContract: Address;
    }> = {},
  ): Promise<Hex> {
    const nonce = msg.nonce ?? (await v8.read.nonces([msg.human]));
    return signer.signTypedData({
      account: signer.account,
      domain: {
        name: "Aequitas",
        version: "8",
        chainId,
        verifyingContract: v8.address,
        ...domainOverride,
      },
      types: REGISTER_TYPES,
      primaryType: "Register",
      message: { ...msg, nonce },
    });
  }

  async function deadlineIn(seconds: bigint) {
    return BigInt(await networkHelpers.time.latest()) + seconds;
  }

  function register(
    v8: V8,
    args: {
      proof?: Proof;
      commitment: bigint;
      nullifier: bigint;
      human: Address;
      deadline: bigint;
      signature: Hex;
    },
    caller: Wallet = registrar,
  ) {
    const p = args.proof ?? GOOD_PROOF;
    return v8.write.registerWithSig(
      [
        p.pA,
        p.pB,
        p.pC,
        [args.commitment, args.nullifier],
        args.human,
        args.deadline,
        args.signature,
      ],
      { account: caller.account },
    );
  }

  async function signedFor(
    v8: V8,
    who: Wallet,
    commitment: bigint,
    nullifier: bigint,
  ) {
    const human = who.account.address;
    const deadline = await deadlineIn(600n);
    const signature = await sign(v8, who, {
      human,
      commitment,
      nullifier,
      deadline,
    });
    return { human, commitment, nullifier, deadline, signature };
  }

  // ─── good path ──────────────────────────────────────────────────────────

  it("registers a human with the human's own EIP-712 signature and a valid proof", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1001n, 2001n);

    await viem.assertions.emitWithArgs(register(v8, req), v8, "Registered", [
      alice.account.address,
      1001n,
      padHex(toHex(2001n), { size: 32 }),
    ]);

    assert.equal(await v8.read.isHuman([alice.account.address]), true);
    assert.equal(await v8.read.commitmentOf([alice.account.address]), 1001n);
    assert.equal(
      await v8.read.nullifierOf([alice.account.address]),
      padHex(toHex(2001n), { size: 32 }),
    );
    assert.equal(await v8.read.usedCommitments([1001n]), true);
    assert.equal(
      (await v8.read.usedNullifiers([padHex(toHex(2001n), { size: 32 })])).toLowerCase(),
      alice.account.address.toLowerCase(),
    );
    assert.equal(await v8.read.nonces([alice.account.address]), 1n);
    assert.equal(await v8.read.totalHumans(), 1n);
    // No starting balance in the contract — Go books it.
    assert.equal(await v8.read.balanceOf([alice.account.address]), 0n);
    assert.equal(await v8.read.totalSupply(), 0n);
  });

  it("accepts the second genesis registrar too", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1002n, 2002n);
    await register(v8, req, registrar2);
    assert.equal(await v8.read.isHuman([alice.account.address]), true);
  });

  it("registrationDigest matches an independent EIP-712 encoding (for Go and the app)", async function () {
    const { v8 } = await deployV8();
    const human = alice.account.address;
    const deadline = 1_900_000_000n;
    const domainTypeHash = keccak256(
      toHex(
        "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)",
      ),
    );
    const registerTypeHash = keccak256(
      toHex(
        "Register(address human,uint256 commitment,uint256 nullifier,uint256 nonce,uint256 deadline)",
      ),
    );
    const domainSeparator = keccak256(
      encodeAbiParameters(
        [
          { type: "bytes32" },
          { type: "bytes32" },
          { type: "bytes32" },
          { type: "uint256" },
          { type: "address" },
        ],
        [
          domainTypeHash,
          keccak256(toHex("Aequitas")),
          keccak256(toHex("8")),
          chainId,
          v8.address,
        ],
      ),
    );
    const structHash = keccak256(
      encodeAbiParameters(
        [
          { type: "bytes32" },
          { type: "address" },
          { type: "uint256" },
          { type: "uint256" },
          { type: "uint256" },
          { type: "uint256" },
        ],
        [registerTypeHash, human, 7n, 8n, 0n, deadline],
      ),
    );
    const digest = keccak256(concatHex(["0x1901", domainSeparator, structHash]));

    assert.equal(await v8.read.EIP712_DOMAIN_TYPEHASH(), domainTypeHash);
    assert.equal(await v8.read.REGISTER_TYPEHASH(), registerTypeHash);
    assert.equal(await v8.read.DOMAIN_SEPARATOR(), domainSeparator);
    assert.equal(
      await v8.read.registrationDigest([human, 7n, 8n, deadline]),
      digest,
    );
  });

  // ─── who may call / who signs ───────────────────────────────────────────

  it("rejects a caller that is not a genesis registrar, even with a perfect request", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1010n, 2010n);
    // The human itself submitting is not enough either: the node decides
    // whether the proof came from a biometric check (public proving key).
    for (const caller of [alice, mallory]) {
      await viem.assertions.revertWith(
        register(v8, req, caller),
        "V8: caller is not a registrar",
      );
    }
    assert.equal(await v8.read.isHuman([alice.account.address]), false);
  });

  it("rejects a signature by someone other than the human (operator's or a stranger's)", async function () {
    const { v8 } = await deployV8();
    const human = alice.account.address;
    const deadline = await deadlineIn(600n);
    for (const forger of [registrar, mallory]) {
      const signature = await sign(v8, forger, {
        human,
        commitment: 1020n,
        nullifier: 2020n,
        deadline,
      });
      await viem.assertions.revertWith(
        register(v8, { human, commitment: 1020n, nullifier: 2020n, deadline, signature }),
        "V8: invalid signature",
      );
    }
    assert.equal(await v8.read.isHuman([human]), false);
  });

  it("front-running: the same signed request cannot be redirected to another wallet", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1030n, 2030n);
    // Mallory copies Alice's pending request and swaps in her own address.
    await viem.assertions.revertWith(
      register(v8, { ...req, human: mallory.account.address }),
      "V8: invalid signature",
    );
    // Alice's original still goes through afterwards.
    await register(v8, req);
    assert.equal(await v8.read.isHuman([alice.account.address]), true);
    assert.equal(await v8.read.isHuman([mallory.account.address]), false);
  });

  it("the signature covers commitment, nullifier and deadline (tampering any of them fails)", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1040n, 2040n);
    await viem.assertions.revertWith(
      register(v8, { ...req, commitment: 1041n }),
      "V8: invalid signature",
    );
    await viem.assertions.revertWith(
      register(v8, { ...req, nullifier: 2041n }),
      "V8: invalid signature",
    );
    await viem.assertions.revertWith(
      register(v8, { ...req, deadline: req.deadline + 1n }),
      "V8: invalid signature",
    );
  });

  it("registrar cannot make itself human", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, registrar, 1050n, 2050n);
    await viem.assertions.revertWith(
      register(v8, req),
      "V8: registrar cannot be human",
    );
  });

  it("rejects the zero address as human", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1060n, 2060n);
    await viem.assertions.revertWith(
      register(v8, {
        ...req,
        human: "0x0000000000000000000000000000000000000000",
      }),
      "V8: human is zero",
    );
  });

  // ─── domain: chain id, contract, name, version ──────────────────────────

  it("rejects a signature made for another chain id", async function () {
    const { v8 } = await deployV8();
    const human = alice.account.address;
    const deadline = await deadlineIn(600n);
    const signature = await sign(
      v8,
      alice,
      { human, commitment: 1070n, nullifier: 2070n, deadline },
      { chainId: 1926n === chainId ? 1n : 1926n },
    );
    await viem.assertions.revertWith(
      register(v8, { human, commitment: 1070n, nullifier: 2070n, deadline, signature }),
      "V8: invalid signature",
    );
  });

  it("rejects a signature made for another contract (cross-deployment replay)", async function () {
    const { v8 } = await deployV8();
    const { v8: other } = await deployV8();
    const human = alice.account.address;
    const deadline = await deadlineIn(600n);
    const signature = await sign(other, alice, {
      human,
      commitment: 1080n,
      nullifier: 2080n,
      deadline,
    });
    // Valid on `other` …
    await register(other, { human, commitment: 1080n, nullifier: 2080n, deadline, signature });
    // … but not replayable on v8.
    await viem.assertions.revertWith(
      register(v8, { human, commitment: 1080n, nullifier: 2080n, deadline, signature }),
      "V8: invalid signature",
    );
  });

  it("rejects a signature with another domain name or version (e.g. a V7-style or V9 domain)", async function () {
    const { v8 } = await deployV8();
    const human = alice.account.address;
    const deadline = await deadlineIn(600n);
    for (const override of [{ name: "AequitasV7" }, { version: "7" }]) {
      const signature = await sign(
        v8,
        alice,
        { human, commitment: 1090n, nullifier: 2090n, deadline },
        override,
      );
      await viem.assertions.revertWith(
        register(v8, { human, commitment: 1090n, nullifier: 2090n, deadline, signature }),
        "V8: invalid signature",
      );
    }
  });

  // ─── deadline ───────────────────────────────────────────────────────────

  it("rejects an expired signature", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1100n, 2100n);
    await networkHelpers.time.increaseTo(req.deadline + 1n);
    await viem.assertions.revertWith(register(v8, req), "V8: signature expired");
  });

  it("rejects a deadline beyond MAX_SIGNATURE_LIFETIME (no forever-valid signatures)", async function () {
    const { v8 } = await deployV8();
    const human = alice.account.address;
    const lifetime = await v8.read.MAX_SIGNATURE_LIFETIME();
    assert.equal(lifetime, 86_400n);
    // +10 s headroom: the next block's timestamp is latest + 1.
    const deadline = (await deadlineIn(lifetime)) + 10n;
    const signature = await sign(v8, alice, {
      human,
      commitment: 1110n,
      nullifier: 2110n,
      deadline,
    });
    await viem.assertions.revertWith(
      register(v8, { human, commitment: 1110n, nullifier: 2110n, deadline, signature }),
      "V8: deadline too far",
    );
  });

  // ─── replay / uniqueness ────────────────────────────────────────────────

  it("replay: the identical request a second time is rejected", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1120n, 2120n);
    await register(v8, req);
    await viem.assertions.revertWith(register(v8, req), "V8: already registered");
    assert.equal(await v8.read.totalHumans(), 1n);
  });

  it("replay after the register was cleared (e.g. Go sweep writes isHuman/nullifier back to zero): the nonce makes the old signature void", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1130n, 2130n);
    await register(v8, req);

    // Simulate Go releasing everything the register holds for Alice,
    // using the documented slots (v8_slots.json) — everything but the nonce.
    const addrKey = padHex(alice.account.address, { size: 32 });
    const nullKey = padHex(toHex(2130n), { size: 32 });
    const mapSlot = (key: Hex, slot: bigint) =>
      keccak256(concatHex([key, padHex(toHex(slot), { size: 32 })]));
    const zero = padHex("0x0", { size: 32 });
    const test = await viem.getTestClient();
    for (const slot of [
      mapSlot(addrKey, 3n), // isHuman
      mapSlot(padHex(toHex(1130n), { size: 32 }), 4n), // usedCommitments
      mapSlot(nullKey, 5n), // usedNullifiers
      mapSlot(addrKey, 6n), // commitmentOf
      mapSlot(addrKey, 7n), // nullifierOf
    ]) {
      await test.setStorageAt({ address: v8.address, index: slot, value: zero });
    }
    assert.equal(await v8.read.isHuman([alice.account.address]), false);
    assert.equal(await v8.read.nonces([alice.account.address]), 1n);

    await viem.assertions.revertWith(register(v8, req), "V8: invalid signature");

    // A fresh signature over the new nonce works — the nonce, not luck, decides.
    const fresh = await signedFor(v8, alice, 1130n, 2130n);
    await register(v8, fresh);
    assert.equal(await v8.read.nonces([alice.account.address]), 2n);
  });

  it("one biometric = one registration: a used nullifier is rejected for another wallet", async function () {
    const { v8 } = await deployV8();
    await register(v8, await signedFor(v8, alice, 1140n, 2140n));
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, bob, 1141n, 2140n)),
      "V8: nullifier used",
    );
    assert.equal(await v8.read.isHuman([bob.account.address]), false);
  });

  it("a used commitment is rejected for another wallet, even with a fresh nullifier", async function () {
    const { v8 } = await deployV8();
    await register(v8, await signedFor(v8, alice, 1150n, 2150n));
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, bob, 1150n, 2151n)),
      "V8: commitment used",
    );
  });

  it("field aliasing: nullifier + r (same field element) is rejected, not treated as new", async function () {
    const { v8 } = await deployV8();
    await register(v8, await signedFor(v8, alice, 1160n, 2160n));
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, bob, 1161n, 2160n + SNARK_SCALAR_FIELD)),
      "V8: public signal out of field",
    );
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, bob, 1161n + SNARK_SCALAR_FIELD, 2161n)),
      "V8: public signal out of field",
    );
  });

  it("rejects zero public signals", async function () {
    const { v8 } = await deployV8();
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, alice, 0n, 2170n)),
      "V8: zero public signal",
    );
    await viem.assertions.revertWith(
      register(v8, await signedFor(v8, alice, 1170n, 0n)),
      "V8: zero public signal",
    );
  });

  // ─── proof ──────────────────────────────────────────────────────────────

  it("rejects an invalid proof (tampered proof, and verifier saying no)", async function () {
    const { v8, verifier } = await deployV8();
    const req = await signedFor(v8, alice, 1180n, 2180n);
    await viem.assertions.revertWith(
      register(v8, { ...req, proof: BAD_PROOF }),
      "V8: invalid proof",
    );
    await verifier.write.setAccept([false]);
    await viem.assertions.revertWith(register(v8, req), "V8: invalid proof");
    assert.equal(await v8.read.isHuman([alice.account.address]), false);
    assert.equal(await v8.read.nonces([alice.account.address]), 0n);
    assert.equal(await v8.read.usedCommitments([1180n]), false);
  });

  // ─── signature encoding ─────────────────────────────────────────────────

  it("rejects the malleable twin (high s) of a valid signature", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1190n, 2190n);
    const r = sliceHex(req.signature, 0, 32);
    const s = hexToBigInt(sliceHex(req.signature, 32, 64));
    const v = hexToBigInt(sliceHex(req.signature, 64, 65));
    const twin = concatHex([
      r,
      numberToHex(SECP256K1_N - s, { size: 32 }),
      numberToHex(v === 27n ? 28n : 27n, { size: 1 }),
    ]);
    await viem.assertions.revertWith(
      register(v8, { ...req, signature: twin }),
      "V8: high s",
    );
    // The original still works.
    await register(v8, req);
  });

  it("rejects wrong length, v not in {27,28}, and garbage that recovers to address(0)", async function () {
    const { v8 } = await deployV8();
    const req = await signedFor(v8, alice, 1200n, 2200n);
    await viem.assertions.revertWith(
      register(v8, { ...req, signature: sliceHex(req.signature, 0, 64) }),
      "V8: bad signature length",
    );
    const v = hexToBigInt(sliceHex(req.signature, 64, 65));
    const vRaw = concatHex([
      sliceHex(req.signature, 0, 64),
      numberToHex(v - 27n, { size: 1 }),
    ]);
    await viem.assertions.revertWith(
      register(v8, { ...req, signature: vRaw }),
      "V8: bad signature v",
    );
    const garbage = concatHex([
      padHex("0x0", { size: 32 }),
      padHex("0x1", { size: 32 }),
      "0x1b",
    ]);
    await viem.assertions.revertWith(
      register(v8, { ...req, signature: garbage }),
      "V8: invalid signature",
    );
  });

  // ─── documented residual gap (circuit v3 does not bind the wallet) ──────

  it("GAP (documented, needs circuit v4): whoever holds an unused proof can sign it for their own wallet", async function () {
    // Circuit v3 has pubSignals = [commitment, nullifier]; the wallet is a
    // private input. So the contract cannot tell whether Mallory's wallet is
    // the one the proof was made for. What stops this in production is the
    // registrar check (only the node's relayer submits, after the node's
    // /prove provenance check) — NOT the contract. This test pins the
    // limitation so that circuit v4 (wallet as public signal) flips it
    // deliberately. See docs/V8_ENTWURF.md, "Circuit v4".
    const { v8 } = await deployV8();
    const stolen = await signedFor(v8, mallory, 1210n, 2210n);
    await register(v8, stolen);
    assert.equal(await v8.read.isHuman([mallory.account.address]), true);
  });
});

describe("AequitasV8 constructor", async function () {
  const { viem } = await network.create();
  const [registrar, other] = await viem.getWalletClients();

  it("rejects a zero verifier and a verifier without code", async function () {
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [
        "0x0000000000000000000000000000000000000000",
        [registrar.account.address],
      ]),
      "V8: verifier is zero",
    );
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [
        other.account.address,
        [registrar.account.address],
      ]),
      "V8: verifier has no code",
    );
  });

  it("bounds and validates the registrar list", async function () {
    const verifier = await viem.deployContract("MockGroth16Verifier");
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [verifier.address, []]),
      "V8: registrar count",
    );
    const seventeen = Array.from(
      { length: 17 },
      (_, i) => padHex(toHex(i + 1), { size: 20 }) as Address,
    );
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [verifier.address, seventeen]),
      "V8: registrar count",
    );
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [
        verifier.address,
        [registrar.account.address, registrar.account.address],
      ]),
      "V8: duplicate registrar",
    );
    await viem.assertions.revertWith(
      viem.deployContract("AequitasV8", [
        verifier.address,
        ["0x0000000000000000000000000000000000000000"],
      ]),
      "V8: registrar is zero",
    );
    const v8 = await viem.deployContract("AequitasV8", [
      verifier.address,
      seventeen.slice(0, 16),
    ]);
    assert.equal(await v8.read.isRegistrar([seventeen[15]]), true);
    assert.equal(await v8.read.isRegistrar([registrar.account.address]), false);
    assert.equal(
      (await v8.read.verifier()).toLowerCase(),
      verifier.address.toLowerCase(),
    );
  });
});

describe("AequitasV8 ERC-20 facade", async function () {
  const { viem } = await network.create();
  const [registrar, alice, bob] = await viem.getWalletClients();

  async function deploy() {
    const verifier = await viem.deployContract("MockGroth16Verifier");
    return viem.deployContract("AequitasV8", [
      verifier.address,
      [registrar.account.address],
    ]);
  }

  it("name/symbol/decimals are constants", async function () {
    const v8 = await deploy();
    assert.equal(await v8.read.name(), "Aequitas");
    assert.equal(await v8.read.symbol(), "AEQ");
    assert.equal(await v8.read.decimals(), 18);
  });

  it("transfer never moves value in the EVM: it reverts (the node books it in Go)", async function () {
    const v8 = await deploy();
    const publicClient = await viem.getPublicClient();
    const data = encodeFunctionData({
      abi: v8.abi,
      functionName: "transfer",
      args: [bob.account.address, 1n],
    });
    // As an eth_call …
    await assert.rejects(
      publicClient.call({ account: alice.account, to: v8.address, data }),
      /V8: transfer is booked by the chain/,
    );
    // … and as a real transaction (what a bypassed interception would send).
    await viem.assertions.revertWith(
      alice.sendTransaction({
        account: alice.account,
        chain: undefined,
        to: v8.address,
        data,
      }),
      "V8: transfer is booked by the chain",
    );
    assert.equal(
      toFunctionSelector("transfer(address,uint256)"),
      "0xa9059cbb",
      "selector must stay the ERC-20 one the node intercepts",
    );
  });

  it("has no approve/transferFrom/allowance (no second economy)", async function () {
    const v8 = await deploy();
    const names = v8.abi
      .filter((x) => x.type === "function")
      .map((x) => (x as { name: string }).name);
    for (const banned of ["approve", "transferFrom", "allowance", "mint", "burn"]) {
      assert.ok(!names.includes(banned), `${banned} must not exist on V8`);
    }
    const nonView = v8.abi.filter(
      (x) =>
        x.type === "function" &&
        (x as { stateMutability: string }).stateMutability === "nonpayable",
    );
    assert.deepEqual(
      nonView.map((x) => (x as { name: string }).name),
      ["registerWithSig"],
      "registerWithSig must be the only state-changing function",
    );
  });
});
