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
	net "github.com/libp2p/go-libp2p-net"
	protocol "github.com/libp2p/go-libp2p-protocol"
	"github.com/stretchr/testify/require"
)

func TestPingPong(t *testing.T) {
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
	require.NoError(t, err)
	c.Peerstore().AddAddr(h.ID(), h.Addrs()[0], 30*time.Second)

	conn := connection{id: discover.NodeID{1}, received: new(atomic.Value), period: 10 * time.Second}
	conn.Update(time.Time{})
	h.SetStreamHandler(pingproto, makePingHandler(conn, 1*time.Second, 10*time.Second, 10*time.Second))

	s, err := c.NewStream(context.TODO(), h.ID(), pingproto)
	require.NoError(t, err)
	received := [1]byte{}
	require.NoError(t, s.SetReadDeadline(time.Now().Add(3*time.Second)))
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

func TestConnectionClosed(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	require.NoError(t, err)
	h, err := libp2p.New(context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/8888"),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)

	priv, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 0, rng)
	c, err := libp2p.New(context.Background(),
		libp2p.Identity(priv),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	c.Peerstore().AddAddr(h.ID(), h.Addrs()[0], 30*time.Second)

	conn := connection{id: discover.NodeID{1}, received: new(atomic.Value), period: 10 * time.Second}
	conn.Update(time.Time{})
	errors := make(chan error, 1)
	h.SetStreamHandler(protocol.TestingID, func(s net.Stream) {
		rcv := [1]byte{}
		for {
			_, err := s.Read(rcv[:])
			if err != nil {
				errors <- err
				return
			}
		}
	})
	h.SetStreamHandler(pingproto, makePingHandler(conn, time.Second, time.Second, time.Second))

	_, err = c.NewStream(context.TODO(), h.ID(), pingproto)
	require.NoError(t, err)

	_, err = c.NewStream(context.TODO(), h.ID(), protocol.TestingID)
	require.NoError(t, err)
	select {
	case err := <-errors:
		require.EqualError(t, err, "stream reset")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "connection wasn't closed in expected time")
	}
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
