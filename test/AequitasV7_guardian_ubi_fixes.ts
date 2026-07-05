import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// Regression tests for the two beta-launch-audit (2026-07-05) fixes bundled
// into this AequitasV7 version alongside the register() removal:
//   1. confirmGuardian() TOCTOU: re-validates both layering invariants at
//      confirm time (not just propose time), closing a race where two
//      pending proposals confirmed in either order could still assemble a
//      multi-hop guardian chain (C guards B, B guards A).
//   2. triggerEscrowToUBI() forfeited-UBI leak: a swept human's unclaimed
//      UBI entitlement is now recycled into ubiPool instead of silently
//      violating the SUM(balanceOf)+SUM(escrowOf)+ubiPool == totalSupply
//      invariant forever.
describe("AequitasV7 beta-launch-audit fixes", async function () {
  const DAY = 24 * 60 * 60;
  const GUARDIAN_TIMELOCK = 7 * DAY;
  const INACTIVITY_ESCROW = 910 * DAY;
  const INACTIVITY_UBI = 1460 * DAY;

  const { viem, networkHelpers } = await network.create();

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7, bio };
  }

  let nextCommitment = 1n;
  async function registerHuman(
    v7: Awaited<ReturnType<typeof deployV7>>["v7"],
    relayer: Awaited<ReturnType<typeof viem.getWalletClients>>[number],
    human: Awaited<ReturnType<typeof viem.getWalletClients>>[number],
  ) {
    const commitment = nextCommitment++;
    const nullifier = nextCommitment++;
    const nullifierHex = padHex(toHex(nullifier), { size: 32 });
    const chainId = BigInt(await (await viem.getPublicClient()).getChainId());

    const messageHash = keccak256(
      encodePacked(
        ["uint256", "address", "string", "uint256", "bytes32"],
        [chainId, v7.address, "register", commitment, nullifierHex],
      ),
    );
    const signature = await human.signMessage({
      message: { raw: messageHash },
    });

    await v7.write.registerWithSig(
      [
        [0n, 0n],
        [
          [0n, 0n],
          [0n, 0n],
        ],
        [0n, 0n],
        [commitment, nullifier],
        human.account.address,
        signature,
        nullifierHex,
      ],
      { account: relayer.account },
    );
  }

  it("confirmGuardian rejects completing a chain when the guardian confirmed their own guardian first (B confirms C, then A tries to confirm B)", async function () {
    const { v7 } = await deployV7();
    const [relayer, a, b, c] = await viem.getWalletClients();
    await registerHuman(v7, relayer, a);
    await registerHuman(v7, relayer, b);
    await registerHuman(v7, relayer, c);

    await v7.write.proposeGuardian([b.account.address], { account: a.account });
    await v7.write.proposeGuardian([c.account.address], { account: b.account });

    await networkHelpers.time.increase(GUARDIAN_TIMELOCK + 1);

    // B confirms first: guardianOf[B] = C.
    await v7.write.confirmGuardian({ account: b.account });
    assert.equal(
      (await v7.read.guardianOf([b.account.address])).toLowerCase(),
      c.account.address.toLowerCase(),
    );

    // A's confirm must now fail: B (the guardian A is confirming) has
    // acquired their own guardian during the timelock window.
    await viem.assertions.revertWith(
      v7.write.confirmGuardian({ account: a.account }),
      "Guardian now has own guardian",
    );
  });

  it("confirmGuardian rejects completing a chain when the ward confirms first (D confirms E, then E tries to confirm F)", async function () {
    const { v7 } = await deployV7();
    const [relayer, d, e, f] = await viem.getWalletClients();
    await registerHuman(v7, relayer, d);
    await registerHuman(v7, relayer, e);
    await registerHuman(v7, relayer, f);

    await v7.write.proposeGuardian([e.account.address], { account: d.account });
    await v7.write.proposeGuardian([f.account.address], { account: e.account });

    await networkHelpers.time.increase(GUARDIAN_TIMELOCK + 1);

    // D confirms first: guardianOf[D] = E, wardCount[E] = 1.
    await v7.write.confirmGuardian({ account: d.account });
    assert.equal(await v7.read.wardCount([e.account.address]), 1n);

    // E's own confirm (E wants F as E's guardian) must now fail: E is
    // already acting as D's guardian (wardCount[E] > 0).
    await viem.assertions.revertWith(
      v7.write.confirmGuardian({ account: e.account }),
      "Cannot confirm a guardian while acting as one",
    );
  });

  it("still allows a normal, non-racing guardian confirmation", async function () {
    const { v7 } = await deployV7();
    const [relayer, g, h] = await viem.getWalletClients();
    await registerHuman(v7, relayer, g);
    await registerHuman(v7, relayer, h);

    await v7.write.proposeGuardian([h.account.address], { account: g.account });
    await networkHelpers.time.increase(GUARDIAN_TIMELOCK + 1);
    await v7.write.confirmGuardian({ account: g.account });

    assert.equal(
      (await v7.read.guardianOf([g.account.address])).toLowerCase(),
      h.account.address.toLowerCase(),
    );
    assert.equal(await v7.read.wardCount([h.account.address]), 1n);
  });

  it("triggerEscrowToUBI recycles a swept human's forfeited UBI entitlement into ubiPool instead of losing it", async function () {
    const { v7 } = await deployV7();
    const [relayer, w1, w2] = await viem.getWalletClients();
    await registerHuman(v7, relayer, w1);
    await registerHuman(v7, relayer, w2);

    // Generate a fee (and therefore ubiPool balance) via a transfer.
    await v7.write.transfer([w2.account.address, 100n * 10n ** 18n], {
      account: w1.account,
    });

    const poolAfterTransfer = await v7.read.ubiPool();
    assert.ok(poolAfterTransfer > 0n, "expected transfer fee to fund ubiPool");

    await v7.write.accumulateUBI();
    const perHuman = await v7.read.ubiPerHumanAccumulated();
    assert.ok(perHuman > 0n, "expected accumulateUBI to record a per-human entitlement");

    // w1 never calls claimUBI(), so their full entitlement is outstanding.
    const w1ClaimedBefore = await v7.read.ubiClaimed([w1.account.address]);
    assert.equal(w1ClaimedBefore, 0n);
    const expectedForfeited = perHuman - w1ClaimedBefore;

    // Send w1 to sleep, then far enough past INACTIVITY_ESCROW to escrow them.
    await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrow([w1.account.address]);
    const escrowedAmount = await v7.read.escrowOf([w1.account.address]);
    assert.ok(escrowedAmount > 0n);

    const poolBeforeSweep = await v7.read.ubiPool();

    // Advance the remaining time to pass INACTIVITY_UBI (measured from the
    // same lastActivity as escrow, so only the remaining delta is needed).
    await networkHelpers.time.increase(INACTIVITY_UBI - INACTIVITY_ESCROW + 1);

    await viem.assertions.emitWithArgs(
      v7.write.triggerEscrowToUBI([w1.account.address]),
      v7,
      "ForfeitedUBIRecycled",
      [w1.account.address, expectedForfeited],
    );

    const poolAfterSweep = await v7.read.ubiPool();
    assert.equal(
      poolAfterSweep,
      poolBeforeSweep + escrowedAmount + expectedForfeited,
      "ubiPool must gain both the escrowed balance and the forfeited UBI entitlement",
    );
    assert.equal(await v7.read.isHuman([w1.account.address]), false);
    assert.equal(
      await v7.read.ubiClaimed([w1.account.address]),
      perHuman,
      "ubiClaimed should be settled to the full accumulated amount, closing the gap",
    );
  });
});
