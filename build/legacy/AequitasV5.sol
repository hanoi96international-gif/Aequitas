// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

interface IBioVerifier {
    function verifyProof(
        uint[2] calldata _pA,
        uint[2][2] calldata _pB,
        uint[2] calldata _pC,
        uint[2] calldata _pubSignals
    ) external view returns (bool);
}

/**
 * @title AequitasV5
 * @notice Decentralized human-centric monetary system
 * @dev Money supply = verified humans x 1,000 AEQ
 *
 * Key features:
 * - 1,000 AEQ initial grant per verified human (immutable)
 * - 0.1% transaction fee (40% validators, 30% LPs, 20% UBI, 10% treasury)
 * - Algorithmic inflation 0-1.5% based on on-chain data only
 * - Dynamic wealth cap with waterfall redistribution
 * - No deflation - overflow redistributed instead
 * - Phased activation based on registration count
 */
contract AequitasV5 {

    // ─── ERC-20 ──────────────────────────────────────────────────────────────────
    string public constant name = "Aequitas";
    string public constant symbol = "AEQ";
    uint8 public constant decimals = 18;

    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    // ─── CORE STATE ──────────────────────────────────────────────────────────────
    IBioVerifier public verifier;
    mapping(uint256 => bool) public usedCommitments;
    address[] public humanList;
    mapping(address => bool) public isHuman;
    uint256 public totalSupply;
    uint256 public totalHumans;

    // ─── IMMUTABLE CONSTANTS ─────────────────────────────────────────────────────
    uint256 public constant INITIAL_GRANT = 1000 * 10**18;
    uint256 public constant FEE_BPS = 10; // 0.1% = 10 basis points
    uint256 public constant MAX_INFLATION_BPS = 150; // 1.5% max annual inflation

    // ─── FEE POOLS ───────────────────────────────────────────────────────────────
    uint256 public validatorPool;    // 40% of fees
    uint256 public lpPool;           // 30% of fees
    uint256 public ubiPool;          // 20% of fees
    uint256 public treasury;         // 10% of fees

    // ─── AEQUITAS INDEX ──────────────────────────────────────────────────────────
    uint256 public lastVelocity;
    uint256 public lastGrowth;
    uint256 public lastGini;
    uint256 public aequitasIndex;
    uint256 public lastCycleTime;
    uint256 public transferVolume30Days;
    uint256 public activeWallets30Days;
    uint256 public lastInflationTime;

    // ─── EVENTS ──────────────────────────────────────────────────────────────────
    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    event HumanRegistered(address wallet, uint256 totalHumans);
    event Inflation(uint256 amountPerWallet, uint256 inflationBps, uint256 index);
    event WealthRedistributed(address indexed wallet, uint256 overflow, uint256 perPerson);
    event FeeCollected(uint256 validatorShare, uint256 lpShare, uint256 ubiShare, uint256 treasuryShare);
    event UBIDistributed(uint256 amountPerHuman);
    event CycleRun(uint256 velocity, uint256 growth, uint256 gini, uint256 index);

    constructor(address _verifier) {
        verifier = IBioVerifier(_verifier);
        lastInflationTime = block.timestamp;
        lastCycleTime = block.timestamp;
    }

    // ─── ERC-20 WITH FEE ─────────────────────────────────────────────────────────

    function transfer(address to, uint256 amount) external returns (bool) {
        _transferWithFee(msg.sender, to, amount);
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
        return true;
    }

    function _transferWithFee(address from, address to, uint256 amount) internal {
        require(balanceOf[from] >= amount, "Insufficient balance");

        // Calculate 0.1% fee
        uint256 fee = (amount * FEE_BPS) / 10000;
        uint256 amountAfterFee = amount - fee;

        // Distribute fee
        if (fee > 0) {
            uint256 toValidators = (fee * 40) / 100;
            uint256 toLPs       = (fee * 30) / 100;
            uint256 toUBI       = (fee * 20) / 100;
            uint256 toTreasury  = fee - toValidators - toLPs - toUBI;

            validatorPool += toValidators;
            lpPool        += toLPs;
            ubiPool       += toUBI;
            treasury      += toTreasury;

            balanceOf[from] -= fee;
            emit FeeCollected(toValidators, toLPs, toUBI, toTreasury);
        }

        // Transfer net amount
        balanceOf[from] -= amountAfterFee;
        balanceOf[to]   += amountAfterFee;

        // Track volume for inflation calculation
        transferVolume30Days += amount;

        emit Transfer(from, to, amountAfterFee);

        // Check wealth cap after transfer
        _applyWealthCap(to);
    }

    // ─── REGISTRATION ────────────────────────────────────────────────────────────

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
            "Wallet mismatch"
        );

        uint256 commitment = _pubSignals[1];
        require(!usedCommitments[commitment], "Commitment already used");
        require(!isHuman[msg.sender], "Already registered");

        usedCommitments[commitment] = true;
        isHuman[msg.sender] = true;
        totalHumans += 1;
        humanList.push(msg.sender);
        balanceOf[msg.sender] = INITIAL_GRANT;
        totalSupply += INITIAL_GRANT;

        emit Transfer(address(0), msg.sender, INITIAL_GRANT);
        emit HumanRegistered(msg.sender, totalHumans);
    }

    // ─── WEALTH CAP + WATERFALL REDISTRIBUTION ───────────────────────────────────

    function _applyWealthCap(address wallet) internal {
        if (totalHumans == 0) return;

        uint256 cap = getWealthCap();
        if (cap == 0) return; // Phase 0 - no cap

        if (balanceOf[wallet] > cap) {
            uint256 overflow = balanceOf[wallet] - cap;
            balanceOf[wallet] = cap;

            // Redistribute overflow equally to all humans
            uint256 perPerson = overflow / totalHumans;
            if (perPerson > 0) {
                for (uint256 i = 0; i < humanList.length; i++) {
                    balanceOf[humanList[i]] += perPerson;
                }
                // Remainder to UBI pool
                ubiPool += overflow - (perPerson * totalHumans);
            } else {
                ubiPool += overflow;
            }

            emit WealthRedistributed(wallet, overflow, perPerson);
        }
    }

    function getWealthCap() public view returns (uint256) {
        if (totalHumans == 0) return 0;

        uint256 fairShare = totalSupply / totalHumans;

        // Phase 0: 0-100 registrations - no cap
        if (totalHumans <= 100) return 0;

        // Phase 1: 100-1,000 - cap at 20x fairShare
        if (totalHumans <= 1000) return fairShare * 20;

        // Phase 2: 1,000-10,000 - cap at 10x fairShare
        if (totalHumans <= 10000) return fairShare * 10;

        // Phase 3: 10,000-100,000 - cap at 5x fairShare
        if (totalHumans <= 100000) return fairShare * 5;

        // Phase 4: 100,000+ - Gini-dynamic cap between 3x and 5x
        uint256 gini = calculateGini();
        if (gini <= 25) return fairShare * 5;
        if (gini <= 50) return fairShare * 4;
        return fairShare * 3;
    }

    // ─── GINI COEFFICIENT ────────────────────────────────────────────────────────

    function calculateGini() public view returns (uint256) {
        uint256 n = humanList.length;
        if (n <= 1) return 0;

        // Limit calculation to first 500 humans for gas efficiency
        uint256 limit = n > 500 ? 500 : n;
        uint256 sumDiff = 0;

        for (uint256 i = 0; i < limit; i++) {
            for (uint256 j = 0; j < limit; j++) {
                uint256 a = balanceOf[humanList[i]];
                uint256 b = balanceOf[humanList[j]];
                sumDiff += a > b ? a - b : b - a;
            }
        }

        uint256 mean = totalSupply / n;
        if (mean == 0) return 0;
        return (sumDiff * 100) / (2 * limit * limit * mean);
    }

    // ─── ALGORITHMIC INFLATION (0 - 1.5% annual) ─────────────────────────────────

    function calculateInflationBps() public view returns (uint256) {
        if (totalHumans < 10000) return 0; // Only active from 10,000 humans

        uint256 n = totalHumans;

        // Velocity: transfer volume / total supply (0-100 scale)
        uint256 velocity = totalSupply > 0
            ? (transferVolume30Days * 100) / totalSupply
            : 0;
        if (velocity > 100) velocity = 100;

        // Active ratio: active wallets / total humans (0-100 scale)
        uint256 activeRatio = n > 0
            ? (activeWallets30Days * 100) / n
            : 0;
        if (activeRatio > 100) activeRatio = 100;

        // Gini (0-100)
        uint256 gini = calculateGini();

        // Growth: assume passed via cycle
        uint256 growth = lastGrowth > 100 ? 100 : lastGrowth;

        // Inflation increases when economy is weak
        // Low velocity, low activity, high Gini, low growth = more inflation
        int256 score = 0;
        score += int256(50) - int256(velocity);      // Low velocity → +inflation
        score += int256(50) - int256(activeRatio);   // Low activity → +inflation
        score += int256(gini) - int256(25);           // High Gini → +inflation
        score += int256(10) - int256(growth);         // Low growth → +inflation

        if (score <= 0) return 0;

        // Map score to 0-150 bps (0-1.5%)
        uint256 inflationBps = uint256(score) * 150 / 200;
        if (inflationBps > MAX_INFLATION_BPS) inflationBps = MAX_INFLATION_BPS;

        return inflationBps;
    }

    // ─── CYCLE: MONETARY POLICY ──────────────────────────────────────────────────

    function runCycle(uint256 velocity, uint256 growth) external {
        require(humanList.length > 0, "No users");

        uint256 gini = calculateGini();
        uint256 giniScore = gini <= 100 ? 100 - gini : 0;
        uint256 index = (velocity * 40 + growth * 35 + giniScore * 25) / 100;

        lastVelocity = velocity;
        lastGrowth = growth;
        lastGini = gini;
        aequitasIndex = index;
        lastCycleTime = block.timestamp;

        emit CycleRun(velocity, growth, gini, index);

        // Apply inflation if enough time has passed (monthly)
        _applyInflation();

        // Distribute UBI pool
        _distributeUBI();

        // Reset 30-day counters
        transferVolume30Days = 0;
        activeWallets30Days = 0;
    }

    function _applyInflation() internal {
        // Only once per 30 days
        if (block.timestamp < lastInflationTime + 30 days) return;

        uint256 inflationBps = calculateInflationBps();
        if (inflationBps == 0) return;

        uint256 inflationAmount = (totalSupply * inflationBps) / 120000; // Annual/12 months
        if (inflationAmount == 0) return;

        uint256 perHuman = inflationAmount / humanList.length;
        if (perHuman == 0) return;

        for (uint256 i = 0; i < humanList.length; i++) {
            balanceOf[humanList[i]] += perHuman;
        }

        totalSupply += perHuman * humanList.length;
        lastInflationTime = block.timestamp;

        emit Inflation(perHuman, inflationBps, aequitasIndex);
    }

    function _distributeUBI() internal {
        if (ubiPool == 0 || humanList.length == 0) return;

        uint256 perHuman = ubiPool / humanList.length;
        if (perHuman == 0) return;

        for (uint256 i = 0; i < humanList.length; i++) {
            balanceOf[humanList[i]] += perHuman;
        }

        ubiPool = ubiPool - (perHuman * humanList.length);
        emit UBIDistributed(perHuman);
    }

    // ─── VIEW FUNCTIONS ──────────────────────────────────────────────────────────

    function maxCap() public view returns (uint256) {
        return totalHumans * INITIAL_GRANT;
    }

    function fairShare() public view returns (uint256) {
        if (totalHumans == 0) return 0;
        return totalSupply / totalHumans;
    }

    function getStatus() external view returns (
        uint256 humans,
        uint256 supply,
        uint256 cap,
        uint256 gini,
        uint256 index,
        uint256 wealthCap,
        uint256 inflationBps,
        uint256 vPool,
        uint256 lPool,
        uint256 uPool,
        uint256 tPool
    ) {
        return (
            totalHumans,
            totalSupply,
            maxCap(),
            calculateGini(),
            aequitasIndex,
            getWealthCap(),
            calculateInflationBps(),
            validatorPool,
            lpPool,
            ubiPool,
            treasury
        );
    }

    function getPhase() public view returns (uint256) {
        if (totalHumans <= 100) return 0;
        if (totalHumans <= 1000) return 1;
        if (totalHumans <= 10000) return 2;
        if (totalHumans <= 100000) return 3;
        return 4;
    }
}
