package libp2p

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/status-im/whisper/whisperv6"
)

const (
	proto = "/shh/6.0"
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
	if c.period == 0 {
		return false
	}
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

func PubKeyToNodeID(pubkey crypto.PubKey) (n discover.NodeID, err error) {
	seckey, ok := pubkey.(*crypto.Secp256k1PublicKey)
	if !ok {
		return n, errors.New("public key is not on the secp256k1 curve")
	}
	return discover.PubkeyID((*ecdsa.PublicKey)(seckey)), nil
}

func NewWhisperHandler(whisper *whisperv6.Whisper, flaky, keapalive, read, write time.Duration) *WhisperWithKeepaliveHandler {
	return &WhisperWithKeepaliveHandler{
		whisper:     whisper,
		connections: map[peer.ID]*connection{},
		flaky:       flaky,
		keapalive:   keapalive,
		read:        read,
		write:       write,
	}
}

type WhisperWithKeepaliveHandler struct {
	whisper *whisperv6.Whisper

	mu          sync.Mutex
	connections map[peer.ID]*connection

	keapalive   time.Duration
	read, write time.Duration
	flaky       time.Duration

	wg sync.WaitGroup
}

func (w *WhisperWithKeepaliveHandler) Register(h host.Host) {
	h.SetStreamHandler(proto, func(s inet.Stream) {
		id, err := PubKeyToNodeID(s.Conn().RemotePublicKey())
		if err != nil {
			log.Error("can't convert pubkey to node id", err)
			return
		}
		w.mu.Lock()
		conn, exist := w.connections[s.Conn().RemotePeer()]
		if !exist {
			conn = &connection{id: id, period: 0, received: new(atomic.Value)}
			conn.Update(time.Time{})
		}
		w.connections[s.Conn().RemotePeer()] = conn
		w.mu.Unlock()
		if err := w.whisper.HandleConnection(conn, Stream{
			s:   s,
			rlp: rlp.NewStream(s, 0),
		}); err != nil {
			log.Error("error runnign whisper", "error", err)
		}
	})
	h.SetStreamHandler(pingproto, func(s inet.Stream) {
		id, err := PubKeyToNodeID(s.Conn().RemotePublicKey())
		if err != nil {
			log.Error("can't convert pubkey to node id", err)
			return
		}
		w.mu.Lock()
		conn, exist := w.connections[s.Conn().RemotePeer()]
		if !exist {
			conn = &connection{id: id, period: w.flaky, received: new(atomic.Value)}
			conn.Update(time.Time{})
		}
		w.connections[s.Conn().RemotePeer()] = conn
		w.mu.Unlock()
		makePingHandler(conn, w.keapalive, w.read, w.write)(s)
	})
}

func (w *WhisperWithKeepaliveHandler) Dial(ctx context.Context, h host.Host, p peer.ID) error {
	pingStream, err := h.NewStream(ctx, p, pingproto)
	if err != nil {
		return err
	}
	id, err := PubKeyToNodeID(pingStream.Conn().RemotePublicKey())
	if err != nil {
		log.Error("can't convert pubkey to node id", err)
		return err
	}
	w.mu.Lock()
	conn := &connection{id: id, period: w.flaky, received: new(atomic.Value)}
	conn.Update(time.Time{})
	w.connections[p] = conn
	w.mu.Unlock()

	whisperStream, err := h.NewStream(ctx, p, proto)
	if err != nil {
		return err
	}
	w.wg.Add(1)
	go func() {
		makePingHandler(conn, w.keapalive, w.read, w.write)(pingStream)
		w.wg.Done()
	}()
	w.wg.Add(1)
	go func() {
		if err := w.whisper.HandleConnection(conn, Stream{
			s:   whisperStream,
			rlp: rlp.NewStream(whisperStream, 0),
		}); err != nil {
			log.Error("error runnign whisper", "error", err)
		}
		w.wg.Done()
	}()
	return nil
}

func (w *WhisperWithKeepaliveHandler) Start() error {
	// TODO enable whisper to start withot *p2p.Server
	return w.whisper.Start(nil)
}

func (w *WhisperWithKeepaliveHandler) Close() {
	w.whisper.Stop()
	w.wg.Wait()
}
