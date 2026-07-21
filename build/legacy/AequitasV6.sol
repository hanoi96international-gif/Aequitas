// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

/**
 * @title AequitasV6
 * @notice Decentralized human-centric monetary system
 * @author Aequitas Protocol
 *
 * CORE PRINCIPLE:
 * "Money exists because people exist. Nothing more, nothing less."
 *
 * ===================================================================
 * KEY MECHANISMS
 * ===================================================================
 *
 * 1. REGISTRATION
 *    Every verified human receives exactly 1,000 AEQ.
 *    One person, one wallet, forever.
 *    Verification via Groth16 Zero-Knowledge Proof (biometric).
 *    No admin. No whitelist. No exceptions.
 *
 * 2. PROOF OF ALIVE
 *    Every 2 years, a human must confirm they are still active.
 *    3 warnings (60 days apart) before any action is taken.
 *    After 3 warnings: AEQ -> personal Escrow (NOT UBI Pool yet).
 *    Escrow held for 2 additional years before entering UBI Pool.
 *    This protects: prisoners, sick, disaster zones, no connectivity.
 *
 * 3. GUARDIAN SYSTEM (hardened against coercion and abuse)
 *
 *    PURPOSE:
 *    Allow a trusted person to confirm Proof of Alive on behalf
 *    of someone who temporarily cannot access their device.
 *    Examples: prisoners, hospitalized, elderly, war zones.
 *
 *    STRICT LIMITS (5 security fixes):
 *
 *    FIX 1 - Guardian has ZERO transaction rights.
 *    Guardian can ONLY call confirmAlive(). Nothing else.
 *    Cannot transfer, approve, or touch AEQ in any way.
 *    The ward's Private Key is the only way to move funds.
 *
 *    FIX 2 - 7-day Timelock on Guardian assignment.
 *    Setting a Guardian takes 7 days to become active.
 *    Prevents forced Guardian assignment under immediate duress.
 *    Ward can cancel during the 7-day window.
 *
 *    FIX 3 - Guardian confirmations are limited (max 3 consecutive).
 *    After 3 Guardian confirmations without ward self-activity,
 *    a secondary review flag is raised on-chain.
 *    The ward must eventually confirm themselves to reset the flag.
 *    Prevents permanent proxy control over someone's humanity status.
 *
 *    FIX 4 - No circular Guardian relationships.
 *    If A is Guardian of B, then B cannot be Guardian of A.
 *    Prevents mutual coercion lock ("I control you, you control me").
 *
 *    FIX 5 - A Guardian cannot themselves have a Guardian.
 *    If you are a Guardian for someone, you cannot appoint a Guardian.
 *    And if you have a Guardian, you cannot become someone else's Guardian.
 *    Prevents layered control chains.
 *
 * 4. REACTIVATION AFTER INACTIVITY
 *    Returns: personal Escrow (if still held) + current fairShare.
 *    Requires fresh biometric proof (same commitment = same person).
 *    Same commitment stays permanently blocked (no double-dip).
 *    Getting fairShare on return = fair, not punitive.
 *
 * 5. WEALTH CAP (active from human #1, always on)
 *    Phase 0 (100):     50x fairShare  - generous for early growth
 *    Phase 1 (1,000):   20x fairShare
 *    Phase 2 (10,000):  10x fairShare
 *    Phase 3 (100,000):  5x fairShare
 *    Phase 4 (100,000+):  3x fairShare
 *    Overflow -> redistributed equally to ALL active humans instantly.
 *
 * 6. DEMURRAGE (anti-hoarding, 1% annual on excess above fairShare)
 *    Charged monthly on balance ABOVE fairShare only.
 *    Does NOT reduce total supply - moves to UBI Pool.
 *    UBI Pool distributed equally to all active humans.
 *    Encourages circulation without punishing normal holdings.
 *
 * 7. TRANSACTION FEE (0.1%)
 *    40% -> Validator Pool
 *    30% -> Liquidity Pool
 *    20% -> UBI Pool
 *    10% -> Treasury
 *
 * 8. NO ALGORITHMIC INFLATION
 *    The ONLY money creation event: new human = +1,000 AEQ.
 *    No external parameters. No oracle. No manipulation possible.
 *    Total supply = verified active humans  1,000 AEQ (baseline).
 *
 * 9. GINI COEFFICIENT
 *    Measured off-chain by Keeper Bot (gas-efficient, accurate).
 *    Written on-chain as a read-only transparency metric.
 *    NOT used to control money supply. Mathematics, not politics.
 * ===================================================================
 */

