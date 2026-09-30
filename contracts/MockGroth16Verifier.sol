// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

// Test-only stand-in for the ceremony's Groth16 verifier (AequitasV8 tests).
// Unlike MockBioVerifier it can say no: `accept` switches the answer, so the
// "invalid proof" path of AequitasV8.registerWithSig is testable. It also only
// accepts proofs whose pA[0] equals VALID_MARKER, so a test can tell a
// "valid" proof from a tampered one without real pairing math.
contract MockGroth16Verifier {
    uint256 public constant VALID_MARKER = 0xA11CE;

    bool public accept = true;

    function setAccept(bool accept_) external {
        accept = accept_;
    }

    function verifyProof(
        uint256[2] calldata pA,
        uint256[2][2] calldata,
        uint256[2] calldata,
        uint256[2] calldata
    ) external view returns (bool) {
        return accept && pA[0] == VALID_MARKER;
    }
}
