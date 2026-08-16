import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { network } from "hardhat";
import { encodePacked, keccak256, toHex, padHex } from "viem";

// AUDIT (2026-08-16 full contract audit, launch readiness): targeted proof
// tests for the "one human = one registration" invariant at the CONTRACT
// LAYER specifically (independent of the Go relayer's own nullifier/bioHash
// pre-checks in register.go, which are a separate defense-in-depth layer).
//
// These exercise registerWithSig() directly via MockBioVerifier (always
// returns true), isolating the contract's own bookkeeping — usedNullifiers,
// usedCommitments, isHuman, and the ECDSA signature binding — from ZK proof
// validity, exactly like the existing beta-launch-audit test file does.
describe("AequitasV7 registerWithSig: nullifier/commitment/signature audit", async function () {
  const { viem } = await network.create();

  async function deployV7() {
    const bio = await viem.deployContract("MockBioVerifier");
    const v7 = await viem.deployContract("AequitasV7", [bio.address]);
    return { v7, bio };
  }

  async function buildSignedRegistration(
    v7: Awaited<ReturnType<typeof deployV7>>["v7"],
    signer: Awaited<ReturnType<typeof viem.getWalletClients>>[number],
    claimedAddress: `0x${string}`,
    commitment: bigint,
    nullifier: bigint,
  ) {
    const nullifierHex = padHex(toHex(nullifier), { size: 32 });
    const chainId = BigInt(await (await viem.getPublicClient()).getChainId());
    const messageHash = keccak256(
      encodePacked(
        ["uint256", "address", "string", "uint256", "bytes32"],
        [chainId, v7.address, "register", commitment, nullifierHex],
      ),
    );
    const signature = await signer.signMessage({ message: { raw: messageHash } });
    return { commitment, nullifier, nullifierHex, signature };
  }

  it("rejects a second registration reusing the SAME nullifier under a different wallet+commitment (the core 'one biometric = one registration' guarantee)", async function () {
    const { v7 } = await deployV7();
    const [relayer, human1, human2] = await viem.getWalletClients();

    const sharedNullifier = 111n;
    const reg1 = await buildSignedRegistration(v7, human1, human1.account.address, 1n, sharedNullifier);
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [reg1.commitment, reg1.nullifier], human1.account.address, reg1.signature, reg1.nullifierHex],
      { account: relayer.account },
    );
    assert.equal(await v7.read.isHuman([human1.account.address]), true);

    // Same biometric (same nullifier), different wallet, different
    // commitment, a genuinely valid signature from human2 over their own
    // (different) commitment — everything about this attempt is "valid"
    // EXCEPT that the nullifier was already burned by human1's registration.
    const reg2 = await buildSignedRegistration(v7, human2, human2.account.address, 2n, sharedNullifier);
    await viem.assertions.revertWith(
      v7.write.registerWithSig(
        [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
         [reg2.commitment, reg2.nullifier], human2.account.address, reg2.signature, reg2.nullifierHex],
        { account: relayer.account },
      ),
      "Nullifier used",
    );
    assert.equal(await v7.read.isHuman([human2.account.address]), false);
    assert.equal(await v7.read.totalHumans(), 1n);
  });

  it("rejects reusing the same commitment under a fresh nullifier", async function () {
    const { v7 } = await deployV7();
    const [relayer, human1, human2] = await viem.getWalletClients();

    const sharedCommitment = 42n;
    const reg1 = await buildSignedRegistration(v7, human1, human1.account.address, sharedCommitment, 201n);
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [reg1.commitment, reg1.nullifier], human1.account.address, reg1.signature, reg1.nullifierHex],
      { account: relayer.account },
    );

    const reg2 = await buildSignedRegistration(v7, human2, human2.account.address, sharedCommitment, 202n);
    await viem.assertions.revertWith(
      v7.write.registerWithSig(
        [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
         [reg2.commitment, reg2.nullifier], human2.account.address, reg2.signature, reg2.nullifierHex],
        { account: relayer.account },
      ),
      "Commitment used",
    );
  });

  it("rejects replaying the exact same successful registration a second time (byte-identical calldata resubmission)", async function () {
    const { v7 } = await deployV7();
    const [relayer, human1] = await viem.getWalletClients();

    const reg = await buildSignedRegistration(v7, human1, human1.account.address, 5n, 501n);
    const args = [
      [0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
      [reg.commitment, reg.nullifier], human1.account.address, reg.signature, reg.nullifierHex,
    ] as const;

    await v7.write.registerWithSig(args, { account: relayer.account });
    // Anyone (not just the original relayer) resubmitting the identical
    // calldata must fail — this is the front-running scenario the contract's
    // own comment documents register() as having been vulnerable to.
    await viem.assertions.revertWith(
      v7.write.registerWithSig(args, { account: human1.account }),
      "Already registered",
    );
  });

  it("REJECTS front-running: an attacker cannot redirect a victim's valid signed proof to their own address", async function () {
    const { v7 } = await deployV7();
    const [relayer, victim, attacker] = await viem.getWalletClients();

    // Victim signs a registration message binding THEIR OWN address as
    // claimedHuman (this is what a legitimate client does).
    const reg = await buildSignedRegistration(v7, victim, victim.account.address, 7n, 701n);

    // Attacker observes (pA,pB,pC,pubSignals,signature) in the mempool and
    // tries to resubmit with claimedHuman swapped to their own address,
    // hoping to steal the INITIAL_GRANT and burn the victim's nullifier.
    await viem.assertions.revertWith(
      v7.write.registerWithSig(
        [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
         [reg.commitment, reg.nullifier], attacker.account.address, reg.signature, reg.nullifierHex],
        { account: attacker.account },
      ),
      "Invalid signature",
    );
    assert.equal(await v7.read.isHuman([attacker.account.address]), false);
    assert.equal(await v7.read.usedNullifiers([reg.nullifierHex]), "0x0000000000000000000000000000000000000000",
      "the victim's nullifier must NOT be burned by the failed attack attempt");

    // The victim can still register normally afterward — proving the failed
    // attack left no residue (no partial nullifier/commitment burn).
    await v7.write.registerWithSig(
      [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
       [reg.commitment, reg.nullifier], victim.account.address, reg.signature, reg.nullifierHex],
      { account: relayer.account },
    );
    assert.equal(await v7.read.isHuman([victim.account.address]), true);
  });

  it("rejects a v1-circuit proof (pubSignals[1] == 0, no ZK-bound nullifier)", async function () {
    const { v7 } = await deployV7();
    const [relayer, human1] = await viem.getWalletClients();
    const reg = await buildSignedRegistration(v7, human1, human1.account.address, 9n, 0n);
    await viem.assertions.revertWith(
      v7.write.registerWithSig(
        [[0n, 0n], [[0n, 0n], [0n, 0n]], [0n, 0n],
         [reg.commitment, 0n], human1.account.address, reg.signature, reg.nullifierHex],
        { account: relayer.account },
      ),
      "v1 circuit not accepted: ZK-bound nullifier required",
    );
  });
});
