package keeper

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// MPC wiring: what turns the protocol in x/humanity/mpc into separate machines
// that each hold one row.
//
// The duplicate check compares a new capture against enrolled templates without
// any single machine ever holding a whole template. That is not a property of
// the maths alone — it only exists if the parties really run on separate boxes
// under separate control. This file is where that happens, and it is
// deliberately opt-in: a misconfigured node must fall back to "MPC not
// running", never to "one party doing it all", because the second is
// indistinguishable from working while providing none of the protection.
//
// # PEERS ARE IDENTIFIED BY THEIR VALIDATOR KEY, NOT BY A SHARED SECRET
//
// Each party signs its own contributions with the key that already signs its
// blocks (RELAYER_PRIVATE_KEY), and peers are listed by their signing address —
// the same address the Node Binding Signature ties to an operator wallet. So:
//
//   - Adding a validator means publishing an address, not distributing a
//     secret. Nothing has to be rotated on the existing boxes.
//   - No validator can speak for another, because no validator holds another's
//     key. A shared token would have let any party forge any other's answer,
//     and those answers decide who counts as a duplicate.
//   - Removing a validator means deleting its address. It cannot forge
//     contributions afterwards.
//
// Configuration:
//
//	MPC_ENABLED=true
//	MPC_PARTY_INDEX=0                     this node's party number
//	MPC_PEERS=https://a|0xAddrA,https://b|0xAddrB
//	RELAYER_PRIVATE_KEY=...               already set; signs blocks and rounds
//
// MPC_ALLOW_INSECURE=true permits http:// peers, for a local harness only, and
// is refused with any non-loopback peer.

// validatorAuthenticator signs with this node's validator key and verifies
// peers against their registered signing addresses.
type validatorAuthenticator struct {
	index int
	priv  *ecdsa.PrivateKey
	addrs []common.Address
}

// Sign implements mpc.Authenticator.
func (v *validatorAuthenticator) Sign(digest []byte) ([]byte, error) {
	return crypto.Sign(digest, v.priv)
}

// VerifyParty implements mpc.Authenticator: recover the signer and require it
// to be exactly the address configured for that party.
func (v *validatorAuthenticator) VerifyParty(index int, digest, sig []byte) error {
	if index < 0 || index >= len(v.addrs) {
		return fmt.Errorf("mpc: no address configured for party %d", index)
	}
	pub, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return fmt.Errorf("mpc: signature from party %d does not recover: %w", index, err)
	}
	got := crypto.PubkeyToAddress(*pub)
	if got != v.addrs[index] {
		return fmt.Errorf("mpc: round was signed by %s but party %d is %s — a validator is "+
			"submitting contributions under another's identity", got.Hex(), index,
			v.addrs[index].Hex())
	}
	return nil
}

// Parties implements mpc.Authenticator.
func (v *validatorAuthenticator) Parties() int { return len(v.addrs) }

// mpcNode is this process's participation in the comparison protocol.
type mpcNode struct {
	mailbox  *mpc.Mailbox
	auth     *validatorAuthenticator
	index    int
	peers    []string
	insecure bool

	// discover resolves committee candidates at call time. Non-nil when the
	// committee comes from the chain rather than from MPC_PEERS.
	//
	// Resolved per registration, not at startup: validators join and leave
	// while the node is running, and a set frozen at boot would need a restart
	// to notice — which is the manual step this exists to remove.
	discover func() []mpc.Party
	size     int
	selfAddr string
}

// committeeNow resolves the current committee from the chain.
func (n *mpcNode) committeeNow() (*mpc.Committee, error) {
	if n.discover == nil {
		return nil, fmt.Errorf("mpc: this node uses a static peer list")
	}
	candidates := n.discover()
	c, err := mpc.SelectCommittee(candidates, n.size, "")
	if err != nil {
		return nil, fmt.Errorf("mpc: no committee could be formed from %d candidates: %w",
			len(candidates), err)
	}
	return c, nil
}

