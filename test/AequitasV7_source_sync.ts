import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Hardhat 3's artifact generation only produces a deployable artifact for
// contracts under contracts/, and it does not do so for a file reached via
// a "../" import out of that directory — so contracts/AequitasV7.sol exists
// purely as a test fixture letting AequitasV7_guardian_ubi_fixes.ts deploy
// and exercise the real contract. The canonical, actually-deployed source is
// /AequitasV7.sol at the repo root (see deploy_v7.cjs and
// x/humanity/keeper/contract_deploy.go, both of which reference it there).
// If these two ever drift apart, the test suite would silently start
// verifying a contract that isn't the one running in production — exactly
// the kind of stale-duplicate trap this codebase has been bitten by before
// (see BioVerifier.sol's own header comment for another instance of it).
// This test makes that drift loud and immediate instead of silent.
describe("AequitasV7.sol test-fixture stays in sync with the deployed source", function () {
  it("contracts/AequitasV7.sol is byte-identical to the root AequitasV7.sol", function () {
    const here = path.dirname(fileURLToPath(import.meta.url));
    const root = readFileSync(path.join(here, "..", "AequitasV7.sol"), "utf8");
    const fixture = readFileSync(
      path.join(here, "..", "contracts", "AequitasV7.sol"),
      "utf8",
    );
    assert.equal(
      fixture,
      root,
      "contracts/AequitasV7.sol has drifted from the root AequitasV7.sol — " +
        "copy the root file over the fixture (or vice versa, if the fixture " +
        "was edited first) before trusting any test result against it.",
    );
  });
});
