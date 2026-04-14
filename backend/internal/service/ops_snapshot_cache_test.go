package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsSnapshotMapCache_ExpiresAndEvictsEntry(t *testing.T) {
	cache := &opsSnapshotMapCache[string, string]{}
	base := time.Unix(1_700_000_000, 0)

	cache.set("fresh", "value", 3*time.Second, base)

	got, ok := cache.get("fresh", base.Add(3*time.Second))
	require.True(t, ok)
	require.Equal(t, "value", got)

	got, ok = cache.get("fresh", base.Add(3*time.Second).Add(time.Nanosecond))
	require.False(t, ok)
	require.Empty(t, got)
	require.NotContains(t, cache.entries, "fresh")
}

func TestOpsSnapshotValueCache_ExpiresAndClearsEntry(t *testing.T) {
	cache := &opsSnapshotValueCache[string]{}
	base := time.Unix(1_700_100_000, 0)

	cache.set("value", 5*time.Second, base)

	got, ok := cache.get(base.Add(5 * time.Second))
	require.True(t, ok)
	require.Equal(t, "value", got)

	got, ok = cache.get(base.Add(5 * time.Second).Add(time.Nanosecond))
	require.False(t, ok)
	require.Empty(t, got)
	require.Nil(t, cache.entry)
}
