import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// AUDIT (2026-08-16 full contract audit, launch readiness): checks the
// contract's core supply invariant — extended to include outstanding
// (unclaimed) UBI entitlements, see assertInvariant's own comment below for
// why the plain SUM(balanceOf)+SUM(escrowOf)+ubiPool version from the
// contract's header comment isn't exactly true mid-cycle — holds at the
// CONTRACT LAYER across a realistic sequence touching every balance-moving
// path: registration, a fee-generating transfer, accumulateUBI/claimUBI,
// escrow, and the confirmAlive wake-up mint. This is the property that
// "total supply == registered humans x 1000 AEQ (plus documented wake-up
// mints)" ultimately rests on — if any path silently drops or double-counts
// value, this assertion is where it would show up.
//
// First draft of this test used the literal header-comment formula and
// failed after accumulateUBI() with an apparent ~5 AEQ shortfall — traced by
// hand to unclaimed claimableUBI() (an intentional O(1) pull-payment
// accrual, not a leak) rather than a real bug.
describe("AequitasV7 core supply invariant across a mixed operation sequence", async function () {
  const { viem, networkHelpers } = await network.create();
  const DAY = 24 * 60 * 60;
  const INACTIVITY_ESCROW = 910 * DAY;

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7, bio };
  }

  let nextId = 1n;
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
  }

  // NOTE: the contract's header comment states the invariant as
  //   SUM(balanceOf) + SUM(escrowOf) + ubiPool == totalSupply
  // but that is only exactly true immediately after accumulateUBI() if no
  // human has an outstanding (unclaimed) UBI entitlement. accumulateUBI()
  // moves value OUT of ubiPool by crediting a per-head ACCRUAL RATE
  // (ubiPerHumanAccumulated) rather than looping over every human's
  // balanceOf (the whole point of the O(1) pull-UBI pattern the contract's
  // own top comment calls out as "FIX [1]") — so between an accumulateUBI()
  // call and each human's own claimUBI(), that human's share is real,
  // owed value that is temporarily accounted for in NEITHER balanceOf NOR
  // ubiPool. It is fully and exactly represented by the contract's own
  // claimableUBI(address) view instead. This extended form is the TRUE
  // invariant; the version in the header comment is a simplified
  // description that implicitly assumes all UBI has been claimed.
  async function assertInvariant(v7: Awaited<ReturnType<typeof deployV7>>["v7"], addrs: `0x${string}`[], label: string) {
    let sumBalances = 0n;
    let sumEscrow = 0n;
    let sumClaimable = 0n;
    for (const a of addrs) {
      sumBalances += await v7.read.balanceOf([a]);
      sumEscrow += await v7.read.escrowOf([a]);
      sumClaimable += await v7.read.claimableUBI([a]);
    }
    const ubiPool = await v7.read.ubiPool();
    const totalSupply = await v7.read.totalSupply();
    assert.equal(
      sumBalances + sumEscrow + ubiPool + sumClaimable,
      totalSupply,
      `[${label}] SUM(balanceOf)+SUM(escrowOf)+ubiPool+SUM(claimableUBI) must equal totalSupply — ` +
        `got balances=${sumBalances} escrow=${sumEscrow} ubiPool=${ubiPool} claimable=${sumClaimable} totalSupply=${totalSupply}`,
    );
  }

  it("holds after register -> transfer (fee) -> accumulateUBI -> claimUBI -> escrow -> confirmAlive (wake-up mint)", async function () {
    const { v7 } = await deployV7();
    const [relayer, w1, w2, w3] = await viem.getWalletClients();
    const addrs = [w1, w2, w3].map((w) => w.account.address);

    await registerHuman(v7, relayer, w1);
    await registerHuman(v7, relayer, w2);
    await registerHuman(v7, relayer, w3);
    await assertInvariant(v7, addrs, "after 3 registrations");
    assert.equal(await v7.read.totalSupply(), 3_000n * 10n ** 18n, "3 humans x 1000 AEQ");

    // w1 is a large holder relative to totalSupply (33%) so the concentration
    // friction fee kicks in on this transfer even with TX_FEE_BPS == 0.
    await v7.write.transfer([w2.account.address, 500n * 10n ** 18n], { account: w1.account });
    await assertInvariant(v7, addrs, "after fee-generating transfer");
    const poolAfterTransfer = await v7.read.ubiPool();

    // With UBI_SHARE_BPS == 10_000 (100%), the fee should be fully routed to
    // ubiPool and NOTHING burned — totalSupply must be unchanged by a transfer.
    assert.equal(await v7.read.totalSupply(), 3_000n * 10n ** 18n,
      "a plain transfer must never change totalSupply given UBI_SHARE_BPS=100%");

    if (poolAfterTransfer > 0n) {
      await v7.write.accumulateUBI();
      await assertInvariant(v7, addrs, "after accumulateUBI");

      await v7.write.claimUBI({ account: w3.account });
      await assertInvariant(v7, addrs, "after claimUBI");
    }

    // Send w1 to sleep and escrow them.
    await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrow([w1.account.address]);
    await assertInvariant(v7, addrs, "after triggerEscrow");
    assert.ok((await v7.read.escrowOf([w1.account.address])) > 0n);

    // w1 wakes up: confirmAlive() returns the escrowed amount and pays a
    // wake-up bonus. Since v7.14 (RED 1) that bonus is funded from ubiPool
    // rather than minted, so totalSupply must NOT move — money is created in
    // exactly one place, registration.
    const supplyBeforeWake = await v7.read.totalSupply();
    const poolBeforeWake = await v7.read.ubiPool();
    await v7.write.confirmAlive({ account: w1.account });
    await assertInvariant(v7, addrs, "after confirmAlive wake-up");
    assert.equal(
      await v7.read.totalSupply(),
      supplyBeforeWake,
      "confirmAlive must not change totalSupply — the wake-up bonus comes out of ubiPool, " +
        "it is not minted (regression of RED 1, the unbounded-mint finding)",
    );
    // Whatever the human received beyond their escrow came out of the pool,
    // never from thin air.
    assert.ok(
      (await v7.read.ubiPool()) <= poolBeforeWake,
      "the wake-up bonus must be debited from ubiPool",
    );
    assert.equal(await v7.read.escrowOf([w1.account.address]), 0n);
  });
});
