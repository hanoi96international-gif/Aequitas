// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/**
 * @title  AequitasV8
 * @notice "Money exists because people exist."
 *
 * V8 replaces AequitasV7 when the chain restarts at height zero. It has exactly
 * two jobs and holds no economic logic of its own:
 *
 *   (a) REGISTER OF HUMANS — registerWithSig() checks a Groth16 proof through
 *       the verifier from the trusted-setup ceremony, binds it to the wallet
 *       that becomes human with that wallet's own EIP-712 signature, and
 *       records every nullifier and every commitment exactly once.
 *       No starting balance is created here; the Go keeper books it.
 *
 *   (b) ERC-20 FACADE for wallets and explorers — name/symbol/decimals are
 *       constants, totalSupply/balanceOf are plain storage the Go keeper
 *       mirrors. transfer() is intercepted by the node (selector 0xa9059cbb,
 *       evm_rpc.go) and booked in Go; if it ever reaches this bytecode it
 *       reverts, because this contract does not move value.
 *
 * Universal basic income, demurrage, wealth cap, escrow, guardians and fees
 * live ONLY in Go. There is no owner, no admin function, no upgrade path.
 *
 * ─── STORAGE LAYOUT (fixed; see contracts/v8_slots.json) ────────────────────
 *
 * The Go keeper reads and writes these slots directly. The order below is part
 * of the chain's consensus interface. test/AequitasV8_storage_layout.ts
 * compares the compiler's storageLayout output with contracts/v8_slots.json
 * entry by entry, so a moved, added, removed or retyped variable turns the
 * build red instead of silently making Go write to the wrong place.
 * NEVER insert, remove, reorder or retype a state variable without updating
 * v8_slots.json AND the Go constants generated from it.
 *
 *   slot  variable          type                          written by
 *   ----  ----------------  ----------------------------  -------------------
 *     0   totalSupply       uint256                       Go mirror only
 *     1   totalHumans       uint256                       contract (+ Go migration)
 *     2   balanceOf         mapping(address => uint256)   Go mirror only
 *     3   isHuman           mapping(address => bool)      contract (+ Go mirror)
 *     4   usedCommitments   mapping(uint256 => bool)      contract
 *     5   usedNullifiers    mapping(bytes32 => address)   contract
 *     6   commitmentOf      mapping(address => uint256)   contract
 *     7   nullifierOf       mapping(address => bytes32)   contract
 *     8   nonces            mapping(address => uint256)   contract
 *     9   isRegistrar       mapping(address => bool)      constructor only
 *
 *   Mapping value location = keccak256(pad32(key) . pad32(slot)), i.e. Go's
 *   mappingSlot(addr.Bytes(), slot) / mappingSlotBytes32(key, slot).
 *   `verifier` is immutable and occupies no storage slot.
 *
 * ─── WHY THE USER SIGNS (EIP-712) AND WHY A REGISTRAR MUST SUBMIT ───────────
 *
 * The v3 circuit's public signals are [commitment, nullifier]. The wallet is
 * a PRIVATE input hidden inside commitment = Poseidon(bio, wallet, salt), so
 * the proof alone does not say which wallet it is for. Two independent checks
 * close that here:
 *
 *   1. The human's own EIP-712 signature over
 *        Register(human, commitment, nullifier, nonce, deadline)
 *      in the domain (name "Aequitas", version "8", chainId, this contract).
 *      Nobody can register a wallet whose key they do not hold, and nobody
 *      can move a pending registration to another wallet (front-running),
 *      replay it on another chain/contract, or reuse it after the nonce moved
 *      or the deadline passed.
 *   2. msg.sender must be a registrar fixed at deployment (genesis). Groth16
 *      proving keys are public, so a valid proof on its own is not evidence
 *      of a human: anyone can prove a made-up biometric. Whether the proof
 *      came out of the node's biometric /prove path is decided by the node
 *      (prove_provenance.go). Only the node's relayer can therefore write
 *      the register. This also keeps the register and the Go ledger on one
 *      path (the same rule checkPersistedCallAllowed enforces in Go).
 *
 * Residual gap (documented in docs/V8_ENTWURF.md): a party that obtains
 * someone else's proof before it is used can sign it for its OWN wallet.
 * Closing that fully needs circuit v4 with the wallet as a public signal.
 */

interface IGroth16Verifier {
    function verifyProof(
        uint256[2] calldata pA,
        uint256[2][2] calldata pB,
        uint256[2] calldata pC,
        uint256[2] calldata pubSignals
    ) external view returns (bool);
}

