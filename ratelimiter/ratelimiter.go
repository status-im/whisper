package ratelimiter

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/juju/ratelimit"
	"github.com/syndtr/goleveldb/leveldb"
)

// Interface describes common ratelimiter methods.
type Interface interface {
	Create([]byte, Config) error
	Remove([]byte, time.Duration) error
	TakeAvailable([]byte, int64) int64
	Available([]byte) int64
}

// Config is a set of options used by rate limiter.
type Config struct {
	Interval, Capacity, Quantum uint64
}

func newBucket(c Config) *ratelimit.Bucket {
	return ratelimit.NewBucketWithQuantum(time.Duration(c.Interval), int64(c.Capacity), int64(c.Quantum))
}

// NewPersisted returns instance of rate limiter with persisted black listed records and capacity before peer was removed.
func NewPersisted(db DBInterface) *PersistedRateLimiter {
	return &PersistedRateLimiter{
		db:          db,
		initialized: map[string]*ratelimit.Bucket{},
		now:         time.Now,
	}
}

// PersistedRateLimiter persists latest capacity and updated config per unique ID.
type PersistedRateLimiter struct {
	db DBInterface

	mu          sync.Mutex
	initialized map[string]*ratelimit.Bucket

	now func() time.Time
}

func (r *PersistedRateLimiter) blacklist(id []byte, duration time.Duration) error {
	if duration == 0 {
		return nil
	}
	record := BlacklistRecord{ID: id, Deadline: r.now().Add(duration)}
	if err := record.Write(r.db); err != nil {
		return fmt.Errorf("error blacklisting %#x: %v", id, err)
	}
	return nil
}

func (r *PersistedRateLimiter) get(id []byte) (bucket *ratelimit.Bucket, exist bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, exist = r.initialized[string(id)]
	return
}

func (r *PersistedRateLimiter) isBlacklisted(id []byte) bool {
	bl := BlacklistRecord{ID: id}
	if err := bl.Read(r.db); err != leveldb.ErrNotFound {
		if bl.Deadline.After(r.now()) {
			return true
		}
		bl.Remove(r.db)
	}
	return false
}

func (r *PersistedRateLimiter) adjustBucket(bucket *ratelimit.Bucket, id []byte) {
	capacity := CapacityRecord{ID: id}
	if err := capacity.Read(r.db); err != nil {
		return
	}
	bucket.TakeAvailable(capacity.Taken)
}

// Create creates an instance for a provided ID. If ID was blacklisted error is returned.
func (r *PersistedRateLimiter) Create(id []byte, cfg Config) error {
	if r.isBlacklisted(id) {
		return fmt.Errorf("identity %#x is blacklisted", id)
	}
	bucket := newBucket(cfg)
	r.mu.Lock()
	r.initialized[string(id)] = bucket
	r.mu.Unlock()
	r.adjustBucket(bucket, id)
	// TODO refill rate limiter due to time difference. e.g. if record was stored at T and C seconds passed since T.
	// we need to add RATE_PER_SECOND*C to a bucket
	return nil
}

// Remove removes key from memory but ensures that the latest information is persisted.
func (r *PersistedRateLimiter) Remove(id []byte, duration time.Duration) error {
	if duration != 0 {
		if err := r.blacklist(id, duration); err != nil {
			return err
		}
	}
	r.mu.Lock()
	bucket, exist := r.initialized[string(id)]
	delete(r.initialized, string(id))
	r.mu.Unlock()
	if !exist || bucket == nil {
		return nil
	}
	return r.store(id, bucket)
}

func (r *PersistedRateLimiter) store(id []byte, bucket *ratelimit.Bucket) error {
	capacity := CapacityRecord{
		ID:        id,
		Taken:     bucket.Capacity() - bucket.Available(),
		Timestamp: r.now(),
	}
	if err := capacity.Write(r.db); err != nil {
		return fmt.Errorf("failed to write current capacicity %d for id %x: %v",
			bucket.Capacity(), id, err)
	}
	return nil
}

// TakeAvailable subtracts requested amount from a rate limiter with ID.
func (r *PersistedRateLimiter) TakeAvailable(id []byte, count int64) int64 {
	bucket, exist := r.get(id)
	if !exist {
		return math.MaxInt64
	}
	rst := bucket.TakeAvailable(count)
	if err := r.store(id, bucket); err != nil {
		log.Error(err.Error())
	}
	return rst
}

// Available peeks into available amount with a given ID.
func (r *PersistedRateLimiter) Available(id []byte) int64 {
	bucket, exist := r.get(id)
	if !exist {
		return math.MaxInt64
	}
	return bucket.Available()
}
