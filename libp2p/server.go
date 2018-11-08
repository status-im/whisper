package libp2p

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	host "github.com/libp2p/go-libp2p-host"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/status-im/whisper/whisperv6"
)

var _ inet.Notifiee = (*Server)(nil)

func NewServer(h host.Host, w *whisperv6.Whisper) *Server {
	srv := &Server{
		host:    h,
		whisper: w,
	}
	h.Network().Notify(srv)
	return srv
}

type Server struct {
	host    host.Host
	whisper *whisperv6.Whisper

	feed event.Feed

	wg sync.WaitGroup
}

type Peer struct {
	ID        peer.ID
	Addr      ma.Multiaddr
	Protocols []protocol.ID
}

func (srv *Server) AddPeer(peer Peer) error {
	srv.host.Peerstore().AddAddr(peer.ID, peer.Addr, time.Second)
	sid, err := srv.host.NewStream(context.TODO(), peer.ID, peer.Protocols...)
	if err != nil {
		return err
	}
	srv.wg.Add(1)
	go func() {
		if err := Handle(srv.whisper, sid, 0, 0, 0); err != nil {
			log.Error("error running whisper", "error", err)
		}
		srv.wg.Done()
	}()
	return nil
}

func (srv *Server) RemovePeer(peer Peer) error {
	return srv.host.Network().ClosePeer(peer.ID)
}

func (srv *Server) Close() error {
	if err := srv.host.Close(); err != nil {
		return err
	}
	srv.wg.Wait()
	return nil
}

func (srv *Server) Subscribe(events chan<- p2p.PeerEvent) event.Subscription {
	return srv.feed.Subscribe(events)
}

func (srv *Server) Listen(net inet.Network, a ma.Multiaddr)      {}
func (srv *Server) ListenClose(net inet.Network, a ma.Multiaddr) {}
func (srv *Server) OpenedStream(net inet.Network, s inet.Stream) {}
func (srv *Server) ClosedStream(net inet.Network, s inet.Stream) {}
func (srv *Server) Connected(net inet.Network, c inet.Conn) {
	id, _ := PubKeyToNodeID(c.RemotePublicKey())
	srv.feed.Send(p2p.PeerEvent{
		Type: p2p.PeerEventTypeAdd,
		Peer: id,
	})
}
func (srv *Server) Disconnected(net inet.Network, c inet.Conn) {
	id, _ := PubKeyToNodeID(c.RemotePublicKey())
	srv.feed.Send(p2p.PeerEvent{
		Type: p2p.PeerEventTypeDrop,
		Peer: id,
	})
}
