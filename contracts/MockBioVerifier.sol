// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

// Test-only stand-in for the real Groth16 BioVerifier: always approves,
// letting tests exercise AequitasV7's registration/guardian/escrow logic
// without needing a real ZK proof.
contract MockBioVerifier {
    function verifyProof(uint[2] calldata, uint[2][2] calldata, uint[2] calldata, uint[2] calldata) external pure returns (bool) {
        return true;
    }
}
