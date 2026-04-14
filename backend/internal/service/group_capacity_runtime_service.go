package service

import (
	"context"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

const (
	defaultGroupCapacityRuntimeTTL       = 2 * time.Second
	defaultGroupCapacityRuntimeChunkSize = 100
)

type groupCapacityRuntimeProviderService struct {
	concurrency *ConcurrencyService
	sessionLim  SessionLimitCache
	rpm         RPMCache
	cache       *gocache.Cache
	ttl         time.Duration
	chunkSize   int
	sf          singleflight.Group
	now         func() time.Time
}

type groupCapacityRuntimeCacheEntry struct {
	usage     GroupCapacityRuntimeUsage
	expiresAt time.Time
}

func NewGroupCapacityRuntimeProviderService(
	concurrencyService *ConcurrencyService,
	sessionLimitCache SessionLimitCache,
	rpmCache RPMCache,
	ttl time.Duration,
	chunkSize int,
) *groupCapacityRuntimeProviderService {
	if ttl <= 0 {
		ttl = defaultGroupCapacityRuntimeTTL
	}
	if chunkSize <= 0 {
		chunkSize = defaultGroupCapacityRuntimeChunkSize
	}
	return &groupCapacityRuntimeProviderService{
		concurrency: concurrencyService,
		sessionLim:  sessionLimitCache,
		rpm:         rpmCache,
		cache:       gocache.New(ttl, time.Minute),
		ttl:         ttl,
		chunkSize:   chunkSize,
		now:         time.Now,
	}
}

func (s *groupCapacityRuntimeProviderService) GetGroupCapacityRuntimeUsage(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error) {
	if s == nil {
		return GroupCapacityRuntimeUsage{}, ErrGroupCapacityProviderUnavailable
	}
	cacheKey := s.cacheKey(snapshot)
	if usage, ok := s.getCachedUsage(cacheKey); ok {
		return usage, nil
	}

	value, err, _ := s.sf.Do(cacheKey, func() (any, error) {
		if usage, ok := s.getCachedUsage(cacheKey); ok {
			return usage, nil
		}
		usage, loadErr := s.loadRuntimeUsage(ctx, snapshot)
		if loadErr != nil {
			return nil, loadErr
		}
		s.setCachedUsage(cacheKey, usage)
		return usage, nil
	})
	if err != nil {
		return GroupCapacityRuntimeUsage{}, err
	}
	usage, ok := value.(GroupCapacityRuntimeUsage)
	if !ok {
		return GroupCapacityRuntimeUsage{}, fmt.Errorf("group capacity runtime: unexpected singleflight value %T", value)
	}
	return usage, nil
}

func (s *groupCapacityRuntimeProviderService) loadRuntimeUsage(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error) {
	usage := GroupCapacityRuntimeUsage{GroupID: snapshot.GroupID}

	concurrencyIDs := selectGroupCapacityAllAccounts(snapshot)
	if len(concurrencyIDs) > 0 && s.concurrency != nil {
		concurrencyUsed, err := s.loadConcurrencyUsed(ctx, concurrencyIDs)
		if err != nil {
			return GroupCapacityRuntimeUsage{}, err
		}
		usage.ConcurrencyUsed = concurrencyUsed
	}

	sessionIDs, sessionTimeouts := selectGroupCapacitySessionAccounts(snapshot)
	if len(sessionIDs) > 0 && s.sessionLim != nil {
		sessionsUsed, err := s.loadSessionsUsed(ctx, sessionIDs, sessionTimeouts)
		if err != nil {
			return GroupCapacityRuntimeUsage{}, err
		}
		usage.ActiveSessions = sessionsUsed
	}

	rpmIDs := selectGroupCapacityRPMAccounts(snapshot)
	if len(rpmIDs) > 0 && s.rpm != nil {
		rpmUsed, err := s.loadRPMUsed(ctx, rpmIDs)
		if err != nil {
			return GroupCapacityRuntimeUsage{}, err
		}
		usage.CurrentRPM = rpmUsed
	}

	return usage, nil
}

func (s *groupCapacityRuntimeProviderService) loadConcurrencyUsed(ctx context.Context, accountIDs []int64) (int, error) {
	var total int
	for _, chunk := range chunkGroupCapacityAccountIDs(accountIDs, s.chunkSize) {
		accounts := make([]AccountWithConcurrency, 0, len(chunk))
		for _, accountID := range chunk {
			accounts = append(accounts, AccountWithConcurrency{ID: accountID})
		}
		loads, err := s.concurrency.GetAccountsLoadBatchFast(ctx, accounts)
		if err != nil {
			return 0, err
		}
		for _, accountID := range chunk {
			if load := loads[accountID]; load != nil {
				total += load.CurrentConcurrency
			}
		}
	}
	return total, nil
}

func (s *groupCapacityRuntimeProviderService) loadSessionsUsed(ctx context.Context, accountIDs []int64, timeouts map[int64]time.Duration) (int, error) {
	var total int
	for _, chunk := range chunkGroupCapacityAccountIDs(accountIDs, s.chunkSize) {
		chunkTimeouts := subsetSessionTimeouts(chunk, timeouts)
		counts, err := s.sessionLim.GetActiveSessionCountBatch(ctx, chunk, chunkTimeouts)
		if err != nil {
			return 0, err
		}
		for _, accountID := range chunk {
			total += counts[accountID]
		}
	}
	return total, nil
}

func (s *groupCapacityRuntimeProviderService) loadRPMUsed(ctx context.Context, accountIDs []int64) (int, error) {
	var total int
	for _, chunk := range chunkGroupCapacityAccountIDs(accountIDs, s.chunkSize) {
		counts, err := s.rpm.GetRPMBatch(ctx, chunk)
		if err != nil {
			return 0, err
		}
		for _, accountID := range chunk {
			total += counts[accountID]
		}
	}
	return total, nil
}

func (s *groupCapacityRuntimeProviderService) getCachedUsage(cacheKey string) (GroupCapacityRuntimeUsage, bool) {
	if s.cache == nil {
		return GroupCapacityRuntimeUsage{}, false
	}
	cached, ok := s.cache.Get(cacheKey)
	if !ok {
		return GroupCapacityRuntimeUsage{}, false
	}
	entry, ok := cached.(groupCapacityRuntimeCacheEntry)
	if !ok {
		return GroupCapacityRuntimeUsage{}, false
	}
	if !entry.expiresAt.After(s.now()) {
		s.cache.Delete(cacheKey)
		return GroupCapacityRuntimeUsage{}, false
	}
	return entry.usage, true
}

func (s *groupCapacityRuntimeProviderService) setCachedUsage(cacheKey string, usage GroupCapacityRuntimeUsage) {
	if s.cache == nil {
		return
	}
	s.cache.Set(cacheKey, groupCapacityRuntimeCacheEntry{
		usage:     usage,
		expiresAt: s.now().Add(s.ttl),
	}, s.ttl)
}

func (s *groupCapacityRuntimeProviderService) cacheKey(snapshot GroupCapacityStaticSnapshot) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatInt(snapshot.GroupID, 10)))
	writeInt64SliceHash(h, selectGroupCapacityAllAccounts(snapshot))
	writeInt64SliceHash(h, selectGroupCapacitySessionAccountsOnly(snapshot))
	writeInt64SliceHash(h, selectGroupCapacityRPMAccountsOnly(snapshot))
	writeSessionTimeoutHash(h, snapshot.SessionTimeouts)
	return fmt.Sprintf("group-capacity-runtime:%x", h.Sum64())
}

