package ratelimiter

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestIDMode(t *testing.T) {
	cfg := Config{}
	peer := p2p.NewPeer(discover.NodeID{1}, "test", nil)
	ctrl := gomock.NewController(t)
	rl := NewMockInterface(ctrl)
	rl.EXPECT().Create(peer.ID().Bytes())
	rl.EXPECT().TakeAvailable(peer.ID().Bytes(), int64(0))
	rl.EXPECT().Available(peer.ID().Bytes())
	rl.EXPECT().Remove(peer.ID().Bytes(), time.Duration(0))
	rl.EXPECT().UpdateConfig(peer.ID().Bytes(), cfg)
	peerrl := NewP2PRateLimiter(IDMode, rl)
	require.NoError(t, peerrl.Create(peer))
	peerrl.TakeAvailable(peer, 0)
	peerrl.Available(peer)
	require.NoError(t, peerrl.Remove(peer, 0))
	require.NoError(t, peerrl.UpdateConfig(peer, cfg))
}
