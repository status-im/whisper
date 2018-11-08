package libp2p

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/status-im/whisper/whisperv6"
	"github.com/stretchr/testify/require"
)

func init() {
	logs := flag.String("logs", "CRIT", "verbosity for tests")
	flag.Parse()
	lvl, err := log.LvlFromString(strings.ToLower(*logs))
	if err != nil {
		panic(err)
	}
	filteredHandler := log.LvlFilterHandler(lvl, log.StderrHandler)
	log.Root().SetHandler(filteredHandler)
}

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
	srv1 := NewServer(h)
	require.NoError(t, srv1.RegisterHandler("whisper", NewWhisperHandler(w1, 0, time.Second, 10*time.Second, 10*time.Second)))
	w2 := whisperv6.New(&whisperv6.Config{
		MinimumAcceptedPOW: 0,
		MaxMessageSize:     whisperv6.DefaultMaxMessageSize,
	})
	srv := NewServer(c)
	require.NoError(t, srv.RegisterHandler("whisper", NewWhisperHandler(w2, 0, time.Second, 10*time.Second, 10*time.Second)))
	p1 := Peer{
		ID:    h.ID(),
		Addr:  h.Addrs()[0],
		Topic: "whisper",
	}
	require.NoError(t, srv.AddPeer(context.TODO(), p1))

	api1 := whisperv6.NewPublicWhisperAPI(w1)
	key := [32]byte{}
	rand.Read(key[:])
	id1, err := w1.AddSymKeyDirect(key[:])
	id2, err := w2.AddSymKeyDirect(key[:])
	require.NoError(t, err)
	require.NoError(t, err)

	api2 := whisperv6.NewPublicWhisperAPI(w2)
	fid, err := api2.NewMessageFilter(whisperv6.Criteria{
		SymKeyID: id2,
		Topics:   []whisperv6.TopicType{{1, 2, 3, 4}},
	})
	require.NoError(t, err)

	_, err = api1.Post(context.TODO(), whisperv6.NewMessage{
		SymKeyID: id1,
		TTL:      10,
		Topic:    [4]byte{1, 2, 3, 4},
		Payload:  []byte("hello"),
	})
	require.NoError(t, err)
	require.NoError(t, Eventually(func() error {
		messages, err := api2.GetFilterMessages(fid)
		if err != nil {
			return err
		}
		if len(messages) == 1 {
			return nil
		}
		return fmt.Errorf("unexpected messages: %d", len(messages))
	}, 1*time.Second, 5*time.Second))
}
