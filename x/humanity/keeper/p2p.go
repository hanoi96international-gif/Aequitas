package keeper

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	ProtocolID      = "/aequitas/1.0.0"
	BlockProtocolID = "/aequitas/blocks/1.0.0"
	ListenPort      = 4001
	// FIX: this was hardcoded to "thomas.proxy.rlwy.net:47298" — a Railway
	// TCP-proxy domain:port pair tied to a specific service instance. Railway
	// regenerates this domain whenever the service is recreated (e.g. renamed,
	// redeployed from scratch, or moved to a new project), and the old domain
	// stops resolving/accepting connections entirely with no warning. Every
	// node's P2P bootstrap dial then times out forever ("failed to dial:
	// context deadline exceeded"), silently disabling real P2P block
	// broadcast/merging network-wide — HTTP sync masked this because it's a
	// separate, working fallback path, but it only pulls from ONE primary and
	// can't merge multiple validators' blocks the way actual P2P gossip can.
	// Confirmed in production: the address had gone stale. The first
	// replacement TCP-proxy address found (zephyr.proxy.rlwy.net:22303)
	// turned out to forward to port 8080 (the HTTP API), not 4001 (the P2P
	// listener) — Railway only allows one TCP-proxy mapping per service, so
	// the proxy had to be repointed at port 4001 specifically, yielding
	// reseau.proxy.rlwy.net:41277. The peer ID suffix is unaffected (NODE_KEY-
	// derived, stable across address changes) — only the domain:port needed
	// updating. BOOTSTRAP_P2P_ADDR now overrides this default so a future
	// Railway domain regeneration is an env var change, not a code deploy.
	defaultBootstrapNode = "/dns4/reseau.proxy.rlwy.net/tcp/41277/p2p/12D3KooWFuP5HtD1Xy9bj3ZdWL7eisWTx72V26hpGieMmqsGLV5R"
)

// BootstrapNode returns the FIRST configured P2P bootstrap multiaddr — kept
// for any external callers expecting a single address; prefer BootstrapNodes
// for actually connecting (see its comment for why a single fixed address is
// a scale/resilience problem at a 100-node target).
func BootstrapNode() string {
	nodes := BootstrapNodes()
	if len(nodes) == 0 {
		return ""
	}
	return nodes[0]
}

// BootstrapNodes returns every configured P2P bootstrap multiaddr to try on
// startup: BOOTSTRAP_P2P_ADDR (comma-separated, if set) plus the built-in
// default. A single hardcoded bootstrap address has already gone stale in
// production once (see defaultBootstrapNode's comment) — at a 100-node
// target a lone bootstrap address being down (redeploy, restart, outage)
// would strand every node that hasn't already connected, since P2P
// connectivity (unlike HTTP peer discovery, which already supports multiple
// seeds via PRIMARY_NODE_URLS) had no fallback at all. Trying every
// configured address is a cheap, safe widening: ConnectToPeer attempts are
// independent and a failed dial to one address doesn't affect the others.
func BootstrapNodes() []string {
	var out []string
	if raw := os.Getenv("BOOTSTRAP_P2P_ADDR"); raw != "" {
		for _, addr := range strings.Split(raw, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				out = append(out, addr)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, defaultBootstrapNode)
	}
	return out
}

type P2PNode struct {
	host   host.Host
	keeper *Keeper
	dag    *BlockDAG
	peers  []peer.AddrInfo
}

