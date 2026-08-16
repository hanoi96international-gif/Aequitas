import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// AUDIT (2026-08-16, pre-launch full-contract audit): demurrage timing, the
// wealth cap, the fee schedule, and ERC-20 event surface.
//
// As in AequitasV7_escrow_lifecycle_audit.ts, these tests are GREEN on
// purpose — they pin down what the contract does TODAY and cross-reference
// the Go keeper's counterpart of each rule. "WHEN FIXED" notes say which
// assertion has to be inverted once a bug is addressed.
describe("AequitasV7 demurrage / wealth-cap / fee audit (Go-keeper cross-check)", async function () {
  const { viem, networkHelpers } = await network.create();
  const DAY = 24 * 60 * 60;
  const YEAR = 365 * DAY;
  const AEQ = 10n ** 18n;

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7 };
  }

  let nextId = 5000n;
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

  // ── FINDING D1 ────────────────────────────────────────────────────────────
  // transfer() settles demurrage for the SENDER only (AequitasV7.sol:271) and
  // updates lastActivity[to] but NOT lastDemurrage[to] (AequitasV7.sol:282).
  // claimUBI() credits balanceOf without settling either (AequitasV7.sol:367).
  // So a long-dormant human's demurrage clock keeps pointing at their
  // registration date, and the first time they act after receiving money they
  // are charged years of decay on funds they held for seconds.
  //
  // The Go keeper settles the RECIPIENT before crediting them —
  // transferWithV7FeeLocked calls settleDemurrageLockedCtx(ctx, toAcc) at
  // state.go:4972, immediately before `toAcc.Balance.Add(...)` at
  // state.go:4976 — and does the same on escrow recovery (guardian.go:369).
  // CORRECT SIDE: the Go keeper. Demurrage is a charge for holding idle
  // money; charging for a period before the money existed in that account is
  // not that.
  it("FINDING D1: a dormant human is charged YEARS of demurrage on funds they received seconds ago", async function () {
    const { v7 } = await deployV7();
    const [relayer, dormant, funder, control] = await viem.getWalletClients();
    await registerHuman(v7, relayer, dormant);
    await registerHuman(v7, relayer, funder);
    await registerHuman(v7, relayer, control);

    // Four years pass. Nobody touches `dormant`, so lastDemurrage[dormant] is
    // still their registration timestamp. Their balance is exactly fairShare,
    // so they genuinely owe nothing for this period.
    await networkHelpers.time.increase(4 * YEAR);

    // CONTROL: `control` is equally dormant but receives nothing. Their first
    // transfer costs zero demurrage, because balance <= fairShare().
    const controlBefore = await v7.read.balanceOf([control.account.address]);
    await v7.write.transfer([funder.account.address, 1n * AEQ], { account: control.account });
    const controlAfter = await v7.read.balanceOf([control.account.address]);
    assert.equal(
      controlBefore - controlAfter,
      1n * AEQ,
      "control: an equally-dormant human at fair share pays exactly the transferred amount, no demurrage",
    );

    // Now `funder` sends `dormant` a large amount. dormant's lastDemurrage is
    // untouched by this credit — that is the bug.
    await v7.write.transfer([dormant.account.address, 900n * AEQ], { account: funder.account });
    const dormantBal = await v7.read.balanceOf([dormant.account.address]);
    const fairShare = await v7.read.fairShare();
    const excess = dormantBal - fairShare;
    assert.ok(excess > 0n, "dormant is now above fair share");

    // dormant acts immediately — the money has been theirs for one block.
    const before = await v7.read.balanceOf([dormant.account.address]);
    await v7.write.transfer([funder.account.address, 1n * AEQ], { account: dormant.account });
    const after = await v7.read.balanceOf([dormant.account.address]);
    const demurrageCharged = before - after - 1n * AEQ; // minus the amount actually sent

    // Four years of decay on the excess, per the contract's own formula
    // (AequitasV7.sol:320): excess * DEMURRAGE_BPS(100) * elapsed / (1e4 * 1y).
    const fourYearCharge = (excess * 100n * BigInt(4 * YEAR)) / (10_000n * BigInt(YEAR));
    assert.ok(fourYearCharge > 30n * AEQ, `sanity: 4y on ${excess} wei should exceed 30 AEQ`);

    // FIXED 2026-08-16 (v7.14): transfer() settles the recipient before
    // crediting them, so the clock starts when the money arrives. The finding's
    // own "WHEN FIXED" line called for exactly this inversion: the charge must
    // now be a rounding artefact of the single elapsed block, not four years.
    assert.ok(
      demurrageCharged < AEQ / 1_000_000n,
      `dormant was charged ${demurrageCharged} wei of demurrage on money held for one block. ` +
        `A full four years of decay would be ${fourYearCharge} wei; anything near that figure ` +
        `means transfer() is again billing the recipient for a period during which they did ` +
        `not hold the funds (regression of FINDING D1).`,
    );
    // The control path must be unaffected: settling the recipient must not
    // charge anyone who is at or below fair share.
    assert.equal(controlBefore - controlAfter, 1n * AEQ);
  });

  // ── FINDING W1 ────────────────────────────────────────────────────────────
  // wealthCap() = CAPS[phase] * fairShare() (AequitasV7.sol:602-604) and
  // fairShare() = totalSupply / totalHumans (AequitasV7.sol:597-600). At
  // phase 0 CAPS[0] is 50 (AequitasV7.sol:95), so the cap equals
  // 50 * totalSupply / totalHumans. For any single address to reach it, it
  // must hold more than 50/totalHumans of the ENTIRE supply — impossible
  // while there are 50 or fewer registered humans. The live chain has ~15.
  //
  // The Go keeper's cap binds immediately instead: bootstrapMultiplierLocked
  // (state.go:3668-3678) returns max(5, min(N, 25)) and
  // getAverageBalanceLocked (state.go:3604-3618) returns a flat 1000, so at
  // 15 humans the Go cap is 15 * 1000 = 15,000 AEQ against a 15,000 AEQ total
  // supply — real and enforceable — versus the Solidity cap of 50,000 AEQ
  // against the same 15,000 AEQ supply. CORRECT SIDE: the Go keeper. The
  // Solidity cap is inert at every plausible launch-day population.
  it("FINDING W1: the Solidity wealth cap is mathematically unreachable below 51 humans", async function () {
    const { v7 } = await deployV7();
    const wallets = await viem.getWalletClients();
    const relayer = wallets[0];

    for (let n = 1; n <= 6; n++) {
      await registerHuman(v7, relayer, wallets[n]);
      const cap = await v7.read.wealthCap();
      const supply = await v7.read.totalSupply();
      const humans = await v7.read.totalHumans();
      assert.equal(humans, BigInt(n));
      assert.ok(
        cap > supply,
        `with ${n} human(s) the wealth cap is ${cap} wei but the entire money supply is only ` +
          `${supply} wei — no address can possibly be capped, so _applyWealthCap is dead code at this scale`,
      );
    }

    // State the general inequality the loop is sampling: CAPS[0] == 50, so
    // cap > supply for every totalHumans < 50.
    const finalCap = await v7.read.wealthCap();
    const finalSupply = await v7.read.totalSupply();
    assert.equal(finalCap, 50n * (finalSupply / 6n), "cap == CAPS[0] * fairShare()");
    // WHEN FIXED: the Solidity cap should track the Go bootstrap schedule
    // (max(5, min(N,25)) * 1000 AEQ), which binds from the first human.
  });

  // ── FINDING W2 ────────────────────────────────────────────────────────────
  // _applyWealthCap() returns immediately for any address that is not a
  // registered human (AequitasV7.sol:335), and registerWithSig() never
  // applies the cap at all (AequitasV7.sol:223-237) — so an address can be
  // funded above the cap while unregistered and then carry that balance into
  // registration untouched.
  //
  // The Go keeper deliberately does the opposite on BOTH counts.
  // enforceWealthCapLockedCtx is explicitly not gated on IsHuman and its
  // comment (state.go:3703-3709) describes this exact bypass as the reason:
  // "capping only registered humans would let someone bypass the entire
  // mechanism just by parking AEQ in any ordinary, unregistered address".
  // And registerHumanLocked calls enforceWealthCapLockedCtx right after
  // crediting the 1000 AEQ grant (state.go:3923). CORRECT SIDE: the Go
  // keeper, on both counts.
  //
  // NOTE ON SCOPE: because of FINDING W1 the cap cannot actually bind below
  // 51 humans, so this test proves the GUARD exists (the structural bypass)
  // rather than exhibiting a live over-cap balance. That combination — an
  // inert cap plus an exemption for exactly the addresses a whale would use —
  // is why W1 and W2 are reported together.
  it("FINDING W2: the wealth cap exempts unregistered addresses, and registration never applies it", async function () {
    const { v7 } = await deployV7();
    const [relayer, human, outsider] = await viem.getWalletClients();
    await registerHuman(v7, relayer, human);

    // The permissionless cap-enforcement entry point is closed for exactly
    // the addresses that are exempt from the internal one.
    await viem.assertions.revertWith(
      v7.write.applyWealthCap([outsider.account.address]),
      "Not human",
    );

    // A transfer INTO an unregistered address runs _applyWealthCap(to), which
    // returns at AequitasV7.sol:335 without inspecting the balance at all.
    await v7.write.transfer([outsider.account.address, 500n * AEQ], { account: human.account });
    assert.ok((await v7.read.balanceOf([outsider.account.address])) > 0n);

    // Registration does not cap either: the pre-existing balance survives it
    // and is simply added to, exactly as AequitasV7.sol:228's `+=` implies.
    const preRegistration = await v7.read.balanceOf([outsider.account.address]);
    await registerHuman(v7, relayer, outsider);
    assert.equal(
      await v7.read.balanceOf([outsider.account.address]),
      preRegistration + 1_000n * AEQ,
      "registerWithSig adds the grant to a pre-funded balance and never calls _applyWealthCap",
    );
    // WHEN FIXED: _applyWealthCap must drop the isHuman guard (matching
    // state.go:3703) and registerWithSig must call it after crediting the
    // grant (matching state.go:3923).
  });

  // ── FINDING F1 ────────────────────────────────────────────────────────────
  // AequitasV7 exposes name/symbol/decimals/totalSupply/balanceOf/transfer
  // and declares the standard ERC-20 Transfer event (AequitasV7.sol:114), but
  // that event is emitted in exactly ONE place: the wake-up bonus mint in
  // _confirmAlive (AequitasV7.sol:439). Ordinary transfers emit only the
  // custom Transferred event (AequitasV7.sol:284) and the registration mint
  // emits only Registered (AequitasV7.sol:237). There is also no
  // approve/allowance/transferFrom.
  //
  // Consequence: every ERC-20 indexer, block explorer and wallet that derives
  // balances from Transfer logs will show every holder at zero except for
  // wake-up mints. This is a launch-visible correctness problem for an
  // "EVM-compatible surface", independent of any Go divergence.
  // FIXED 2026-08-16 (v7.14): every value-moving path emits a standard
  // Transfer event, under the model documented on the event declaration —
  // address(0) is creation/burn, address(this) is value held by the contract
  // (ubiPool + escrows), anything else is that holder's balanceOf.
  //
  // The test does what the finding said was impossible: replay the Transfer
  // log and reconstruct balanceOf from nothing else. That is the property an
  // indexer, explorer or wallet actually depends on — counting events would
  // pass just as well against events that don't add up.
  it("F1 (FIXED): balances are reconstructable from the standard Transfer log alone", async function () {
    const { v7 } = await deployV7();
    const publicClient = await viem.getPublicClient();
    const [relayer, a, b] = await viem.getWalletClients();

    const from = await publicClient.getBlockNumber();
    await registerHuman(v7, relayer, a);
    await registerHuman(v7, relayer, b);
    await v7.write.transfer([b.account.address, 100n * AEQ], { account: a.account });

    // Exercise a pool-funded and a pool-fed path too, so the reconstruction is
    // tested against more than plain transfers.
    if ((await v7.read.ubiPool()) > 0n) {
      await v7.write.accumulateUBI();
      await v7.write.claimUBI({ account: b.account });
    }

    const transferLogs = await publicClient.getContractEvents({
      abi: v7.abi, address: v7.address, eventName: "Transfer", fromBlock: from,
    });
    assert.ok(
      transferLogs.length >= 3,
      `expected at least two registration mints and one transfer to be visible as standard ` +
        `Transfer events, got ${transferLogs.length}`,
    );

    // Replay the log into a balance table, exactly as an indexer would.
    const ZERO = "0x0000000000000000000000000000000000000000";
    const reconstructed = new Map<string, bigint>();
    const move = (addr: string, delta: bigint) => {
      const key = addr.toLowerCase();
      reconstructed.set(key, (reconstructed.get(key) ?? 0n) + delta);
    };
    for (const log of transferLogs) {
      const { from: f, to: t, value } = log.args as { from: string; to: string; value: bigint };
      if (f.toLowerCase() !== ZERO) move(f, -value);
      if (t.toLowerCase() !== ZERO) move(t, value);
    }

    for (const holder of [a, b]) {
      const addr = holder.account.address.toLowerCase();
      assert.equal(
        reconstructed.get(addr) ?? 0n,
        await v7.read.balanceOf([holder.account.address]),
        `balance reconstructed from Transfer logs disagrees with balanceOf for ${addr} — ` +
          `an ERC-20 indexer would show this holder the wrong amount`,
      );
    }

    // The contract's own line must equal what it actually holds. That is the
    // half of the model which cannot be checked against balanceOf, because the
    // contract deliberately does not mirror its holdings there (see the
    // event's declaration), so it is checked against the three places its
    // holdings really live:
    //
    //   ubiPool            — unallocated pool
    //   escrowOf           — held on behalf of a specific human
    //   claimableUBI       — allocated by accumulateUBI but not yet pulled.
    //                        accumulateUBI debits ubiPool and credits an
    //                        entitlement WITHOUT moving custody, so it
    //                        correctly emits no Transfer event; the money is
    //                        still the contract's until someone claims it.
    let contractHolds = await v7.read.ubiPool();
    for (const holder of [a, b]) {
      contractHolds += await v7.read.escrowOf([holder.account.address]);
      contractHolds += await v7.read.claimableUBI([holder.account.address]);
    }
    assert.equal(
      reconstructed.get(v7.address.toLowerCase()) ?? 0n,
      contractHolds,
      "value attributed to the contract by the log must equal ubiPool + escrows + unclaimed UBI",
    );

    // The contract-specific event is still emitted alongside, unchanged.
    const transferredLogs = await publicClient.getContractEvents({
      abi: v7.abi, address: v7.address, eventName: "Transferred", fromBlock: from,
    });
    assert.equal(transferredLogs.length, 1);

    // The escrow round-trip is the path that used to be the ONLY source of a
    // Transfer event (as a mint). It must now report both legs as custody
    // moving to and from the contract, and the reconstruction must still tie
    // out afterwards.
    await networkHelpers.time.increase(910 * DAY + 1);
    const beforeWake = await publicClient.getBlockNumber();
    await v7.write.triggerEscrow([a.account.address]);
    await v7.write.confirmAlive({ account: a.account });

    const wakeLogs = await publicClient.getContractEvents({
      abi: v7.abi, address: v7.address, eventName: "Transfer", fromBlock: beforeWake,
    });
    assert.ok(
      wakeLogs.length >= 2,
      `the escrow round-trip must be visible as value leaving and returning to the contract, ` +
        `got ${wakeLogs.length} Transfer event(s)`,
    );
    // No leg of it may look like creation or destruction — the wake-up bonus
    // is funded from the pool now, and a mint event here would be a lie.
    for (const log of wakeLogs) {
      const { from: f, to: t } = log.args as { from: string; to: string };
      assert.ok(
        f.toLowerCase() !== ZERO && t.toLowerCase() !== ZERO,
        "an escrow round-trip must not emit a mint or burn — it moves existing money only",
      );
    }

    for (const log of wakeLogs) {
      const { from: f, to: t, value } = log.args as { from: string; to: string; value: bigint };
      move(f, -value);
      move(t, value);
    }
    assert.equal(
      reconstructed.get(a.account.address.toLowerCase()) ?? 0n,
      await v7.read.balanceOf([a.account.address]),
      "reconstruction must still match balanceOf after an escrow round-trip",
    );
  });

  // ── FINDING F2 ────────────────────────────────────────────────────────────
  // TX_FEE_BPS is 0 (AequitasV7.sol:41) while the Go ledger's calcV7Fee
  // charges a 0.1% base on every real transfer (state.go:5016:
  // `base := amount * 10.0 / 10_000.0`). The concentration surcharge tiers
  // are identical on both sides. So the published contract source understates
  // the fee a user actually pays by exactly 10 bps.
  //
  // The contract's own comment (AequitasV7.sol:30-40) documents setting this
  // to 0 deliberately, on the grounds that the RPC layer intercepts the
  // transfer() selector so this code never runs. That reasoning is sound
  // about reachability but leaves the human-readable source stating a fee
  // schedule the chain does not use — arguably the same misleading-reader
  // problem the change was made to solve, just in the other direction.
  it("FINDING F2: the contract's own fee schedule is 10 bps cheaper than the fee the Go ledger actually charges", async function () {
    const { v7 } = await deployV7();
    const wallets = await viem.getWalletClients();
    const relayer = wallets[0];
    for (let n = 1; n <= 3; n++) await registerHuman(v7, relayer, wallets[n]);

    const amount = 100n * AEQ;
    const holder = wallets[1].account.address;
    const solidityFee = await v7.read.calcFee([holder, amount]);

    // At 3 humans a 1000-AEQ holder is 3333 bps of supply, so the top
    // concentration tier applies: 100 bps. Solidity adds TX_FEE_BPS(0) on top.
    assert.equal(await v7.read.totalSupply(), 3_000n * AEQ);
    assert.equal(solidityFee, (amount * 100n) / 10_000n, "Solidity: 0 bps base + 100 bps surcharge");

    // The Go schedule for the identical inputs: base 10 bps + the same
    // 100 bps surcharge (state.go:5015-5031).
    const goFee = (amount * 10n) / 10_000n + (amount * 100n) / 10_000n;
    assert.equal(goFee - solidityFee, (amount * 10n) / 10_000n, "the divergence is exactly the 0.1% base");
    assert.ok(goFee > solidityFee);
    // WHEN FIXED: set TX_FEE_BPS = 10 so the on-chain source matches
    // state.go:5016, or document the split in the contract's NatSpec.
  });
});