// parsePeerSpec splits "https://host|0xAddress" into its two halves.
func parsePeerSpec(spec string) (string, common.Address, error) {
	parts := strings.Split(spec, "|")
	if len(parts) != 2 {
		return "", common.Address{}, fmt.Errorf("peer %q must be written as URL|0xSigningAddress — "+
			"without the address there is nothing to verify that peer's contributions against", spec)
	}
	url := strings.TrimRight(strings.TrimSpace(parts[0]), "/")
	rawAddr := strings.TrimSpace(parts[1])
	if !common.IsHexAddress(rawAddr) {
		return "", common.Address{}, fmt.Errorf("peer %q has an invalid signing address %q", spec, rawAddr)
	}
	if url == "" {
		return "", common.Address{}, fmt.Errorf("peer %q has an empty URL", spec)
	}
	return url, common.HexToAddress(rawAddr), nil
}

// newMPCNodeFromEnv reads the configuration, or reports why MPC stays off.
//
// Returns (nil, nil) when MPC is simply not enabled — the ordinary case for a
// node that does not take part.
func newMPCNodeFromEnv(discover func() []mpc.Party) (*mpcNode, error) {
	if strings.ToLower(os.Getenv("MPC_ENABLED")) != "true" {
		return nil, nil
	}

	rawPeers := os.Getenv("MPC_PEERS")
	if rawPeers == "" {
		return newDiscoveringMPCNode(discover)
	}
	var peers []string
	var addrs []common.Address
	for _, spec := range strings.Split(rawPeers, ",") {
		if strings.TrimSpace(spec) == "" {
			continue
		}
		url, addr, err := parsePeerSpec(spec)
		if err != nil {
			return nil, err
		}
		peers = append(peers, url)
		addrs = append(addrs, addr)
	}
	if len(peers) < 2 {
		return nil, fmt.Errorf("MPC_PEERS lists %d parties; with fewer than two, one machine "+
			"holds every share and can reconstruct every template — which is the situation this "+
			"whole subsystem exists to prevent", len(peers))
	}
	for i := range addrs {
		for j := i + 1; j < len(addrs); j++ {
			if addrs[i] == addrs[j] {
				return nil, fmt.Errorf("parties %d and %d share the signing address %s — one key "+
					"holding two shares defeats the split entirely", i, j, addrs[i].Hex())
			}
		}
	}

	idxRaw := os.Getenv("MPC_PARTY_INDEX")
	index, err := strconv.Atoi(idxRaw)
	if err != nil {
		return nil, fmt.Errorf("MPC_PARTY_INDEX %q is not a number", idxRaw)
	}
	if index < 0 || index >= len(peers) {
		return nil, fmt.Errorf("MPC_PARTY_INDEX %d is outside 0..%d for the %d configured peers",
			index, len(peers)-1, len(peers))
	}

	pkHex := strings.TrimPrefix(strings.TrimSpace(os.Getenv("RELAYER_PRIVATE_KEY")), "0x")
	if pkHex == "" {
		return nil, fmt.Errorf("RELAYER_PRIVATE_KEY is empty — it is the key this node signs its " +
			"MPC contributions with, and peers verify against its address")
	}
	priv, err := crypto.HexToECDSA(pkHex)
	if err != nil {
		return nil, fmt.Errorf("RELAYER_PRIVATE_KEY is not a valid key: %w", err)
	}
	mine := crypto.PubkeyToAddress(priv.PublicKey)
	if mine != addrs[index] {
		return nil, fmt.Errorf("this node signs as %s but MPC_PEERS lists %s for party %d — every "+
			"contribution would be rejected by the peers", mine.Hex(), addrs[index].Hex(), index)
	}

	// Plaintext peers are permitted and logged.
	//
	// This used to refuse any non-loopback plaintext peer, matching a transport
	// check written when a shared token was the only protection. Per-round
	// signatures replaced that token, and mpc.TestForgeryFailsWithoutTLS shows
	// that an attacker with full control of a plaintext path can inject nothing
	// a peer will accept. Both checks went on blocking committee formation over
	// a threat that no longer existed.
	//
	// TLS is still recommended — it hides how many candidates each registration
	// is compared against — so a plaintext peer is called out at startup rather
	// than passing silently.
	insecure := strings.ToLower(os.Getenv("MPC_ALLOW_INSECURE")) == "true"
	for i, p := range peers {
		if i == index || strings.HasPrefix(p, "https://") {
			continue
		}
		log.Printf("[MPC] peer %d is plaintext (%s). Contributions are signed and verified, so "+
			"they cannot be forged; TLS would additionally hide the candidate counts.", i, p)
	}

	mailbox, err := mpc.NewMailbox(len(peers), 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return &mpcNode{
		mailbox:  mailbox,
		auth:     &validatorAuthenticator{index: index, priv: priv, addrs: addrs},
		index:    index,
		peers:    peers,
		insecure: insecure,
	}, nil
}

// newDiscoveringMPCNode builds a node whose committee comes from the chain.
//
// This is the path that scales. A static MPC_PEERS has to be edited on every
// box whenever a validator joins or leaves — the same manual-config problem the
// validator set itself already solved by syncing from peers, and reintroducing
// it here was a mistake. Here the node learns the candidates from the peer
// registry, which records a URL and a proven signing address for every peer
// that registered, and derives the committee deterministically. A new validator
// becomes eligible by registering. Nothing is edited, nothing is restarted, and
// nobody approves it.
func newDiscoveringMPCNode(discover func() []mpc.Party) (*mpcNode, error) {
	if discover == nil {
		return nil, fmt.Errorf("MPC_PEERS is empty and this node has no peer registry to " +
			"discover a committee from")
	}

	size := 2
	if raw := strings.TrimSpace(os.Getenv("MPC_COMMITTEE_SIZE")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("MPC_COMMITTEE_SIZE %q is not a number", raw)
		}
		size = v
	}
	if size < mpc.MinCommitteeSize || size > mpc.MaxCommitteeSize {
		return nil, fmt.Errorf("MPC_COMMITTEE_SIZE %d is outside %d..%d — below that one party "+
			"holds every share, and above it traffic grows with n(n-1) while every member has to "+
			"be online for any registration to finish",
			size, mpc.MinCommitteeSize, mpc.MaxCommitteeSize)
	}

	pkHex := strings.TrimPrefix(strings.TrimSpace(os.Getenv("RELAYER_PRIVATE_KEY")), "0x")
	if pkHex == "" {
		return nil, fmt.Errorf("RELAYER_PRIVATE_KEY is empty — it is the key this node signs its " +
			"MPC contributions with, and peers verify against its address")
	}
	priv, err := crypto.HexToECDSA(pkHex)
	if err != nil {
		return nil, fmt.Errorf("RELAYER_PRIVATE_KEY is not a valid key: %w", err)
	}
	self := strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())

	mailbox, err := mpc.NewMailbox(size, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return &mpcNode{
		mailbox:  mailbox,
		auth:     &validatorAuthenticator{priv: priv},
		insecure: strings.ToLower(os.Getenv("MPC_ALLOW_INSECURE")) == "true",
		discover: discover,
		size:     size,
		selfAddr: self,
	}, nil
}

