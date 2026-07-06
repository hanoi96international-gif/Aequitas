// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title AequitasV7
 * @author Aequitas Foundation
 * @notice "Money exists because people exist."
 *
 * CORE INVARIANT:
 *   SUM(balanceOf) + SUM(escrowOf) + ubiPool = totalSupply
 *
 * FIXES vs V6:
 *   [1] O(n) loops eliminated — Pull UBI pattern
 *   [2] totalSupply invariant strictly maintained
 *   [3] Concentration friction on transfers
 *   [4] No admin/owner, no upgrades
 *   [5] All state reconstructable from events
 */

interface IBioVerifier {
    function verifyProof(uint[2] calldata,uint[2][2] calldata,uint[2] calldata,uint[2] calldata) external view returns (bool);
}

contract AequitasV7 {
    string  public constant name     = "Aequitas";
    string  public constant symbol   = "AEQ";
    uint8   public constant decimals = 18;

    uint256 public constant INITIAL_GRANT     = 1_000 * 1e18;
    // FIX (P2-e, audit 2026-07-06): was 700 (7%) — but transfer()/_calcFee()
    // below are never reached for a real transfer (the RPC layer's own
    // transfer()-selector intercept routes it through the Go ledger's
    // TransferWithV7Fee/calcV7Fee instead — the actual, active 0.1%+tiered
    // schedule — before this contract's logic ever runs; see state.go's own
    // comment on that split). Left at 700 this constant was correct-looking
    // but misleading: anyone reading the contract directly (Etherscan-style
    // tools, auditors) with no reason to know about the RPC-layer intercept
    // would see a 7% fee that no real transfer on this chain ever pays. Set
    // to 0 so the contract's own on-chain, human-readable source no longer
    // implies a fee this chain doesn't actually charge.
    uint256 public constant TX_FEE_BPS        = 0;
    uint256 public constant UBI_SHARE_BPS     = 10_000; // 100% of fee goes to UBI pool
    uint256 public constant DEMURRAGE_BPS     = 100;
    uint256 public constant SECONDS_PER_YEAR  = 365 days;
    uint256 public constant INACTIVITY_ESCROW = 910 days;
    uint256 public constant INACTIVITY_UBI    = 1460 days;
    uint256 public constant GUARDIAN_TIMELOCK = 7 days;
    uint256 public constant MAX_WARDS         = 3;

    IBioVerifier public immutable bioVerifier;

    uint256 public totalSupply;
    uint256 public totalHumans;
    uint256 public ubiPool;
    uint256 public ubiPerHumanAccumulated;

    mapping(address => uint256) public balanceOf;
    mapping(address => uint256) public escrowOf;
    mapping(address => bool)    public isHuman;
    mapping(uint256 => bool)    public usedCommitments;
    // usedNullifiers stores the on-chain record of each biometric nullifier.
    // Slot 8. Prevents the same biometric from registering a second wallet
    // even when bypassing the API layer and calling the contract directly.
    mapping(bytes32 => address) public usedNullifiers;
    mapping(address => uint256) public commitmentOf;
    mapping(address => uint256) public lastActivity;
    mapping(address => uint256) public lastDemurrage;
    mapping(address => uint256) public ubiClaimed;
    mapping(address => address) public guardianOf;
    mapping(address => address) public pendingGuardian;
    mapping(address => uint256) public guardianRequestedAt;
    mapping(address => uint256) public wardCount;

    // FIX 10: CAPS vs Go-chain discrepancy — documented.
    // Solidity CAPS[0]=50 means Phase 0 (<100 humans) cap = 50 × fairShare = 50,000 AEQ.
    // The Go chain enforces a tighter server-side cap of max(5, totalHumans) × fairShare
    // (which grows from 5× to 25× as humans register, never exceeding 25,000 AEQ at Phase 0).
    // The Solidity cap is a last-resort on-chain backstop; routine enforcement is by the Go layer.
    // These two values are intentionally different: the Solidity cap is deliberately loose so it
    // only triggers if the Go-layer cap is bypassed by a direct contract call.
    uint256[5] public CAPS       = [50, 20, 10, 5, 3];
    // FIX (P3-18, beta-launch audit 2026-07-05): THRESHOLDS didn't have its
    // own version of CAPS' comment above explaining the same intentional
    // Go/Solidity discrepancy. The Go chain's actual phase-in schedule
    // (bootstrapMultiplierLocked, state.go) is a much simpler two-point
    // ramp — the multiplier grows 1:1 with the registered-human count from
    // 5x at ≤5 humans up to 25x once the count reaches 25, then stays flat
    // at 25x forever after — not this array's 5-step, population-gated
    // breakpoint schedule. Same reasoning as CAPS: this is a deliberately
    // loose, rarely-relevant on-chain backstop for a direct contract call
    // that bypasses the Go layer, not a value meant to track the Go
    // schedule exactly.
    uint256[5] public THRESHOLDS = [0, 100, 1_000, 10_000, 100_000];

    event Registered(address indexed human, uint256 commitment, uint256 grant);
    /// @dev Standard ERC-20 Transfer event used to signal mints (from == address(0)).
    /// Emitted in _confirmAlive() for the wake-up bonus mint so off-chain indexers can track supply changes.
    event Transfer(address indexed from, address indexed to, uint256 value);
    event Transferred(address indexed from, address indexed to, uint256 amount, uint256 fee);
    event DemurrageApplied(address indexed human, uint256 amount);
    event WealthCapApplied(address indexed human, uint256 excess);
    event UBIAccumulated(uint256 addedPerHuman, uint256 total);
    event UBIClaimed(address indexed human, uint256 amount);
    event EscrowCreated(address indexed human, uint256 amount);
    event EscrowReleased(address indexed human, uint256 amount);
    event EscrowToUBI(address indexed human, uint256 amount);
    event GuardianProposed(address indexed human, address indexed proposed);
    event GuardianConfirmed(address indexed human, address indexed guardian);
    event GuardianRevoked(address indexed human, address indexed guardian);
    event AliveConfirmed(address indexed human, address indexed by);
    event ForfeitedUBIRecycled(address indexed human, uint256 amount);

    constructor(address _bioVerifier) {
        require(_bioVerifier != address(0), "BioVerifier cannot be zero address");
        bioVerifier = IBioVerifier(_bioVerifier);
    }

    // SECURITY (P0, launch audit 2026-07-03): a plain register(), keyed off
    // msg.sender with no binding whatsoever to the proof's public signals
    // (pubSignals = [commitment, nullifier] — no wallet-address slot exists
    // in this circuit version, unlike V5/V6's [wallet, commitment] layout),
    // used to live here. Anyone who observed a pending register() tx in the
    // mempool could resubmit the identical calldata from their own address
    // with higher gas: their tx mines first, THEY get INITIAL_GRANT, and
    // usedNullifiers[effectiveNullifier] permanently records THEIR address —
    // burning the real human's biometric nullifier forever, since nullifiers
    // are deterministic per-biometric and can never be regenerated. This was
    // a real, live, exploitable front-running theft-plus-permanent-DoS bug on
    // a publicly callable function, independent of and bypassable around the
    // Go relay layer's own protections.
    //
    // register() is removed rather than patched: this circuit's public
    // signals have no wallet-binding data to check msg.sender against (that
    // would require a circuit change, out of this contract's control), so
    // there is no way to make an unsigned, sender-agnostic registration path
    // safe here. registerWithSig() below is the only registration path —
    // it is already what the entire production Go backend (register.go)
    // exclusively uses — and is safe because the beneficiary (claimedHuman)
    // is fixed by an independent ECDSA signature, not by whoever broadcasts
    // the transaction.

    /**
     * @notice Register as a verified human via meta-transaction (gasless for the user)
     * @dev A relayer submits this transaction and pays gas, but the new human is
     *      claimedHuman, NOT msg.sender. claimedHuman must have personally signed
     *      this exact commitment, for this exact contract, on this exact chain,
     *      verified on-chain via ecrecover. usedCommitments prevents replay across
     *      either registration path.
     */
    // registerWithSig v2: circuit now outputs the nullifier as pubSignals[1]
    // so "1 biometric = 1 nullifier" is enforced by the ZK proof itself.
    // pubSignals[0] = commitment, pubSignals[1] = nullifier (ZK-bound).
    // The external bytes32 nullifier param is still accepted for compatibility
    // but MUST match pubSignals[1] when circuit version >= 2.
    function registerWithSig(
        uint[2] calldata pA,
        uint[2][2] calldata pB,
        uint[2] calldata pC,
        uint[2] calldata pubSignals,
        address claimedHuman,
        bytes calldata signature,
        bytes32 nullifier
    ) external {
        // FIX (ecrecover zero-address): ecrecover returns address(0) on
        // malformed signature data instead of reverting. Without this check,
        // claimedHuman == address(0) combined with garbage signature bytes
        // would make `_recoverSigner(...) == claimedHuman` pass (0 == 0),
        // registering address(0) as human and minting INITIAL_GRANT into an
        // unusable balance — needless attack surface even though it can't be
        // profitably exploited.
        require(claimedHuman != address(0), "claimedHuman cannot be zero address");
        require(!isHuman[claimedHuman], "Already registered");

        uint256 commitment = pubSignals[0];
        require(!usedCommitments[commitment], "Commitment used");

        // FIX (v2-only nullifier): same reasoning as register() above — a v1
        // circuit (pubSignals[1] == 0) provides no cryptographic nullifier at
        // all, so the old fallback to the caller-supplied `nullifier` param
        // let anyone pick an arbitrary value, breaking "one biometric = one
        // registration". Require the ZK-bound nullifier unconditionally.
        require(pubSignals[1] != 0, "v1 circuit not accepted: ZK-bound nullifier required");
        bytes32 effectiveNullifier = bytes32(pubSignals[1]);
        require(nullifier == bytes32(0) || nullifier == effectiveNullifier,
            "Nullifier mismatch: submitted nullifier does not match ZK-derived value");

        require(usedNullifiers[effectiveNullifier] == address(0), "Nullifier used");

        // FIX 2: sign over effectiveNullifier (ZK-derived) not the raw nullifier param
        bytes32 messageHash = keccak256(abi.encodePacked(
            block.chainid,
            address(this),
            "register",
            commitment,
            effectiveNullifier
        ));
        bytes32 ethSignedHash = keccak256(abi.encodePacked(
            "\x19Ethereum Signed Message:\n32",
            messageHash
        ));
        require(
            _recoverSigner(ethSignedHash, signature) == claimedHuman,
            "Invalid signature"
        );

        // CEI: write all state before the external call
        usedCommitments[commitment] = true;
        commitmentOf[claimedHuman] = commitment;
        usedNullifiers[effectiveNullifier] = claimedHuman;
        isHuman[claimedHuman] = true;
        totalHumans++;
        balanceOf[claimedHuman] += INITIAL_GRANT;
        totalSupply += INITIAL_GRANT;
        ubiClaimed[claimedHuman] = ubiPerHumanAccumulated;
        lastActivity[claimedHuman] = block.timestamp;
        lastDemurrage[claimedHuman] = block.timestamp;

        // External call last
        require(bioVerifier.verifyProof(pA, pB, pC, pubSignals), "Invalid proof");

        emit Registered(claimedHuman, commitment, INITIAL_GRANT);
    }

    function _recoverSigner(bytes32 ethSignedHash, bytes calldata signature) internal pure returns (address) {
        require(signature.length == 65, "Invalid signature length");
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }
        if (v < 27) v += 27;
        // Prevent signature malleability (EIP-2)
        require(
            uint256(s) <= 0x7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0,
            "Invalid signature: high s value"
        );
        return ecrecover(ethSignedHash, v, r, s);
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        require(isHuman[msg.sender], "Not human");
        require(balanceOf[msg.sender] >= amount, "Insufficient balance");
        require(to != address(0) && to != msg.sender, "Invalid recipient");
        _applyDemurrage(msg.sender);
        require(balanceOf[msg.sender] >= amount, "Insufficient after demurrage");
        uint256 fee = _calcFee(msg.sender, amount);
        uint256 ubiContrib = (fee * UBI_SHARE_BPS) / 10_000;
        uint256 burned = fee - ubiContrib;
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount - fee;
        ubiPool += ubiContrib;
        totalSupply -= burned;
        lastActivity[msg.sender] = block.timestamp;
        if (isHuman[to]) lastActivity[to] = block.timestamp;
        _applyWealthCap(to);
        emit Transferred(msg.sender, to, amount, fee);
        return true;
    }

    function _calcFee(address sender, uint256 amount) internal view returns (uint256) {
        uint256 base = (amount * TX_FEE_BPS) / 10_000;
        if (totalSupply == 0) return base;
        uint256 shareBPS = (balanceOf[sender] * 10_000) / totalSupply;
        uint256 extra = shareBPS >= 1000 ? 100 : shareBPS >= 500 ? 50 : shareBPS >= 100 ? 10 : 0;
        return base + (amount * extra) / 10_000;
    }

    function applyDemurrage(address human) external { require(isHuman[human],"Not human"); _applyDemurrage(human); }

    function _applyDemurrage(address human) internal {
        uint256 fs = fairShare();
        if (balanceOf[human] <= fs) { lastDemurrage[human] = block.timestamp; return; }
        uint256 elapsed = block.timestamp - lastDemurrage[human];
        if (elapsed == 0) return;
        uint256 excess = balanceOf[human] - fs;
        uint256 fee = (excess * DEMURRAGE_BPS * elapsed) / (10_000 * SECONDS_PER_YEAR);
        if (fee == 0) return;
        if (fee > excess) fee = excess;
        balanceOf[human] -= fee;
        ubiPool += fee;
        lastDemurrage[human] = block.timestamp;
        emit DemurrageApplied(human, fee);
    }

    function applyWealthCap(address human) external { require(isHuman[human],"Not human"); _applyWealthCap(human); }

    function _applyWealthCap(address human) internal {
        if (!isHuman[human]) return; // Only apply wealth cap to registered humans
        uint256 cap = wealthCap();
        if (balanceOf[human] <= cap) return;
        uint256 excess = balanceOf[human] - cap;
        balanceOf[human] = cap;
        ubiPool += excess;
        emit WealthCapApplied(human, excess);
    }

    function accumulateUBI() external {
        require(totalHumans > 0, "No humans");
        require(ubiPool > 0, "Pool empty");
        uint256 addPerHuman = ubiPool / totalHumans;
        require(addPerHuman > 0, "Too small");
        // FIX 6: prevent dust distributions — minimum 0.1 AEQ per human (INITIAL_GRANT / 10000)
        require(addPerHuman >= INITIAL_GRANT / 10_000, "Distribution below minimum threshold");
        ubiPool -= addPerHuman * totalHumans;
        ubiPerHumanAccumulated += addPerHuman;
        emit UBIAccumulated(addPerHuman, ubiPerHumanAccumulated);
    }

    function claimUBI() external {
        require(isHuman[msg.sender], "Not human");
        require(escrowOf[msg.sender] == 0, "In escrow");
        // FIX 7: guard against underflow if ubiClaimed somehow exceeds ubiPerHumanAccumulated
        // (mirrors the same guard in claimableUBI). Solidity 0.8+ would revert with a cryptic
        // arithmetic error; this gives a clear, user-friendly message instead.
        require(ubiClaimed[msg.sender] <= ubiPerHumanAccumulated, "UBI accounting error");
        uint256 owed = ubiPerHumanAccumulated - ubiClaimed[msg.sender];
        require(owed > 0, "Nothing to claim");
        ubiClaimed[msg.sender] = ubiPerHumanAccumulated;
        balanceOf[msg.sender] += owed;
        // P0-5 FIX: do NOT increase totalSupply here.
        // UBI funds were already counted in totalSupply when accumulated into
        // ubiPool. Incrementing again would violate the invariant
        // SUM(balanceOf) + ubiPool + SUM(escrowOf) == totalSupply, causing
        // fairShare() and wealthCap() to drift upward with every claim.
        // totalSupply += owed;  <-- REMOVED
        lastActivity[msg.sender] = block.timestamp;
        _applyWealthCap(msg.sender);
        emit UBIClaimed(msg.sender, owed);
    }

    function claimableUBI(address human) external view returns (uint256) {
        if (!isHuman[human]) return 0;
        if (ubiClaimed[human] > ubiPerHumanAccumulated) return 0;
        return ubiPerHumanAccumulated - ubiClaimed[human];
    }

    function confirmAlive() external {
        require(isHuman[msg.sender], "Not human");
        _confirmAlive(msg.sender);
        emit AliveConfirmed(msg.sender, msg.sender);
    }

    function _confirmAlive(address human) internal {
        lastActivity[human] = block.timestamp;
        if (escrowOf[human] > 0) {
            uint256 amount = escrowOf[human];
            escrowOf[human] = 0;
            // FIX: lastDemurrage was never reset on escrow release. Without
            // this, the very next _applyDemurrage(human) call computes
            // elapsed = block.timestamp - lastDemurrage[human] over the ENTIRE
            // span since before the account went inactive (inactivity period
            // + up to 1.5 years in escrow) — easily years. The resulting fee
            // is capped at the full excess-over-fairShare(), so the recovering
            // human's balance (their returned escrow + wake-up bonus) would
            // be taxed down to fairShare() almost immediately, defeating the
            // entire point of the wake-up incentive below.
            lastDemurrage[human] = block.timestamp;
            uint256 fs = fairShare();
            // FIX (P0-2, beta-launch audit 2026-07-05): a human sitting in
            // escrow keeps counting in totalHumans and keeps accruing a
            // per-head UBI entitlement via accumulateUBI() (already deducted
            // from ubiPool on their behalf) the whole time — they just can't
            // call claimUBI() themselves while escrowOf[human] > 0 (see that
            // function's "In escrow" guard). This used to overwrite
            // ubiClaimed[human] = ubiPerHumanAccumulated below without ever
            // crediting the difference to balanceOf, unlike claimUBI()'s own
            // correct owed calculation — silently discarding money already
            // taken out of ubiPool on every ordinary escrow recovery with an
            // outstanding entitlement (the expected common case, not an
            // attack scenario), breaking
            // SUM(balanceOf)+SUM(escrowOf)+ubiPool == totalSupply. Same bug
            // class as the one already fixed for triggerEscrowToUBI (see
            // ForfeitedUBIRecycled) — this parallel path was missed by that
            // fix. Mirrors claimUBI()'s guard and calculation exactly.
            require(ubiClaimed[human] <= ubiPerHumanAccumulated, "UBI accounting error");
            uint256 owedUBI = ubiPerHumanAccumulated - ubiClaimed[human];
            // NOTE (FIX 5 / FIX 9): When recovering from escrow, the human receives their
            // escrowed amount PLUS one fairShare() of newly minted AEQ as an incentive to
            // confirm aliveness. totalSupply increases by fairShare() (not by owedUBI — that
            // AEQ was already counted in totalSupply when accumulated into ubiPool, exactly
            // as in claimUBI()) to maintain the supply invariant. This is intentional
            // economic policy — not a bug.
            // Three events are emitted for auditability:
            //   EscrowReleased — the return of the original escrowed balance
            //   Transfer(address(0), human, fs) — the mint of the wake-up bonus
            //   UBIClaimed — the UBI entitlement accrued while in escrow, same event claimUBI() emits
            balanceOf[human] += amount + fs + owedUBI;
            totalSupply += fs;
            ubiClaimed[human] = ubiPerHumanAccumulated;
            emit EscrowReleased(human, amount);
            if (fs > 0) emit Transfer(address(0), human, fs); // mint event for the wake-up bonus
            if (owedUBI > 0) emit UBIClaimed(human, owedUBI);
            // FIX (P2, launch audit 2026-07-03): a returning human's pre-escrow
            // balance (which could already have been near the OLD cap) plus the
            // fairShare() wake-up bonus above can exceed the CURRENT cap. Every
            // other balance-crediting path (transfer's recipient side, claimUBI)
            // already reapplies the cap after crediting; this one didn't, leaving
            // a returning human over cap until someone happened to call the
            // permissionless applyWealthCap() on their behalf.
            _applyWealthCap(human);
        }
    }

    function triggerEscrow(address human) external {
        require(isHuman[human], "Not human");
        require(escrowOf[human] == 0, "Already in escrow");
        require(block.timestamp >= lastActivity[human] + INACTIVITY_ESCROW, "Not inactive enough");
        uint256 amount = balanceOf[human];
        balanceOf[human] = 0;
        escrowOf[human] = amount;
        emit EscrowCreated(human, amount);
    }

    function triggerEscrowToUBI(address human) external {
        // FIX 2: guard isHuman before decrementing totalHumans to avoid underflow
        // and to prevent confusing revert messages if called on a non-registered address.
        require(isHuman[human], "Not a registered human");
        require(escrowOf[human] > 0, "Not in escrow");
        require(block.timestamp >= lastActivity[human] + INACTIVITY_UBI, "Too soon");
        // FIX (P2-a, audit 2026-07-06): a human who is themselves acting as
        // guardian for others (wardCount[human] > 0) must not be swept away
        // here — doing so would clear isHuman[human] while every ward's
        // guardianOf[ward] still points at this now-deregistered address,
        // permanently orphaning their guardian safety net (guardianOf can
        // only be changed by the ward's own proposeGuardian/confirmGuardian,
        // never by this function, since there's no reverse index from a
        // guardian back to their wards to clean up here). Each affected ward
        // can unblock this at any time via their own revokeGuardian() —
        // self-service, doesn't require this human's cooperation — so this
        // is a delay, not a deadlock.
        require(wardCount[human] == 0, "Still guarding wards - they must revoke first");
        uint256 amount = escrowOf[human];
        escrowOf[human] = 0;
        ubiPool += amount;
        // FIX (P2, beta-launch audit 2026-07-05): a human being swept here may
        // still hold an unclaimed UBI entitlement (ubiPerHumanAccumulated minus
        // their own ubiClaimed), accrued while they were still counted in
        // totalHumans at the time of a prior accumulateUBI() call. That call
        // already deducted their share out of ubiPool (as part of
        // addPerHuman*totalHumans) — it is NOT sitting in ubiPool waiting to
        // be reclaimed here. Once isHuman is cleared below, claimUBI()'s
        // isHuman check permanently forecloses this human from ever claiming
        // it, silently violating the SUM(balanceOf)+SUM(escrowOf)+ubiPool ==
        // totalSupply invariant by that amount forever. Recycle it back into
        // ubiPool — the same treatment already given to their escrowed
        // balance above — instead of letting it evaporate untracked.
        if (ubiPerHumanAccumulated > ubiClaimed[human]) {
            uint256 forfeited = ubiPerHumanAccumulated - ubiClaimed[human];
            ubiPool += forfeited;
            ubiClaimed[human] = ubiPerHumanAccumulated;
            emit ForfeitedUBIRecycled(human, forfeited);
        }
        isHuman[human] = false;
        totalHumans--;
        // FIX (wardCount leak): revokeGuardian()'s comment claimed this cleanup
        // was "handled by triggerEscrowToUBI", but it never actually was — a
        // ward swept to UBI by inactivity kept occupying a wardCount slot on
        // their guardian forever (de-registered humans can't call
        // revokeGuardian themselves, since it requires isHuman). Release it
        // here so the guardian's capacity is correctly freed.
        address g = guardianOf[human];
        if (g != address(0)) {
            wardCount[g]--;
            guardianOf[human] = address(0);
            emit GuardianRevoked(human, g);
        }
        emit EscrowToUBI(human, amount);
    }

    function proposeGuardian(address guardian) external {
        require(isHuman[msg.sender] && isHuman[guardian], "Must be human");
        require(guardian != msg.sender, "Cannot guard yourself");
        require(guardianOf[guardian] == address(0), "Guardian has own guardian");
        require(wardCount[guardian] < MAX_WARDS, "Max wards reached");
        require(guardianOf[msg.sender] != guardian, "Circular dependency");
        // FIX (P2, launch audit 2026-07-03): V6 blocked layered guardian chains
        // both ways — a guardian couldn't have a guardian of their own, AND a
        // ward couldn't already be acting as someone else's guardian. V7 kept
        // only the first half (checked above via guardianOf[guardian]); nothing
        // stopped msg.sender, while already guarding up to MAX_WARDS others,
        // from also acquiring their own guardian, letting multi-hop chains
        // (C guards A, A guards B) re-emerge. Restore the other half.
        require(wardCount[msg.sender] == 0, "Cannot have a guardian while acting as one");
        pendingGuardian[msg.sender] = guardian;
        guardianRequestedAt[msg.sender] = block.timestamp;
        emit GuardianProposed(msg.sender, guardian);
    }

    function confirmGuardian() external {
        // FIX: a human de-registered via triggerEscrowToUBI while a guardian
        // proposal was still pending could call this afterward (isHuman was
        // never checked here, unlike revokeGuardian) and permanently consume
        // a wardCount slot on the proposed guardian — one that can then never
        // be released, since both revokeGuardian and triggerEscrowToUBI
        // require isHuman[msg.sender]==true to release a wardCount slot.
        require(isHuman[msg.sender], "Not human");
        require(pendingGuardian[msg.sender] != address(0), "No pending guardian");
        require(block.timestamp >= guardianRequestedAt[msg.sender] + GUARDIAN_TIMELOCK, "Timelock active");
        // FIX (P1, beta-launch audit 2026-07-05): re-validate both layering
        // invariants at CONFIRM time, not just at PROPOSE time. The 7-day
        // timelock between propose and confirm is a window during which
        // EITHER side of a pending guardian relationship can independently
        // propose AND confirm an unrelated relationship of their own —
        // wardCount only increments on confirm, so proposeGuardian's
        // wardCount[msg.sender]==0 check doesn't see a still-pending proposal
        // on the other side yet. Without re-checking here, two proposals
        // confirmed in either order can still assemble a multi-hop chain
        // (C guards B, B guards A) — exactly what that propose-time check was
        // meant to prevent, just reached via timing instead of the specific
        // pattern it blocks directly.
        require(wardCount[msg.sender] == 0, "Cannot confirm a guardian while acting as one");
        address oldGuardian = guardianOf[msg.sender];
        if (oldGuardian != address(0)) {
            wardCount[oldGuardian]--;
            emit GuardianRevoked(msg.sender, oldGuardian);
        }
        address g = pendingGuardian[msg.sender];
        require(wardCount[g] < MAX_WARDS, "Guardian already has maximum wards");
        require(guardianOf[g] == address(0), "Guardian now has own guardian");
        guardianOf[msg.sender] = g;
        wardCount[g]++;
        pendingGuardian[msg.sender] = address(0);
        guardianRequestedAt[msg.sender] = 0;
        emit GuardianConfirmed(msg.sender, g);
    }

    function revokeGuardian() external {
        // FIX 8: require isHuman so that de-registered users (isHuman=false but
        // guardianOf still set) cannot unexpectedly decrement the guardian's wardCount.
        // Clean-up of guardianOf for de-registered accounts is handled by triggerEscrowToUBI
        // flow; revokeGuardian is an active human action.
        require(isHuman[msg.sender], "Not human");
        address g = guardianOf[msg.sender];
        require(g != address(0), "No guardian");
        wardCount[g]--;
        guardianOf[msg.sender] = address(0);
        pendingGuardian[msg.sender] = address(0);
        guardianRequestedAt[msg.sender] = 0;
        emit GuardianRevoked(msg.sender, g);
    }

    function guardianConfirmAlive(address ward) external {
        require(isHuman[msg.sender], "Not human");
        require(guardianOf[ward] == msg.sender, "Not the guardian");
        _confirmAlive(ward);
        emit AliveConfirmed(ward, msg.sender);
    }

    function fairShare() public view returns (uint256) {
        if (totalHumans == 0) return INITIAL_GRANT;
        return totalSupply / totalHumans;
    }

    function wealthCap() public view returns (uint256) {
        return CAPS[currentPhase()] * fairShare();
    }

    function currentPhase() public view returns (uint256) {
        for (uint256 i = 4; i > 0; i--) {
            if (totalHumans >= THRESHOLDS[i]) return i;
        }
        return 0;
    }

    function getStats() external view returns (
        uint256 _supply, uint256 _humans, uint256 _pool,
        uint256 _fair, uint256 _cap, uint256 _phase, uint256 _ubiAcc
    ) {
        return (totalSupply, totalHumans, ubiPool, fairShare(), wealthCap(), currentPhase(), ubiPerHumanAccumulated);
    }

    function calcFee(address sender, uint256 amount) external view returns (uint256) {
        return _calcFee(sender, amount);
    }
}