func loadOrCreateKey() (crypto.PrivKey, error) {
	if keyStr := os.Getenv("NODE_KEY"); keyStr != "" {
		keyBytes, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			fmt.Printf("⚠ NODE_KEY is set but invalid base64: %v — generating new key\n", err)
		} else {
			priv, err := crypto.UnmarshalPrivateKey(keyBytes)
			if err == nil {
				fmt.Println("✓ Node key loaded from environment")
				return priv, nil
			}
		}
	}

	fmt.Println("⚠ No NODE_KEY found – generating new key...")
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, -1, rand.Reader)
	if err != nil {
		return nil, err
	}

	keyBytes, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(keyBytes)
	// Fix 9: NODE_KEY is visible in hosted log dashboards even on stderr.
	// Operators must treat it as a secret and move it to a NODE_KEY env var.
	fmt.Fprintln(os.Stderr, "════════════════════════════════════════")
	fmt.Fprintln(os.Stderr, "⚠ WARNING: NODE_KEY is visible in hosted log dashboards. Treat this as a secret.")
	fmt.Fprintln(os.Stderr, "SAVE THIS AS NODE_KEY ENVIRONMENT VAR, then restart the service:")
	fmt.Fprintln(os.Stderr, encoded)
	fmt.Fprintln(os.Stderr, "════════════════════════════════════════")

	return priv, nil
}

func NewP2PNode(keeper *Keeper) (*P2PNode, error) {
	priv, err := loadOrCreateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", ListenPort),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	node := &P2PNode{
		host:   h,
		keeper: keeper,
	}

	h.SetStreamHandler(protocol.ID(ProtocolID), node.handleStream)
	h.SetStreamHandler(protocol.ID(BlockProtocolID), node.handleBlockStream)
	return node, nil
}

func (n *P2PNode) SetDAG(dag *BlockDAG) {
	n.dag = dag
}

// handleStream — status messages
func (n *P2PNode) handleStream(s network.Stream) {
	defer s.Close()
	// P3-6: use LimitReader (64KB) instead of fixed 1024-byte buffer.
	// A 1024-byte hard limit silently truncates longer messages; LimitReader
	// lets us read the full message while still bounding resource usage.
	data, err := io.ReadAll(io.LimitReader(s, 64*1024))
	if err != nil || len(data) == 0 {
		return
	}
	msg := string(data)
	fmt.Printf("[P2P] Message from %s: %s\n", s.Conn().RemotePeer().String()[:12], msg)
	// FIX (audit 2026-06-29): n.keeper is this package's separate, legacy
	// in-memory Keeper (keeper.go) — its RegisterHuman has zero callers
	// anywhere in the codebase, since real registration goes entirely
	// through ChainState.RegisterHumanAtomic (state.go). n.keeper.humans is
	// therefore always empty, so this reported "humans=0" regardless of the
	// chain's actual state. dag.state is the real ChainState (same source
	// ProduceBlock uses for the block.Humans field — see block.go), and is
	// always non-nil once SetDAG has been called (which happens before this
	// stream handler can ever receive a connection).
	humans := 0
	if n.dag != nil && n.dag.state != nil {
		humans = n.dag.state.TotalHumans()
	}
	response := fmt.Sprintf("AEQUITAS_NODE|humans=%d|chainid=aequitas-1", humans)
	s.Write([]byte(response))
}

// handleBlockStream — receive blocks from peers
func (n *P2PNode) handleBlockStream(s network.Stream) {
	defer s.Close()
	if n.dag == nil {
		return
	}

	// io.ReadAll with a cap prevents TCP fragmentation issues — a single
	// s.Read() call may return only a partial message if the TCP segment
	// is fragmented; ReadAll accumulates all bytes until EOF/close.
	body, err := io.ReadAll(io.LimitReader(s, 512<<10)) // 512 KB cap
	if err != nil || len(body) == 0 {
		return
	}

	var block Block
	if err := json.Unmarshal(body, &block); err != nil {
		fmt.Printf("[BLOCK-SYNC] ✗ Parse error from peer %s: %v\n",
			s.Conn().RemotePeer().String()[:12], err)
		return
	}

	// Log only when the block is actually accepted — logging before
	// AddPeerBlock caused "Received" messages for blocks that were rejected.
	sender := s.Conn().RemotePeer()
	if n.dag.AddPeerBlock(&block) {
		fmt.Printf("[BLOCK-SYNC] ✓ Accepted block #%d from peer %s\n",
			block.Height, sender.String()[:12])
		// Relay to all other peers (gossip) so every node sees every block
		// even when not directly connected to the originator.
		go n.broadcastExcept(&block, sender)
	}
}

