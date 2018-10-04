package ratelimiter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

func TestLimitIsPersisted(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	require.NoError(t, err)
	var (
		total int64 = 10000
		cfg         = Config{1 << 62, uint64(10000), 1}
		rl          = NewPersisted(db)
		tid         = []byte("test")
	)
	require.NoError(t, rl.Create(tid, cfg))
	taken := rl.TakeAvailable(tid, total/2)
	require.Equal(t, total/2, taken)
	require.NoError(t, rl.Remove(tid, 0))
	require.NoError(t, rl.Create(tid, cfg))
	require.Equal(t, total/2, rl.Available(tid))
}

func TestBlacklistedEntityReturnsError(t *testing.T) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	require.NoError(t, err)
	var (
		cfg = Config{1 << 62, uint64(10000), 1}
		rl  = NewPersisted(db)
		tid = []byte("test")
	)
	require.NoError(t, rl.Create(tid, cfg))
	require.NoError(t, rl.Remove(tid, 10*time.Minute))
	require.EqualError(t, fmt.Errorf("identity %x is blacklisted", tid), rl.Create(tid, cfg).Error())
	rl.timeFunc = func() time.Time {
		return time.Now().Add(11 * time.Minute)
	}
	require.NoError(t, rl.Create(tid, cfg))
}
