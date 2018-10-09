package ratelimiter

import (
	"net"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

const (
	// IDMode enables rate limiting based on peers public key identity.
	IDMode = 1 + iota
	// IPMode enables rate limiting based on peer external ip address.
	IPMode
)

func byIP(peer *p2p.Peer) []byte {
	addr := peer.RemoteAddr().Network()
	ip := net.ParseIP(strings.Split(addr, ":")[0])
	return []byte(ip)
}

func byID(peer *p2p.Peer) []byte {
	return peer.ID().Bytes()
}

// modeFunc specifies function to obtain ID value from peer.
type modeFunc func(peer *p2p.Peer) []byte

// selectFunc returns idModeFunc by default.
func selectFunc(mode int) modeFunc {
	if mode == IPMode {
		return byIP
	}
	return byID
}

// NewP2PRateLimiter returns an instance of P2PRateLimiter.
func NewP2PRateLimiter(mode int, ratelimiter Interface) P2PRateLimiter {
	return P2PRateLimiter{
		getID:       selectFunc(mode),
		ratelimiter: ratelimiter,
	}
}

// P2PRateLimiter implements rate limiter that accepts p2p.Peer as identifier.
type P2PRateLimiter struct {
	getID       modeFunc
	ratelimiter Interface
}

// Create instantiates rate limiter with for a peer.
func (r P2PRateLimiter) Create(peer *p2p.Peer, cfg Config) error {
	return r.ratelimiter.Create(r.getID(peer), cfg)
}

// Remove drops peer from in-memory rate limiter. If duration is non-zero peer will be blacklisted.
func (r P2PRateLimiter) Remove(peer *p2p.Peer, duration time.Duration) error {
	return r.ratelimiter.Remove(r.getID(peer), duration)
}

// TakeAvailable subtracts given amount up to the available limit.
func (r P2PRateLimiter) TakeAvailable(peer *p2p.Peer, count int64) int64 {
	return r.ratelimiter.TakeAvailable(r.getID(peer), count)
}

// Available peeks into the current available limit.
func (r P2PRateLimiter) Available(peer *p2p.Peer) int64 {
	return r.ratelimiter.Available(r.getID(peer))
}

// Whisper is a convenience wrapper for whisper.
type Whisper struct {
	Ingress, Egress P2PRateLimiter
	Config          Config
}

// ForWhisper returns a convenient wrapper to be used in whisper.
func ForWhisper(mode int, db DBInterface, ingress Config) Whisper {
	return Whisper{
		Ingress: NewP2PRateLimiter(mode, NewPersisted(WithPrefix(db, []byte("i")))),
		Egress:  NewP2PRateLimiter(mode, NewPersisted(WithPrefix(db, []byte("e")))),
		Config:  ingress,
	}
}
