import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// AUDIT (2026-08-16, pre-launch full-contract audit): the escrow lifecycle.
//
// Every test in this file was written to CHARACTERISE what AequitasV7.sol
// actually does today, and each one is cross-referenced against the Go
// keeper's counterpart of the same rule (x/humanity/keeper/guardian.go,
// state.go). Where the two implementations disagree the comment names which
// side this audit believes is correct and why.
//
// These tests are GREEN on purpose: they assert the CURRENT (in three cases,
// broken) behaviour so the suite documents it precisely. Each one carries a
// "WHEN FIXED" note saying which assertion must be inverted once the
// underlying bug is addressed. The separate intentionally-RED tests that
// assert the *desired* behaviour live in
// AequitasV7_intentionally_red_audit.ts.
describe("AequitasV7 escrow lifecycle audit (Go-keeper cross-check)", async function () {
  const { viem, networkHelpers } = await network.create();
  const DAY = 24 * 60 * 60;
  const INACTIVITY_ESCROW = 910 * DAY;
  const INACTIVITY_UBI = 1460 * DAY;
  const AEQ = 10n ** 18n;

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7 };
  }

  let nextId = 1000n;
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

  // ── FINDING E1 ────────────────────────────────────────────────────────────
  // _confirmAlive() (AequitasV7.sol:436) mints fairShare() on every escrow
  // recovery, and fairShare() (AequitasV7.sol:597-600) is totalSupply /
  // totalHumans. With a single registered human that ratio IS the entire
  // supply, so one escrow->wake cycle DOUBLES totalSupply. The cycle is
  // repeatable every INACTIVITY_ESCROW (910 days) and needs no privileges:
  // triggerEscrow() is permissionless (AequitasV7.sol:452) and confirmAlive()
  // is the human's own call.
  //
  // The Go keeper has NO counterpart to this mint at all. RecoverFromEscrow
  // (guardian.go:372) credits exactly the escrowed `amount` and nothing more,
  // and TotalSupply() (state.go:5939-5966) is defined as humanCount x 1000
  // with the comment "each registered human receives exactly 1,000 AEQ, and
  // nothing else creates any". CORRECT SIDE: the Go keeper. The Solidity mint
  // is real money creation outside registration.
  // FIXED 2026-08-16 (v7.14): the wake-up bonus is now paid out of ubiPool
  // instead of minted, so totalSupply cannot move outside registration. This
  // test was the finding; it is now the regression guard, asserting exactly
  // what the finding's own "WHEN FIXED" line demanded.
  it("E1 (FIXED): totalSupply is unchanged by any number of escrow->confirmAlive cycles", async function () {
    const { v7 } = await deployV7();
    const [relayer, solo] = await viem.getWalletClients();
    await registerHuman(v7, relayer, solo);

    assert.equal(await v7.read.totalSupply(), 1_000n * AEQ);
    assert.equal(await v7.read.totalHumans(), 1n);

    for (let cycle = 0; cycle < 7; cycle++) {
      await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
      await v7.write.triggerEscrow([solo.account.address]);
      await v7.write.confirmAlive({ account: solo.account });

      assert.equal(
        await v7.read.totalSupply(),
        1_000n * AEQ,
        `cycle ${cycle}: totalSupply moved — money was created outside registration`,
      );
      // The core invariant must hold at every step, not just at the end:
      // paying the bonus from ubiPool moves value, it must not create it.
      const [supply, , pool] = await v7.read.getStats();
      assert.equal(
        (await v7.read.balanceOf([solo.account.address])) +
          (await v7.read.escrowOf([solo.account.address])) +
          pool,
        supply,
        `cycle ${cycle}: SUM(balanceOf)+SUM(escrowOf)+ubiPool != totalSupply`,
      );
    }

    assert.equal(await v7.read.totalSupply(), 1_000n * AEQ);
    assert.equal(await v7.read.totalHumans(), 1n);
  });

  // ── FINDING E2 ────────────────────────────────────────────────────────────
  // The escrow RECOVERY WINDOW is not anchored to when escrow actually
  // started. triggerEscrow() gates on lastActivity + INACTIVITY_ESCROW
  // (AequitasV7.sol:455) and triggerEscrowToUBI() gates on lastActivity +
  // INACTIVITY_UBI (AequitasV7.sol:467) — BOTH measured from the same
  // lastActivity stamp, and triggerEscrow() never updates it. Nothing
  // obliges anyone to call triggerEscrow() promptly at day 910, so an
  // attacker can simply wait until day 1460 and then call both in
  // immediate succession, collapsing the intended ~550-day recovery
  // window to zero.
  //
  // The Go keeper anchors the second stage to the escrow row's own moved_at
  // timestamp instead — releaseEscrowToUBILocked (guardian.go:622:
  // `threshold := time.Now().Unix() - escrowToUBISeconds` matched against
  // `WHERE moved_at < $1`) — so the 548-day window (escrowToUBISeconds,
  // guardian.go:35) is always granted in full no matter when the sweep to
  // escrow happened. CORRECT SIDE: the Go keeper.
  // FIXED 2026-08-16 (v7.14): triggerEscrow now stamps escrowedAt and
  // triggerEscrowToUBI measures the window from it, so the window is granted
  // in full however late the first stage is called.
  it("E2 (FIXED): a late triggerEscrow still grants the full ~550-day recovery window", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();
    await registerHuman(v7, relayer, victim);

    // Nobody calls triggerEscrow at day 910. The attacker waits out the full
    // INACTIVITY_UBI period first — the exact scenario that used to collapse
    // the window to zero.
    await networkHelpers.time.increase(INACTIVITY_UBI + 1);

    await v7.write.triggerEscrow([victim.account.address], { account: attacker.account });
    assert.equal(
      await v7.read.escrowOf([victim.account.address]),
      1_000n * AEQ,
      "the victim's whole balance is now in escrow",
    );

    // The sweep must NOT be reachable in the same block any more.
    await viem.assertions.revertWith(
      v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account }),
      "Too soon",
    );

    // Still blocked one second before the window closes.
    await networkHelpers.time.increase(INACTIVITY_UBI - INACTIVITY_ESCROW - 60);
    await viem.assertions.revertWith(
      v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account }),
      "Too soon",
    );

    // The victim, who now actually had the window the constants promise, wakes
    // up and reclaims everything.
    await v7.write.confirmAlive({ account: victim.account });
    assert.equal(await v7.read.escrowOf([victim.account.address]), 0n);
    assert.equal(await v7.read.balanceOf([victim.account.address]), 1_000n * AEQ);
    assert.equal(await v7.read.isHuman([victim.account.address]), true);
  });

  // Control for E2: the window DOES exist when triggerEscrow is called
  // promptly. This proves the ~550 days is the designed intent and that E2 is
  // a real loss of a guarantee, not a misreading of the constants.
  it("FINDING E2 (control): calling triggerEscrow promptly at day 910 does grant the intended ~550-day window", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();
    await registerHuman(v7, relayer, victim);

    await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrow([victim.account.address], { account: attacker.account });

    // Immediately afterwards the sweep is still ~550 days away.
    await viem.assertions.revertWith(
      v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account }),
      "Too soon",
    );

    // The victim can still wake up and reclaim everything.
    await v7.write.confirmAlive({ account: victim.account });
    assert.equal(await v7.read.escrowOf([victim.account.address]), 0n);
    assert.ok((await v7.read.balanceOf([victim.account.address])) >= 1_000n * AEQ);
    assert.equal(await v7.read.isHuman([victim.account.address]), true);
  });

  // ── FINDING E3 ────────────────────────────────────────────────────────────
  // triggerEscrowToUBI() clears isHuman and decrements totalHumans
  // (AequitasV7.sol:501-502) but NEVER clears usedNullifiers
  // (AequitasV7.sol:64, written at :225) or usedCommitments (:223). Because a
  // nullifier is a deterministic one-way function of a person's biometric
  // (register.go:1394-1399: "nullifier = SHA256(bioHash + ...)"), it can
  // never be regenerated. A human swept for inactivity is therefore banned
  // from the "one human = one registration" system permanently and with no
  // appeal — the exact opposite of "money exists because people exist".
  //
  // The Go keeper never de-registers anyone at all: there is no
  // `IsHuman = false` and no `humanCount--` anywhere in the non-test Go
  // sources (verified by grep across x/humanity/keeper). releaseEscrowToUBI
  // (guardian.go:618-681) takes the escrowed funds and leaves the human
  // registered. CORRECT SIDE: the Go keeper.
  // FIXED 2026-08-16 (v7.14): the sweep now releases usedNullifiers and
  // usedCommitments, so a living person can return. The second half of this
  // test is just as important as the first: returning must NOT mint a second
  // grant, or "release the nullifier" would have become a money printer on a
  // 1460-day timer.
  it("E3 (FIXED): a swept human can re-register, and does NOT receive a second grant", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();
    const reg = await registerHuman(v7, relayer, victim);

    await networkHelpers.time.increase(INACTIVITY_UBI + 1);
    await v7.write.triggerEscrow([victim.account.address], { account: attacker.account });
    await networkHelpers.time.increase(INACTIVITY_UBI - INACTIVITY_ESCROW + 1);
    await v7.write.triggerEscrowToUBI([victim.account.address], { account: attacker.account });
    assert.equal(await v7.read.isHuman([victim.account.address]), false);

    // The biometric records are released, not burned.
    assert.equal(
      await v7.read.usedNullifiers([reg.nullifierHex]),
      "0x0000000000000000000000000000000000000000",
      "the nullifier must be released, or the sweep is a permanent, unappealable exclusion",
    );

    const supplyBeforeReturn = await v7.read.totalSupply();

    // The very same living person, with the very same biometric, presents the
    // very same (still cryptographically valid) proof again. Accepted.
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [reg.commitment, reg.nullifier], victim.account.address, reg.signature, reg.nullifierHex],
      { account: relayer.account },
    );
    assert.equal(await v7.read.isHuman([victim.account.address]), true);
    assert.equal(await v7.read.totalHumans(), 1n);

    // ...but comes back with personhood, not with new money. Their original
    // 1000 AEQ went to the UBI pool when they were swept and stays there.
    assert.equal(
      await v7.read.totalSupply(),
      supplyBeforeReturn,
      "re-registration minted a second grant — the sweep/return cycle is a money printer",
    );
    assert.equal(await v7.read.balanceOf([victim.account.address]), 0n);
  });

  // ── FINDING E4 ────────────────────────────────────────────────────────────
  // Going through escrow FORGIVES every second of accrued demurrage.
  // triggerEscrow() (AequitasV7.sol:456-458) zeroes the balance without ever
  // calling _applyDemurrage(), and _confirmAlive() then resets the clock
  // outright with `lastDemurrage[human] = block.timestamp`
  // (AequitasV7.sol:405). Combined with the E1 wake-up mint, staying
  // maximally idle is strictly more profitable than participating — an
  // inversion of the entire premise of a demurrage currency.
  //
  // The Go keeper settles demurrage on BOTH edges: on the way in
  // ("balance zeroed + demurrage settled at the same moment",
  // guardian.go:404-406) and on the way out (RecoverFromEscrow calls
  // settleDemurrageLockedCtx at guardian.go:369 BEFORE crediting at :372).
  // CORRECT SIDE: the Go keeper.
  // FIXED 2026-08-16 (v7.14): triggerEscrow settles demurrage before zeroing
  // the balance, exactly as the finding's "WHEN FIXED" line demanded.
  //
  // The assertion deliberately measures the ESCROWED AMOUNT rather than the
  // balance after a full round-trip. Round-tripping would prove nothing here:
  // the wake-up bonus is now funded from ubiPool, and the demurrage just paid
  // went INTO that pool, so with only two humans the bonus hands most of it
  // straight back and the final balance looks untouched for reasons that have
  // nothing to do with whether the debt was settled. What must be true is that
  // the debt was charged at the moment of entry.
  it("E4 (FIXED): demurrage accrued before escrow is settled on the way in, not forgiven", async function () {
    const { v7 } = await deployV7();
    const [relayer, sleeper, funder] = await viem.getWalletClients();
    await registerHuman(v7, relayer, sleeper);
    await registerHuman(v7, relayer, funder);

    // Give `sleeper` an excess above fairShare so demurrage has something to
    // bite on (demurrage only touches the portion above fairShare()).
    await v7.write.transfer([sleeper.account.address, 900n * AEQ], { account: funder.account });
    const balBeforeSleep = await v7.read.balanceOf([sleeper.account.address]);
    const fairShare = await v7.read.fairShare();
    assert.ok(balBeforeSleep > fairShare, "sleeper must be above fair share for demurrage to apply");
    const excess = balBeforeSleep - fairShare;

    await networkHelpers.time.increase(INACTIVITY_ESCROW + 1);

    const poolBeforeEscrow = await v7.read.ubiPool();
    await v7.write.triggerEscrow([sleeper.account.address]);

    // Whatever was NOT escrowed was charged as demurrage at the moment of entry.
    const escrowed = await v7.read.escrowOf([sleeper.account.address]);
    const charged = balBeforeSleep - escrowed;

    // What 910 idle days cost, by the contract's own formula
    // (excess * DEMURRAGE_BPS * elapsed / (1e4 * 365d)).
    const owedFor910Days = (excess * 100n * BigInt(INACTIVITY_ESCROW)) / (10_000n * BigInt(365 * DAY));
    assert.ok(owedFor910Days > 20n * AEQ, `sanity: 910 days should cost >20 AEQ, got ${owedFor910Days}`);

    // Within 1% of the full figure — the debt was settled, not forgiven.
    const delta = charged > owedFor910Days ? charged - owedFor910Days : owedFor910Days - charged;
    assert.ok(
      delta * 100n < owedFor910Days,
      `entering escrow charged ${charged} wei of demurrage, but 910 idle days are worth ` +
        `${owedFor910Days} wei by the contract's own formula. A charge near zero means escrow ` +
        `is again forgiving the debt (regression of FINDING E4), which would make idling more ` +
        `profitable than participating.`,
    );

    // The settled demurrage lands in ubiPool, keeping the core invariant exact:
    // it is a transfer of value, not a deletion of it.
    assert.equal(
      await v7.read.ubiPool(),
      poolBeforeEscrow + charged,
      "settled demurrage must be credited to ubiPool, not destroyed",
    );

    // And the clock is reset for the escrow period itself: while in escrow the
    // human holds nothing, so no further decay may accrue for that span.
    await v7.write.confirmAlive({ account: sleeper.account });
    const balAfterWake = await v7.read.balanceOf([sleeper.account.address]);
    const fsAfterWake = await v7.read.fairShare();
    if (balAfterWake > fsAfterWake) {
      const balBeforeApply = balAfterWake;
      await v7.write.applyDemurrage([sleeper.account.address]);
      const chargedAfterWake = balBeforeApply - (await v7.read.balanceOf([sleeper.account.address]));
      assert.ok(
        chargedAfterWake * 1_000n < owedFor910Days,
        `waking up must not immediately re-charge the escrow period: ${chargedAfterWake} wei taken`,
      );
    }
  });
});
