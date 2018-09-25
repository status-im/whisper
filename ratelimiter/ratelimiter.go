package ratelimiter

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/juju/ratelimit"
	"github.com/status-im/status-go/db"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// Interface describes common interface methods.
type Interface interface {
	Create([]byte) error
	Remove([]byte, time.Duration) error
	TakeAvailable([]byte, int64) int64
	Available([]byte) int64
	UpdateConfig([]byte, Config) error
	Config() Config
}

// DBInterface defines leveldb methods used by ratelimiter.
type DBInterface interface {
	Put(key, value []byte, wo *opt.WriteOptions) error
	Get(key []byte, ro *opt.ReadOptions) (value []byte, err error)
	Delete(key []byte, wo *opt.WriteOptions) error
}

// Config is a set of options used by rate limiter.
type Config struct {
	Interval, Capacity, Quantum uint64
}

// compare config with existing ratelimited bucket.
func compare(c Config, bucket *ratelimit.Bucket) bool {
	return int64(c.Capacity) == bucket.Capacity() &&
		1e9*float64(c.Quantum)/float64(c.Interval) == bucket.Rate()
}

func newBucket(c Config) *ratelimit.Bucket {
	return ratelimit.NewBucketWithQuantum(time.Duration(c.Interval), int64(c.Capacity), int64(c.Quantum))
}

func NewPersisted(db DBInterface, config Config, prefix []byte) *PersistedRateLimiter {
	return &PersistedRateLimiter{
		db:            db,
		defaultConfig: config,
		initialized:   map[string]*ratelimit.Bucket{},
		prefix:        prefix,
		timeFunc:      time.Now,
	}
}

// PersistedRateLimiter persists latest capacity and updated config per unique ID.
type PersistedRateLimiter struct {
	db            DBInterface
	prefix        []byte // TODO move prefix outside of the rate limiter using database interface
	defaultConfig Config

	mu          sync.Mutex
	initialized map[string]*ratelimit.Bucket

	timeFunc func() time.Time
}

func (r *PersistedRateLimiter) blacklist(id []byte, duration time.Duration) error {
	fkey := db.Key(db.RateLimitBlacklist, r.prefix, id)
	buf := [8]byte{}
	binary.BigEndian.PutUint64(buf[:], uint64(r.timeFunc().Add(duration).Unix()))
	if err := r.db.Put(fkey, buf[:], nil); err != nil {
		return fmt.Errorf("error blacklisting %x: %v", id, err)
	}
	return nil
}

func (r *PersistedRateLimiter) Config() Config {
	return r.defaultConfig
}

func (r *PersistedRateLimiter) getOrCreate(id []byte, config Config) (bucket *ratelimit.Bucket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, exist := r.initialized[string(id)]
	if !exist {
		bucket = newBucket(config)
		r.initialized[string(id)] = bucket
	} else {
		bucket = old
	}
	return
}

func (r *PersistedRateLimiter) Create(id []byte) error {
	fkey := db.Key(db.RateLimitBlacklist, r.prefix, id)
	val, err := r.db.Get(fkey, nil)
	if err != leveldb.ErrNotFound {
		deadline := binary.BigEndian.Uint64(val)
		if deadline >= uint64(r.timeFunc().Unix()) {
			return fmt.Errorf("identity %x is blacklisted", id)
		}
		r.db.Delete(fkey, nil)
	}
	fkey = db.Key(db.RateLimitConfig, r.prefix, id)
	val, err = r.db.Get(fkey, nil)
	var cfg Config
	if err == leveldb.ErrNotFound {
		cfg = r.defaultConfig
	} else if err != nil {
		log.Error("faield to read config from db. using default", "err", err)
		cfg = r.defaultConfig
	} else {
		if err := rlp.DecodeBytes(val, &cfg); err != nil {
			log.Error("failed to decode config. using default", "err", err)
			cfg = r.defaultConfig
		}
	}
	bucket := r.getOrCreate(id, cfg)
	fkey = db.Key(db.RateLimitCapacity, r.prefix, id)
	val, err = r.db.Get(fkey, nil)
	if err == leveldb.ErrNotFound {
		return nil
	} else if len(val) != 16 {
		log.Error("stored value is of unexpected length", "expected", 8, "stored", len(val))
		return nil
	}
	bucket.TakeAvailable(int64(binary.BigEndian.Uint64(val[:8])))
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
	buf := [16]byte{}
	binary.BigEndian.PutUint64(buf[:], uint64(bucket.Capacity()-bucket.Available()))
	binary.BigEndian.PutUint64(buf[8:], uint64(r.timeFunc().Unix()))
	err := r.db.Put(db.Key(db.RateLimitCapacity, r.prefix, id), buf[:], nil)
	if err != nil {
		return fmt.Errorf("failed to write current capacicity %d for id %x: %v",
			bucket.Capacity(), id, err)
	}
	return nil
}

func (r *PersistedRateLimiter) TakeAvailable(id []byte, count int64) int64 {
	bucket := r.getOrCreate(id, r.defaultConfig)
	rst := bucket.TakeAvailable(count)
	if err := r.store(id, bucket); err != nil {
		log.Error(err.Error())
	}
	return rst
}

func (r *PersistedRateLimiter) Available(id []byte) int64 {
	return r.getOrCreate(id, r.defaultConfig).Available()
}

func (r *PersistedRateLimiter) UpdateConfig(id []byte, config Config) error {
	r.mu.Lock()
	old, _ := r.initialized[string(id)]
	if compare(config, old) {
		r.mu.Unlock()
		return nil
	}
	delete(r.initialized, string(id))
	r.mu.Unlock()
	taken := int64(0)
	if old != nil {
		taken = old.Capacity() - old.Available()
	}
	r.getOrCreate(id, config).TakeAvailable(taken)
	fkey := db.Key(db.RateLimitConfig, r.prefix, id)
	data, err := rlp.EncodeToBytes(config)
	if err != nil {
		log.Error("failed to update config", "cfg", config, "err", err)
		return nil
	}
	r.db.Put(fkey, data, nil)
	return nil
}
