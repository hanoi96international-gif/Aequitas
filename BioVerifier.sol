// SPDX-License-Identifier: GPL-3.0
/*
    ⚠ AUDIT NOTE (launch audit 2026-07-03, resolved): this file does NOT
    match the contract actually deployed at BIO_VERIFIER_ADDR in production.
    x/humanity/keeper/contract_deploy.go hardcodes a DIFFERENT bytecode blob
    (BioVerifierRuntimeBytecode), identified by its own comment as compiled
    from "BioVerifierV3.sol (Poseidon-based nullifier circuit)". That file
    does not exist in THIS repo — it lives in the separate proof-server
    project (github.com/hanoi96international-gif/aequitas-proof-server,
    local checkout at C:\Users\aequitas-proof-server\BioVerifierV3.sol),
    which owns the ZK circuit (biometric_v3.circom) this verifier matches.
    Confirmed by exact bytecode comparison: that repo's
    bioVerifierV3_bytecode.json, with the standard ~56-byte Solidity
    constructor prefix stripped, is byte-for-byte identical to
    BioVerifierRuntimeBytecode below — this is genuinely the deployed
    verifier's source, just cross-repo, not a lost/missing file.

    DO NOT "fix" this by recompiling THIS file and pasting its bytecode into
    BioVerifierRuntimeBytecode — EnsureContractsDeployed overwrites the live
    contract whenever that constant changes, and this file's 2-input Groth16
    verifier does not match the circuit the mobile app / proof-generation
    pipeline actually produces proofs for. Swapping it in would not "correct"
    anything; it would make every future verifyProof() call check the wrong
    statement, silently breaking (or arbitrarily un-gating) every new human's
    registration. This file is kept only because it documents the *shape* of
    a standard snarkjs Groth16 verifier (same template, different
    verification key) — for the real, currently-deployed source, see
    aequitas-proof-server/BioVerifierV3.sol.

    Copyright 2021 0KIMS association.

    This file is generated with [snarkJS](https://github.com/iden3/snarkjs).

    snarkJS is a free software: you can redistribute it and/or modify it
    under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    snarkJS is distributed in the hope that it will be useful, but WITHOUT
    ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
    or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public
    License for more details.

    You should have received a copy of the GNU General Public License
    along with snarkJS. If not, see <https://www.gnu.org/licenses/>.
*/

pragma solidity >=0.7.0 <0.9.0;