// BroadcastBlock — send new block to all connected peers
func (n *P2PNode) BroadcastBlock(block *Block) {
	n.broadcastExcept(block, "")
}

// broadcastExcept — send block to all peers except the given sender (empty = send to all)
func (n *P2PNode) broadcastExcept(block *Block, exclude peer.ID) {
	peers := n.host.Network().Peers()
	if len(peers) == 0 {
		return
	}

	data, err := json.Marshal(block)
	if err != nil {
		return
	}

	for _, peerID := range peers {
		if peerID == exclude {
			continue
		}
		go func(pid peer.ID) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			s, err := n.host.NewStream(ctx, pid, protocol.ID(BlockProtocolID))
			if err != nil {
				return
			}
			defer s.Close()
			// P3-AUDIT: log write errors so network issues are visible in logs.
			if _, writeErr := s.Write(data); writeErr != nil {
				fmt.Printf("[BLOCK-SYNC] ✗ Failed to send block #%d to %s: %v\n", block.Height, pid.String()[:12], writeErr)
				return
			}
			fmt.Printf("[BLOCK-SYNC] → Sent block #%d to %s\n", block.Height, pid.String()[:12])
		}(peerID)
	}
}

func (n *P2PNode) Start() {
	fmt.Println("── P2P Network ──────────────────────────")
	fmt.Printf("✓ Node ID: %s\n", n.host.ID().String()[:20]+"...")
	fmt.Printf("✓ Listening on port %d\n", ListenPort)
	for _, addr := range n.host.Addrs() {
		fmt.Printf("✓ Address: %s/p2p/%s\n", addr, n.host.ID())
	}
	fmt.Println()

	selfID := n.host.ID().String()
	if selfID != "12D3KooWFuP5HtD1Xy9bj3ZdWL7eisWTx72V26hpGieMmqsGLV5R" {
		fmt.Println("── Connecting to Bootstrap Node(s) ──────")
		// Try every configured bootstrap address, not just one (scale
		// audit): a single hardcoded/fixed bootstrap address has already
		// gone stale in production before (see defaultBootstrapNode's
		// comment) and stranded every node dialing it. Each dial is
		// independent, so a failure on one address doesn't affect the rest.
		connected := 0
		for _, addr := range BootstrapNodes() {
			if err := n.ConnectToPeer(addr); err != nil {
				// P2P bootstrap is best-effort — HTTP block sync is the
				// primary mechanism. Failure here is expected when port 4001
				// is firewalled (e.g. Railway, Docker without -p 4001:4001)
				// and does not prevent the node from syncing blocks or
				// producing correctly.
				fmt.Printf("⚠ P2P bootstrap %s unreachable: %v\n", addr, err)
				continue
			}
			fmt.Printf("✓ Connected to bootstrap node: %s\n", addr)
			connected++
		}
		if connected == 0 {
			fmt.Println("⚠ No P2P bootstrap address reachable (HTTP sync still works)")
		}
		fmt.Println()
	}
}

func (n *P2PNode) ConnectToPeer(peerAddr string) error {
	addrInfo, err := peer.AddrInfoFromString(peerAddr)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := n.host.Connect(ctx, *addrInfo); err != nil {
		return err
	}

	fmt.Printf("✓ Connected to peer: %s\n", addrInfo.ID.String()[:12]+"...")
	return nil
}

func (n *P2PNode) GetMultiaddr() string {
	if len(n.host.Addrs()) == 0 {
		return ""
	}
	return fmt.Sprintf("%s/p2p/%s", n.host.Addrs()[0], n.host.ID())
}

func (n *P2PNode) GetNodeID() string {
	return n.host.ID().String()
}

func (n *P2PNode) ConnectedPeers() int {
	return len(n.host.Network().Peers())
}