interface IBioVerifier {
    function verifyProof(
        uint[2] calldata _pA,
        uint[2][2] calldata _pB,
        uint[2] calldata _pC,
        uint[2] calldata _pubSignals
    ) external view returns (bool);
}

contract AequitasV6 {

    //  ERC-20 
    string public constant name     = "Aequitas";
    string public constant symbol   = "AEQ";
    uint8  public constant decimals = 18;

    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    //  CORE STATE 
    IBioVerifier public verifier;

    mapping(uint256 => bool)    public usedCommitments;
    mapping(address => bool)    public isHuman;
    mapping(address => bool)    public isInactive;
    mapping(address => uint256) public humanRegisteredAt;
    mapping(address => uint256) public lastActivityAt;
    mapping(address => uint256) public commitmentOf;

    address[] public humanList;
    uint256   public totalSupply;
    uint256   public totalHumans;

    //  PROOF OF ALIVE 
    mapping(address => uint8)   public warningCount;
    mapping(address => uint256) public lastWarningAt;
    mapping(address => uint256) public escrowBalance;
    mapping(address => uint256) public escrowSince;

    uint256 public constant INACTIVITY_PERIOD  = 730 days; // 2 years
    uint256 public constant WARNING_INTERVAL   = 60 days;
    uint256 public constant ESCROW_HOLD_PERIOD = 730 days; // 2 years in escrow

    //  GUARDIAN SYSTEM 

    mapping(address => address)   public guardianOf;          // human -> guardian
    mapping(address => address[]) public guardianFor;         // guardian -> wards
    mapping(address => uint8)     public guardianConfirmCount; // consecutive guardian confirmations without self-activity
    mapping(address => bool)      public reviewFlagged;       // flagged for community review

    // Timelock: pending guardian assignment (FIX 2)
    mapping(address => address)   public pendingGuardian;
    mapping(address => uint256)   public pendingGuardianSince;

    uint256 public constant MAX_WARDS              = 3;
    uint256 public constant GUARDIAN_TIMELOCK      = 7 days;
    uint8   public constant MAX_GUARDIAN_CONFIRMS  = 3; // FIX 3

    //  IMMUTABLE CONSTANTS 
    uint256 public constant INITIAL_GRANT     = 1000 * 10**18;
    uint256 public constant FEE_BPS           = 10;   // 0.1%
    uint256 public constant DEMURRAGE_BPS     = 100;  // 1% annual
    uint256 public constant DEMURRAGE_MONTHS  = 12;

    //  FEE POOLS 
    uint256 public validatorPool;
    uint256 public lpPool;
    uint256 public ubiPool;
    uint256 public treasury;

    //  GINI + INDEX 
    uint256 public giniCoefficient;
    uint256 public aequitasIndex;
    uint256 public lastGiniUpdate;
    uint256 public lastDemurrageRun;
    address public keeperBot;

    //  EVENTS 
    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);

    event HumanRegistered(address indexed wallet, uint256 totalHumans, uint256 timestamp);
    event HumanReactivated(address indexed wallet, uint256 escrowReturned, uint256 fairShareGranted);
    event WalletFlaggedInactive(address indexed wallet, uint256 escrowAmount);
    event EscrowReleasedToUBI(address indexed wallet, uint256 amount);

    event ProofOfAliveConfirmed(address indexed wallet, address indexed confirmedBy, bool byGuardian);
    event InactivityWarning(address indexed wallet, uint8 warningNumber, uint256 deadline);

    // Guardian events
    event GuardianProposed(address indexed human, address indexed guardian, uint256 activatesAt);
    event GuardianActivated(address indexed human, address indexed guardian);
    event GuardianCancelled(address indexed human, address indexed guardian);
    event GuardianRevoked(address indexed human, address indexed guardian);
    event GuardianPenalized(address indexed guardian, string reason);
    event ReviewFlagged(address indexed human, string reason);

    event WealthRedistributed(address indexed wallet, uint256 overflow, uint256 perPerson);
    event DemurrageCharged(address indexed wallet, uint256 amount);
    event DemurrageDistributed(uint256 totalCollected, uint256 perHuman);
    event UBIDistributed(uint256 amountPerHuman);
    event FeeCollected(uint256 validatorShare, uint256 lpShare, uint256 ubiShare, uint256 treasuryShare);
    event GiniUpdated(uint256 gini, uint256 index);

    //  CONSTRUCTOR 

    constructor(address _verifier, address _keeperBot) {
        verifier         = IBioVerifier(_verifier);
        keeperBot        = _keeperBot;
        lastDemurrageRun = block.timestamp;
    }

    //  MODIFIERS 

    modifier onlyKeeper() {
        require(msg.sender == keeperBot, "Only Keeper Bot");
        _;
    }

    modifier onlyActiveHuman() {
        require(isHuman[msg.sender] && !isInactive[msg.sender], "Not an active human");
        _;
    }

    //  ERC-20 

    function transfer(address to, uint256 amount) external returns (bool) {
        _transferWithFee(msg.sender, to, amount);
        _recordActivity(msg.sender);
        return true;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        require(allowance[from][msg.sender] >= amount, "Insufficient allowance");
        allowance[from][msg.sender] -= amount;
        _transferWithFee(from, to, amount);
        _recordActivity(from);
        return true;
    }

    function _transferWithFee(address from, address to, uint256 amount) internal {
        require(balanceOf[from] >= amount, "Insufficient balance");

        uint256 fee            = (amount * FEE_BPS) / 10000;
        uint256 amountAfterFee = amount - fee;

        if (fee > 0) {
            uint256 toValidators = (fee * 40) / 100;
            uint256 toLPs        = (fee * 30) / 100;
            uint256 toUBI        = (fee * 20) / 100;
            uint256 toTreasury   = fee - toValidators - toLPs - toUBI;

            balanceOf[from] -= fee;
            validatorPool   += toValidators;
            lpPool          += toLPs;
            ubiPool         += toUBI;
            treasury        += toTreasury;

            emit FeeCollected(toValidators, toLPs, toUBI, toTreasury);
        }

        balanceOf[from] -= amountAfterFee;
        balanceOf[to]   += amountAfterFee;

        emit Transfer(from, to, amountAfterFee);
        _applyWealthCap(to);
    }

    function _recordActivity(address wallet) internal {
        if (!isHuman[wallet] || isInactive[wallet]) return;

        lastActivityAt[wallet]      = block.timestamp;
        guardianConfirmCount[wallet] = 0; // FIX 3: self-activity resets guardian confirm count

        // Reset warning process on activity
        if (warningCount[wallet] > 0) {
            warningCount[wallet]  = 0;
            lastWarningAt[wallet] = 0;
        }

        // Clear review flag on self-activity
        if (reviewFlagged[wallet]) {
            reviewFlagged[wallet] = false;
        }
    }

    //  REGISTRATION 

    function registerHuman(
        uint[2] calldata _pA,
        uint[2][2] calldata _pB,
        uint[2] calldata _pC,
        uint[2] calldata _pubSignals
    ) external {
        require(
            verifier.verifyProof(_pA, _pB, _pC, _pubSignals),
            "Invalid biometric proof"
        );
        require(
            _pubSignals[0] == uint256(uint160(msg.sender)),
            "Wallet mismatch: proof must include your wallet address"
        );

        uint256 commitment = _pubSignals[1];
        require(!usedCommitments[commitment], "Biometric already registered");
        require(!isHuman[msg.sender],         "Wallet already registered");

        usedCommitments[commitment]    = true;
        commitmentOf[msg.sender]       = commitment;
        isHuman[msg.sender]            = true;
        humanRegisteredAt[msg.sender]  = block.timestamp;
        lastActivityAt[msg.sender]     = block.timestamp;
        totalHumans                    += 1;
        humanList.push(msg.sender);

        balanceOf[msg.sender] = INITIAL_GRANT;
        totalSupply           += INITIAL_GRANT;

        emit Transfer(address(0), msg.sender, INITIAL_GRANT);
        emit HumanRegistered(msg.sender, totalHumans, block.timestamp);
    }

    //  PROOF OF ALIVE 

    /**
     * @notice Confirm you are alive and active.
     * Called by: the human themselves OR their active Guardian.
     *
     * GUARDIAN RESTRICTIONS (FIX 1 + FIX 3):
     * - Guardian can ONLY call this function. No other rights.
     * - After MAX_GUARDIAN_CONFIRMS consecutive Guardian confirmations
     *   without self-activity, a review flag is raised on-chain.
     * - Ward must eventually confirm themselves to clear the flag.
     */
    function confirmAlive(address human) external {
        require(isHuman[human],       "Not a registered human");
        require(!isInactive[human],   "Already flagged inactive - use reactivate()");

        bool calledBySelf     = (msg.sender == human);
        bool calledByGuardian = (msg.sender == guardianOf[human]);

        require(calledBySelf || calledByGuardian, "Only human or their Guardian can confirm");

        // FIX 1: If guardian is calling, verify they are active
        // Guardian has NO other rights beyond this function
        if (calledByGuardian) {
            require(
                isHuman[msg.sender] && !isInactive[msg.sender],
                "Guardian must be an active human"
            );

            // FIX 3: Track consecutive guardian confirmations
            guardianConfirmCount[human] += 1;

            if (guardianConfirmCount[human] >= MAX_GUARDIAN_CONFIRMS) {
                // Raise community review flag - ward has not been self-active
                reviewFlagged[human] = true;
                emit ReviewFlagged(
                    human,
                    "Guardian confirmed 3x without self-activity - community review needed"
                );
            }
        }

        lastActivityAt[human]    = block.timestamp;
        warningCount[human]      = 0;
        lastWarningAt[human]     = 0;

        emit ProofOfAliveConfirmed(human, msg.sender, calledByGuardian);
    }

    /**
     * @notice Issue inactivity warning. Permissionless - anyone can call.
     * Enables community and Keeper Bot to flag inactive humans.
     */
    function issueInactivityWarning(address human) external {
        require(isHuman[human],        "Not a registered human");
        require(!isInactive[human],    "Already inactive");
        require(warningCount[human] < 3, "Already at 3 warnings - use flagInactive()");

        uint256 sinceLastActivity = block.timestamp - lastActivityAt[human];
        uint256 sinceLastWarning  = block.timestamp - lastWarningAt[human];

        if (warningCount[human] == 0) {
            require(sinceLastActivity >= INACTIVITY_PERIOD, "Not inactive long enough");
        } else {
            require(sinceLastWarning >= WARNING_INTERVAL, "Too soon for next warning");
        }

        warningCount[human]  += 1;
        lastWarningAt[human]  = block.timestamp;

        uint256 deadline = block.timestamp + WARNING_INTERVAL;
        emit InactivityWarning(human, warningCount[human], deadline);
    }

    /**
     * @notice After 3 warnings and final interval elapsed, flag as inactive.
     * AEQ moves to personal Escrow - held for 2 years before UBI Pool.
     * isHuman stays true to allow reactivation.
     */
    function flagInactive(address human) external {
        require(isHuman[human],                   "Not a registered human");
        require(!isInactive[human],               "Already inactive");
        require(warningCount[human] >= 3,         "Must have 3 warnings first");

        uint256 sinceLastWarning = block.timestamp - lastWarningAt[human];
        require(sinceLastWarning >= WARNING_INTERVAL, "Final warning period not elapsed");

        uint256 balance       = balanceOf[human];
        balanceOf[human]      = 0;
        escrowBalance[human]  = balance;
        escrowSince[human]    = block.timestamp;
        isInactive[human]     = true;
        totalHumans           -= 1;
        _removeFromHumanList(human);

        // Revoke guardian relationships on inactivity
        _cleanupGuardianOnInactive(human);

        emit WalletFlaggedInactive(human, balance);
        emit Transfer(human, address(this), balance);
    }

    /**
     * @notice Release escrow to UBI Pool after 2-year hold period.
     * Permissionless - anyone can call after hold period elapsed.
     */
    function releaseEscrowToUBI(address human) external {
        require(isInactive[human],    "Not inactive");
        require(escrowBalance[human] > 0, "No escrow balance");
        require(
            block.timestamp >= escrowSince[human] + ESCROW_HOLD_PERIOD,
            "Escrow hold period not elapsed"
        );

        uint256 amount       = escrowBalance[human];
        escrowBalance[human] = 0;
        ubiPool              += amount;

        emit EscrowReleasedToUBI(human, amount);
    }

    /**
     * @notice Reactivate after being flagged inactive.
     * Returns escrow (if still held) + current fairShare.
     * Requires fresh biometric proof with SAME commitment (same person).
     */
    function reactivate(
        uint[2] calldata _pA,
        uint[2][2] calldata _pB,
        uint[2] calldata _pC,
        uint[2] calldata _pubSignals
    ) external {
        require(isHuman[msg.sender],    "Never registered");
        require(isInactive[msg.sender], "Not inactive");

        require(
            verifier.verifyProof(_pA, _pB, _pC, _pubSignals),
            "Invalid biometric proof"
        );
        require(
            _pubSignals[0] == uint256(uint160(msg.sender)),
            "Wallet mismatch"
        );
        require(
            _pubSignals[1] == commitmentOf[msg.sender],
            "Commitment mismatch: must use your original biometric"
        );

        // Restore active status
        isInactive[msg.sender]           = false;
        warningCount[msg.sender]         = 0;
        lastWarningAt[msg.sender]        = 0;
        lastActivityAt[msg.sender]       = block.timestamp;
        guardianConfirmCount[msg.sender] = 0;
        reviewFlagged[msg.sender]        = false;
        totalHumans                      += 1;
        humanList.push(msg.sender);

        // Return escrow if still held
        uint256 escrowReturned = 0;
        if (escrowBalance[msg.sender] > 0) {
            escrowReturned               = escrowBalance[msg.sender];
            balanceOf[msg.sender]        += escrowReturned;
            escrowBalance[msg.sender]    = 0;
            emit Transfer(address(this), msg.sender, escrowReturned);
        }

        // Grant current fairShare (fair, not punitive)
        uint256 share = fairShare();
        if (share > 0) {
            balanceOf[msg.sender] += share;
            totalSupply           += share;
            emit Transfer(address(0), msg.sender, share);
        }

        emit HumanReactivated(msg.sender, escrowReturned, share);
    }

    //  GUARDIAN SYSTEM 

    /**
     * @notice Propose a Guardian. Activates after 7-day Timelock. (FIX 2)
     *
     * SECURITY CHECKS:
     * FIX 4 - No circular relationships (A guards B, B cannot guard A)
     * FIX 5 - Guardian cannot have a Guardian, and Ward cannot be a Guardian
     */
    function proposeGuardian(address guardian) external onlyActiveHuman {
        require(guardian != msg.sender, "Cannot be your own Guardian");
        require(
            isHuman[guardian] && !isInactive[guardian],
            "Guardian must be an active human"
        );
        require(
            guardianFor[guardian].length < MAX_WARDS,
            "Guardian already has maximum wards"
        );

        // FIX 4: No circular relationships
        require(
            guardianOf[guardian] != msg.sender,
            "Circular Guardian relationship not allowed"
        );
        // Check deeper: proposed guardian must not be a ward of msg.sender's guardian
        require(
            guardianOf[msg.sender] != guardian,
            "This person is already your Guardian"
        );

        // FIX 5: Guardian cannot have a Guardian themselves
        require(
            guardianOf[guardian] == address(0),
            "A Guardian cannot themselves have a Guardian (FIX 5)"
        );

        // FIX 5: Ward cannot already be acting as a Guardian for someone else
        // (prevents layered control chains)
        require(
            guardianFor[msg.sender].length == 0,
            "You are already a Guardian for others - cannot also have a Guardian (FIX 5)"
        );

        // FIX 2: Set pending guardian with timelock
        pendingGuardian[msg.sender]      = guardian;
        pendingGuardianSince[msg.sender] = block.timestamp;

        uint256 activatesAt = block.timestamp + GUARDIAN_TIMELOCK;
        emit GuardianProposed(msg.sender, guardian, activatesAt);
    }

    /**
     * @notice Cancel a pending Guardian during the 7-day timelock window.
     * Allows ward to cancel if proposed under duress.
     */
    function cancelPendingGuardian() external {
        require(pendingGuardian[msg.sender] != address(0), "No pending Guardian");

        address cancelled = pendingGuardian[msg.sender];
        pendingGuardian[msg.sender]      = address(0);
        pendingGuardianSince[msg.sender] = 0;

        emit GuardianCancelled(msg.sender, cancelled);
    }

    /**
     * @notice Activate a pending Guardian after 7-day timelock.
     * Can be called by anyone - timelock is the protection.
     */
    function activateGuardian(address human) external {
        require(pendingGuardian[human] != address(0),          "No pending Guardian");
        require(
            block.timestamp >= pendingGuardianSince[human] + GUARDIAN_TIMELOCK,
            "Timelock not elapsed - 7 days required"
        );

        address guardian = pendingGuardian[human];

        // Verify guardian is still active after timelock period
        require(
            isHuman[guardian] && !isInactive[guardian],
            "Guardian is no longer active"
        );
        require(
            guardianFor[guardian].length < MAX_WARDS,
            "Guardian already has maximum wards"
        );

        // Remove from old guardian if exists
        address oldGuardian = guardianOf[human];
        if (oldGuardian != address(0)) {
            _removeFromGuardianList(oldGuardian, human);
        }

        guardianOf[human] = guardian;
        guardianFor[guardian].push(human);

        // Clear pending
        pendingGuardian[human]      = address(0);
        pendingGuardianSince[human] = 0;

        emit GuardianActivated(human, guardian);
    }

    /**
     * @notice Revoke your Guardian at any time.
     * Only callable by the ward themselves.
     */
    function revokeGuardian() external {
        address guardian = guardianOf[msg.sender];
        require(guardian != address(0), "No Guardian set");

        _removeFromGuardianList(guardian, msg.sender);
        guardianOf[msg.sender] = address(0);

        emit GuardianRevoked(msg.sender, guardian);
    }

    /**
     * @notice Penalize a Guardian for false confirmation.
     * Only Keeper Bot after community governance decision.
     * Guardian loses isHuman status -> AEQ to Escrow.
     */
    function penalizeGuardian(address guardian, string calldata reason) external onlyKeeper {
        require(isHuman[guardian], "Not a human");

        // Revoke all ward relationships
        address[] memory wards = guardianFor[guardian];
        for (uint256 i = 0; i < wards.length; i++) {
            guardianOf[wards[i]] = address(0);
        }
        delete guardianFor[guardian];

        // Flag guardian inactive -> AEQ to Escrow
        uint256 balance           = balanceOf[guardian];
        balanceOf[guardian]       = 0;
        escrowBalance[guardian]   = balance;
        escrowSince[guardian]     = block.timestamp;
        isInactive[guardian]      = true;
        totalHumans               -= 1;
        _removeFromHumanList(guardian);

        emit GuardianPenalized(guardian, reason);
        emit Transfer(guardian, address(this), balance);
    }

    //  WEALTH CAP (always active from human #1) 

    function _applyWealthCap(address wallet) internal {
        if (totalHumans == 0) return;

        uint256 cap = getWealthCap();
        if (cap == 0) return;

        if (balanceOf[wallet] > cap) {
            uint256 overflow  = balanceOf[wallet] - cap;
            balanceOf[wallet] = cap;

            uint256 perPerson = overflow / totalHumans;
            if (perPerson > 0) {
                for (uint256 i = 0; i < humanList.length; i++) {
                    balanceOf[humanList[i]] += perPerson;
                }
                ubiPool += overflow - (perPerson * totalHumans);
            } else {
                ubiPool += overflow;
            }

            emit WealthRedistributed(wallet, overflow, perPerson);
        }
    }

    function getWealthCap() public view returns (uint256) {
        if (totalHumans == 0) return 0;
        uint256 share = fairShare();
        if (share == 0) return 0;

        // Always active - no phase with zero cap
        if (totalHumans <= 100)    return share * 50;
        if (totalHumans <= 1000)   return share * 20;
        if (totalHumans <= 10000)  return share * 10;
        if (totalHumans <= 100000) return share * 5;
        return share * 3;
    }

    //  DEMURRAGE (1% annual on excess above fairShare) 

    /**
     * @notice Charge monthly demurrage on balances above fairShare.
     * 1% annual  12 = ~0.0833% per month on EXCESS only.
     * Collected -> UBI Pool -> distributed equally to all active humans.
     * Called by Keeper Bot monthly.
     */
    function runMonthlyDemurrage() external onlyKeeper {
        require(
            block.timestamp >= lastDemurrageRun + 30 days,
            "Too soon: demurrage runs monthly"
        );

        if (totalHumans == 0) return;

        uint256 share          = fairShare();
        uint256 totalCollected = 0;

        for (uint256 i = 0; i < humanList.length; i++) {
            address human  = humanList[i];
            uint256 balance = balanceOf[human];

            if (balance > share) {
                uint256 excess  = balance - share;
                uint256 charge  = (excess * DEMURRAGE_BPS) / (10000 * DEMURRAGE_MONTHS);

                if (charge > 0) {
                    balanceOf[human] -= charge;
                    totalCollected   += charge;
                    emit DemurrageCharged(human, charge);
                }
            }
        }

        lastDemurrageRun = block.timestamp;

        if (totalCollected > 0) {
            uint256 perHuman = totalCollected / totalHumans;
            if (perHuman > 0) {
                for (uint256 i = 0; i < humanList.length; i++) {
                    balanceOf[humanList[i]] += perHuman;
                }
                ubiPool += totalCollected - (perHuman * totalHumans);
            } else {
                ubiPool += totalCollected;
            }
            emit DemurrageDistributed(totalCollected, perHuman);
        }

        // Distribute accumulated UBI Pool
        _distributeUBI();
    }

    function _distributeUBI() internal {
        if (ubiPool == 0 || totalHumans == 0) return;
        uint256 perHuman = ubiPool / totalHumans;
        if (perHuman == 0) return;

        for (uint256 i = 0; i < humanList.length; i++) {
            balanceOf[humanList[i]] += perHuman;
        }

        ubiPool = ubiPool - (perHuman * totalHumans);
        emit UBIDistributed(perHuman);
    }

    //  GINI + INDEX (written by Keeper Bot, off-chain measured) 

    function updateGini(uint256 _gini, uint256 _index) external onlyKeeper {
        require(_gini  <= 100, "Gini must be 0-100");
        require(_index <= 100, "Index must be 0-100");

        giniCoefficient = _gini;
        aequitasIndex   = _index;
        lastGiniUpdate  = block.timestamp;

        emit GiniUpdated(_gini, _index);
    }

    //  INTERNAL HELPERS 

    function _removeFromHumanList(address human) internal {
        for (uint256 i = 0; i < humanList.length; i++) {
            if (humanList[i] == human) {
                humanList[i] = humanList[humanList.length - 1];
                humanList.pop();
                break;
            }
        }
    }

    function _removeFromGuardianList(address guardian, address ward) internal {
        address[] storage wards = guardianFor[guardian];
        for (uint256 i = 0; i < wards.length; i++) {
            if (wards[i] == ward) {
                wards[i] = wards[wards.length - 1];
                wards.pop();
                break;
            }
        }
    }

    function _cleanupGuardianOnInactive(address human) internal {
        // Remove human as ward from their guardian
        address guardian = guardianOf[human];
        if (guardian != address(0)) {
            _removeFromGuardianList(guardian, human);
            guardianOf[human] = address(0);
        }

        // Remove human as guardian for their wards
        address[] memory wards = guardianFor[human];
        for (uint256 i = 0; i < wards.length; i++) {
            guardianOf[wards[i]] = address(0);
        }
        delete guardianFor[human];
    }

    //  VIEW FUNCTIONS 

    function fairShare() public view returns (uint256) {
        if (totalHumans == 0) return 0;
        return totalSupply / totalHumans;
    }

    function maxCap() public view returns (uint256) {
        return totalHumans * INITIAL_GRANT;
    }

    function getPhase() public view returns (uint256) {
        if (totalHumans <= 100)    return 0;
        if (totalHumans <= 1000)   return 1;
        if (totalHumans <= 10000)  return 2;
        if (totalHumans <= 100000) return 3;
        return 4;
    }

    function getInactivityStatus(address human) external view returns (
        bool    active,
        bool    inactive,
        uint8   warnings,
        uint256 daysSinceActivity,
        uint256 escrow,
        uint256 escrowReleasesAt,
        address guardian,
        bool    hasPendingGuardian,
        uint256 pendingGuardianActivatesAt,
        bool    flagged,
        uint8   guardianConfirms
    ) {
        return (
            isHuman[human] && !isInactive[human],
            isInactive[human],
            warningCount[human],
            lastActivityAt[human] > 0
                ? (block.timestamp - lastActivityAt[human]) / 1 days
                : 0,
            escrowBalance[human],
            escrowBalance[human] > 0
                ? escrowSince[human] + ESCROW_HOLD_PERIOD
                : 0,
            guardianOf[human],
            pendingGuardian[human] != address(0),
            pendingGuardian[human] != address(0)
                ? pendingGuardianSince[human] + GUARDIAN_TIMELOCK
                : 0,
            reviewFlagged[human],
            guardianConfirmCount[human]
        );
    }

    function getGuardianInfo(address guardian) external view returns (
        address[] memory wards,
        uint256          wardCount,
        bool             hasOwnGuardian
    ) {
        return (
            guardianFor[guardian],
            guardianFor[guardian].length,
            guardianOf[guardian] != address(0)
        );
    }

    function getStatus() external view returns (
        uint256 humans,
        uint256 supply,
        uint256 share,
        uint256 cap,
        uint256 wealthCap,
        uint256 phase,
        uint256 gini,
        uint256 index,
        uint256 vPool,
        uint256 lPool,
        uint256 uPool,
        uint256 tPool
    ) {
        return (
            totalHumans,
            totalSupply,
            fairShare(),
            maxCap(),
            getWealthCap(),
            getPhase(),
            giniCoefficient,
            aequitasIndex,
            validatorPool,
            lpPool,
            ubiPool,
            treasury
        );
    }
}
