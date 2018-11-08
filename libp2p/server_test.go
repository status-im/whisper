package libp2p

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	net "github.com/libp2p/go-libp2p-net"
	protocol "github.com/libp2p/go-libp2p-protocol"
	"github.com/status-im/whisper/whisperv6"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	require.NoError(t, err)
	h, err := libp2p.New(context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/9999"),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)

	priv, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	c, err := libp2p.New(context.Background(),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)

	w1 := whisperv6.New(&whisperv6.Config{
		MinimumAcceptedPOW: 0,
		MaxMessageSize:     whisperv6.DefaultMaxMessageSize,
	})
	h.SetStreamHandler(proto, func(s net.Stream) {
		if err := Handle(w1, s, 0, 0, 0); err != nil {
			log.Error("Connection was closed with error", "peer", s.Conn().RemotePeer(), "error", err)
		}
	})

	w2 := whisperv6.New(&whisperv6.Config{
		MinimumAcceptedPOW: 0,
		MaxMessageSize:     whisperv6.DefaultMaxMessageSize,
	})
	srv := NewServer(c, w2)
	p1 := Peer{
		ID:        h.ID(),
		Addr:      h.Addrs()[0],
		Protocols: []protocol.ID{proto},
	}
	require.NoError(t, srv.AddPeer(p1))
	time.Sleep(2 * time.Second)
	require.NoError(t, srv.RemovePeer(p1))
	time.Sleep(2 * time.Second)
	require.NoError(t, srv.Close())
}
