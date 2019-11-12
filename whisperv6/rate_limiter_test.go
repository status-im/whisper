package whisperv6

import (
	"bytes"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/stretchr/testify/require"
)

func TestPeerRateLimiterDecorator(t *testing.T) {
	in, out := p2p.MsgPipe()
	payload := []byte{0x01, 0x02, 0x03}
	msg := p2p.Msg{
		Code:       1,
		Size:       uint32(len(payload)),
		Payload:    bytes.NewReader(payload),
		ReceivedAt: time.Now(),
	}

	go func() {
		err := in.WriteMsg(msg)
		require.NoError(t, err)
	}()

	messages := make(chan p2p.Msg, 1)
	runLoop := func(p *Peer, rw p2p.MsgReadWriter) error {
		msg, err := rw.ReadMsg()
		if err != nil {
			return err
		}
		messages <- msg
		return nil
	}

	r := newPeerRateLimiter(&mockRateLimiterHandler{})
	err := r.Decorate(nil, out, runLoop)
	require.NoError(t, err)

	receivedMsg := <-messages
	receivedPayload := make([]byte, receivedMsg.Size)
	_, err = receivedMsg.Payload.Read(receivedPayload)
	require.NoError(t, err)
	require.Equal(t, msg.Code, receivedMsg.Code)
	require.Equal(t, payload, receivedPayload)
}

func TestPeerLimiterHandler(t *testing.T) {
	h := &mockRateLimiterHandler{}
	r := newPeerRateLimiter(h)
	p := &Peer{
		peer: p2p.NewPeer(enode.ID{0xaa, 0xbb, 0xcc}, "test-peer", nil),
	}
	rw1, rw2 := p2p.MsgPipe()
	count := 100

	go func() {
		err := r.Decorate(p, rw2, func(p *Peer, rw p2p.MsgReadWriter) error {
			for {
				msg, err := rw.ReadMsg()
				if err != nil {
					return err
				}
				err = rw.WriteMsg(msg)
				if err != nil {
					return err
				}
			}
		})
		require.NoError(t, err)
	}()

	done := make(chan struct{})
	go func() {
		for i := 0; i < count; i++ {
			msg, err := rw1.ReadMsg()
			require.NoError(t, err)
			require.EqualValues(t, 101, msg.Code)
		}
		close(done)
	}()

	for i := 0; i < count; i += 1 {
		err := rw1.WriteMsg(p2p.Msg{Code: 101})
		require.NoError(t, err)
	}

	<-done

	require.Equal(t, 100-rateLimitPerSecIP, h.exceedIPLimit)
	require.Equal(t, 100-rateLimitPerSecPeerID, h.exceedPeerLimit)
}

type mockRateLimiterHandler struct {
	exceedPeerLimit int
	exceedIPLimit   int
}

func (m *mockRateLimiterHandler) ExceedPeerLimit() { m.exceedPeerLimit += 1 }
func (m *mockRateLimiterHandler) ExceedIPLimit()   { m.exceedIPLimit += 1 }
