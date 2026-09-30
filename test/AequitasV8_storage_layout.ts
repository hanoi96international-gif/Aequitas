import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { network } from "hardhat";
import {
  type Hex,
  concatHex,
  keccak256,
  padHex,
  toFunctionSelector,
  toFunctionSignature,
  toHex,
} from "viem";

// The Go keeper writes AequitasV8's storage directly (mirror of balances,
// totalSupply, register). contracts/v8_slots.json is the one table both
// sides use. This file makes any drift loud:
//
//   1. compiler storageLayout  ==  v8_slots.json "storage"  (complete, exact)
//   2. compiled ABI selectors  ==  v8_slots.json "selectors" (both directions)
//   3. the table's slot math, applied from outside the way Go applies it,
//      really reaches the values the contract reads and writes.

type SlotRow = {
  slot: number;
  offset: number;
  label: string;
  type: string;
  keyEncoding: "address" | "uint256" | "bytes32" | null;
  writer: "go" | "contract" | "constructor";
};
type SelectorRow = { selector: Hex; signature: string; persist: string };
type Table = {
  contract: string;
  storage: SlotRow[];
  selectors: { functions: SelectorRow[] };
};

// Any non-zero network salt; the register tests cover what it binds.
const NETZ_SALT = keccak256(toHex("aequitas-31337-1790000000"));

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, "..");
const table: Table = JSON.parse(
  readFileSync(path.join(root, "contracts", "v8_slots.json"), "utf8"),
);

type LayoutEntry = {
  label: string;
  offset: number;
  slot: string;
  type: string;
};
type Layout = {
  storage: LayoutEntry[];
  types: Record<string, { label: string; encoding: string }>;
};

function compiledLayout(): Layout {
  // Read the build info the CURRENT artifact was produced by (Hardhat 3
  // keeps older build infos around after incremental compiles).
  const artifact = JSON.parse(
    readFileSync(
      path.join(root, "artifacts", "contracts", "AequitasV8.sol", "AequitasV8.json"),
      "utf8",
    ),
  );
  const id: string = artifact.buildInfoId;
  assert.ok(id, "artifact has no buildInfoId — run npx hardhat compile");
  const out = JSON.parse(
    readFileSync(path.join(root, "artifacts", "build-info", `${id}.output.json`), "utf8"),
  );
  const found: Layout[] = [];
  for (const [src, contracts] of Object.entries(
    (out.output?.contracts ?? {}) as Record<string, Record<string, { storageLayout?: Layout }>>,
  )) {
    if (src.endsWith("contracts/AequitasV8.sol") && contracts.AequitasV8) {
      const layout = contracts.AequitasV8.storageLayout;
      assert.ok(
        layout,
        "storageLayout missing from the compiler output — hardhat.config.ts must request it (outputSelection)",
      );
      found.push(layout);
    }
  }
  assert.equal(found.length, 1, `expected exactly one AequitasV8 in build info ${id}, found ${found.length}`);
  return found[0];
}

describe("AequitasV8 storage layout == contracts/v8_slots.json", function () {
  it("every compiler slot is in the table and every table slot is in the compiler output, in order", function () {
    const layout = compiledLayout();
    const fromCompiler = layout.storage.map((e) => ({
      slot: Number(e.slot),
      offset: e.offset,
      label: e.label,
      type: layout.types[e.type].label,
    }));
    const fromTable = table.storage.map(({ slot, offset, label, type }) => ({
      slot,
      offset,
      label,
      type,
    }));
    assert.deepEqual(
      fromTable,
      fromCompiler,
      "contracts/v8_slots.json does not match the compiled AequitasV8 layout. " +
        "A state variable was added, removed, moved or retyped. Update the table " +
        "AND the Go slot constants (docs/V8_ENTWURF.md, Go-Anbindung) together.",
    );
  });

  it("the table is well-formed: unique slots, no packing, key encoding matches the type", function () {
    const slots = table.storage.map((r) => r.slot);
    assert.equal(new Set(slots).size, slots.length, "duplicate slot number");
    for (const r of table.storage) {
      assert.equal(r.offset, 0, `${r.label}: packed variables are not allowed (Go writes whole words)`);
      const m = /^mapping\((\w+) => \w+\)$/.exec(r.type);
      if (m) {
        assert.equal(r.keyEncoding, m[1], `${r.label}: keyEncoding must equal the mapping key type`);
      } else {
        assert.equal(r.keyEncoding, null, `${r.label}: value slots have no key encoding`);
      }
      assert.ok(
        ["go", "contract", "constructor"].includes(r.writer),
        `${r.label}: unknown writer ${r.writer}`,
      );
    }
  });

  it("selector table == compiled ABI, both directions; only registerWithSig changes state", async function () {
    const artifact = JSON.parse(
      readFileSync(
        path.join(root, "artifacts", "contracts", "AequitasV8.sol", "AequitasV8.json"),
        "utf8",
      ),
    );
    const abiFns = (artifact.abi as { type: string; stateMutability: string }[])
      .filter((x) => x.type === "function")
      .map((f) => ({
        selector: toFunctionSelector(f as never),
        signature: toFunctionSignature(f as never),
        mutability: f.stateMutability,
      }));
    const bySelector = new Map(table.selectors.functions.map((r) => [r.selector, r]));

    for (const f of abiFns) {
      const row = bySelector.get(f.selector);
      assert.ok(row, `ABI function ${f.signature} (${f.selector}) missing from v8_slots.json selectors`);
      assert.equal(row.signature, f.signature);
      const expected =
        f.signature === "transfer(address,uint256)"
          ? "intercepted"
          : f.mutability === "nonpayable" || f.mutability === "payable"
            ? "registrar"
            : "view";
      assert.equal(row.persist, expected, `${f.signature}: persist class`);
    }
    assert.equal(
      table.selectors.functions.length,
      abiFns.length,
      "v8_slots.json lists selectors that the compiled ABI does not have",
    );
    for (const r of table.selectors.functions) {
      assert.equal(toFunctionSelector(r.signature), r.selector, `${r.signature}: selector`);
    }
    // The selectors the node already hard-codes must not change.
    assert.equal(bySelector.get("0xa9059cbb")?.signature, "transfer(address,uint256)");
    assert.equal(bySelector.get("0x70a08231")?.signature, "balanceOf(address)");
    assert.equal(bySelector.get("0xf72c436f")?.signature, "isHuman(address)");
  });
});