contract Groth16Verifier {
    // Scalar field size
    uint256 constant r    = 21888242871839275222246405745257275088548364400416034343698204186575808495617;
    // Base field size
    uint256 constant q   = 21888242871839275222246405745257275088696311157297823662689037894645226208583;

    // Verification Key data
    uint256 constant alphax  = 5595115509439491105793265528459723572424544540984275355158735445261793245146;
    uint256 constant alphay  = 9648129285049298631290606692131332349498517941505757760195124362924067741564;
    uint256 constant betax1  = 16266305600164491384846150164952840896683820133670650187518940451221223929931;
    uint256 constant betax2  = 8132268232166375521908121125943886058820862672184310301112472883166184422904;
    uint256 constant betay1  = 9497630629933357579648537667423622621263050134727938877385206427126219132005;
    uint256 constant betay2  = 10400740768619454483301093541534353813317405501605014623820160161703088416539;
    uint256 constant gammax1 = 11559732032986387107991004021392285783925812861821192530917403151452391805634;
    uint256 constant gammax2 = 10857046999023057135944570762232829481370756359578518086990519993285655852781;
    uint256 constant gammay1 = 4082367875863433681332203403145435568316851327593401208105741076214120093531;
    uint256 constant gammay2 = 8495653923123431417604973247489272438418190587263600148770280649306958101930;
    uint256 constant deltax1 = 12927439555443631341036130927368552889220985486953529181624061613380546074021;
    uint256 constant deltax2 = 14599880269479201470172668488086352517552725581464960421142776427251099745547;
    uint256 constant deltay1 = 19610749917367646501516567976860785899892251300604645938909158222866612009264;
    uint256 constant deltay2 = 9288792584295273032955242281440014379357150478239641112254987972371533896596;

    
    uint256 constant IC0x = 12451449043236090295573576758990318250880086353090813125992907865644175231673;
    uint256 constant IC0y = 21723875559777005878686237381686913215292281882584802531248977640789874737788;
    
    uint256 constant IC1x = 16959486260101614317186074692913329359032359446300088386982269299104374941027;
    uint256 constant IC1y = 9366888358748728084518322300783881120273020287145927845035054872706107717814;
    
    uint256 constant IC2x = 19285806247710368274683042353137822943729107525731349961360740925662458545999;
    uint256 constant IC2y = 8115696115455213474080229022827910467663971901638582422084594944497499096476;
    
 
    // Memory data
    uint16 constant pVk = 0;
    uint16 constant pPairing = 128;

    uint16 constant pLastMem = 896;

    function verifyProof(uint[2] calldata _pA, uint[2][2] calldata _pB, uint[2] calldata _pC, uint[2] calldata _pubSignals) public view returns (bool) {
        assembly {
            function checkField(v) {
                if iszero(lt(v, r)) {
                    mstore(0, 0)
                    return(0, 0x20)
                }
            }
            
            // G1 function to multiply a G1 value(x,y) to value in an address
            function g1_mulAccC(pR, x, y, s) {
                let success
                let mIn := mload(0x40)
                mstore(mIn, x)
                mstore(add(mIn, 32), y)
                mstore(add(mIn, 64), s)

                success := staticcall(sub(gas(), 2000), 7, mIn, 96, mIn, 64)

                if iszero(success) {
                    mstore(0, 0)
                    return(0, 0x20)
                }

                mstore(add(mIn, 64), mload(pR))
                mstore(add(mIn, 96), mload(add(pR, 32)))

                success := staticcall(sub(gas(), 2000), 6, mIn, 128, pR, 64)

                if iszero(success) {
                    mstore(0, 0)
                    return(0, 0x20)
                }
            }

            function checkPairing(pA, pB, pC, pubSignals, pMem) -> isOk {
                let _pPairing := add(pMem, pPairing)
                let _pVk := add(pMem, pVk)

                mstore(_pVk, IC0x)
                mstore(add(_pVk, 32), IC0y)

                // Compute the linear combination vk_x
                
                g1_mulAccC(_pVk, IC1x, IC1y, calldataload(add(pubSignals, 0)))
                
                g1_mulAccC(_pVk, IC2x, IC2y, calldataload(add(pubSignals, 32)))
                

                // -A
                mstore(_pPairing, calldataload(pA))
                mstore(add(_pPairing, 32), mod(sub(q, calldataload(add(pA, 32))), q))

                // B
                mstore(add(_pPairing, 64), calldataload(pB))
                mstore(add(_pPairing, 96), calldataload(add(pB, 32)))
                mstore(add(_pPairing, 128), calldataload(add(pB, 64)))
                mstore(add(_pPairing, 160), calldataload(add(pB, 96)))

                // alpha1
                mstore(add(_pPairing, 192), alphax)
                mstore(add(_pPairing, 224), alphay)

                // beta2
                mstore(add(_pPairing, 256), betax1)
                mstore(add(_pPairing, 288), betax2)
                mstore(add(_pPairing, 320), betay1)
                mstore(add(_pPairing, 352), betay2)

                // vk_x
                mstore(add(_pPairing, 384), mload(add(pMem, pVk)))
                mstore(add(_pPairing, 416), mload(add(pMem, add(pVk, 32))))


                // gamma2
                mstore(add(_pPairing, 448), gammax1)
                mstore(add(_pPairing, 480), gammax2)
                mstore(add(_pPairing, 512), gammay1)
                mstore(add(_pPairing, 544), gammay2)

                // C
                mstore(add(_pPairing, 576), calldataload(pC))
                mstore(add(_pPairing, 608), calldataload(add(pC, 32)))

                // delta2
                mstore(add(_pPairing, 640), deltax1)
                mstore(add(_pPairing, 672), deltax2)
                mstore(add(_pPairing, 704), deltay1)
                mstore(add(_pPairing, 736), deltay2)


                let success := staticcall(sub(gas(), 2000), 8, _pPairing, 768, _pPairing, 0x20)

                isOk := and(success, mload(_pPairing))
            }

            let pMem := mload(0x40)
            mstore(0x40, add(pMem, pLastMem))

            // Validate that all evaluations ∈ F
            
            checkField(calldataload(add(_pubSignals, 0)))
            
            checkField(calldataload(add(_pubSignals, 32)))
            

            // Validate all evaluations
            let isValid := checkPairing(_pA, _pB, _pC, _pubSignals, pMem)

            mstore(0, isValid)
             return(0, 0x20)
         }
     }
 }
