package libp2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	host "github.com/libp2p/go-libp2p-host"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

var _ inet.Notifiee = (*Server)(nil)

func NewServer(h host.Host) *Server {
	srv := &Server{
		host:   h,
		topics: map[string]ProtocolHandler{},
	}
	h.Network().Notify(srv)
	return srv
}

type ProtocolHandler interface {
	Register(host.Host)
	Start() error
	Dial(ctx context.Context, h host.Host, p peer.ID) error
	Close()
}

type Server struct {
	host host.Host

	mu     sync.Mutex
	topics map[string]ProtocolHandler

	feed event.Feed
}

type Peer struct {
	ID    peer.ID
	Addr  ma.Multiaddr
	Topic string
}

func (srv *Server) RegisterHandler(topic string, handler ProtocolHandler) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	_, exist := srv.topics[topic]
	if exist {
		return fmt.Errorf("handler for %s already registered", topic)
	}
	srv.topics[topic] = handler
	handler.Register(srv.host)
	return handler.Start()
}

func (srv *Server) AddPeer(ctx context.Context, peer Peer) error {
	srv.host.Peerstore().AddAddr(peer.ID, peer.Addr, time.Second)
	srv.mu.Lock()
	h, exist := srv.topics[peer.Topic]
	srv.mu.Unlock()
	if !exist {
		return fmt.Errorf("no handler for topic %s", peer.Topic)
	}
	return h.Dial(ctx, srv.host, peer.ID)
}

func (srv *Server) RemovePeer(peer Peer) error {
	return srv.host.Network().ClosePeer(peer.ID)
}

func (srv *Server) Close() error {
	if err := srv.host.Close(); err != nil {
		return err
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for topic, h := range srv.topics {
		log.Debug("waiting for handler", "topic", topic)
		h.Close()
	}
	return nil
}

func (srv *Server) Subscribe(events chan<- *p2p.PeerEvent) event.Subscription {
	return srv.feed.Subscribe(events)
}

func (srv *Server) Connected(net inet.Network, c inet.Conn) {
	id, _ := PubKeyToNodeID(c.RemotePublicKey())
	srv.feed.Send(&p2p.PeerEvent{
		Type: p2p.PeerEventTypeAdd,
		Peer: id,
	})
}
func (srv *Server) Disconnected(net inet.Network, c inet.Conn) {
	id, _ := PubKeyToNodeID(c.RemotePublicKey())
	srv.feed.Send(&p2p.PeerEvent{
		Type: p2p.PeerEventTypeDrop,
		Peer: id,
	})
}

func (srv *Server) Listen(net inet.Network, a ma.Multiaddr)      {}
func (srv *Server) ListenClose(net inet.Network, a ma.Multiaddr) {}
func (srv *Server) OpenedStream(net inet.Network, s inet.Stream) {}
func (srv *Server) ClosedStream(net inet.Network, s inet.Stream) {}