contract AequitasV8 {
    // ─── Constants (no storage) ─────────────────────────────────────────────

    string  public constant name     = "Aequitas";
    string  public constant symbol   = "AEQ";
    uint8   public constant decimals = 18;

    /// EIP-712 domain version of this contract.
    string  public constant VERSION  = "8";

    /// BN254 scalar field. Public signals are field elements; a value >= r
    /// would alias another one (x and x + r are the same input to the
    /// pairing), so uniqueness keyed on the raw uint256 requires x < r.
    uint256 public constant SNARK_SCALAR_FIELD =
        21888242871839275222246405745257275088548364400416034343698204186575808495617;

    /// Upper bound on how far in the future a signature's deadline may lie.
    /// Every wait has a fixed limit (AGENTS.md); a signature that stays valid
    /// forever is a standing replay risk.
    uint256 public constant MAX_SIGNATURE_LIFETIME = 1 days;

    /// Upper bound on the registrar list passed to the constructor.
    uint256 public constant MAX_REGISTRARS = 16;

    /// secp256k1n / 2 — signatures with a higher s are malleable (EIP-2).
    uint256 private constant SECP256K1N_HALF =
        0x7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0;

    bytes32 public constant EIP712_DOMAIN_TYPEHASH = keccak256(
        "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
    );

    bytes32 public constant REGISTER_TYPEHASH = keccak256(
        "Register(address human,uint256 commitment,uint256 nullifier,uint256 nonce,uint256 deadline)"
    );

    bytes32 private constant NAME_HASH    = keccak256(bytes("Aequitas"));
    bytes32 private constant VERSION_HASH = keccak256(bytes("8"));

    // ─── Immutable (in bytecode, no storage) ────────────────────────────────

    /// Groth16 verifier from the trusted-setup ceremony. A new ceremony key
    /// means a new verifier contract and therefore a new V8 deployment,
    /// which is a consensus change anyway.
    IGroth16Verifier public immutable verifier;

    // ─── Storage — ORDER IS CONSENSUS, see header and v8_slots.json ─────────

    /* slot 0 */ uint256 public totalSupply;
    /* slot 1 */ uint256 public totalHumans;
    /* slot 2 */ mapping(address => uint256) public balanceOf;
    /* slot 3 */ mapping(address => bool)    public isHuman;
    /* slot 4 */ mapping(uint256 => bool)    public usedCommitments;
    /* slot 5 */ mapping(bytes32 => address) public usedNullifiers;
    /* slot 6 */ mapping(address => uint256) public commitmentOf;
    /* slot 7 */ mapping(address => bytes32) public nullifierOf;
    /* slot 8 */ mapping(address => uint256) public nonces;
    /* slot 9 */ mapping(address => bool)    public isRegistrar;

    // ─── Events ─────────────────────────────────────────────────────────────

    /// A wallet became human. No amount: the starting balance is booked in Go.
    event Registered(address indexed human, uint256 commitment, bytes32 indexed nullifier);

    /// Standard ERC-20 event, declared for the ABI that wallets and explorers
    /// read. Value moves in Go, so this contract never emits it itself.
    event Transfer(address indexed from, address indexed to, uint256 value);

    // ─── Constructor ────────────────────────────────────────────────────────

    /// @param verifier_   Groth16 verifier (BioVerifier from the ceremony).
    /// @param registrars_ Relayer addresses of the validators, fixed in
    ///                    genesis. 1..MAX_REGISTRARS, non-zero, no duplicates.
    constructor(address verifier_, address[] memory registrars_) {
        require(verifier_ != address(0), "V8: verifier is zero");
        require(verifier_.code.length > 0, "V8: verifier has no code");
        require(
            registrars_.length > 0 && registrars_.length <= MAX_REGISTRARS,
            "V8: registrar count"
        );
        verifier = IGroth16Verifier(verifier_);
        for (uint256 i = 0; i < registrars_.length; i++) {
            address r = registrars_[i];
            require(r != address(0), "V8: registrar is zero");
            require(!isRegistrar[r], "V8: duplicate registrar");
            isRegistrar[r] = true;
        }
    }

    // ─── (a) Register of humans ─────────────────────────────────────────────

    /**
     * @notice Registers `human` as a verified person.
     * @param pA,pB,pC   Groth16 proof.
     * @param pubSignals [commitment, nullifier] from circuit v3.
     * @param human      The wallet that becomes human; it signed the request.
     * @param deadline   Unix time after which the signature is void.
     * @param signature  65-byte (r, s, v) EIP-712 signature by `human` over
     *                   Register(human, commitment, nullifier, nonces[human], deadline).
     *
     * Callable only by a registrar (see header). Selector:
     * registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,uint256,bytes)
     */
    function registerWithSig(
        uint256[2] calldata pA,
        uint256[2][2] calldata pB,
        uint256[2] calldata pC,
        uint256[2] calldata pubSignals,
        address human,
        uint256 deadline,
        bytes calldata signature
    ) external {
        // Who may write the register.
        require(isRegistrar[msg.sender], "V8: caller is not a registrar");

        // Who is being registered.
        require(human != address(0), "V8: human is zero");
        require(!isRegistrar[human], "V8: registrar cannot be human");
        require(!isHuman[human], "V8: already registered");

        // When — bounded validity window.
        require(block.timestamp <= deadline, "V8: signature expired");
        require(deadline <= block.timestamp + MAX_SIGNATURE_LIFETIME, "V8: deadline too far");

        // What — the proof's public signals, each once.
        uint256 commitment = pubSignals[0];
        uint256 nullifierU = pubSignals[1];
        require(commitment != 0 && nullifierU != 0, "V8: zero public signal");
        require(
            commitment < SNARK_SCALAR_FIELD && nullifierU < SNARK_SCALAR_FIELD,
            "V8: public signal out of field"
        );
        bytes32 nullifier = bytes32(nullifierU);
        require(!usedCommitments[commitment], "V8: commitment used");
        require(usedNullifiers[nullifier] == address(0), "V8: nullifier used");

        // The human's consent, bound to everything above.
        uint256 nonce = nonces[human];
        bytes32 digest = _registerDigest(human, commitment, nullifierU, nonce, deadline);
        require(_recover(digest, signature) == human, "V8: invalid signature");

        // The proof. verifyProof is `view`, so this is a STATICCALL: the
        // verifier cannot re-enter or change state. Checked before any write.
        require(verifier.verifyProof(pA, pB, pC, pubSignals), "V8: invalid proof");

        // Effects.
        nonces[human] = nonce + 1;
        usedCommitments[commitment] = true;
        usedNullifiers[nullifier] = human;
        commitmentOf[human] = commitment;
        nullifierOf[human] = nullifier;
        isHuman[human] = true;
        totalHumans += 1;

        emit Registered(human, commitment, nullifier);
    }

    /// EIP-712 domain separator. Computed on every call from address(this)
    /// and block.chainid, never cached: the node stores the runtime code
    /// under the genesis address after deploying it elsewhere
    /// (contract_deploy.go), so a separator cached in the constructor would
    /// name the wrong contract.
    function DOMAIN_SEPARATOR() public view returns (bytes32) {
        return keccak256(abi.encode(
            EIP712_DOMAIN_TYPEHASH, NAME_HASH, VERSION_HASH, block.chainid, address(this)
        ));
    }

    /// The digest `human` has to sign for a registration with the CURRENT
    /// nonce. For the app and for Go to cross-check their own encoding.
    function registrationDigest(address human, uint256 commitment, uint256 nullifier, uint256 deadline)
        external view returns (bytes32)
    {
        return _registerDigest(human, commitment, nullifier, nonces[human], deadline);
    }

    function _registerDigest(address human, uint256 commitment, uint256 nullifier, uint256 nonce, uint256 deadline)
        internal view returns (bytes32)
    {
        bytes32 structHash = keccak256(abi.encode(
            REGISTER_TYPEHASH, human, commitment, nullifier, nonce, deadline
        ));
        return keccak256(abi.encodePacked("\x19\x01", DOMAIN_SEPARATOR(), structHash));
    }

    /// Strict ECDSA recovery: 65 bytes, v in {27, 28}, low s, non-zero result.
    function _recover(bytes32 digest, bytes calldata signature) internal pure returns (address) {
        require(signature.length == 65, "V8: bad signature length");
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }
        require(v == 27 || v == 28, "V8: bad signature v");
        require(uint256(s) <= SECP256K1N_HALF, "V8: high s");
        address signer = ecrecover(digest, v, r, s);
        require(signer != address(0), "V8: invalid signature");
        return signer;
    }

    // ─── (b) ERC-20 facade ──────────────────────────────────────────────────

    /// ERC-20 transfer selector 0xa9059cbb. The node intercepts this call for
    /// this contract's address before the EVM runs and books it in Go
    /// (TransferWithV7FeeAtomic in evm_rpc.go). Reaching this bytecode means
    /// the interception was bypassed; fail closed rather than pretend value
    /// moved on a ledger that is not the real one.
    function transfer(address, uint256) external pure returns (bool) {
        revert("V8: transfer is booked by the chain");
    }
}