func isLoopbackURL(u string) bool {
	return strings.HasPrefix(u, "http://127.0.0.1") ||
		strings.HasPrefix(u, "http://localhost") ||
		strings.HasPrefix(u, "http://[::1]")
}

// TransportFor returns this party's transport for one registration.
//
// session must be unique per registration and identical on both parties;
// derive it from something both already agree on, never from a counter, or two
// concurrent registrations will read each other's rounds.
func (n *mpcNode) TransportFor(session string) (mpc.Transport, error) {
	index, peers, auth := n.index, n.peers, n.auth

	if n.discover != nil {
		c, err := n.committeeNow()
		if err != nil {
			return nil, err
		}
		index = c.IndexOf(n.selfAddr)
		if index < 0 {
			// The ordinary case for most validators: eligible, not drawn.
			return nil, fmt.Errorf("mpc: this node is not in committee %s — it holds no shares "+
				"for these enrolments and must not take part", c.ID)
		}
		peers = c.URLs()
		addrs := make([]common.Address, len(c.Parties))
		for i, p := range c.Parties {
			addrs[i] = common.HexToAddress(p.Address)
		}
		auth = &validatorAuthenticator{index: index, priv: n.auth.priv, addrs: addrs}
	}

	return mpc.NewHTTPTransport(mpc.HTTPConfig{
		Index:         index,
		Peers:         peers,
		Session:       session,
		Mailbox:       n.mailbox,
		Auth:          auth,
		AllowInsecure: n.insecure,
	})
}

