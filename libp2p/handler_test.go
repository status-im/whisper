package libp2p

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	net "github.com/libp2p/go-libp2p-net"
	"github.com/status-im/whisper/whisperv6"
	"github.com/stretchr/testify/require"
)

const (
	proto = "/shh/6.0"
)

func TestWhisperOverLibp2p(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	require.NoError(t, err)
	h, err := libp2p.New(context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/7777"),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	w := whisperv6.New(&whisperv6.Config{
		MinimumAcceptedPOW: 0,
		MaxMessageSize:     whisperv6.DefaultMaxMessageSize,
	})
	h.SetStreamHandler(proto, func(s net.Stream) {
		if err := Handle(w, s, 0, 0, 0); err != nil {
			log.Error("Connection was closed with error", "peer", s.Conn().RemotePeer(), "error", err)
		}
	})
	priv, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	require.NoError(t, err)
	c, err := libp2p.New(context.Background(),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	c.Peerstore().AddAddr(h.ID(), h.Addrs()[0], 30*time.Second)
	s, err := c.NewStream(context.TODO(), h.ID(), proto)
	require.NoError(t, err)
	msgs := NewStream(s, 2*time.Second, 0)
	_, err = msgs.ReadMsg()
	require.NoError(t, err)
	require.NoError(t, p2p.SendItems(msgs, 0, uint64(6), math.Float64bits(w.MinPow()), w.BloomFilter(), true))
	sent := &whisperv6.Envelope{
		Expiry: uint32(time.Now().Add(time.Second).Unix()),
		TTL:    uint32(time.Second),
		Topic:  whisperv6.TopicType{1},
		Data:   []byte("hello"),
	}
	require.NoError(t, w.Send(sent))
	msg, err := msgs.ReadMsg()
	require.NoError(t, err)
	require.Equal(t, 1, int(msg.Code))
	var envelopes []*whisperv6.Envelope
	require.NoError(t, msg.Decode(&envelopes))
	require.Len(t, envelopes, 1)
	require.Equal(t, sent.Data, envelopes[0].Data)
}