describe("AequitasV8 table slots, used from outside like Go does", async function () {
  const { viem, networkHelpers } = await network.create();
  const publicClient = await viem.getPublicClient();
  const testClient = await viem.getTestClient();
  const [registrar, alice] = await viem.getWalletClients();

  const row = (label: string) => {
    const r = table.storage.find((x) => x.label === label);
    assert.ok(r, `${label} not in v8_slots.json`);
    return r;
  };
  const word = (v: bigint | Hex) =>
    padHex(typeof v === "bigint" ? toHex(v) : v, { size: 32 });
  // Go: mappingSlot(key, slot) / mappingSlotBytes32(key, slot)
  const mapSlot = (key: Hex, slot: number) =>
    keccak256(concatHex([word(key), word(BigInt(slot))]));
  const valueSlot = (slot: number) => word(BigInt(slot));

  async function deploy() {
    const verifier = await viem.deployContract("MockGroth16Verifier");
    const v8 = await viem.deployContract("AequitasV8", [
      verifier.address,
      [registrar.account.address],
      NETZ_SALT,
    ]);
    return { v8 };
  }

  it("Go mirror writes to balanceOf/totalSupply slots are what the ERC-20 views return", async function () {
    const { v8 } = await deploy();
    const bal = 1_000n * 10n ** 18n;
    await testClient.setStorageAt({
      address: v8.address,
      index: mapSlot(alice.account.address, row("balanceOf").slot),
      value: word(bal),
    });
    await testClient.setStorageAt({
      address: v8.address,
      index: valueSlot(row("totalSupply").slot),
      value: word(bal * 3n),
    });
    assert.equal(await v8.read.balanceOf([alice.account.address]), bal);
    assert.equal(await v8.read.totalSupply(), bal * 3n);
  });

  it("a registration lands exactly on the table's slots (isHuman, commitment, nullifier, nonce, totalHumans)", async function () {
    const { v8 } = await deploy();
    const human = alice.account.address;
    const commitment = 31337n;
    const nullifier = 4242n;
    const deadline = BigInt(await networkHelpers.time.latest()) + 600n;
    const signature = await alice.signTypedData({
      account: alice.account,
      domain: {
        name: "Aequitas",
        version: "8",
        chainId: BigInt(await publicClient.getChainId()),
        verifyingContract: v8.address,
        salt: NETZ_SALT,
      },
      types: {
        Register: [
          { name: "human", type: "address" },
          { name: "commitment", type: "uint256" },
          { name: "nullifier", type: "uint256" },
          { name: "nonce", type: "uint256" },
          { name: "deadline", type: "uint256" },
        ],
      },
      primaryType: "Register",
      message: { human, commitment, nullifier, nonce: 0n, deadline },
    });
    await v8.write.registerWithSig(
      [
        [0xa11cen, 0n],
        [
          [0n, 0n],
          [0n, 0n],
        ],
        [0n, 0n],
        [commitment, nullifier],
        human,
        deadline,
        signature,
      ],
      { account: registrar.account },
    );

    const at = (index: Hex) =>
      publicClient.getStorageAt({ address: v8.address, slot: index });

    assert.equal(await at(valueSlot(row("totalHumans").slot)), word(1n));
    assert.equal(await at(mapSlot(human, row("isHuman").slot)), word(1n));
    assert.equal(await at(mapSlot(word(commitment), row("usedCommitments").slot)), word(1n));
    assert.equal(
      (await at(mapSlot(word(nullifier), row("usedNullifiers").slot)))?.toLowerCase(),
      word(human).toLowerCase(),
    );
    assert.equal(await at(mapSlot(human, row("commitmentOf").slot)), word(commitment));
    assert.equal(await at(mapSlot(human, row("nullifierOf").slot)), word(nullifier));
    assert.equal(await at(mapSlot(human, row("nonces").slot)), word(1n));
    assert.equal(
      await at(mapSlot(registrar.account.address, row("isRegistrar").slot)),
      word(1n),
    );
    // Slots written only by Go stay untouched by the contract.
    assert.equal(await at(valueSlot(row("totalSupply").slot)), word(0n));
    assert.equal(await at(mapSlot(human, row("balanceOf").slot)), word(0n));
  });
});
