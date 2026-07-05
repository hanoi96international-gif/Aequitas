import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// FIX (P0-2, beta-launch audit 2026-07-05): _confirmAlive() used to
// silently forfeit a recovering human's already-accrued, already-pool-
// deducted UBI entitlement instead of crediting it to balanceOf, unlike
// claimUBI() which did this correctly. Same bug class as the
// triggerEscrowToUBI forfeited-UBI leak fixed earlier (see
// ForfeitedUBIRecycled in AequitasV7.sol and the regression tests in
// AequitasV7_guardian_ubi_fixes.ts) — the parallel path through
// _confirmAlive (reached via confirmAlive()/guardianConfirmAlive()) was
// missed by that fix pass. Fixed by mirroring claimUBI()'s owed
// calculation exactly before ubiClaimed[human] is overwritten.
//
// This test was originally written to FAIL against the buggy contract,
// proving the invariant SUM(balanceOf)+SUM(escrowOf)+ubiPool == totalSupply
// was violated and that the owed UBI never landed in balanceOf. Now that
// _confirmAlive() is fixed, it's a regression test guarding the same
// invariant going forward — the assertions are unchanged from the original
// proof, only this header comment and the describe() label changed.
describe("AequitasV7 _confirmAlive UBI-forfeiture fix (P0 regression)", async function () {
  const DAY = 24 * 60 * 60;
  const INACTIVITY_ESCROW = 910 * DAY;

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

  it("confirmAlive() must credit UBI accrued while the human sat in escrow, not silently discard it", async function () {
    const { v7 } = await deployV7();
    const [relayer, w1, w2] = await viem.getWalletClients();
    await registerHuman(v7, relayer, w1);
    await registerHuman(v7, relayer, w2);

    // Fund ubiPool via a transfer fee, then accumulate a first UBI round
    // BEFORE w1 goes to sleep, so w1 has a real, already-pool-deducted
    // entitlement outstanding when they enter escrow.
    await v7.write.transfer([w2.account.address, 100n * 10n ** 18n], {
      account: w1.account,
    });
    assert.ok((await v7.read.ubiPool()) > 0n, "expected transfer fee to fund ubiPool");
    await v7.write.accumulateUBI();
    const perHumanBeforeEscrow = await v7.read.ubiPerHumanAccumulated();
    assert.ok(perHumanBeforeEscrow > 0n, "expected first accumulateUBI round to record an entitlement");

    // w1 goes inactive long enough to be escrowed. w1 has NOT called
    // claimUBI(), so their full entitlement (perHumanBeforeEscrow) is
    // outstanding at the moment of escrow.
    await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrow([w1.account.address]);
    const escrowedAmount = await v7.read.escrowOf([w1.account.address]);
    assert.ok(escrowedAmount > 0n, "expected w1 to have a positive balance moved into escrow");
    assert.equal(
      await v7.read.ubiClaimed([w1.account.address]),
      0n,
      "w1 must still show zero claimed at the moment of escrow",
    );

    // While w1 sits in escrow, isHuman[w1] is still true and totalHumans is
    // unchanged (triggerEscrow does not touch either), so w1 keeps counting
    // in every subsequent accumulateUBI() division and their per-head share
    // keeps being correctly reserved out of ubiPool on their behalf. Fund a
    // second round via a w2 transfer, then accumulate again.
    await v7.write.transfer([w1.account.address, 1n], { account: w2.account });
    // (w1 balanceOf is 0 while in escrow; a 1-wei transfer TO w1 just adds
    // 1 wei minus fee to their zeroed balance and is irrelevant to the bug —
    // it only exists to generate a fresh fee into ubiPool from w2's side.)
    if ((await v7.read.ubiPool()) > 0n) {
      await v7.write.accumulateUBI();
    }
    const perHumanAfterEscrowAccrual = await v7.read.ubiPerHumanAccumulated();
    assert.ok(
      perHumanAfterEscrowAccrual > perHumanBeforeEscrow,
      "expected ubiPerHumanAccumulated to grow further while w1 was still in escrow",
    );

    const owedAtWakeup = perHumanAfterEscrowAccrual - (await v7.read.ubiClaimed([w1.account.address]));
    assert.ok(owedAtWakeup > 0n, "w1 should be owed a non-zero UBI entitlement at wake-up");

    const balanceBeforeWakeup = await v7.read.balanceOf([w1.account.address]);
    const fairShareAtWakeup = await v7.read.fairShare();

    // Wake up: this returns the escrow principal + one fairShare() wake-up
    // bonus mint. It should ALSO credit owedAtWakeup, mirroring claimUBI().
    await v7.write.confirmAlive({ account: w1.account });

    const balanceAfterWakeup = await v7.read.balanceOf([w1.account.address]);
    const actualIncrease = balanceAfterWakeup - balanceBeforeWakeup;
    const expectedIncrease = escrowedAmount + fairShareAtWakeup + owedAtWakeup;

    // THE BUG: current AequitasV7 code sets ubiClaimed[w1] = ubiPerHumanAccumulated
    // without ever adding owedAtWakeup to balanceOf[w1]. actualIncrease will
    // equal only (escrowedAmount + fairShareAtWakeup), silently short by
    // owedAtWakeup — the exact amount already deducted from ubiPool on w1's
    // behalf, now unaccounted for anywhere in the contract's tracked state.
    assert.equal(
      actualIncrease,
      expectedIncrease,
      `confirmAlive() must credit the UBI owed at wake-up. ` +
        `balanceOf increased by ${actualIncrease} but should have increased by ` +
        `${expectedIncrease} (escrow ${escrowedAmount} + wake-up bonus ${fairShareAtWakeup} + owed UBI ${owedAtWakeup}). ` +
        `Missing ${expectedIncrease - actualIncrease} wei of AEQ that was already deducted from ubiPool ` +
        `on w1's behalf but never landed in balanceOf, escrowOf, or ubiPool — a silent, permanent leak ` +
        `that breaks SUM(balanceOf)+SUM(escrowOf)+ubiPool == totalSupply.`,
    );

    // Also confirm the invariant directly, summed across both humans plus
    // the pool, against totalSupply. Under the pull-UBI pattern this only
    // holds once every human has actually pulled what accumulateUBI()
    // already reserved for them out of ubiPool — w2 never called
    // claimUBI() in this test, so w2 still has an outstanding entitlement
    // sitting nowhere in balanceOf/escrowOf/ubiPool (exactly the same kind
    // of "reserved but not yet pulled" gap w1 had before confirmAlive(),
    // not a leak). Have w2 claim it so this checks the real
    // fund-conservation property (everything reserved is accounted for
    // once claimed), not a false positive from an unrelated human's
    // unclaimed balance.
    if ((await v7.read.claimableUBI([w2.account.address])) > 0n) {
      await v7.write.claimUBI({ account: w2.account });
    }
    const totalSupply = await v7.read.totalSupply();
    const sumBalances =
      (await v7.read.balanceOf([w1.account.address])) + (await v7.read.balanceOf([w2.account.address]));
    const sumEscrow =
      (await v7.read.escrowOf([w1.account.address])) + (await v7.read.escrowOf([w2.account.address]));
    const pool = await v7.read.ubiPool();
    assert.equal(
      sumBalances + sumEscrow + pool,
      totalSupply,
      "core invariant SUM(balanceOf)+SUM(escrowOf)+ubiPool == totalSupply must hold once every entitlement is claimed",
    );
  });
});
