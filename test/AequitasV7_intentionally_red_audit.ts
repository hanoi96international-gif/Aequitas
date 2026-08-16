import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// ╔══════════════════════════════════════════════════════════════════════════╗
// ║  INTENTIONALLY RED — EVERY TEST IN THIS FILE IS EXPECTED TO FAIL TODAY.  ║
// ╚══════════════════════════════════════════════════════════════════════════╝
//
// AUDIT (2026-08-16, pre-launch full-contract audit).
//
// The companion files AequitasV7_escrow_lifecycle_audit.ts and
// AequitasV7_demurrage_cap_fee_audit.ts are green: they characterise what
// AequitasV7.sol does today. THIS file asserts what the audit believes it
// SHOULD do — the behaviour the whitepaper, the README, the contract's own
// comments, and (in every case below) the Go keeper already implement.
//
// These failures are the findings. They are not flaky, not environment
// dependent, and must not be "fixed" by weakening the assertions. Each one
// goes green exactly when the corresponding contract bug is fixed, at which
// point this file becomes an ordinary regression suite.
//
// Expected state at time of writing: 4 failing, 0 passing.
describe("AequitasV7 INTENTIONALLY-RED audit assertions (expected to fail until fixed)", async function () {
  const { viem, networkHelpers } = await network.create();
  const DAY = 24 * 60 * 60;
  const YEAR = 365 * DAY;
  const INACTIVITY_ESCROW = 910 * DAY;
  const INACTIVITY_UBI = 1460 * DAY;
  const AEQ = 10n ** 18n;

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7 };
  }

  let nextId = 9000n;
  async function registerHuman(
    v7: Awaited<ReturnType<typeof deployV7>>["v7"],
    relayer: Awaited<ReturnType<typeof viem.getWalletClients>>[number],
    human: Awaited<ReturnType<typeof viem.getWalletClients>>[number],
  ) {
    const commitment = nextId++;
    const nullifier = nextId++;
    const nullifierHex = padHex(toHex(nullifier), { size: 32 });
    const chainId = BigInt(await (await viem.getPublicClient()).getChainId());
    const messageHash = keccak256(
      encodePacked(
        ["uint256", "address", "string", "uint256", "bytes32"],
        [chainId, v7.address, "register", commitment, nullifierHex],
      ),
    );
    const signature = await human.signMessage({ message: { raw: messageHash } });
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [commitment, nullifier], human.account.address, signature, nullifierHex],
      { account: relayer.account },
    );
    return { commitment, nullifier, nullifierHex, signature };
  }

  // RED 1 — the supply promise.
  // "Money is created in exactly one place: registration grants 1000 AEQ."
  // Go enforces this by construction: TotalSupply() IS humanCount * 1000
  // (state.go:5939-5966). Solidity lets one human double the entire supply
  // every 910 days via the _confirmAlive wake-up mint (AequitasV7.sol:436).
  it("RED 1: totalSupply must equal 1000 AEQ x registrations, no matter how many escrow cycles run", async function () {
    const { v7 } = await deployV7();
    const [relayer, solo] = await viem.getWalletClients();
    await registerHuman(v7, relayer, solo);

    for (let cycle = 0; cycle < 5; cycle++) {
      await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
      await v7.write.triggerEscrow([solo.account.address]);
      await v7.write.confirmAlive({ account: solo.account });
    }

    assert.equal(
      await v7.read.totalSupply(),
      1_000n * AEQ,
      "one registration must mean exactly 1000 AEQ of supply, forever — see state.go:5939-5966",
    );
  });

  // RED 2 — the escrow recovery guarantee.
  // The constants promise ~550 days between escrow and forfeiture
  // (INACTIVITY_UBI 1460d - INACTIVITY_ESCROW 910d). Because both stages are
  // measured from the same lastActivity stamp (AequitasV7.sol:455 and :467),
  // a late triggerEscrow erases the window entirely. Go anchors forfeiture to
  // the escrow row's own moved_at (guardian.go:622), so the window is always
  // granted in full.
  it("RED 2: forfeiture must never be possible in the same block escrow was created", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();
    await registerHuman(v7, relayer, victim);

    await networkHelpers.time.increase(INACTIVITY_UBI + 1);
    await v7.write.triggerEscrow([victim.account.address], { account: attacker.account });

    await viem.assertions.revertWith(
      v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account }),
      "Too soon",
    );
  });

  // RED 3 — "one human = one registration", not "one human = one chance".
  // triggerEscrowToUBI de-registers (AequitasV7.sol:501-502) without ever
  // releasing usedNullifiers/usedCommitments, and a biometric nullifier is
  // deterministic and unregenerable (register.go:1394-1399). Go never
  // de-registers anyone: there is no `IsHuman = false` in the non-test Go
  // sources at all.
  it("RED 3: a living human swept for inactivity must be able to register again", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();
    const reg = await registerHuman(v7, relayer, victim);

    await networkHelpers.time.increase(INACTIVITY_UBI + 1);
    await v7.write.triggerEscrow([victim.account.address], { account: attacker.account });
    // The recovery window that RED 2 restored has to actually elapse now.
    // Waiting it out is what a legitimate sweep looks like — it does not
    // weaken this test's assertion, which is about what happens AFTER a
    // completed sweep.
    await networkHelpers.time.increase(INACTIVITY_UBI - INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account });

    // Same person, same biometric, same still-valid proof.
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [reg.commitment, reg.nullifier], victim.account.address, reg.signature, reg.nullifierHex],
      { account: relayer.account },
    );
    assert.equal(await v7.read.isHuman([victim.account.address]), true);
  });

  // RED 4 — demurrage must not be retroactive.
  // transfer() settles the sender only (AequitasV7.sol:271) and never touches
  // lastDemurrage[to] even though it does update lastActivity[to]
  // (AequitasV7.sol:282). Go settles the recipient before crediting
  // (state.go:4972, immediately before the credit at state.go:4976).
  it("RED 4: a human must not be charged demurrage for time before they held the funds", async function () {
    const { v7 } = await deployV7();
    const [relayer, dormant, funder] = await viem.getWalletClients();
    await registerHuman(v7, relayer, dormant);
    await registerHuman(v7, relayer, funder);

    await networkHelpers.time.increase(4 * YEAR);
    await v7.write.transfer([dormant.account.address, 900n * AEQ], { account: funder.account });

    // The funds have been dormant's for one block. Acting now must cost only
    // the amount sent.
    const before = await v7.read.balanceOf([dormant.account.address]);
    await v7.write.transfer([funder.account.address, 1n * AEQ], { account: dormant.account });
    const after = await v7.read.balanceOf([dormant.account.address]);

    const demurrageCharged = before - after - 1n * AEQ;
    // Allow a dust tolerance for the one block that genuinely elapsed.
    assert.ok(
      demurrageCharged < AEQ / 1_000_000n,
      `demurrage of ${demurrageCharged} wei was charged on funds held for a single block; ` +
        `expected ~0. transfer() must call _applyDemurrage(to) before crediting.`,
    );
  });
});