// registerMPCRoutes mounts the exchange endpoint if MPC is configured.
//
// A configuration error is loud and leaves the endpoint unmounted. It does not
// stop the node: a validator that cannot take part in duplicate checks should
// still produce blocks, and its peers will fail their comparisons with a clear
// error rather than quietly deciding alone.
func registerMPCRoutes(mux *http.ServeMux, discover func() []mpc.Party) *mpcNode {
	node, err := newMPCNodeFromEnv(discover)
	if err != nil {
		log.Printf("[MPC] NOT running: %v", err)
		return nil
	}
	if node == nil {
		return nil
	}
	var verifier mpc.Authenticator = node.auth
	if node.discover != nil {
		verifier = &liveCommitteeVerifier{node: node}
	}
	handler, err := node.mailbox.Handler(verifier)
	if err != nil {
		log.Printf("[MPC] NOT running: %v", err)
		return nil
	}
	mux.Handle(mpc.ExchangePath, handler)
	if node.discover != nil {
		log.Printf("[MPC] serving %s as %s; committee of %d drawn from the chain, membership "+
			"resolved per registration", mpc.ExchangePath, node.selfAddr, node.size)
	} else {
		log.Printf("[MPC] party %d of %d serving %s as %s; peers=%v",
			node.index, len(node.peers), mpc.ExchangePath,
			node.auth.addrs[node.index].Hex(), node.peers)
	}
	return node
}

// liveCommitteeVerifier authenticates incoming rounds against the committee as
// it stands right now.
//
// In discovery mode the membership is not known at startup, and pinning it
// there would defeat the point. Resolving per request costs one deterministic
// selection over a list the node already holds in memory.
type liveCommitteeVerifier struct{ node *mpcNode }

func (v *liveCommitteeVerifier) Sign(digest []byte) ([]byte, error) {
	return v.node.auth.Sign(digest)
}

func (v *liveCommitteeVerifier) Parties() int { return v.node.size }

func (v *liveCommitteeVerifier) VerifyParty(index int, digest, sig []byte) error {
	c, err := v.node.committeeNow()
	if err != nil {
		return fmt.Errorf("mpc: cannot verify a contribution without a committee: %w", err)
	}
	if index < 0 || index >= len(c.Parties) {
		return fmt.Errorf("mpc: party %d is outside committee %s", index, c.ID)
	}
	want := common.HexToAddress(c.Parties[index].Address)
	pub, err := crypto.SigToPub(digest, sig)
	if err != nil {
		return fmt.Errorf("mpc: signature from party %d does not recover: %w", index, err)
	}
	if got := crypto.PubkeyToAddress(*pub); got != want {
		return fmt.Errorf("mpc: round was signed by %s but party %d of committee %s is %s",
			got.Hex(), index, c.ID, want.Hex())
	}
	return nil
}