func selectGroupCapacityAllAccounts(snapshot GroupCapacityStaticSnapshot) []int64 {
	if len(snapshot.AllAccountIDs) > 0 {
		return snapshot.AllAccountIDs
	}
	return snapshot.AccountIDs
}

func selectGroupCapacitySessionAccounts(snapshot GroupCapacityStaticSnapshot) ([]int64, map[int64]time.Duration) {
	if len(snapshot.SessionLimitedAccountIDs) > 0 {
		return snapshot.SessionLimitedAccountIDs, snapshot.SessionTimeouts
	}
	if len(snapshot.SessionTimeouts) > 0 {
		ids := make([]int64, 0, len(snapshot.SessionTimeouts))
		for accountID := range snapshot.SessionTimeouts {
			ids = append(ids, accountID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		return ids, snapshot.SessionTimeouts
	}
	return nil, nil
}

func selectGroupCapacitySessionAccountsOnly(snapshot GroupCapacityStaticSnapshot) []int64 {
	if len(snapshot.SessionLimitedAccountIDs) > 0 {
		return snapshot.SessionLimitedAccountIDs
	}
	if len(snapshot.SessionTimeouts) > 0 {
		ids := make([]int64, 0, len(snapshot.SessionTimeouts))
		for accountID := range snapshot.SessionTimeouts {
			ids = append(ids, accountID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		return ids
	}
	return nil
}

func selectGroupCapacityRPMAccounts(snapshot GroupCapacityStaticSnapshot) []int64 {
	if len(snapshot.RPMLimitedAccountIDs) > 0 {
		return snapshot.RPMLimitedAccountIDs
	}
	return selectGroupCapacityAllAccounts(snapshot)
}

func selectGroupCapacityRPMAccountsOnly(snapshot GroupCapacityStaticSnapshot) []int64 {
	if len(snapshot.RPMLimitedAccountIDs) > 0 {
		return snapshot.RPMLimitedAccountIDs
	}
	return nil
}

func chunkGroupCapacityAccountIDs(accountIDs []int64, chunkSize int) [][]int64 {
	if len(accountIDs) == 0 {
		return nil
	}
	if chunkSize <= 0 || len(accountIDs) <= chunkSize {
		return [][]int64{append([]int64(nil), accountIDs...)}
	}
	chunks := make([][]int64, 0, (len(accountIDs)+chunkSize-1)/chunkSize)
	for start := 0; start < len(accountIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(accountIDs) {
			end = len(accountIDs)
		}
		chunks = append(chunks, append([]int64(nil), accountIDs[start:end]...))
	}
	return chunks
}

func subsetSessionTimeouts(accountIDs []int64, timeouts map[int64]time.Duration) map[int64]time.Duration {
	if len(accountIDs) == 0 || len(timeouts) == 0 {
		return nil
	}
	subset := make(map[int64]time.Duration, len(accountIDs))
	for _, accountID := range accountIDs {
		if timeout, ok := timeouts[accountID]; ok {
			subset[accountID] = timeout
		}
	}
	if len(subset) == 0 {
		return nil
	}
	return subset
}

func writeInt64SliceHash(h hash.Hash64, values []int64) {
	if h == nil {
		return
	}
	for _, value := range values {
		_, _ = h.Write([]byte(strconv.FormatInt(value, 10)))
		_, _ = h.Write([]byte{','})
	}
	_, _ = h.Write([]byte{'|'})
}

func writeSessionTimeoutHash(h hash.Hash64, timeouts map[int64]time.Duration) {
	if h == nil || len(timeouts) == 0 {
		return
	}
	keys := make([]int64, 0, len(timeouts))
	for accountID := range timeouts {
		keys = append(keys, accountID)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	builder := strings.Builder{}
	for _, accountID := range keys {
		_, _ = builder.WriteString(strconv.FormatInt(accountID, 10))
		_ = builder.WriteByte('=')
		_, _ = builder.WriteString(strconv.FormatInt(int64(timeouts[accountID]), 10))
		_ = builder.WriteByte(',')
	}
	_, _ = h.Write([]byte(builder.String()))
}
