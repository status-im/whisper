package whisperv6

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p"
)

type runLoop func(p *Peer, rw p2p.MsgReadWriter) error

type peerRateLimiter struct{}

func (r *peerRateLimiter) Decorator(p *Peer, rw p2p.MsgReadWriter, runLoop runLoop) error {
	in, out := p2p.MsgPipe()
	defer in.Close()
	defer out.Close()
	errC := make(chan error, 1)

	go func() {
		for {
			packet, err := rw.ReadMsg()
			if err != nil {
				errC <- fmt.Errorf("failed to read packet: %v", err)
				return
			}

			// TODO: check rate limit for IP and peer ID.
			// TODO: use whitelisting for cluster peers.

			if err := in.WriteMsg(packet); err != nil {
				errC <- fmt.Errorf("failed to write packet: %v", err)
				return
			}
		}
	}()

	go func() {
		errC <- runLoop(p, out)
	}()

	return <-errC
}
