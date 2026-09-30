import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Same guard as AequitasV7_bytecode_sync.ts, for V8: the V8ContractBytecode
// constant the Go node deploys (contract_deploy.go) must equal a fresh
// compile of contracts/AequitasV8.sol, so the node never ships bytecode that
// differs from the reviewed and tested source.
describe("V8ContractBytecode (contract_deploy.go) stays in sync with a fresh compile", function () {
  it("matches Hardhat's freshly-compiled AequitasV8 creation bytecode", function () {
    const here = path.dirname(fileURLToPath(import.meta.url));

    const artifactPath = path.join(
      here,
      "..",
      "artifacts",
      "contracts",
      "AequitasV8.sol",
      "AequitasV8.json",
    );
    const artifact = JSON.parse(readFileSync(artifactPath, "utf8"));
    const freshBytecode: string = artifact.bytecode;
    assert.ok(
      freshBytecode && freshBytecode.startsWith("0x") && freshBytecode.length > 100,
      "Hardhat artifact bytecode looks empty or malformed — did the compile actually run before this test?",
    );
    const freshHex = freshBytecode.slice(2).toLowerCase();

    const goSourcePath = path.join(here, "..", "x", "humanity", "keeper", "contract_deploy.go");
    const goSource = readFileSync(goSourcePath, "utf8");
    const marker = 'const V8ContractBytecode = "';
    const start = goSource.indexOf(marker);
    assert.ok(start >= 0, `could not find ${marker} in contract_deploy.go — has the constant been renamed?`);
    const valueStart = start + marker.length;
    const valueEnd = goSource.indexOf('"', valueStart);
    assert.ok(valueEnd > valueStart, "could not find the closing quote for V8ContractBytecode");
    const goHex = goSource.slice(valueStart, valueEnd).toLowerCase();

    assert.equal(
      goHex,
      freshHex,
      "V8ContractBytecode in contract_deploy.go does not match a fresh compile of AequitasV8.sol. " +
        "If the contract source changed, regenerate the constant from " +
        "artifacts/contracts/AequitasV8.sol/AequitasV8.json's `bytecode` field " +
        "(strip the 0x prefix) — see this test file's own comment for why this matters.",
    );
  });
});
