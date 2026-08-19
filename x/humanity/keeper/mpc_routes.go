package keeper

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// MPC wiring: what turns the protocol in x/humanity/mpc into two machines that
// actually hold one row each.
//
// The duplicate check compares a new capture against enrolled templates without
// any single machine ever holding a whole template. That property is not a
// property of the maths alone — it only exists if the parties really run on
// separate boxes under separate control. This file is where that happens, and
// it is deliberately opt-in: a misconfigured node must fall back to "MPC not
// running", never to "one party doing it all", because the second is
// indistinguishable from working while providing none of the protection.
//
// Configuration (all required together):
//
//	MPC_ENABLED=true
//	MPC_PARTY_INDEX=0            this node's party number
//	MPC_PEERS=https://a,https://b   base URL per party, in party order
//	MPC_PEER_TOKEN=<shared secret>  authenticates peer contributions
//
// MPC_ALLOW_INSECURE=true permits http:// peers. It exists for a local harness
// and is refused in combination with a non-loopback peer, because an attacker
// who can inject on that path can make a returning person look new.

// mpcNode is this process's participation in the comparison protocol.
type mpcNode struct {
	mailbox *mpc.Mailbox
	index   int
	peers   []string
	token   string
	insecure bool
}

// newMPCNodeFromEnv reads the configuration, or reports why MPC stays off.
//
// Returns (nil, nil) when MPC is simply not enabled — the ordinary case for a
// node that does not take part.
func newMPCNodeFromEnv() (*mpcNode, error) {
	if strings.ToLower(os.Getenv("MPC_ENABLED")) != "true" {
		return nil, nil
	}

	rawPeers := os.Getenv("MPC_PEERS")
	if rawPeers == "" {
		return nil, fmt.Errorf("MPC_ENABLED is set but MPC_PEERS is empty")
	}
	var peers []string
	for _, p := range strings.Split(rawPeers, ",") {
		if p = strings.TrimSpace(p); p != "" {
			peers = append(peers, strings.TrimRight(p, "/"))
		}
	}
	if len(peers) < 2 {
		return nil, fmt.Errorf("MPC_PEERS lists %d parties; with fewer than two, one machine "+
			"holds every share and can reconstruct every template — which is the situation this "+
			"whole subsystem exists to prevent", len(peers))
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

	token := os.Getenv("MPC_PEER_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("MPC_PEER_TOKEN is empty — an unauthenticated exchange endpoint " +
			"lets anyone steer a duplicate check into saying 'no match'")
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("MPC_PEER_TOKEN is %d characters; use at least 32 — this token is "+
			"the only thing standing between a stranger and a forged 'this person is new'",
			len(token))
	}

	insecure := strings.ToLower(os.Getenv("MPC_ALLOW_INSECURE")) == "true"
	if insecure {
		for i, p := range peers {
			if i == index {
				continue
			}
			if !isLoopbackURL(p) {
				return nil, fmt.Errorf("MPC_ALLOW_INSECURE is set with a non-loopback peer %q — "+
					"that combination is for a local harness only", p)
			}
		}
	}

	mailbox, err := mpc.NewMailbox(len(peers), 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return &mpcNode{
		mailbox:  mailbox,
		index:    index,
		peers:    peers,
		token:    token,
		insecure: insecure,
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
	return mpc.NewHTTPTransport(mpc.HTTPConfig{
		Index:         n.index,
		Peers:         n.peers,
		Session:       session,
		Token:         n.token,
		Mailbox:       n.mailbox,
		AllowInsecure: n.insecure,
	})
}

// registerMPCRoutes mounts the exchange endpoint if MPC is configured.
//
// A configuration error is loud and leaves the endpoint unmounted. It does not
// stop the node: a validator that cannot take part in duplicate checks should
// still produce blocks, and the peer will fail its comparison with a clear
// error rather than quietly deciding alone.
func registerMPCRoutes(mux *http.ServeMux) *mpcNode {
	node, err := newMPCNodeFromEnv()
	if err != nil {
		log.Printf("[MPC] NOT running: %v", err)
		return nil
	}
	if node == nil {
		return nil
	}
	handler, err := node.mailbox.Handler(node.token)
	if err != nil {
		log.Printf("[MPC] NOT running: %v", err)
		return nil
	}
	mux.Handle(mpc.ExchangePath, handler)
	log.Printf("[MPC] party %d of %d serving %s; peers=%v",
		node.index, len(node.peers), mpc.ExchangePath, node.peers)
	return node
}
