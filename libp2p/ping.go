package libp2p

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	net "github.com/libp2p/go-libp2p-net"
)

const (
	pingproto      = "/ping/0.0.1"
	pingMsg   byte = 1
	pongMsg   byte = 2
)

func makePingHandler(c connection, period, r, w time.Duration) func(s net.Stream) {
	return func(s net.Stream) {
		defer s.Close()
		ping := [1]byte{pingMsg}
		pong := [1]byte{pongMsg}
		received := [1]byte{}
		quit := make(chan struct{})
		go func() {
			tick := time.NewTicker(period)
			defer tick.Stop()
			for {
				select {
				case <-quit:
					return
				case <-tick.C:
					s.SetWriteDeadline(time.Now().Add(w))
					if _, err := s.Write(ping[:]); err != nil {
						log.Error("error sending the ping", "error", err)
						return
					}
				}
			}
		}()
		for {
			received[0] = 0
			s.SetReadDeadline(time.Now().Add(r))
			if _, err := s.Read(received[:]); err != nil {
				log.Error("error on read", "error", err)
				close(quit)
				s.Conn().Close()
				return
			}
			switch received[0] {
			case pingMsg:
				s.SetWriteDeadline(time.Now().Add(w))
				if _, err := s.Write(pong[:]); err != nil {
					log.Error("error sending a pong", "error", err)
					close(quit)
					s.Conn().Close()
					return
				}
				c.Update(time.Now())
			case pongMsg:
				c.Update(time.Now())
			default:
				log.Error("received unknown message", "msg", fmt.Sprintf("%x", received))
			}
		}
	}
}
