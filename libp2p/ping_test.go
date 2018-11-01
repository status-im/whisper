package libp2p

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/stretchr/testify/require"
)

func TestPing(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	require.NoError(t, err)
	h, err := libp2p.New(context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/7777"),
		libp2p.Identity(priv),
	)

	priv, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	c, err := libp2p.New(context.Background(),
		libp2p.Identity(priv),
	)
	require.NoError(t, err)
	c.Peerstore().AddAddr(h.ID(), h.Addrs()[1], 30*time.Second)

	conn := connection{id: discover.NodeID{1}, received: new(atomic.Value), period: 10 * time.Second}
	conn.Update(time.Time{})
	h.SetStreamHandler(pingproto, makePingHandler(conn, 1*time.Second, 10*time.Second, 10*time.Second))

	c.Peerstore().AddAddr(h.ID(), h.Addrs()[1], 30*time.Second)
	s, err := c.NewStream(context.TODO(), h.ID(), pingproto)
	require.NoError(t, err)
	received := [1]byte{}
	require.NoError(t, s.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = s.Read(received[:])
	require.NoError(t, err)
	require.Equal(t, pingMsg, received[0])
	require.True(t, conn.IsFlaky())
	_, err = s.Write([]byte{pingMsg})
	require.NoError(t, err)
	require.NoError(t, Eventually(func() error {
		if conn.IsFlaky() {
			return errors.New("connection is flaky")
		}
		return nil
	}, 100*time.Millisecond, 1*time.Second))
	require.NoError(t, Eventually(func() error {
		if err := s.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		received := [1]byte{}
		_, err := s.Read(received[:])
		if err != nil {
			return err
		}
		if received[0] != pongMsg {
			return fmt.Errorf("not a pong: %x", received)
		}
		return nil
	}, 100*time.Millisecond, 1*time.Second))
}

func Eventually(f func() error, period, timeout time.Duration) (err error) {
	after := time.NewTimer(timeout)
	defer after.Stop()
	tick := time.NewTicker(period)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			err = f()
			if err == nil {
				return
			}
		case <-after.C:
			return
		}
	}

}
