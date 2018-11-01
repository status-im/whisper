package libp2p

import (
	"crypto/ecdsa"
	"errors"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	crypto "github.com/libp2p/go-libp2p-crypto"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/status-im/whisper/whisperv6"
)

type connection struct {
	id discover.NodeID

	received *atomic.Value
	period   time.Duration
}

func (c connection) ID() discover.NodeID {
	return c.id
}

func (c connection) IsFlaky() bool {
	val := c.received.Load()
	received, ok := val.(time.Time)
	if !ok {
		log.Error("can't cast to time.Time", "value", val)
		return false
	}
	return time.Since(received) > c.period
}

func (c connection) Update(t time.Time) {
	c.received.Store(t)
}

// PeerIDToNodeID casts peer.ID (b58 encoded string) to discover.NodeID
func PeerIDToNodeID(pid string) (n discover.NodeID, err error) {
	nodeid, err := peer.IDB58Decode(pid)
	if err != nil {
		return n, err
	}
	pubkey, err := nodeid.ExtractPublicKey()
	if err != nil {
		return n, err
	}
	seckey, ok := pubkey.(*crypto.Secp256k1PublicKey)
	if !ok {
		return n, errors.New("public key is not on the secp256k1 curve")
	}
	return discover.PubkeyID((*ecdsa.PublicKey)(seckey)), nil
}

func Handle(w *whisperv6.Whisper, s net.Stream, read, write time.Duration) error {
	id, err := PeerIDToNodeID(peer.IDB58Encode(s.Conn().RemotePeer()))
	if err != nil {
		return err
	}
	return w.HandleConnection(connection{id: id}, Stream{
		s:            s,
		rlp:          rlp.NewStream(s, 0),
		readTimeout:  read,
		writeTimeout: write,
	})
}
