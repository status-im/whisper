package whisperv6

import (
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/status-im/whisper/ratelimiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

const (
	testCode = 42 // any non-defined code will work
)

func setupOneConnection(t *testing.T, rlconf, egressConf *ratelimiter.Config) (*Whisper, *p2p.MsgPipeRW, chan error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	require.NoError(t, err)
	rl := ratelimiter.ForWhisper(ratelimiter.IDMode, db, *rlconf)
	conf := &Config{
		MinimumAcceptedPOW: 0,
		MaxMessageSize:     100 << 10,
	}
	w := New(conf)
	w.UseRateLimiter(rl)
	idx, _ := discover.BytesID([]byte{0x01})
	p := p2p.NewPeer(idx, "1", []p2p.Cap{{"shh", 6}})
	rw1, rw2 := p2p.MsgPipe()
	errorc := make(chan error, 1)
	go func() {
		err := w.HandlePeer(p, rw2)
		errorc <- err
	}()
	require.NoError(t, p2p.ExpectMsg(rw1, statusCode, []interface{}{ProtocolVersion, math.Float64bits(w.MinPow()), w.BloomFilter(), false, rlconf}))
	require.NoError(t, p2p.SendItems(rw1, statusCode, ProtocolVersion, math.Float64bits(w.MinPow()), w.BloomFilter(), true, egressConf))
	return w, rw1, errorc
}

func TestRatePeerDropsConnection(t *testing.T) {
	cfg := &ratelimiter.Config{Interval: uint64(time.Hour), Capacity: 10 << 10, Quantum: 1 << 10}
	_, rw1, errorc := setupOneConnection(t, cfg, cfg)

	require.NoError(t, p2p.Send(rw1, testCode, make([]byte, 11<<10))) // limit is 1024
	select {
	case err := <-errorc:
		require.Error(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "failed waiting for HandlePeer to exit")
	}
}

func TestRateLimitedDelivery(t *testing.T) {
	cfg := &ratelimiter.Config{Interval: uint64(time.Hour), Capacity: 3 << 10, Quantum: 1 << 10}
	type testCase struct {
		description string
		cfg         *ratelimiter.Config
		received    int
	}
	for _, tc := range []testCase{
		{
			description: "NoEgress",
			received:    3,
		},
		{
			description: "EgressSmallerThanIngress",
			received:    1,
			cfg:         &ratelimiter.Config{Interval: uint64(time.Hour), Capacity: 2 << 10, Quantum: 1 << 10},
		},
		{
			description: "EgressSameAsIngress",
			received:    2,
			cfg:         cfg,
		},
	} {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			small1 := Envelope{
				Expiry: uint32(time.Now().Add(10 * time.Second).Unix()),
				TTL:    10,
				Topic:  TopicType{1},
				Data:   make([]byte, 1<<10),
				Nonce:  1,
			}
			small2 := small1
			small2.Nonce = 2
			small3 := small1
			small3.Nonce = 3

			w, rw1, _ := setupOneConnection(t, cfg, tc.cfg)

			require.NoError(t, w.Send(&small1))
			require.NoError(t, w.Send(&small2))
			require.NoError(t, w.Send(&small3))

			received := map[uint64]struct{}{}
			// we can not guarantee that all expected envelopes will be delivered in a one batch
			// so allow whisper to write multiple times and read every message
			go func() {
				time.Sleep(3 * time.Second)
				rw1.Close()
			}()
			for {
				msg, err := rw1.ReadMsg()
				if err == p2p.ErrPipeClosed {
					require.Len(t, received, tc.received)
					break
				}
				require.NoError(t, err)
				require.Equal(t, messagesCode, int(msg.Code))
				var rst []*Envelope
				require.NoError(t, msg.Decode(&rst))
				for _, e := range rst {
					received[e.Nonce] = struct{}{}
				}
			}
		})
	}
}

func TestRateRandomizedDelivery(t *testing.T) {
	cfg := &ratelimiter.Config{Interval: uint64(time.Hour), Capacity: 10 << 10, Quantum: 1 << 10}
	w1, rw1, _ := setupOneConnection(t, cfg, cfg)
	w2, rw2, _ := setupOneConnection(t, cfg, cfg)
	w3, rw3, _ := setupOneConnection(t, cfg, cfg)
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		sent     = map[common.Hash]int{}
		received = map[int]int64{}
	)
	for i := uint64(1); i < 15; i++ {
		env := &Envelope{
			Expiry: uint32(time.Now().Add(10 * time.Second).Unix()),
			TTL:    10,
			Topic:  TopicType{1},
			Data:   make([]byte, 1<<10-EnvelopeHeaderLength), // so that 10 envelopes are exactly 10kb
			Nonce:  i,
		}
		sent[env.Hash()] = 0
		for _, w := range []*Whisper{w1, w2, w3} {
			go func(w *Whisper, e *Envelope) {
				time.Sleep(time.Duration(rand.Int63n(10)) * time.Millisecond)
				assert.NoError(t, w.Send(e))
			}(w, env)
		}
	}
	for i, rw := range []*p2p.MsgPipeRW{rw1, rw2, rw3} {
		received[i] = 0
		wg.Add(1)
		go func(rw *p2p.MsgPipeRW) {
			time.Sleep(time.Second)
			rw.Close()
			wg.Done()
		}(rw)
		wg.Add(1)
		go func(i int, rw *p2p.MsgPipeRW) {
			defer wg.Done()
			for {
				msg, err := rw.ReadMsg()
				if err != nil {
					return
				}
				if !assert.Equal(t, uint64(1), msg.Code) {
					return
				}
				var rst []*Envelope
				if !assert.NoError(t, msg.Decode(&rst)) {
					return
				}
				mu.Lock()
				for _, e := range rst {
					received[i] += int64(len(e.Data))
					received[i] += EnvelopeHeaderLength
					sent[e.Hash()]++
				}
				mu.Unlock()
			}
		}(i, rw)
	}
	wg.Wait()
	for i := range received {
		require.Equal(t, int64(10)<<10, received[i], "peer %d didnt' receive 10 kb of data: %d", i, received[i])
	}
	total := 0
	for h := range sent {
		total += sent[h]
	}
	require.Equal(t, 30, total)
}
