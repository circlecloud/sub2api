package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

const (
	defaultOpenAIWarmPoolBucketTargetSize      = 10
	defaultOpenAIWarmPoolBucketRefillBelow     = 3
	defaultOpenAIWarmPoolBucketSyncFillMin     = 1
	defaultOpenAIWarmPoolBucketEntryTTL        = 30 * time.Second
	defaultOpenAIWarmPoolBucketRefillCooldown  = 15 * time.Second
	defaultOpenAIWarmPoolBucketRefillInterval  = 30 * time.Second
	defaultOpenAIWarmPoolGlobalTargetSize      = 30
	defaultOpenAIWarmPoolGlobalRefillBelow     = 10
	defaultOpenAIWarmPoolGlobalEntryTTL        = 5 * time.Minute
	defaultOpenAIWarmPoolGlobalRefillCooldown  = 60 * time.Second
	defaultOpenAIWarmPoolGlobalRefillInterval  = 5 * time.Minute
	defaultOpenAIWarmPoolNetworkErrorPoolSize  = 3
	defaultOpenAIWarmPoolNetworkErrorEntryTTL  = 2 * time.Minute
	defaultOpenAIWarmPoolProbeMaxCandidates    = 24
	defaultOpenAIWarmPoolProbeConcurrency      = 4
	defaultOpenAIWarmPoolProbeTimeout          = 15 * time.Second
	defaultOpenAIWarmPoolProbeFailureCooldown  = 2 * time.Minute
	defaultOpenAIWarmPoolActiveBucketTTL       = 30 * time.Minute
	defaultOpenAIWarmPoolWorkerTick            = 5 * time.Second
	defaultOpenAIWarmPoolStartupBootstrapRetry = time.Second
	defaultOpenAIWarmPoolStartupGateRetry      = 30 * time.Second
	defaultOpenAIWarmPoolStartupGateTimeout    = 10 * time.Second
	defaultOpenAIWarmPoolStartupProxyID        = int64(1)
	openAIWarmPoolMirrorWriteTimeout           = time.Second
	openAIWarmPoolMirrorQueueSize              = 1024
	openAIWarmPoolMirrorWorkerCount            = 2
	openAIWarmPoolMirrorDropLogInterval        = time.Minute
	openAIWarmPoolLogComponent                 = "service.openai_warm_pool"
)

const (
	openAIWarmPoolProbeReadyOutcome            = "ready"
	openAIWarmPoolProbeFailedOutcome           = "failed"
	openAIWarmPoolProbeSkippedOutcome          = "skipped"
	openAIWarmPoolProbeNetworkErrorOutcome     = "network_error"
	openAIWarmPoolProbeNetworkRetryMaxAttempts = 2
)

type openAIWarmPoolUsageReader interface {
	GetUsage(ctx context.Context, accountID int64, forceRefresh bool) (*UsageInfo, error)
}

type openAIWarmPoolConfigView struct {
	Enabled bool

	BucketTargetSize     int
	BucketRefillBelow    int
	BucketSyncFillMin    int
	BucketEntryTTL       time.Duration
	BucketRefillCooldown time.Duration
	BucketRefillInterval time.Duration

	GlobalTargetSize     int
	GlobalRefillBelow    int
	GlobalEntryTTL       time.Duration
	GlobalRefillCooldown time.Duration
	GlobalRefillInterval time.Duration

	NetworkErrorPoolSize int
	NetworkErrorEntryTTL time.Duration

	ProbeMaxCandidates int
	ProbeConcurrency   int
	ProbeTimeout       time.Duration
	FailureCooldown    time.Duration
	ActiveBucketTTL    time.Duration
	StartupGroupIDs    []int64
}

type openAIWarmPoolBucketEntry struct {
	accountID  int64
	promotedAt time.Time
	refreshAt  time.Time
}

type openAIWarmPoolBucketState struct {
	groupID int64
	mu      sync.Mutex
	entries map[int64]openAIWarmPoolBucketEntry

	lastAccess          atomic.Int64
	lastRefill          atomic.Int64
	takeCount           atomic.Int64
	globalBootstrapDone atomic.Bool
}

func (b *openAIWarmPoolBucketState) ensureEntriesLocked() {
	if b.entries == nil {
		b.entries = make(map[int64]openAIWarmPoolBucketEntry)
	}
}

func openAIWarmPoolBucketRefreshAt(now time.Time, ttl time.Duration, accountID int64) time.Time {
	if ttl <= 0 {
		return now
	}
	refreshAt := now.Add(ttl)
	jitterWindow := ttl / 4
	if jitterWindow > 15*time.Second {
		jitterWindow = 15 * time.Second
	}
	if jitterWindow < time.Second {
		return refreshAt
	}
	steps := int64(jitterWindow / time.Second)
	if steps <= 0 {
		return refreshAt
	}
	jitter := time.Duration(accountID%steps) * time.Second
	return refreshAt.Add(jitter)
}

func (b *openAIWarmPoolBucketState) promote(accountID int64, now time.Time, ttl time.Duration) bool {
	if b == nil || accountID <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureEntriesLocked()
	if _, exists := b.entries[accountID]; exists {
		return false
	}
	b.entries[accountID] = openAIWarmPoolBucketEntry{
		accountID:  accountID,
		promotedAt: now,
		refreshAt:  openAIWarmPoolBucketRefreshAt(now, ttl, accountID),
	}
	return true
}

func (b *openAIWarmPoolBucketState) touch(accountID int64, now time.Time, ttl time.Duration) {
	if b == nil || accountID <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureEntriesLocked()
	entry, exists := b.entries[accountID]
	if !exists {
		entry = openAIWarmPoolBucketEntry{accountID: accountID, promotedAt: now}
	}
	if entry.promotedAt.IsZero() {
		entry.promotedAt = now
	}
	entry.refreshAt = openAIWarmPoolBucketRefreshAt(now, ttl, accountID)
	b.entries[accountID] = entry
}

func (b *openAIWarmPoolBucketState) remove(accountID int64) {
	if b == nil || accountID <= 0 {
		return
	}
	b.mu.Lock()
	delete(b.entries, accountID)
	b.mu.Unlock()
}

func (b *openAIWarmPoolBucketState) readyIDs() []int64 {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureEntriesLocked()
	ids := make([]int64, 0, len(b.entries))
	for accountID := range b.entries {
		ids = append(ids, accountID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (b *openAIWarmPoolBucketState) dueIDs(now time.Time) []int64 {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureEntriesLocked()
	ids := make([]int64, 0, len(b.entries))
	for accountID, entry := range b.entries {
		if entry.refreshAt.IsZero() || !now.Before(entry.refreshAt) {
			ids = append(ids, accountID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (b *openAIWarmPoolBucketState) readyCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureEntriesLocked()
	return len(b.entries)
}

type openAIWarmAccountState struct {
	mu sync.Mutex

	probing           bool
	verifiedAt        time.Time
	expiresAt         time.Time
	failUntil         time.Time
	networkErrorAt    time.Time
	networkErrorUntil time.Time
}

func (s *openAIWarmAccountState) cleanupLocked(now time.Time) {
	if !s.failUntil.IsZero() && !now.Before(s.failUntil) {
		s.failUntil = time.Time{}
	}
	if !s.networkErrorUntil.IsZero() && !now.Before(s.networkErrorUntil) {
		s.networkErrorAt = time.Time{}
		s.networkErrorUntil = time.Time{}
	}
}

func (s *openAIWarmAccountState) snapshot(now time.Time) (ready bool, cooling bool, networkError bool, expired bool) {
	if s == nil {
		return false, false, false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	if !s.failUntil.IsZero() && now.Before(s.failUntil) {
		cooling = true
	}
	if !s.networkErrorUntil.IsZero() && now.Before(s.networkErrorUntil) {
		networkError = true
	}
	if !s.verifiedAt.IsZero() && !cooling && !networkError {
		ready = true
		if !s.expiresAt.IsZero() && !now.Before(s.expiresAt) {
			expired = true
		}
	}
	return ready, cooling, networkError, expired
}

func (s *openAIWarmAccountState) inspect(now time.Time) openAIWarmAccountInspection {
	inspection := openAIWarmAccountInspection{}
	if s == nil {
		return inspection
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	inspection.Probing = s.probing
	if !s.failUntil.IsZero() {
		failUntil := s.failUntil.UTC()
		inspection.FailUntil = &failUntil
		if now.Before(s.failUntil) {
			inspection.Cooling = true
		}
	}
	if !s.networkErrorAt.IsZero() {
		networkErrorAt := s.networkErrorAt.UTC()
		inspection.NetworkErrorAt = &networkErrorAt
	}
	if !s.networkErrorUntil.IsZero() {
		networkErrorUntil := s.networkErrorUntil.UTC()
		inspection.NetworkErrorUntil = &networkErrorUntil
		if now.Before(s.networkErrorUntil) {
			inspection.NetworkError = true
		}
	}
	if !s.verifiedAt.IsZero() {
		verifiedAt := s.verifiedAt.UTC()
		inspection.VerifiedAt = &verifiedAt
		if !inspection.Cooling && !inspection.NetworkError {
			inspection.Ready = true
		}
	}
	if !s.expiresAt.IsZero() {
		expiresAt := s.expiresAt.UTC()
		inspection.ExpiresAt = &expiresAt
		if inspection.Ready && !now.Before(s.expiresAt) {
			inspection.Expired = true
		}
	}
	return inspection
}

func (s *openAIWarmAccountState) tryStartProbe(now time.Time, allowWhileReady bool) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	if s.probing {
		return false
	}
	if !s.failUntil.IsZero() && now.Before(s.failUntil) {
		return false
	}
	if !s.networkErrorUntil.IsZero() && now.Before(s.networkErrorUntil) {
		return false
	}
	if !allowWhileReady && !s.expiresAt.IsZero() && now.Before(s.expiresAt) {
		return false
	}
	s.probing = true
	return true
}

func (s *openAIWarmAccountState) finishSuccess(now time.Time, ttl time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probing = false
	s.verifiedAt = now
	if ttl > 0 {
		s.expiresAt = now.Add(ttl)
	} else {
		s.expiresAt = now
	}
	s.failUntil = time.Time{}
	s.networkErrorAt = time.Time{}
	s.networkErrorUntil = time.Time{}
}

func (s *openAIWarmAccountState) clearReady() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verifiedAt = time.Time{}
	s.expiresAt = time.Time{}
}

func (s *openAIWarmAccountState) finishFailure(now time.Time, cooldown time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probing = false
	s.verifiedAt = time.Time{}
	s.expiresAt = time.Time{}
	s.networkErrorAt = time.Time{}
	s.networkErrorUntil = time.Time{}
	if cooldown > 0 {
		s.failUntil = now.Add(cooldown)
	} else {
		s.failUntil = now
	}
}

func (s *openAIWarmAccountState) markNetworkError(now time.Time, ttl time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probing = false
	s.verifiedAt = time.Time{}
	s.expiresAt = time.Time{}
	s.failUntil = time.Time{}
	s.networkErrorAt = now
	if ttl > 0 {
		s.networkErrorUntil = now.Add(ttl)
	} else {
		s.networkErrorUntil = now
	}
}

func (s *openAIWarmAccountState) abortProbe() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.probing = false
	s.mu.Unlock()
}

type openAIWarmPoolProbeResult struct {
	Outcome string
	Reason  string
	Err     error
}

type openAIWarmPoolMirrorWriteTask struct {
	label    string
	critical bool
	run      func(ctx context.Context) error
}

type openAIAccountWarmPoolService struct {
	service *OpenAIGatewayService

	usageReaderMu sync.RWMutex
	usageReader   openAIWarmPoolUsageReader

	accountStates sync.Map // key: int64(accountID) -> *openAIWarmAccountState
	bucketStates  sync.Map // key: int64(groupID) -> *openAIWarmPoolBucketState

	refillSF           singleflight.Group
	startupProxyGateSF singleflight.Group

	lastBucketMaintenance     atomic.Int64
	lastGlobalMaintenance     atomic.Int64
	lastGlobalRefill          atomic.Int64
	totalTakeCount            atomic.Int64
	startupGateLastCheck      atomic.Int64
	startupGateReady          atomic.Bool
	startupBootstrapping      atomic.Bool
	startupBootstrapRequested atomic.Bool
	startupBootstrapRunning   atomic.Bool
	startupBootstrapDone      atomic.Bool
	workerStartOnce           sync.Once

	instanceID string
	stopCh     chan struct{}
	stopOnce   sync.Once

	mirrorWriteCh chan openAIWarmPoolMirrorWriteTask
	mirrorDropMu  sync.Mutex
	mirrorDropAt  time.Time
}

func newOpenAIAccountWarmPoolService(service *OpenAIGatewayService) *openAIAccountWarmPoolService {
	return &openAIAccountWarmPoolService{
		service:       service,
		instanceID:    uuid.NewString(),
		stopCh:        make(chan struct{}),
		mirrorWriteCh: make(chan openAIWarmPoolMirrorWriteTask, openAIWarmPoolMirrorQueueSize),
	}
}

func (p *openAIAccountWarmPoolService) Start() {
	if p == nil {
		return
	}
	p.workerStartOnce.Do(func() {
		go p.runRefillWorker()
		for i := 0; i < openAIWarmPoolMirrorWorkerCount; i++ {
			go p.runMirrorWriteWorker()
		}
	})
}

func (p *openAIAccountWarmPoolService) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
}

func (p *openAIAccountWarmPoolService) SetUsageReader(reader openAIWarmPoolUsageReader) {
	if p == nil {
		return
	}
	p.usageReaderMu.Lock()
	p.usageReader = reader
	p.usageReaderMu.Unlock()
	if reader == nil {
		p.startupBootstrapping.Store(false)
		return
	}
	p.Start()
	if !p.startupBootstrapDone.Load() {
		p.triggerStartupBootstrap()
	}
}

func (p *openAIAccountWarmPoolService) getUsageReader() openAIWarmPoolUsageReader {
	if p == nil {
		return nil
	}
	p.usageReaderMu.RLock()
	defer p.usageReaderMu.RUnlock()
	return p.usageReader
}

func (p *openAIAccountWarmPoolService) isStartupBootstrapping() bool {
	return p != nil && p.startupBootstrapping.Load()
}

func (p *openAIAccountWarmPoolService) opsWarmPoolStatsCacheRevision() int64 {
	if p == nil {
		return 0
	}
	return p.lastGlobalMaintenance.Load()
}

func (p *openAIAccountWarmPoolService) invalidateWarmPoolOverviewSnapshot(ctx context.Context) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = p.service.opsRealtimeCache.DeleteWarmPoolOverviewSnapshot(ctx)
}

func (p *openAIAccountWarmPoolService) waitStartupBootstrapRetry(delay time.Duration) bool {
	if p == nil {
		return false
	}
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-p.stopCh:
		p.startupBootstrapping.Store(false)
		return false
	case <-timer.C:
		return true
	}
}

func (p *openAIAccountWarmPoolService) triggerStartupBootstrap() {
	if p == nil || p.service == nil || p.getUsageReader() == nil || p.startupBootstrapDone.Load() {
		return
	}
	if len(p.config().StartupGroupIDs) == 0 {
		p.startupBootstrapping.Store(false)
		return
	}
	p.startupBootstrapRequested.Store(true)
	if !p.startupBootstrapRunning.CompareAndSwap(false, true) {
		return
	}
	p.startupBootstrapping.Store(true)
	p.invalidateWarmPoolOverviewSnapshot(context.Background())
	go func() {
		defer func() {
			p.startupBootstrapRunning.Store(false)
			if !p.startupBootstrapDone.Load() && p.startupBootstrapRequested.Load() && p.getUsageReader() != nil {
				p.triggerStartupBootstrap()
			}
		}()
		retryDelay := defaultOpenAIWarmPoolStartupBootstrapRetry
		if retryDelay <= 0 {
			retryDelay = time.Second
		}
		for {
			p.startupBootstrapRequested.Store(false)
			cfg := p.config()
			if p.getUsageReader() == nil || !cfg.Enabled || len(cfg.StartupGroupIDs) == 0 {
				p.startupBootstrapping.Store(false)
				return
			}
			if !p.ensureStartupProxyGateReady(context.Background()) {
				if !p.waitStartupBootstrapRetry(retryDelay) {
					return
				}
				continue
			}
			accounts, err := p.listStartupBootstrapAccounts(context.Background(), cfg.StartupGroupIDs)
			if err != nil {
				p.writeRefillLog("warn", "global_refill_failed", "预热池启动 bootstrap 加载可调度账号失败", 0, map[string]any{
					"reason":    "startup_list_schedulable_failed",
					"error":     err.Error(),
					"group_ids": cloneInt64Slice(cfg.StartupGroupIDs),
				})
				if !p.waitStartupBootstrapRetry(retryDelay) {
					return
				}
				continue
			}
			if len(accounts) == 0 {
				p.startupBootstrapDone.Store(true)
				p.startupBootstrapping.Store(false)
				p.invalidateWarmPoolOverviewSnapshot(context.Background())
				return
			}
			key := "openai_warm_pool_global"
			_, err, _ = p.refillSF.Do(key, func() (any, error) {
				return nil, p.refillGlobal(context.Background(), accounts, "startup_bootstrap", true)
			})
			if err != nil {
				p.writeRefillLog("warn", "global_refill_failed", "预热池启动 bootstrap 补全局池失败", 0, map[string]any{
					"reason": "startup_refill_failed",
					"error":  err.Error(),
				})
				if !p.waitStartupBootstrapRetry(retryDelay) {
					return
				}
				continue
			}
			if err := p.bootstrapStartupBuckets(context.Background(), cfg.StartupGroupIDs); err != nil {
				p.writeRefillLog("warn", "bucket_refill_failed", "预热池启动 bootstrap 补分组池失败", 0, map[string]any{
					"reason":    "startup_bucket_refill_failed",
					"error":     err.Error(),
					"group_ids": cloneInt64Slice(cfg.StartupGroupIDs),
				})
				if !p.waitStartupBootstrapRetry(retryDelay) {
					return
				}
				continue
			}
			p.startupBootstrapDone.Store(true)
			p.startupBootstrapping.Store(false)
			p.invalidateWarmPoolOverviewSnapshot(context.Background())
			return
		}
	}()
}

func (p *openAIAccountWarmPoolService) listStartupBootstrapAccounts(ctx context.Context, groupIDs []int64) ([]Account, error) {
	if p == nil || p.service == nil || p.service.accountRepo == nil {
		return nil, nil
	}
	normalizedGroupIDs := normalizePositiveInt64Slice(groupIDs)
	if len(normalizedGroupIDs) == 0 {
		return nil, nil
	}
	seen := make(map[int64]struct{})
	accounts := make([]Account, 0)
	for _, groupID := range normalizedGroupIDs {
		groupAccounts, err := p.service.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, PlatformOpenAI)
		if err != nil {
			return nil, fmt.Errorf("query startup warm group %d accounts failed: %w", groupID, err)
		}
		for _, account := range groupAccounts {
			if _, ok := seen[account.ID]; ok {
				continue
			}
			seen[account.ID] = struct{}{}
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (p *openAIAccountWarmPoolService) bootstrapStartupBuckets(ctx context.Context, groupIDs []int64) error {
	if p == nil || p.service == nil || p.getUsageReader() == nil {
		return nil
	}
	if p.config().BucketSyncFillMin <= 0 {
		return nil
	}
	for _, groupID := range normalizePositiveInt64Slice(groupIDs) {
		accounts, err := p.service.listSchedulableAccounts(ctx, p.groupIDPointer(groupID))
		if err != nil {
			return fmt.Errorf("list startup bucket group %d accounts failed: %w", groupID, err)
		}
		key := strings.TrimSpace("openai_warm_pool_bucket_" + formatInt64(groupID))
		_, err, _ = p.refillSF.Do(key, func() (any, error) {
			return nil, p.refillBucket(ctx, groupID, accounts, "", nil, true)
		})
		if err != nil {
			return fmt.Errorf("startup bucket group %d refill failed: %w", groupID, err)
		}
	}
	return nil
}

func (p *openAIAccountWarmPoolService) startupProxyGateEnabled() bool {
	return p != nil && p.service != nil && p.service.proxyRepo != nil && p.service.proxyProber != nil
}

func (p *openAIAccountWarmPoolService) ensureStartupProxyGateReady(ctx context.Context) bool {
	if p == nil || !p.startupProxyGateEnabled() {
		return true
	}
	if p.startupGateReady.Load() {
		return true
	}
	now := time.Now()
	lastCheck := p.startupGateLastCheck.Load()
	if lastCheck > 0 && now.Sub(time.Unix(0, lastCheck)) < defaultOpenAIWarmPoolStartupGateRetry {
		return false
	}
	val, _, _ := p.startupProxyGateSF.Do("openai_warm_pool_startup_proxy_gate", func() (any, error) {
		if p.startupGateReady.Load() {
			return true, nil
		}
		now := time.Now()
		last := p.startupGateLastCheck.Load()
		if last > 0 && now.Sub(time.Unix(0, last)) < defaultOpenAIWarmPoolStartupGateRetry {
			return false, nil
		}
		p.startupGateLastCheck.Store(now.UnixNano())
		return p.checkStartupProxyGate(ctx), nil
	})
	ready, _ := val.(bool)
	return ready
}

func (p *openAIAccountWarmPoolService) checkStartupProxyGate(ctx context.Context) bool {
	if !p.startupProxyGateEnabled() {
		return true
	}
	checkCtx := context.Background()
	if ctx != nil {
		checkCtx = context.WithoutCancel(ctx)
	}
	if defaultOpenAIWarmPoolStartupGateTimeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(checkCtx, defaultOpenAIWarmPoolStartupGateTimeout)
		defer cancel()
	}
	proxy, err := p.service.proxyRepo.GetByID(checkCtx, defaultOpenAIWarmPoolStartupProxyID)
	if err != nil {
		p.writeRefillLog("warn", "startup_gate_waiting", "预热池启动正在等待默认代理探测通过", 0, map[string]any{
			"reason":   "proxy_lookup_failed",
			"proxy_id": defaultOpenAIWarmPoolStartupProxyID,
			"error":    err.Error(),
		})
		return false
	}
	if proxy == nil {
		p.writeRefillLog("warn", "startup_gate_waiting", "预热池启动正在等待默认代理探测通过", 0, map[string]any{
			"reason":   "proxy_not_found",
			"proxy_id": defaultOpenAIWarmPoolStartupProxyID,
		})
		return false
	}
	exitInfo, latencyMs, err := p.service.proxyProber.ProbeProxy(checkCtx, proxy.URL())
	if err != nil {
		p.writeRefillLog("warn", "startup_gate_waiting", "预热池启动正在等待默认代理探测通过", 0, map[string]any{
			"reason":      "proxy_probe_failed",
			"proxy_id":    defaultOpenAIWarmPoolStartupProxyID,
			"proxy_name":  strings.TrimSpace(proxy.Name),
			"proxy_host":  strings.TrimSpace(proxy.Host),
			"proxy_port":  proxy.Port,
			"probe_error": err.Error(),
		})
		return false
	}
	p.startupGateReady.Store(true)
	fields := map[string]any{
		"proxy_id":   defaultOpenAIWarmPoolStartupProxyID,
		"proxy_name": strings.TrimSpace(proxy.Name),
		"latency_ms": latencyMs,
	}
	if exitInfo != nil {
		fields["exit_ip"] = strings.TrimSpace(exitInfo.IP)
		fields["country"] = strings.TrimSpace(exitInfo.Country)
	}
	p.writeRefillLog("info", "startup_gate_ready", "预热池启动门禁已通过，现已启用预热", 0, fields)
	return true
}

func (p *openAIAccountWarmPoolService) writeOpsLog(level, message string, fields map[string]any) {
	if p == nil {
		return
	}
	eventFields := map[string]any{
		"platform": PlatformOpenAI,
	}
	for k, v := range fields {
		eventFields[k] = v
	}
	logger.WriteSinkEvent(level, openAIWarmPoolLogComponent, message, eventFields)
}

func (p *openAIAccountWarmPoolService) writeRefillLog(level, event, message string, groupID int64, fields map[string]any) {
	eventFields := map[string]any{
		"event":    event,
		"group_id": groupID,
	}
	for k, v := range fields {
		eventFields[k] = v
	}
	p.writeOpsLog(level, message, eventFields)
}

func (p *openAIAccountWarmPoolService) writeProbeLog(level, event, reason, message string, groupID int64, account *Account, fields map[string]any) {
	eventFields := map[string]any{
		"event":    event,
		"reason":   strings.TrimSpace(reason),
		"group_id": groupID,
	}
	if account != nil {
		eventFields["account_id"] = account.ID
		eventFields["account_name"] = strings.TrimSpace(account.Name)
	}
	for k, v := range fields {
		eventFields[k] = v
	}
	p.writeOpsLog(level, message, eventFields)
}

func formatOpenAIWarmPoolGroupLabel(account *Account, groupID int64) string {
	if name := strings.TrimSpace(resolveWarmPoolGroupName(account, groupID)); name != "" {
		return name
	}
	if groupID > 0 {
		return "#" + formatInt64(groupID)
	}
	return "共享分组"
}

func formatOpenAIWarmPoolAccountLabel(account *Account) string {
	if account == nil {
		return "未知账号"
	}
	if name := strings.TrimSpace(account.Name); name != "" {
		return name
	}
	if account.ID > 0 {
		return "#" + formatInt64(account.ID)
	}
	return "未知账号"
}

//nolint:unused // 预留给后续 warm pool 诊断消息拼装复用
func formatOpenAIWarmPoolProbeMessage(message string, groupID int64, account *Account) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}
	if strings.Contains(message, "分组 ") && strings.Contains(message, "账号 ") {
		return message
	}
	groupLabel := formatOpenAIWarmPoolGroupLabel(account, groupID)
	if account == nil {
		return "分组 " + groupLabel + " " + message
	}
	return "分组 " + groupLabel + "，账号 " + formatOpenAIWarmPoolAccountLabel(account) + "：" + message
}

func (p *openAIAccountWarmPoolService) config() openAIWarmPoolConfigView {
	cfg := openAIWarmPoolConfigView{
		Enabled:              true,
		BucketTargetSize:     defaultOpenAIWarmPoolBucketTargetSize,
		BucketRefillBelow:    defaultOpenAIWarmPoolBucketRefillBelow,
		BucketSyncFillMin:    defaultOpenAIWarmPoolBucketSyncFillMin,
		BucketEntryTTL:       defaultOpenAIWarmPoolBucketEntryTTL,
		BucketRefillCooldown: defaultOpenAIWarmPoolBucketRefillCooldown,
		BucketRefillInterval: defaultOpenAIWarmPoolBucketRefillInterval,
		GlobalTargetSize:     defaultOpenAIWarmPoolGlobalTargetSize,
		GlobalRefillBelow:    defaultOpenAIWarmPoolGlobalRefillBelow,
		GlobalEntryTTL:       defaultOpenAIWarmPoolGlobalEntryTTL,
		GlobalRefillCooldown: defaultOpenAIWarmPoolGlobalRefillCooldown,
		GlobalRefillInterval: defaultOpenAIWarmPoolGlobalRefillInterval,
		NetworkErrorPoolSize: defaultOpenAIWarmPoolNetworkErrorPoolSize,
		NetworkErrorEntryTTL: defaultOpenAIWarmPoolNetworkErrorEntryTTL,
		ProbeMaxCandidates:   defaultOpenAIWarmPoolProbeMaxCandidates,
		ProbeConcurrency:     defaultOpenAIWarmPoolProbeConcurrency,
		ProbeTimeout:         defaultOpenAIWarmPoolProbeTimeout,
		FailureCooldown:      defaultOpenAIWarmPoolProbeFailureCooldown,
		ActiveBucketTTL:      defaultOpenAIWarmPoolActiveBucketTTL,
	}
	if p == nil || p.service == nil {
		return cfg
	}

	if p.service.settingService != nil {
		warmSettings := p.service.settingService.GetOpenAIWarmPoolSettings(context.Background())
		cfg.Enabled = warmSettings.Enabled
		if warmSettings.BucketTargetSize > 0 {
			cfg.BucketTargetSize = warmSettings.BucketTargetSize
		}
		if warmSettings.BucketRefillBelow > 0 {
			cfg.BucketRefillBelow = warmSettings.BucketRefillBelow
		}
		if warmSettings.BucketSyncFillMin >= 0 {
			cfg.BucketSyncFillMin = warmSettings.BucketSyncFillMin
		}
		if warmSettings.BucketEntryTTLSeconds > 0 {
			cfg.BucketEntryTTL = time.Duration(warmSettings.BucketEntryTTLSeconds) * time.Second
		}
		if warmSettings.BucketRefillCooldownSeconds >= 0 {
			cfg.BucketRefillCooldown = time.Duration(warmSettings.BucketRefillCooldownSeconds) * time.Second
		}
		if warmSettings.BucketRefillIntervalSeconds >= 0 {
			cfg.BucketRefillInterval = time.Duration(warmSettings.BucketRefillIntervalSeconds) * time.Second
		}
		if warmSettings.GlobalTargetSize > 0 {
			cfg.GlobalTargetSize = warmSettings.GlobalTargetSize
		}
		if warmSettings.GlobalRefillBelow > 0 {
			cfg.GlobalRefillBelow = warmSettings.GlobalRefillBelow
		}
		if warmSettings.GlobalEntryTTLSeconds > 0 {
			cfg.GlobalEntryTTL = time.Duration(warmSettings.GlobalEntryTTLSeconds) * time.Second
		}
		if warmSettings.GlobalRefillCooldownSeconds >= 0 {
			cfg.GlobalRefillCooldown = time.Duration(warmSettings.GlobalRefillCooldownSeconds) * time.Second
		}
		if warmSettings.GlobalRefillIntervalSeconds >= 0 {
			cfg.GlobalRefillInterval = time.Duration(warmSettings.GlobalRefillIntervalSeconds) * time.Second
		}
		if warmSettings.NetworkErrorPoolSize >= 0 {
			cfg.NetworkErrorPoolSize = warmSettings.NetworkErrorPoolSize
		}
		if warmSettings.NetworkErrorEntryTTLSeconds >= 0 {
			cfg.NetworkErrorEntryTTL = time.Duration(warmSettings.NetworkErrorEntryTTLSeconds) * time.Second
		}
		if warmSettings.ProbeMaxCandidates > 0 {
			cfg.ProbeMaxCandidates = warmSettings.ProbeMaxCandidates
		}
		if warmSettings.ProbeConcurrency > 0 {
			cfg.ProbeConcurrency = warmSettings.ProbeConcurrency
		}
		if warmSettings.ProbeTimeoutSeconds > 0 {
			cfg.ProbeTimeout = time.Duration(warmSettings.ProbeTimeoutSeconds) * time.Second
		}
		if warmSettings.ProbeFailureCooldownSeconds >= 0 {
			cfg.FailureCooldown = time.Duration(warmSettings.ProbeFailureCooldownSeconds) * time.Second
		}
		cfg.StartupGroupIDs = cloneInt64Slice(warmSettings.StartupGroupIDs)
	} else if p.service.cfg != nil {
		warmCfg := p.service.cfg.Gateway.OpenAIWS.AccountWarmPool
		cfg.Enabled = warmCfg.Enabled
		if warmCfg.BucketTargetSize > 0 {
			cfg.BucketTargetSize = warmCfg.BucketTargetSize
		}
		if warmCfg.BucketRefillBelow > 0 {
			cfg.BucketRefillBelow = warmCfg.BucketRefillBelow
		}
		if warmCfg.BucketSyncFillMin >= 0 {
			cfg.BucketSyncFillMin = warmCfg.BucketSyncFillMin
		}
		if warmCfg.BucketEntryTTLSeconds > 0 {
			cfg.BucketEntryTTL = time.Duration(warmCfg.BucketEntryTTLSeconds) * time.Second
		}
		if warmCfg.BucketRefillCooldownSeconds >= 0 {
			cfg.BucketRefillCooldown = time.Duration(warmCfg.BucketRefillCooldownSeconds) * time.Second
		}
		if warmCfg.BucketRefillIntervalSeconds >= 0 {
			cfg.BucketRefillInterval = time.Duration(warmCfg.BucketRefillIntervalSeconds) * time.Second
		}
		if warmCfg.GlobalTargetSize > 0 {
			cfg.GlobalTargetSize = warmCfg.GlobalTargetSize
		}
		if warmCfg.GlobalRefillBelow > 0 {
			cfg.GlobalRefillBelow = warmCfg.GlobalRefillBelow
		}
		if warmCfg.GlobalEntryTTLSeconds > 0 {
			cfg.GlobalEntryTTL = time.Duration(warmCfg.GlobalEntryTTLSeconds) * time.Second
		}
		if warmCfg.GlobalRefillCooldownSeconds >= 0 {
			cfg.GlobalRefillCooldown = time.Duration(warmCfg.GlobalRefillCooldownSeconds) * time.Second
		}
		if warmCfg.GlobalRefillIntervalSeconds >= 0 {
			cfg.GlobalRefillInterval = time.Duration(warmCfg.GlobalRefillIntervalSeconds) * time.Second
		}
		if warmCfg.NetworkErrorPoolSize >= 0 {
			cfg.NetworkErrorPoolSize = warmCfg.NetworkErrorPoolSize
		}
		if warmCfg.NetworkErrorEntryTTLSeconds >= 0 {
			cfg.NetworkErrorEntryTTL = time.Duration(warmCfg.NetworkErrorEntryTTLSeconds) * time.Second
		}
		if warmCfg.ProbeMaxCandidates > 0 {
			cfg.ProbeMaxCandidates = warmCfg.ProbeMaxCandidates
		}
		if warmCfg.ProbeConcurrency > 0 {
			cfg.ProbeConcurrency = warmCfg.ProbeConcurrency
		}
		if warmCfg.ProbeTimeoutSeconds > 0 {
			cfg.ProbeTimeout = time.Duration(warmCfg.ProbeTimeoutSeconds) * time.Second
		}
		if warmCfg.ProbeFailureCooldownSeconds >= 0 {
			cfg.FailureCooldown = time.Duration(warmCfg.ProbeFailureCooldownSeconds) * time.Second
		}
	}
	if cfg.BucketTargetSize <= 0 {
		cfg.BucketTargetSize = 1
	}
	if cfg.BucketRefillBelow >= cfg.BucketTargetSize {
		cfg.BucketRefillBelow = cfg.BucketTargetSize - 1
	}
	if cfg.BucketRefillBelow <= 0 {
		cfg.BucketRefillBelow = 1
	}
	if cfg.BucketSyncFillMin < 0 {
		cfg.BucketSyncFillMin = 0
	}
	if cfg.BucketSyncFillMin > cfg.BucketTargetSize {
		cfg.BucketSyncFillMin = cfg.BucketTargetSize
	}
	if cfg.GlobalTargetSize <= 0 {
		cfg.GlobalTargetSize = cfg.BucketTargetSize
	}
	if cfg.GlobalRefillBelow <= 0 {
		cfg.GlobalRefillBelow = minPositiveInt(cfg.GlobalTargetSize, defaultOpenAIWarmPoolGlobalRefillBelow)
	}
	if cfg.GlobalRefillBelow > cfg.GlobalTargetSize {
		cfg.GlobalRefillBelow = cfg.GlobalTargetSize
	}
	if cfg.NetworkErrorPoolSize < 0 {
		cfg.NetworkErrorPoolSize = 0
	}
	if cfg.NetworkErrorEntryTTL < 0 {
		cfg.NetworkErrorEntryTTL = 0
	}
	if cfg.ProbeMaxCandidates <= 0 {
		cfg.ProbeMaxCandidates = defaultOpenAIWarmPoolProbeMaxCandidates
	}
	if cfg.ProbeConcurrency <= 0 {
		cfg.ProbeConcurrency = defaultOpenAIWarmPoolProbeConcurrency
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultOpenAIWarmPoolProbeTimeout
	}
	if cfg.FailureCooldown < 0 {
		cfg.FailureCooldown = 0
	}
	if cfg.ActiveBucketTTL < cfg.BucketEntryTTL*4 {
		cfg.ActiveBucketTTL = cfg.BucketEntryTTL * 4
	}
	if cfg.ActiveBucketTTL < defaultOpenAIWarmPoolActiveBucketTTL {
		cfg.ActiveBucketTTL = defaultOpenAIWarmPoolActiveBucketTTL
	}
	return cfg
}

func minPositiveInt(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func (p *openAIAccountWarmPoolService) normalizeGroupID(groupID *int64) int64 {
	if p == nil || p.service == nil || p.service.cfg == nil {
		if groupID == nil || *groupID <= 0 {
			return 0
		}
		return *groupID
	}
	if p.service.cfg.RunMode == config.RunModeSimple {
		return 0
	}
	if groupID == nil || *groupID <= 0 {
		return 0
	}
	return *groupID
}

func (p *openAIAccountWarmPoolService) groupIDPointer(groupID int64) *int64 {
	if groupID <= 0 {
		return nil
	}
	gid := groupID
	return &gid
}

func (p *openAIAccountWarmPoolService) bucketState(groupID int64) *openAIWarmPoolBucketState {
	actual, _ := p.bucketStates.LoadOrStore(groupID, &openAIWarmPoolBucketState{groupID: groupID, entries: map[int64]openAIWarmPoolBucketEntry{}})
	bucket, _ := actual.(*openAIWarmPoolBucketState)
	if bucket != nil {
		bucket.mu.Lock()
		bucket.ensureEntriesLocked()
		bucket.mu.Unlock()
	}
	return bucket
}

func (p *openAIAccountWarmPoolService) accountState(accountID int64) *openAIWarmAccountState {
	actual, _ := p.accountStates.LoadOrStore(accountID, &openAIWarmAccountState{})
	state, _ := actual.(*openAIWarmAccountState)
	return state
}

func warmBucketMemberToken(instanceID string, accountID int64) string {
	return strings.TrimSpace(instanceID) + ":" + strconv.FormatInt(accountID, 10)
}

func parseWarmBucketMemberToken(token string) (string, int64, bool) {
	token = strings.TrimSpace(token)
	idx := strings.LastIndex(token, ":")
	if idx <= 0 || idx >= len(token)-1 {
		return "", 0, false
	}
	accountID, err := strconv.ParseInt(token[idx+1:], 10, 64)
	if err != nil || accountID <= 0 {
		return "", 0, false
	}
	return token[:idx], accountID, true
}

func (p *openAIAccountWarmPoolService) runMirrorWriteWorker() {
	for {
		select {
		case <-p.stopCh:
			return
		case task := <-p.mirrorWriteCh:
			if task.run == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), openAIWarmPoolMirrorWriteTimeout)
			if err := task.run(ctx); err != nil {
				logger.LegacyPrintf(openAIWarmPoolLogComponent, "[WarmPoolMirror] %s failed: %v", task.label, err)
			}
			cancel()
		}
	}
}

func (p *openAIAccountWarmPoolService) maybeLogMirrorDrop(label string) {
	if p == nil {
		return
	}
	p.mirrorDropMu.Lock()
	defer p.mirrorDropMu.Unlock()
	now := time.Now()
	if !p.mirrorDropAt.IsZero() && now.Sub(p.mirrorDropAt) < openAIWarmPoolMirrorDropLogInterval {
		return
	}
	p.mirrorDropAt = now
	logger.LegacyPrintf(openAIWarmPoolLogComponent, "[WarmPoolMirror] mirror queue full, dropped non-critical task: %s", label)
}

func (p *openAIAccountWarmPoolService) enqueueMirrorWrite(label string, critical bool, run func(ctx context.Context) error) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || run == nil {
		return
	}
	p.Start()
	task := openAIWarmPoolMirrorWriteTask{label: label, critical: critical, run: run}
	select {
	case <-p.stopCh:
		return
	default:
	}
	select {
	case p.mirrorWriteCh <- task:
		return
	default:
		if !critical {
			p.maybeLogMirrorDrop(label)
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), openAIWarmPoolMirrorWriteTimeout)
			defer cancel()
			if err := run(ctx); err != nil {
				logger.LegacyPrintf(openAIWarmPoolLogComponent, "[WarmPoolMirror] %s failed: %v", label, err)
			}
		}()
	}
}

func (p *openAIAccountWarmPoolService) syncWarmAccountState(accountID int64, state *openAIWarmAccountState, now time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || accountID <= 0 || state == nil {
		return
	}
	inspection := state.inspect(now)
	mirrorState := &OpsRealtimeWarmAccountState{
		AccountID:         accountID,
		State:             warmPoolStateLabel(inspection),
		VerifiedAt:        inspection.VerifiedAt,
		ExpiresAt:         inspection.ExpiresAt,
		FailUntil:         inspection.FailUntil,
		NetworkErrorAt:    inspection.NetworkErrorAt,
		NetworkErrorUntil: inspection.NetworkErrorUntil,
		UpdatedAt:         now.UTC(),
	}
	if inspection.Probing {
		mirrorState.State = "probing"
	}
	p.enqueueMirrorWrite(fmt.Sprintf("sync account state account=%d", accountID), true, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.SetWarmAccountState(ctx, mirrorState)
	})
}

func (p *openAIAccountWarmPoolService) touchWarmBucketAccess(groupID int64, at time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil {
		return
	}
	p.enqueueMirrorWrite(fmt.Sprintf("touch bucket access group=%d", groupID), false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.TouchWarmBucketAccess(ctx, groupID, at)
	})
}

func (p *openAIAccountWarmPoolService) touchWarmBucketRefill(groupID int64, at time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil {
		return
	}
	p.enqueueMirrorWrite(fmt.Sprintf("touch bucket refill group=%d", groupID), false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.TouchWarmBucketRefill(ctx, groupID, at)
	})
}

func (p *openAIAccountWarmPoolService) incrementWarmBucketTake(groupID int64, delta int64) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || delta == 0 {
		return
	}
	p.enqueueMirrorWrite(fmt.Sprintf("increment bucket take group=%d delta=%d", groupID, delta), false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.IncrementWarmBucketTake(ctx, groupID, delta)
	})
}

func (p *openAIAccountWarmPoolService) incrementWarmGlobalTake(delta int64) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || delta == 0 {
		return
	}
	p.enqueueMirrorWrite(fmt.Sprintf("increment global take delta=%d", delta), false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.IncrementWarmGlobalTake(ctx, delta)
	})
}

func (p *openAIAccountWarmPoolService) touchWarmLastBucketMaintenance(at time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil {
		return
	}
	p.enqueueMirrorWrite("touch last bucket maintenance", false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.TouchWarmLastBucketMaintenance(ctx, at)
	})
}

func (p *openAIAccountWarmPoolService) touchWarmLastGlobalMaintenance(at time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil {
		return
	}
	p.enqueueMirrorWrite("touch last global maintenance", false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.TouchWarmLastGlobalMaintenance(ctx, at)
	})
}

func warmBucketTimeWithinTTL(now time.Time, ts *time.Time, activeBucketTTL time.Duration) bool {
	if ts == nil {
		return false
	}
	if activeBucketTTL <= 0 {
		return true
	}
	return now.Sub(ts.UTC()) <= activeBucketTTL
}

func warmBucketUnixWithinTTL(now time.Time, tsUnix int64, activeBucketTTL time.Duration) bool {
	if tsUnix <= 0 {
		return false
	}
	timestamp := time.Unix(0, tsUnix)
	return warmBucketTimeWithinTTL(now, &timestamp, activeBucketTTL)
}

func (p *openAIAccountWarmPoolService) shouldRetainOpsBucket(bucket *openAIWarmPoolBucketState, now time.Time, activeBucketTTL time.Duration) bool {
	if bucket == nil {
		return false
	}
	if warmBucketUnixWithinTTL(now, bucket.lastAccess.Load(), activeBucketTTL) {
		return true
	}
	if warmBucketUnixWithinTTL(now, bucket.lastRefill.Load(), activeBucketTTL) {
		return true
	}
	return bucket.readyCount() > 0
}

func (p *openAIAccountWarmPoolService) startupOpsGroupIDs() []int64 {
	if p == nil {
		return nil
	}
	cfg := p.config()
	if len(cfg.StartupGroupIDs) == 0 {
		return nil
	}
	if !p.startupBootstrapRequested.Load() && !p.startupBootstrapping.Load() && !p.startupBootstrapDone.Load() {
		return nil
	}
	return cloneInt64Slice(cfg.StartupGroupIDs)
}

func (p *openAIAccountWarmPoolService) startupOpsLastRefillAt() *time.Time {
	if p == nil {
		return nil
	}
	if last := p.lastGlobalMaintenance.Load(); last > 0 {
		return timePtrUTC(time.Unix(0, last))
	}
	if last := p.lastGlobalRefill.Load(); last > 0 {
		return timePtrUTC(time.Unix(0, last))
	}
	return nil
}

func (p *openAIAccountWarmPoolService) touchWarmBucketMember(groupID int64, accountID int64, touchedAt time.Time) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || accountID <= 0 {
		return
	}
	memberToken := warmBucketMemberToken(p.instanceID, accountID)
	p.enqueueMirrorWrite(fmt.Sprintf("touch bucket member group=%d account=%d", groupID, accountID), false, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.TouchWarmBucketMember(ctx, groupID, memberToken, touchedAt)
	})
}

func (p *openAIAccountWarmPoolService) removeWarmBucketMember(groupID int64, accountID int64) {
	if p == nil || p.service == nil || p.service.opsRealtimeCache == nil || accountID <= 0 {
		return
	}
	memberToken := warmBucketMemberToken(p.instanceID, accountID)
	p.enqueueMirrorWrite(fmt.Sprintf("remove bucket member group=%d account=%d", groupID, accountID), true, func(ctx context.Context) error {
		return p.service.opsRealtimeCache.RemoveWarmBucketMember(ctx, groupID, memberToken)
	})
}

func (p *openAIAccountWarmPoolService) recordTake(groupID *int64) {
	if p == nil {
		return
	}
	p.totalTakeCount.Add(1)
	p.incrementWarmGlobalTake(1)
	normalizedGroupID := p.normalizeGroupID(groupID)
	bucket := p.bucketState(normalizedGroupID)
	if bucket != nil {
		bucket.takeCount.Add(1)
	}
	p.incrementWarmBucketTake(normalizedGroupID, 1)
}

func (p *openAIAccountWarmPoolService) runRefillWorker() {
	if p == nil {
		return
	}
	ticker := time.NewTicker(defaultOpenAIWarmPoolWorkerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.maybeMaintainActiveBuckets()
			p.maybeMaintainGlobalPool()
		case <-p.stopCh:
			return
		}
	}
}

func (p *openAIAccountWarmPoolService) maybeMaintainGlobalPool() {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil || cfg.GlobalRefillInterval <= 0 {
		return
	}
	nowUnix := time.Now().UnixNano()
	last := p.lastGlobalMaintenance.Load()
	if last != 0 && time.Duration(nowUnix-last) < cfg.GlobalRefillInterval {
		return
	}
	if !p.lastGlobalMaintenance.CompareAndSwap(last, nowUnix) {
		return
	}
	p.maintainGlobalPool()
}

func (p *openAIAccountWarmPoolService) maintainGlobalPool() {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil || p.service == nil {
		return
	}
	accounts, loaded, err := p.listGlobalRefillAccountsForMaintenance(context.Background())
	if err != nil {
		p.writeRefillLog("error", "global_refill_failed", "预热池全局维护加载可调度账号失败", 0, map[string]any{
			"reason": "maintenance_list_schedulable_failed",
			"error":  err.Error(),
		})
		return
	}
	if !loaded || !p.needsGlobalRefill(accounts) {
		return
	}
	p.ensureGlobalReady(context.Background(), accounts, "maintenance", false)
}

func (p *openAIAccountWarmPoolService) listSchedulableAccountsForMaintenance(ctx context.Context, groupID *int64) ([]Account, bool, error) {
	if p == nil || p.service == nil {
		return nil, false, nil
	}
	if p.service.schedulerSnapshot == nil {
		accounts, err := p.service.listSchedulableAccounts(ctx, groupID)
		return accounts, true, err
	}
	accounts, _, hit, err := p.service.schedulerSnapshot.listSchedulableAccountsFromCache(ctx, groupID, PlatformOpenAI, false)
	if err != nil {
		return nil, false, err
	}
	return accounts, hit, nil
}

func (p *openAIAccountWarmPoolService) listGlobalRefillAccountsForMaintenance(ctx context.Context, seedGroupIDs ...int64) ([]Account, bool, error) {
	if p == nil || p.service == nil {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	groupIDs := p.activeGlobalRefillGroupIDs(time.Now(), seedGroupIDs...)
	if len(groupIDs) == 0 {
		return nil, false, nil
	}
	seenAccounts := make(map[int64]struct{}, 16)
	accounts := make([]Account, 0, 16)
	loadedAny := false
	for _, groupID := range groupIDs {
		listed, loaded, err := p.listSchedulableAccountsForMaintenance(ctx, p.groupIDPointer(groupID))
		if err != nil {
			return nil, loadedAny, err
		}
		if !loaded {
			continue
		}
		loadedAny = true
		for _, acc := range listed {
			if !acc.IsOpenAI() || !acc.IsSchedulable() {
				continue
			}
			if _, exists := seenAccounts[acc.ID]; exists {
				continue
			}
			seenAccounts[acc.ID] = struct{}{}
			accounts = append(accounts, acc)
		}
	}
	return accounts, loadedAny, nil
}

func (p *openAIAccountWarmPoolService) maybeMaintainActiveBuckets() {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil || cfg.BucketRefillInterval <= 0 {
		return
	}
	nowUnix := time.Now().UnixNano()
	last := p.lastBucketMaintenance.Load()
	if last != 0 && time.Duration(nowUnix-last) < cfg.BucketRefillInterval {
		return
	}
	if !p.lastBucketMaintenance.CompareAndSwap(last, nowUnix) {
		return
	}
	p.maintainActiveBuckets()
}

func (p *openAIAccountWarmPoolService) maintainActiveBuckets() {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil || p.service == nil {
		return
	}
	now := time.Now()
	p.bucketStates.Range(func(key, value any) bool {
		bucket, _ := value.(*openAIWarmPoolBucketState)
		if bucket == nil {
			return true
		}
		if !p.shouldRetainOpsBucket(bucket, now, cfg.ActiveBucketTTL) {
			p.bucketStates.Delete(key)
			return true
		}
		groupID := bucket.groupID
		accounts, loaded, err := p.listSchedulableAccountsForMaintenance(context.Background(), p.groupIDPointer(groupID))
		if err != nil {
			p.writeRefillLog("error", "bucket_refill_failed", "预热池分组池维护加载可调度账号失败", groupID, map[string]any{
				"reason": "maintenance_list_schedulable_failed",
				"error":  err.Error(),
			})
			return true
		}
		if !loaded {
			return true
		}
		maxReadyPossible := p.countSchedulableOpenAIAccounts(accounts, "", nil)
		effectiveRefillBelow := cfg.BucketRefillBelow
		if effectiveRefillBelow > maxReadyPossible {
			effectiveRefillBelow = maxReadyPossible
		}
		if effectiveRefillBelow > 0 && p.countBucketWarmReady(groupID, accounts) >= effectiveRefillBelow {
			return true
		}
		p.ensureBucketReady(context.Background(), groupID, accounts, "", nil, false)
		return true
	})
}

func (p *openAIAccountWarmPoolService) WarmCandidates(
	ctx context.Context,
	groupID *int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
) []*Account {
	cfg := p.config()
	if !cfg.Enabled || len(accounts) == 0 {
		return nil
	}
	if !p.ensureStartupProxyGateReady(ctx) {
		return nil
	}
	normalizedGroupID := p.normalizeGroupID(groupID)
	bucket := p.bucketState(normalizedGroupID)
	if bucket != nil {
		now := time.Now()
		bucket.lastAccess.Store(now.UnixNano())
		p.touchWarmBucketAccess(normalizedGroupID, now)
	}

	warmCandidates := p.collectBucketWarmCandidates(normalizedGroupID, accounts, requestedModel, excludedIDs)
	warmUsableCount := len(warmCandidates)
	bucketReadyBeforeFill := p.countBucketWarmReady(normalizedGroupID, accounts)
	maxReadyPossible := p.countSchedulableOpenAIAccounts(accounts, "", nil)
	targetTotal := cfg.BucketTargetSize
	if targetTotal > maxReadyPossible {
		targetTotal = maxReadyPossible
	}
	if bucket != nil && targetTotal > 0 && bucketReadyBeforeFill < targetTotal {
		bucket.globalBootstrapDone.Store(false)
	}
	maxUsablePossible := p.countSchedulableOpenAIAccounts(accounts, requestedModel, excludedIDs)
	effectiveSyncFillMin := cfg.BucketSyncFillMin
	if effectiveSyncFillMin > maxUsablePossible {
		effectiveSyncFillMin = maxUsablePossible
	}
	needsAsyncContinuation := false
	if effectiveSyncFillMin > 0 && warmUsableCount < effectiveSyncFillMin {
		if warmUsableCount == 0 {
			p.ensureBucketReadyForFirstTake(ctx, normalizedGroupID, accounts, requestedModel, excludedIDs)
			warmCandidates = p.collectBucketWarmCandidates(normalizedGroupID, accounts, requestedModel, excludedIDs)
			warmUsableCount = len(warmCandidates)
		}
		if warmUsableCount < effectiveSyncFillMin {
			needsAsyncContinuation = true
		}
	}

	bucketReadyCount := p.countBucketWarmReady(normalizedGroupID, accounts)
	effectiveRefillBelow := cfg.BucketRefillBelow
	if effectiveRefillBelow > maxReadyPossible {
		effectiveRefillBelow = maxReadyPossible
	}
	if effectiveRefillBelow > 0 && bucketReadyCount < effectiveRefillBelow {
		needsAsyncContinuation = true
	}
	if needsAsyncContinuation {
		p.ensureBucketReady(context.Background(), normalizedGroupID, cloneAccounts(accounts), requestedModel, nil, false)
	} else if targetTotal > 0 && bucketReadyCount >= targetTotal {
		if bucket := p.bucketState(normalizedGroupID); bucket != nil {
			go p.maybeBootstrapGlobalAfterBucketReady(context.Background(), normalizedGroupID, bucket, bucketReadyCount, targetTotal)
		}
	}
	return warmCandidates
}

func cloneAccounts(accounts []Account) []Account {
	if len(accounts) == 0 {
		return nil
	}
	cloned := make([]Account, len(accounts))
	copy(cloned, accounts)
	return cloned
}

func (p *openAIAccountWarmPoolService) collectBucketWarmCandidates(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) []*Account {
	now := time.Now()
	p.maybeRefreshBucketMembers(groupID, accounts)
	bucket := p.bucketState(groupID)
	if bucket == nil {
		return nil
	}
	readyIDs := bucket.readyIDs()
	if len(readyIDs) == 0 {
		return nil
	}
	allowed := make(map[int64]struct{}, len(readyIDs))
	for _, accountID := range readyIDs {
		allowed[accountID] = struct{}{}
	}
	warmed := make([]*Account, 0, len(readyIDs))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if _, exists := allowed[acc.ID]; !exists {
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		state := p.accountState(acc.ID)
		ready, cooling, networkError, expired := state.snapshot(now)
		if cooling || networkError || !ready || expired {
			bucket.remove(acc.ID)
			p.removeWarmBucketMember(groupID, acc.ID)
			continue
		}
		warmed = append(warmed, acc)
	}
	return warmed
}

func (p *openAIAccountWarmPoolService) countBucketWarmReady(groupID int64, accounts []Account) int {
	return len(p.collectBucketWarmCandidates(groupID, accounts, "", nil))
}

func (p *openAIAccountWarmPoolService) maybeRefreshBucketMembers(groupID int64, accounts []Account) {
	if p == nil || len(accounts) == 0 {
		return
	}
	bucket := p.bucketState(groupID)
	if bucket == nil {
		return
	}
	dueIDs := bucket.dueIDs(time.Now())
	if len(dueIDs) == 0 {
		return
	}
	accountsByID := make(map[int64]Account, len(accounts))
	for _, acc := range accounts {
		if !acc.IsOpenAI() {
			continue
		}
		accountsByID[acc.ID] = acc
	}
	for _, accountID := range dueIDs {
		acc, exists := accountsByID[accountID]
		if !exists || !acc.IsSchedulable() {
			bucket.remove(accountID)
			p.removeWarmBucketMember(groupID, accountID)
			continue
		}
		accountCopy := acc
		go p.refreshBucketMember(groupID, bucket, &accountCopy)
	}
}

func (p *openAIAccountWarmPoolService) refreshBucketMember(groupID int64, bucket *openAIWarmPoolBucketState, account *Account) {
	if p == nil || bucket == nil || account == nil {
		return
	}
	cfg := p.config()
	state := p.accountState(account.ID)
	now := time.Now()
	if lastUsedAt := account.LastUsedAt; lastUsedAt != nil && cfg.BucketEntryTTL > 0 {
		inspection := state.inspect(now)
		if inspection.VerifiedAt != nil && lastUsedAt.After(*inspection.VerifiedAt) && now.Sub(*lastUsedAt) <= cfg.BucketEntryTTL {
			state.finishSuccess(now, cfg.GlobalEntryTTL)
			p.syncWarmAccountState(account.ID, state, now)
			bucket.touch(account.ID, now, cfg.BucketEntryTTL)
			p.touchWarmBucketMember(groupID, account.ID, now)
			p.writeProbeLog("info", "bucket_member_recently_used", "recently_used", "分组 "+formatOpenAIWarmPoolGroupLabel(account, groupID)+" 预热池成员近期已被实际使用，跳过复检并刷新就绪有效期", groupID, account, map[string]any{
				"last_used_at": lastUsedAt.UTC().Format(time.RFC3339Nano),
			})
			return
		}
	}
	result := p.probeCandidate(context.Background(), groupID, account)
	if result.Outcome == openAIWarmPoolProbeReadyOutcome {
		touchedAt := time.Now()
		bucket.touch(account.ID, touchedAt, cfg.BucketEntryTTL)
		p.touchWarmBucketMember(groupID, account.ID, touchedAt)
		return
	}
	if result.Outcome == openAIWarmPoolProbeFailedOutcome || result.Outcome == openAIWarmPoolProbeNetworkErrorOutcome {
		bucket.remove(account.ID)
		p.removeWarmBucketMember(groupID, account.ID)
	}
}

func (p *openAIAccountWarmPoolService) countWarmReady(accounts []Account) int {
	now := time.Now()
	count := 0
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		state := p.accountState(acc.ID)
		ready, cooling, networkError, expired := state.snapshot(now)
		if cooling || networkError || !ready || expired {
			continue
		}
		count++
	}
	return count
}

func (p *openAIAccountWarmPoolService) countNetworkErrorPoolAccounts(now time.Time) int {
	if p == nil {
		return 0
	}
	count := 0
	p.accountStates.Range(func(_, value any) bool {
		state, _ := value.(*openAIWarmAccountState)
		if state == nil {
			return true
		}
		_, _, networkError, _ := state.snapshot(now)
		if networkError {
			count++
		}
		return true
	})
	return count
}

//nolint:unused // 预留给后续 warm pool network-error 统计接线
func (p *openAIAccountWarmPoolService) countNetworkErrorPoolAccountsForAccounts(accounts []Account) int {
	now := time.Now()
	count := 0
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		state := p.accountState(acc.ID)
		_, _, networkError, _ := state.snapshot(now)
		if networkError {
			count++
		}
	}
	return count
}

func (p *openAIAccountWarmPoolService) countSchedulableOpenAIAccounts(accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) int {
	count := 0
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		count++
	}
	return count
}

func (p *openAIAccountWarmPoolService) filterBucketAccountsByOwner(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}, includeOwned bool) []Account {
	filtered := make([]Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		ownedByGroup := warmPoolBucketOwnerGroupID(p, acc) == groupID
		if includeOwned != ownedByGroup {
			continue
		}
		filtered = append(filtered, *acc)
	}
	return filtered
}

func (p *openAIAccountWarmPoolService) countOwnedSchedulableOpenAIAccounts(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) int {
	return len(p.filterBucketAccountsByOwner(groupID, accounts, requestedModel, excludedIDs, true))
}

func (p *openAIAccountWarmPoolService) countBorrowableSchedulableOpenAIAccounts(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) int {
	return len(p.filterBucketAccountsByOwner(groupID, accounts, requestedModel, excludedIDs, false))
}

func (p *openAIAccountWarmPoolService) ownedBucketAccounts(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) []Account {
	return p.filterBucketAccountsByOwner(groupID, accounts, requestedModel, excludedIDs, true)
}

func (p *openAIAccountWarmPoolService) borrowableBucketAccounts(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) []Account {
	return p.filterBucketAccountsByOwner(groupID, accounts, requestedModel, excludedIDs, false)
}

func (p *openAIAccountWarmPoolService) shouldAllowBucketBorrow(groupID int64, accounts []Account, requestedModel string, excludedIDs map[int64]struct{}, target int) bool {
	if target <= 0 {
		return false
	}
	ownedCapacity := p.countOwnedSchedulableOpenAIAccounts(groupID, accounts, requestedModel, excludedIDs)
	if ownedCapacity >= target {
		return false
	}
	borrowableCapacity := p.countBorrowableSchedulableOpenAIAccounts(groupID, accounts, requestedModel, excludedIDs)
	return borrowableCapacity > 0
}

type openAIWarmPoolGlobalCoverageSnapshot struct {
	activeGroupIDs   []int64
	activeGroupSet   map[int64]struct{}
	coverageByGroup  map[int64]int
	uniqueReadyCount int
}

func buildInt64Set(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func (p *openAIAccountWarmPoolService) globalCoverageGroupIDs(now time.Time, accounts []Account) []int64 {
	groupIDs := p.activeGlobalRefillGroupIDs(now)
	if len(groupIDs) > 0 {
		return groupIDs
	}
	seen := make(map[int64]struct{}, len(accounts))
	fallback := make([]int64, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		for _, groupID := range warmPoolBucketIDsForAccount(p, acc) {
			if _, exists := seen[groupID]; exists {
				continue
			}
			seen[groupID] = struct{}{}
			fallback = append(fallback, groupID)
		}
	}
	sort.Slice(fallback, func(i, j int) bool { return fallback[i] < fallback[j] })
	return fallback
}

func (p *openAIAccountWarmPoolService) accountActiveGroupIDs(account *Account, activeGroupSet map[int64]struct{}) []int64 {
	if account == nil || len(activeGroupSet) == 0 {
		return nil
	}
	bucketIDs := warmPoolBucketIDsForAccount(p, account)
	activeGroupIDs := make([]int64, 0, len(bucketIDs))
	for _, bucketID := range bucketIDs {
		if _, exists := activeGroupSet[bucketID]; !exists {
			continue
		}
		activeGroupIDs = append(activeGroupIDs, bucketID)
	}
	return activeGroupIDs
}

func (p *openAIAccountWarmPoolService) accountTotalGroupCount(account *Account) int {
	if account == nil {
		return 0
	}
	return len(warmPoolBucketIDsForAccount(p, account))
}

func (p *openAIAccountWarmPoolService) buildGlobalCoverageSnapshot(now time.Time, accounts []Account, activeGroupIDs []int64) openAIWarmPoolGlobalCoverageSnapshot {
	snapshot := openAIWarmPoolGlobalCoverageSnapshot{
		activeGroupIDs:  append([]int64(nil), activeGroupIDs...),
		activeGroupSet:  buildInt64Set(activeGroupIDs),
		coverageByGroup: make(map[int64]int, len(activeGroupIDs)),
	}
	for _, groupID := range activeGroupIDs {
		snapshot.coverageByGroup[groupID] = 0
	}
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		activeOverlap := p.accountActiveGroupIDs(acc, snapshot.activeGroupSet)
		if len(activeOverlap) == 0 {
			continue
		}
		ready, cooling, networkError, expired := p.accountState(acc.ID).snapshot(now)
		if cooling || networkError || !ready || expired {
			continue
		}
		snapshot.uniqueReadyCount++
		for _, groupID := range activeOverlap {
			snapshot.coverageByGroup[groupID]++
		}
	}
	return snapshot
}

func (s openAIWarmPoolGlobalCoverageSnapshot) satisfied(threshold int) bool {
	if len(s.activeGroupIDs) == 0 || threshold <= 0 {
		return true
	}
	for _, groupID := range s.activeGroupIDs {
		if s.coverageByGroup[groupID] < threshold {
			return false
		}
	}
	return true
}

func (s openAIWarmPoolGlobalCoverageSnapshot) deficitCount(threshold int) int {
	if len(s.activeGroupIDs) == 0 || threshold <= 0 {
		return 0
	}
	deficit := 0
	for _, groupID := range s.activeGroupIDs {
		remaining := threshold - s.coverageByGroup[groupID]
		if remaining > 0 {
			deficit += remaining
		}
	}
	return deficit
}

func (p *openAIAccountWarmPoolService) pruneGlobalReadyOutsideActiveGroups(now time.Time, accounts []Account, activeGroupIDs []int64) int {
	activeGroupSet := buildInt64Set(activeGroupIDs)
	relevantAccountIDs := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if len(p.accountActiveGroupIDs(acc, activeGroupSet)) == 0 {
			continue
		}
		relevantAccountIDs[acc.ID] = struct{}{}
	}
	pruned := 0
	p.accountStates.Range(func(key, value any) bool {
		accountID, _ := key.(int64)
		state, _ := value.(*openAIWarmAccountState)
		if accountID <= 0 || state == nil {
			return true
		}
		if _, exists := relevantAccountIDs[accountID]; exists {
			return true
		}
		inspection := state.inspect(now)
		if !inspection.Ready {
			return true
		}
		state.clearReady()
		p.syncWarmAccountState(accountID, state, now)
		pruned++
		return true
	})
	return pruned
}

func (p *openAIAccountWarmPoolService) needsGlobalRefill(accounts []Account) bool {
	now := time.Now()
	activeGroupIDs := p.globalCoverageGroupIDs(now, accounts)
	if len(activeGroupIDs) == 0 {
		_ = p.pruneGlobalReadyOutsideActiveGroups(now, nil, nil)
		return false
	}
	_ = p.pruneGlobalReadyOutsideActiveGroups(now, accounts, activeGroupIDs)
	snapshot := p.buildGlobalCoverageSnapshot(now, accounts, activeGroupIDs)
	return !snapshot.satisfied(p.config().GlobalRefillBelow)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func (p *openAIAccountWarmPoolService) ensureBucketReady(
	ctx context.Context,
	groupID int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	syncFill bool,
) {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil {
		return
	}
	key := strings.TrimSpace("openai_warm_pool_bucket_" + formatInt64(groupID))
	if syncFill {
		_, _, _ = p.refillSF.Do(key, func() (any, error) {
			return nil, p.refillBucket(ctx, groupID, accounts, requestedModel, excludedIDs, syncFill)
		})
		return
	}
	go func() {
		_, _, _ = p.refillSF.Do(key, func() (any, error) {
			return nil, p.refillBucket(context.Background(), groupID, accounts, requestedModel, excludedIDs, false)
		})
	}()
}

func (p *openAIAccountWarmPoolService) ensureGlobalReady(ctx context.Context, accounts []Account, reason string, syncFill bool) {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil {
		return
	}
	key := "openai_warm_pool_global"
	if syncFill {
		_, _, _ = p.refillSF.Do(key, func() (any, error) {
			return nil, p.refillGlobal(ctx, accounts, reason, false)
		})
		return
	}
	go func() {
		_, _, _ = p.refillSF.Do(key, func() (any, error) {
			return nil, p.refillGlobal(context.Background(), accounts, reason, false)
		})
	}()
}

func (p *openAIAccountWarmPoolService) ensureBucketReadyForFirstTake(
	ctx context.Context,
	groupID int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
) {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil {
		return
	}
	key := strings.TrimSpace("openai_warm_pool_bucket_take_" + formatInt64(groupID))
	_, _, _ = p.refillSF.Do(key, func() (any, error) {
		return nil, p.refillBucketForFirstTake(ctx, groupID, accounts, requestedModel, excludedIDs)
	})
}

func (p *openAIAccountWarmPoolService) refillBucketForFirstTake(
	ctx context.Context,
	groupID int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
) error {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil {
		return nil
	}
	if len(accounts) == 0 {
		listed, err := p.service.listSchedulableAccounts(ctx, p.groupIDPointer(groupID))
		if err != nil {
			return err
		}
		accounts = listed
	}
	if len(accounts) == 0 {
		return nil
	}
	if len(p.collectBucketWarmCandidates(groupID, accounts, requestedModel, excludedIDs)) > 0 {
		return nil
	}
	ownedAccounts := p.ownedBucketAccounts(groupID, accounts, requestedModel, excludedIDs)
	if p.promoteBucketFromGlobal(groupID, ownedAccounts, accounts, requestedModel, excludedIDs, 1) > 0 {
		return nil
	}
	if len(ownedAccounts) > 0 {
		_ = p.probeIntoGlobal(ctx, groupID, ownedAccounts, requestedModel, excludedIDs, 1, "bucket_take_owner")
		if p.promoteBucketFromGlobal(groupID, ownedAccounts, accounts, requestedModel, excludedIDs, 1) > 0 {
			return nil
		}
	}
	if !p.shouldAllowBucketBorrow(groupID, accounts, requestedModel, excludedIDs, 1) {
		return nil
	}
	borrowableAccounts := p.borrowableBucketAccounts(groupID, accounts, requestedModel, excludedIDs)
	if p.promoteBucketFromGlobal(groupID, borrowableAccounts, accounts, requestedModel, excludedIDs, 1) > 0 {
		return nil
	}
	_ = p.probeIntoGlobal(ctx, groupID, borrowableAccounts, requestedModel, excludedIDs, 1, "bucket_take_borrow")
	p.promoteBucketFromGlobal(groupID, borrowableAccounts, accounts, requestedModel, excludedIDs, 1)
	return nil
}

func (p *openAIAccountWarmPoolService) refillBucket(
	ctx context.Context,
	groupID int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	syncFill bool,
) error {
	cfg := p.config()
	if !cfg.Enabled {
		p.writeRefillLog("info", "bucket_refill_skipped", "预热池分组池补池已跳过，功能当前已关闭", groupID, map[string]any{
			"reason":    "disabled",
			"sync_fill": syncFill,
		})
		return nil
	}
	bucket := p.bucketState(groupID)
	if bucket == nil {
		return nil
	}
	now := time.Now()
	initialFill := bucket.readyCount() == 0 && bucket.lastRefill.Load() == 0
	lastRefillUnix := bucket.lastRefill.Load()
	lastRefill := time.Unix(0, lastRefillUnix)
	if lastRefillUnix != 0 && cfg.BucketRefillCooldown > 0 && now.Sub(lastRefill) < cfg.BucketRefillCooldown {
		p.writeRefillLog("info", "bucket_refill_skipped", "预热池分组池补池已跳过，分组池仍在冷却中", groupID, map[string]any{
			"reason":                  "cooldown",
			"sync_fill":               syncFill,
			"last_refill_at":          lastRefill.UTC().Format(time.RFC3339Nano),
			"refill_cooldown_seconds": int(cfg.BucketRefillCooldown / time.Second),
		})
		return nil
	}
	bucket.lastRefill.Store(now.UnixNano())
	p.touchWarmBucketRefill(groupID, now)
	p.lastBucketMaintenance.Store(now.UnixNano())
	p.touchWarmLastBucketMaintenance(now)

	if len(accounts) == 0 {
		listed, err := p.service.listSchedulableAccounts(ctx, p.groupIDPointer(groupID))
		if err != nil {
			p.writeRefillLog("error", "bucket_refill_failed", "预热池分组池补池加载可调度账号失败", groupID, map[string]any{
				"reason":    "list_schedulable_failed",
				"sync_fill": syncFill,
				"error":     err.Error(),
			})
			return err
		}

		accounts = listed
	}
	if len(accounts) == 0 {
		p.writeRefillLog("info", "bucket_refill_skipped", "预热池分组池补池已跳过，未找到可调度账号", groupID, map[string]any{
			"reason":    "no_accounts",
			"sync_fill": syncFill,
		})
		return nil
	}

	maxReadyPossible := p.countSchedulableOpenAIAccounts(accounts, "", nil)
	maxUsablePossible := p.countSchedulableOpenAIAccounts(accounts, requestedModel, excludedIDs)
	ownedTotalCapacity := p.countOwnedSchedulableOpenAIAccounts(groupID, accounts, "", nil)
	ownedUsableCapacity := p.countOwnedSchedulableOpenAIAccounts(groupID, accounts, requestedModel, excludedIDs)
	targetTotal := cfg.BucketTargetSize
	if targetTotal > maxReadyPossible {
		targetTotal = maxReadyPossible
	}
	if targetTotal < 0 {
		targetTotal = 0
	}
	targetUsable := 0
	if syncFill {
		targetUsable = cfg.BucketSyncFillMin
		if targetUsable > maxUsablePossible {
			targetUsable = maxUsablePossible
		}
		if targetUsable < 0 {
			targetUsable = 0
		}
	}
	ownerTotalTarget := targetTotal
	if ownerTotalTarget > ownedTotalCapacity {
		ownerTotalTarget = ownedTotalCapacity
	}
	ownerUsableTarget := targetUsable
	if ownerUsableTarget > ownedUsableCapacity {
		ownerUsableTarget = ownedUsableCapacity
	}
	allowBorrowTotal := p.shouldAllowBucketBorrow(groupID, accounts, "", nil, targetTotal)
	allowBorrowUsable := p.shouldAllowBucketBorrow(groupID, accounts, requestedModel, excludedIDs, targetUsable)
	ownedTotalAccounts := p.ownedBucketAccounts(groupID, accounts, "", nil)
	ownedUsableAccounts := p.ownedBucketAccounts(groupID, accounts, requestedModel, excludedIDs)
	borrowableTotalAccounts := p.borrowableBucketAccounts(groupID, accounts, "", nil)
	borrowableUsableAccounts := p.borrowableBucketAccounts(groupID, accounts, requestedModel, excludedIDs)

	currentTotal := p.countBucketWarmReady(groupID, accounts)
	if targetTotal > 0 && currentTotal < targetTotal {
		bucket.globalBootstrapDone.Store(false)
	}
	currentUsable := len(p.collectBucketWarmCandidates(groupID, accounts, requestedModel, excludedIDs))
	if currentTotal >= targetTotal && (targetUsable <= 0 || currentUsable >= targetUsable) {
		p.maybeBootstrapGlobalAfterBucketReady(ctx, groupID, bucket, currentTotal, targetTotal)
		return nil
	}

	refreshCounts := func() {
		currentTotal = p.countBucketWarmReady(groupID, accounts)
		currentUsable = len(p.collectBucketWarmCandidates(groupID, accounts, requestedModel, excludedIDs))
	}

	promotedCount := 0
	if ownerTotalTarget > 0 && currentTotal < ownerTotalTarget {
		promotedCount += p.promoteBucketFromGlobal(groupID, ownedTotalAccounts, accounts, "", nil, ownerTotalTarget)
		refreshCounts()
		if currentTotal < ownerTotalTarget {
			totalNeed := ownerTotalTarget - currentTotal
			if totalNeed > 0 {
				_ = p.probeIntoGlobal(ctx, groupID, ownedTotalAccounts, "", nil, totalNeed, "bucket_owner")
				promotedCount += p.promoteBucketFromGlobal(groupID, ownedTotalAccounts, accounts, "", nil, ownerTotalTarget)
				refreshCounts()
			}
		}
	}
	if targetTotal > 0 && currentTotal < targetTotal && allowBorrowTotal {
		promotedCount += p.promoteBucketFromGlobal(groupID, borrowableTotalAccounts, accounts, "", nil, targetTotal)
		refreshCounts()
		if currentTotal < targetTotal {
			totalNeed := targetTotal - currentTotal
			if totalNeed > 0 {
				_ = p.probeIntoGlobal(ctx, groupID, borrowableTotalAccounts, "", nil, totalNeed, "bucket_borrow")
				promotedCount += p.promoteBucketFromGlobal(groupID, borrowableTotalAccounts, accounts, "", nil, targetTotal)
				refreshCounts()
			}
		}
	}
	if ownerUsableTarget > 0 && currentUsable < ownerUsableTarget {
		promotedCount += p.promoteBucketFromGlobal(groupID, ownedUsableAccounts, accounts, requestedModel, excludedIDs, ownerUsableTarget)
		refreshCounts()
		if currentUsable < ownerUsableTarget {
			usableNeed := ownerUsableTarget - currentUsable
			if usableNeed > 0 {
				_ = p.probeIntoGlobal(ctx, groupID, ownedUsableAccounts, requestedModel, excludedIDs, usableNeed, "bucket_owner")
				promotedCount += p.promoteBucketFromGlobal(groupID, ownedUsableAccounts, accounts, requestedModel, excludedIDs, ownerUsableTarget)
				refreshCounts()
			}
		}
	}
	if targetUsable > 0 && currentUsable < targetUsable && allowBorrowUsable {
		promotedCount += p.promoteBucketFromGlobal(groupID, borrowableUsableAccounts, accounts, requestedModel, excludedIDs, targetUsable)
		refreshCounts()
		if currentUsable < targetUsable {
			usableNeed := targetUsable - currentUsable
			if usableNeed > 0 {
				_ = p.probeIntoGlobal(ctx, groupID, borrowableUsableAccounts, requestedModel, excludedIDs, usableNeed, "bucket_borrow")
				promotedCount += p.promoteBucketFromGlobal(groupID, borrowableUsableAccounts, accounts, requestedModel, excludedIDs, targetUsable)
				refreshCounts()
			}
		}
	}

	p.writeRefillLog("info", "bucket_refill_done", "预热池分组池补池完成", groupID, map[string]any{
		"sync_fill":            syncFill,
		"initial_fill":         initialFill,
		"promoted_count":       promotedCount,
		"final_bucket_ready":   currentTotal,
		"final_bucket_usable":  currentUsable,
		"bucket_target_size":   targetTotal,
		"bucket_sync_fill_min": targetUsable,
		"requested_model":      strings.TrimSpace(requestedModel),
	})

	p.maybeBootstrapGlobalAfterBucketReady(ctx, groupID, bucket, currentTotal, targetTotal)
	return nil
}

func (p *openAIAccountWarmPoolService) maybeBootstrapGlobalAfterBucketReady(ctx context.Context, groupID int64, bucket *openAIWarmPoolBucketState, currentTotal int, targetTotal int) {
	if p == nil || bucket == nil || targetTotal <= 0 || currentTotal < targetTotal {
		return
	}
	if bucket.globalBootstrapDone.Load() {
		return
	}
	allAccounts, err := p.listGlobalRefillAccounts(ctx, groupID)
	if err != nil {
		p.writeRefillLog("warn", "global_refill_skipped", "预热池分组池补满后无法继续补全局池", groupID, map[string]any{
			"reason": "list_global_schedulable_failed",
			"error":  err.Error(),
		})
		return
	}
	if len(allAccounts) == 0 {
		return
	}
	key := "openai_warm_pool_global"
	_, err, _ = p.refillSF.Do(key, func() (any, error) {
		return nil, p.refillGlobal(ctx, allAccounts, "bucket_bootstrap", true)
	})
	if err != nil {
		p.writeRefillLog("warn", "global_refill_skipped", "预热池分组池补满后无法继续补全局池", groupID, map[string]any{
			"reason": "bootstrap_refill_failed",
			"error":  err.Error(),
		})
		return
	}
	bucket.globalBootstrapDone.Store(true)
}

func (p *openAIAccountWarmPoolService) activeGlobalRefillGroupIDs(now time.Time, seedGroupIDs ...int64) []int64 {
	if p == nil {
		return nil
	}
	cfg := p.config()
	seen := make(map[int64]struct{}, 8)
	groupIDs := make([]int64, 0, 8)
	appendGroupID := func(groupID int64) {
		if _, exists := seen[groupID]; exists {
			return
		}
		seen[groupID] = struct{}{}
		groupIDs = append(groupIDs, groupID)
	}
	for _, groupID := range seedGroupIDs {
		appendGroupID(groupID)
	}
	for _, snapshot := range p.collectActiveOpsBuckets(now, cfg.ActiveBucketTTL) {
		appendGroupID(snapshot.groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	return groupIDs
}

func (p *openAIAccountWarmPoolService) listGlobalRefillAccounts(ctx context.Context, seedGroupIDs ...int64) ([]Account, error) {
	if p == nil || p.service == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	groupIDs := p.activeGlobalRefillGroupIDs(time.Now(), seedGroupIDs...)
	if len(groupIDs) == 0 {
		return nil, nil
	}
	seenAccounts := make(map[int64]struct{}, 16)
	accounts := make([]Account, 0, 16)
	for _, groupID := range groupIDs {
		listed, err := p.service.listSchedulableAccounts(ctx, p.groupIDPointer(groupID))
		if err != nil {
			return nil, err
		}
		for _, acc := range listed {
			if !acc.IsOpenAI() || !acc.IsSchedulable() {
				continue
			}
			if _, exists := seenAccounts[acc.ID]; exists {
				continue
			}
			seenAccounts[acc.ID] = struct{}{}
			accounts = append(accounts, acc)
		}
	}
	return accounts, nil
}

func (p *openAIAccountWarmPoolService) promoteBucketFromGlobal(groupID int64, candidateAccounts []Account, currentAccounts []Account, requestedModel string, excludedIDs map[int64]struct{}, target int) int {
	bucket := p.bucketState(groupID)
	if bucket == nil || target <= 0 {
		return 0
	}
	now := time.Now()
	currentCount := p.countBucketWarmReady(groupID, currentAccounts)
	if currentCount >= target {
		return 0
	}
	promoted := 0
	for i := range candidateAccounts {
		if currentCount >= target {
			break
		}
		acc := &candidateAccounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if requestedModel != "" && !acc.IsModelSupported(requestedModel) {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		ready, cooling, networkError, expired := p.accountState(acc.ID).snapshot(now)
		if cooling || networkError || !ready || expired {
			continue
		}
		if bucket.promote(acc.ID, now, p.config().BucketEntryTTL) {
			p.touchWarmBucketMember(groupID, acc.ID, now)
			promoted++
			currentCount++
		}
	}
	return promoted
}

func (p *openAIAccountWarmPoolService) triggerManualGlobalRefill(ctx context.Context) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := "openai_warm_pool_global"
	_, err, _ := p.refillSF.Do(key, func() (any, error) {
		return nil, p.refillGlobal(ctx, nil, "manual_trigger", true)
	})
	return err
}

func (p *openAIAccountWarmPoolService) refillGlobal(ctx context.Context, accounts []Account, reason string, ignoreCooldown bool) error {
	cfg := p.config()
	if !cfg.Enabled || p.getUsageReader() == nil {
		return nil
	}
	now := time.Now()
	p.lastGlobalMaintenance.Store(now.UnixNano())
	p.touchWarmLastGlobalMaintenance(now)
	last := p.lastGlobalRefill.Load()
	if !ignoreCooldown && last != 0 && cfg.GlobalRefillCooldown > 0 && now.Sub(time.Unix(0, last)) < cfg.GlobalRefillCooldown {
		return nil
	}
	p.lastGlobalRefill.Store(now.UnixNano())
	activeGroupIDs := p.globalCoverageGroupIDs(now, accounts)
	if len(accounts) == 0 {
		listed, err := p.listGlobalRefillAccounts(ctx)
		if err != nil {
			p.writeRefillLog("error", "global_refill_failed", "预热池全局池补池加载可调度账号失败", 0, map[string]any{
				"reason": "list_schedulable_failed",
				"scope":  strings.TrimSpace(reason),
				"error":  err.Error(),
			})
			return err
		}
		accounts = listed
		activeGroupIDs = p.globalCoverageGroupIDs(now, accounts)
	}
	if len(activeGroupIDs) == 0 {
		pruned := p.pruneGlobalReadyOutsideActiveGroups(now, nil, nil)
		if pruned > 0 {
			p.writeRefillLog("info", "global_pruned", "预热池全局池已移除不属于当前活跃组的账号", 0, map[string]any{
				"reason":       strings.TrimSpace(reason),
				"pruned_count": pruned,
			})
		}
		return nil
	}
	pruned := p.pruneGlobalReadyOutsideActiveGroups(now, accounts, activeGroupIDs)
	coverage := p.buildGlobalCoverageSnapshot(now, accounts, activeGroupIDs)
	if coverage.satisfied(cfg.GlobalTargetSize) {
		if pruned > 0 {
			p.writeRefillLog("info", "global_pruned", "预热池全局池已移除不属于当前活跃组的账号", 0, map[string]any{
				"reason":             strings.TrimSpace(reason),
				"active_group_count": len(activeGroupIDs),
				"pruned_count":       pruned,
			})
		}
		return nil
	}
	launched, successCount, failureCount, skippedCount, networkErrorCount := p.probeGlobalCoverage(ctx, accounts, activeGroupIDs, cfg.GlobalTargetSize)
	finalCoverage := p.buildGlobalCoverageSnapshot(time.Now(), accounts, activeGroupIDs)
	p.writeRefillLog("info", "global_refill_done", "预热池全局池补池完成", 0, map[string]any{
		"reason":                  strings.TrimSpace(reason),
		"active_group_count":      len(activeGroupIDs),
		"global_target_size":      cfg.GlobalTargetSize,
		"global_unique_ready":     finalCoverage.uniqueReadyCount,
		"global_coverage_deficit": finalCoverage.deficitCount(cfg.GlobalTargetSize),
		"probe_launched_count":    launched,
		"success_count":           successCount,
		"failure_count":           failureCount,
		"skipped_count":           skippedCount,
		"network_error_count":     networkErrorCount,
		"pruned_count":            pruned,
		"network_error_pool":      p.countNetworkErrorPoolAccounts(time.Now()),
	})
	return nil
}

func (p *openAIAccountWarmPoolService) probeGlobalCoverage(ctx context.Context, accounts []Account, activeGroupIDs []int64, targetPerGroup int) (launched int, successCount int, failureCount int, skippedCount int, networkErrorCount int) {
	if p == nil || len(accounts) == 0 || len(activeGroupIDs) == 0 || targetPerGroup <= 0 {
		return 0, 0, 0, 0, 0
	}
	cfg := p.config()
	if ctx == nil {
		ctx = context.Background()
	}
	excludedIDs := make(map[int64]struct{}, len(accounts))
	for {
		coverage := p.buildGlobalCoverageSnapshot(time.Now(), accounts, activeGroupIDs)
		if coverage.satisfied(targetPerGroup) {
			break
		}
		if cfg.ProbeMaxCandidates > 0 && launched >= cfg.ProbeMaxCandidates {
			break
		}
		candidate := p.selectBestGlobalProbeCandidate(accounts, activeGroupIDs, coverage.coverageByGroup, targetPerGroup, excludedIDs)
		if candidate == nil {
			break
		}
		excludedIDs[candidate.ID] = struct{}{}
		launched++
		result := p.probeCandidate(ctx, 0, candidate)
		switch result.Outcome {
		case openAIWarmPoolProbeReadyOutcome:
			successCount++
		case openAIWarmPoolProbeFailedOutcome:
			failureCount++
		case openAIWarmPoolProbeSkippedOutcome:
			skippedCount++
		case openAIWarmPoolProbeNetworkErrorOutcome:
			networkErrorCount++
		}
	}
	return launched, successCount, failureCount, skippedCount, networkErrorCount
}

func (p *openAIAccountWarmPoolService) selectBestGlobalProbeCandidate(accounts []Account, activeGroupIDs []int64, coverageByGroup map[int64]int, targetPerGroup int, excludedIDs map[int64]struct{}) *Account {
	type probeCandidateItem struct {
		account             *Account
		expiredReady        bool
		networkError        bool
		marginalGain        int
		activeCoverageCount int
		totalGroupCount     int
	}
	activeGroupSet := buildInt64Set(activeGroupIDs)
	now := time.Now()
	candidates := make([]probeCandidateItem, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		inspection := p.accountState(acc.ID).inspect(now)
		if inspection.Cooling || inspection.Probing {
			continue
		}
		if inspection.Ready && !inspection.Expired {
			continue
		}
		activeOverlap := p.accountActiveGroupIDs(acc, activeGroupSet)
		if len(activeOverlap) == 0 {
			continue
		}
		marginalGain := 0
		for _, groupID := range activeOverlap {
			if coverageByGroup[groupID] < targetPerGroup {
				marginalGain++
			}
		}
		if marginalGain <= 0 {
			continue
		}
		candidates = append(candidates, probeCandidateItem{
			account:             acc,
			expiredReady:        inspection.Ready && inspection.Expired,
			networkError:        inspection.NetworkError,
			marginalGain:        marginalGain,
			activeCoverageCount: len(activeOverlap),
			totalGroupCount:     p.accountTotalGroupCount(acc),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.marginalGain != right.marginalGain {
			return left.marginalGain > right.marginalGain
		}
		if left.activeCoverageCount != right.activeCoverageCount {
			return left.activeCoverageCount > right.activeCoverageCount
		}
		if left.totalGroupCount != right.totalGroupCount {
			return left.totalGroupCount > right.totalGroupCount
		}
		if left.expiredReady != right.expiredReady {
			return left.expiredReady
		}
		if left.networkError != right.networkError {
			return left.networkError
		}
		a, b := left.account, right.account
		switch {
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return true
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return false
		case a.LastUsedAt != nil && b.LastUsedAt != nil:
			if !a.LastUsedAt.Equal(*b.LastUsedAt) {
				return a.LastUsedAt.After(*b.LastUsedAt)
			}
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
	return candidates[0].account
}

func (p *openAIAccountWarmPoolService) probeIntoGlobal(
	ctx context.Context,
	groupID int64,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	successNeed int,
	scope string,
) error {
	cfg := p.config()
	if successNeed <= 0 || len(accounts) == 0 {
		return nil
	}
	candidates := p.selectProbeCandidates(accounts, requestedModel, excludedIDs)
	if len(candidates) == 0 {
		return nil
	}
	candidateCount := len(candidates)
	if cfg.ProbeMaxCandidates > 0 && len(candidates) > cfg.ProbeMaxCandidates {
		candidates = candidates[:cfg.ProbeMaxCandidates]
	}

	p.writeRefillLog("info", scope+"_probe_start", "预热池探测批次已开始", groupID, map[string]any{
		"scope":                 strings.TrimSpace(scope),
		"probe_candidate_count": candidateCount,
		"probe_candidates_used": len(candidates),
		"success_need":          successNeed,
		"requested_model":       strings.TrimSpace(requestedModel),
	})

	probeCtx := ctx
	if probeCtx == nil {
		probeCtx = context.Background()
	}
	refillCtx, cancelRefill := context.WithCancel(probeCtx)
	defer cancelRefill()

	var successCount, failureCount, skippedCount, networkErrorCount int
	type warmPoolProbeOutcome struct {
		account *Account
		result  openAIWarmPoolProbeResult
	}
	resultsCh := make(chan warmPoolProbeOutcome, maxInt(cfg.ProbeConcurrency, 1))
	inFlight := 0
	nextCandidate := 0
	var promotionSlots atomic.Int64
	promotionSlots.Store(int64(successNeed))
	reservePromotion := func() bool {
		for {
			remaining := promotionSlots.Load()
			if remaining <= 0 {
				return false
			}
			if promotionSlots.CompareAndSwap(remaining, remaining-1) {
				return true
			}
		}
	}
	launchCandidate := func(account *Account) {
		inFlight++
		go func() {
			resultsCh <- warmPoolProbeOutcome{account: account, result: p.probeCandidate(refillCtx, groupID, account, reservePromotion)}
		}()
	}

	stoppedOnTarget := false
	for {
		remainingNeed := int(promotionSlots.Load())
		for nextCandidate < len(candidates) && inFlight < cfg.ProbeConcurrency && remainingNeed > 0 {
			launchCandidate(candidates[nextCandidate])
			nextCandidate++
			remainingNeed = int(promotionSlots.Load())
		}
		if inFlight == 0 {
			break
		}
		outcome := <-resultsCh
		inFlight--
		switch outcome.result.Outcome {
		case openAIWarmPoolProbeReadyOutcome:
			successCount++
		case openAIWarmPoolProbeFailedOutcome:
			failureCount++
		case openAIWarmPoolProbeSkippedOutcome:
			skippedCount++
		case openAIWarmPoolProbeNetworkErrorOutcome:
			networkErrorCount++
		}
		if !stoppedOnTarget && promotionSlots.Load() <= 0 {
			stoppedOnTarget = true
			cancelRefill()
			p.writeRefillLog("info", scope+"_target_reached", "预热池探测批次已达到目标，停止继续发起探测", groupID, map[string]any{
				"scope":                           strings.TrimSpace(scope),
				"success_count":                   successCount,
				"probe_launched_count":            nextCandidate,
				"probe_inflight_cancelled":        inFlight,
				"remaining_candidates_unlaunched": maxInt(len(candidates)-nextCandidate, 0),
			})
		}
	}
	p.writeRefillLog("info", scope+"_probe_done", "预热池探测批次完成", groupID, map[string]any{
		"scope":                           strings.TrimSpace(scope),
		"success_count":                   successCount,
		"failure_count":                   failureCount,
		"skipped_count":                   skippedCount,
		"network_error_count":             networkErrorCount,
		"probe_launched_count":            nextCandidate,
		"launch_stopped_on_target":        stoppedOnTarget,
		"remaining_candidates_unlaunched": maxInt(len(candidates)-nextCandidate, 0),
	})
	return nil
}

func (p *openAIAccountWarmPoolService) selectProbeCandidates(accounts []Account, requestedModel string, excludedIDs map[int64]struct{}) []*Account {
	type probeCandidateItem struct {
		account         *Account
		expiredReady    bool
		networkError    bool
		totalGroupCount int
	}

	now := time.Now()
	candidates := make([]probeCandidateItem, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if excludedIDs != nil {
			if _, excluded := excludedIDs[acc.ID]; excluded {
				continue
			}
		}
		inspection := p.accountState(acc.ID).inspect(now)
		if inspection.Cooling || inspection.Probing {
			continue
		}
		if inspection.Ready && !inspection.Expired {
			continue
		}
		candidates = append(candidates, probeCandidateItem{
			account:         acc,
			expiredReady:    inspection.Ready && inspection.Expired,
			networkError:    inspection.NetworkError,
			totalGroupCount: p.accountTotalGroupCount(acc),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.expiredReady != right.expiredReady {
			return left.expiredReady
		}
		if left.networkError != right.networkError {
			return left.networkError
		}
		if left.totalGroupCount != right.totalGroupCount {
			return left.totalGroupCount > right.totalGroupCount
		}
		a, b := left.account, right.account
		aSupports := requestedModel != "" && a.IsModelSupported(requestedModel)
		bSupports := requestedModel != "" && b.IsModelSupported(requestedModel)
		if aSupports != bSupports {
			return aSupports
		}
		switch {
		case a.LastUsedAt != nil && b.LastUsedAt == nil:
			return true
		case a.LastUsedAt == nil && b.LastUsedAt != nil:
			return false
		case a.LastUsedAt != nil && b.LastUsedAt != nil:
			if !a.LastUsedAt.Equal(*b.LastUsedAt) {
				return a.LastUsedAt.After(*b.LastUsedAt)
			}
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
	result := make([]*Account, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.account)
	}
	return result
}

func (p *openAIAccountWarmPoolService) markNetworkErrorOrFailure(groupID int64, account *Account, state *openAIWarmAccountState, cfg openAIWarmPoolConfigView, reason, message string, fields map[string]any) openAIWarmPoolProbeResult {
	if state == nil {
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: reason}
	}
	now := time.Now()
	if cfg.NetworkErrorEntryTTL > 0 {
		state.markNetworkError(now, cfg.NetworkErrorEntryTTL)
		p.syncWarmAccountState(account.ID, state, now)
		eventFields := map[string]any{
			"network_error_until": now.Add(cfg.NetworkErrorEntryTTL).UTC().Format(time.RFC3339Nano),
		}
		for k, v := range fields {
			eventFields[k] = v
		}
		p.writeProbeLog("warn", "probe_network_error", reason, message, groupID, account, eventFields)
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeNetworkErrorOutcome, Reason: reason}
	}
	state.finishFailure(now, cfg.FailureCooldown)
	p.syncWarmAccountState(account.ID, state, now)
	p.writeProbeLog("warn", "probe_failed", reason, message, groupID, account, fields)
	return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: reason}
}

func (p *openAIAccountWarmPoolService) probeCandidate(ctx context.Context, groupID int64, account *Account, reservePromotion ...func() bool) openAIWarmPoolProbeResult {
	if p == nil || account == nil {
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "empty_account"}
	}
	reader := p.getUsageReader()
	if reader == nil {
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "no_usage_reader"}
	}
	if !account.IsOpenAIOAuth() {
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "not_openai_oauth"}
	}
	cfg := p.config()
	state := p.accountState(account.ID)
	now := time.Now()
	inspection := state.inspect(now)
	expiredReadyRecheck := inspection.Ready && inspection.Expired
	mergeProbeFields := func(fields map[string]any) map[string]any {
		if !expiredReadyRecheck {
			return fields
		}
		merged := make(map[string]any, len(fields)+4)
		merged["expired_ready_recheck"] = true
		merged["recheck_cause"] = "expired_ttl"
		if inspection.VerifiedAt != nil {
			merged["previous_verified_at"] = inspection.VerifiedAt.UTC().Format(time.RFC3339Nano)
		}
		if inspection.ExpiresAt != nil {
			merged["previous_expires_at"] = inspection.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		for k, v := range fields {
			merged[k] = v
		}
		return merged
	}
	if !state.tryStartProbe(now, len(reservePromotion) == 0) {
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "state_busy_or_cooling"}
	}
	p.syncWarmAccountState(account.ID, state, now)
	defer func() {
		if r := recover(); r != nil {
			state.abortProbe()
			p.syncWarmAccountState(account.ID, state, time.Now())
			panic(r)
		}
	}()

	result, err := p.service.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil {
		state.abortProbe()
		p.syncWarmAccountState(account.ID, state, time.Now())
		p.writeProbeLog("error", "probe_failed", "slot_acquire_failed", "预热池探测获取账号槽位失败", groupID, account, mergeProbeFields(map[string]any{
			"error": err.Error(),
		}))
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "slot_acquire_failed", Err: err}
	}

	if result == nil || !result.Acquired {
		state.abortProbe()
		p.syncWarmAccountState(account.ID, state, time.Now())
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "slot_unavailable"}
	}
	defer func() {
		if result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
	}()

	probeBaseCtx := ctx
	if probeBaseCtx == nil {
		probeBaseCtx = context.Background()
	}
	withAttemptFields := func(fields map[string]any, attempt int, retrying bool) map[string]any {
		copied := make(map[string]any, len(fields)+3)
		for k, v := range fields {
			copied[k] = v
		}
		merged := mergeProbeFields(copied)
		if merged == nil {
			merged = copied
		}
		merged["probe_attempt"] = attempt
		merged["probe_max_attempts"] = openAIWarmPoolProbeNetworkRetryMaxAttempts
		if retrying {
			merged["retrying"] = true
		}
		return merged
	}
	logNetworkRetry := func(reason, message string, attempt int, fields map[string]any) {
		p.writeProbeLog("info", "probe_retrying", reason, message, groupID, account, withAttemptFields(fields, attempt, true))
	}

	var usage *UsageInfo
	for attempt := 1; attempt <= openAIWarmPoolProbeNetworkRetryMaxAttempts; attempt++ {
		attemptCtx := probeBaseCtx
		var cancel context.CancelFunc
		if cfg.ProbeTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(attemptCtx, cfg.ProbeTimeout)
		} else {
			attemptCtx, cancel = context.WithCancel(attemptCtx)
		}
		usage, err = reader.GetUsage(attemptCtx, account.ID, true)
		attemptErr := attemptCtx.Err()
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(attemptErr, context.Canceled) {
				state.abortProbe()
				p.syncWarmAccountState(account.ID, state, time.Now())
				return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "canceled", Err: err}
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptErr, context.DeadlineExceeded) {
				fields := map[string]any{"error": err.Error()}
				if attempt < openAIWarmPoolProbeNetworkRetryMaxAttempts {
					logNetworkRetry("timeout", "预热池探测遇到网络超时，正在重试一次", attempt, fields)
					continue
				}
				result := p.markNetworkErrorOrFailure(groupID, account, state, cfg, "timeout", "预热池探测遇到网络超时，重试一次后仍失败，账号已移入网络异常池", withAttemptFields(fields, attempt, false))
				result.Err = err
				return result
			}
			fields := map[string]any{"error": err.Error()}
			if attempt < openAIWarmPoolProbeNetworkRetryMaxAttempts {
				logNetworkRetry("usage_error", "预热池探测遇到网络类用量刷新错误，正在重试一次", attempt, fields)
				continue
			}
			result := p.markNetworkErrorOrFailure(groupID, account, state, cfg, "usage_error", "预热池探测遇到网络类用量刷新错误，重试一次后仍失败，账号已移入网络异常池", withAttemptFields(fields, attempt, false))
			result.Err = err
			return result
		}
		if attemptErr != nil {
			if errors.Is(attemptErr, context.Canceled) {
				state.abortProbe()
				p.syncWarmAccountState(account.ID, state, time.Now())
				return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "canceled", Err: attemptErr}
			}
			fields := map[string]any{"error": attemptErr.Error()}
			if attempt < openAIWarmPoolProbeNetworkRetryMaxAttempts {
				logNetworkRetry("timeout", "预热池探测在超时后返回，正在重试一次", attempt, fields)
				continue
			}
			result := p.markNetworkErrorOrFailure(groupID, account, state, cfg, "timeout", "预热池探测在超时后返回，重试一次后仍失败，账号已移入网络异常池", withAttemptFields(fields, attempt, false))
			result.Err = attemptErr
			return result
		}
		if usage == nil {
			now := time.Now()
			state.finishFailure(now, cfg.FailureCooldown)
			p.syncWarmAccountState(account.ID, state, now)
			p.writeProbeLog("warn", "probe_failed", "usage_nil", "预热池探测返回了空的用量结果", groupID, account, mergeProbeFields(nil))
			return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "usage_nil"}
		}
		if usage.NeedsReauth {
			now := time.Now()
			state.finishFailure(now, cfg.FailureCooldown)
			p.syncWarmAccountState(account.ID, state, now)
			p.writeProbeLog("warn", "probe_failed", "needs_reauth", "预热池探测失败：账号需要重新授权", groupID, account, mergeProbeFields(map[string]any{
				"error_code": strings.TrimSpace(usage.ErrorCode),
				"error":      strings.TrimSpace(usage.Error),
			}))
			return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "needs_reauth"}
		}
		if usage.IsForbidden {
			now := time.Now()
			state.finishFailure(now, cfg.FailureCooldown)
			p.syncWarmAccountState(account.ID, state, now)
			p.writeProbeLog("warn", "probe_failed", "forbidden", "预热池探测失败：上游将账号标记为禁止使用", groupID, account, mergeProbeFields(map[string]any{
				"error_code": usage.ErrorCode,
			}))

			return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "forbidden"}
		}
		if strings.TrimSpace(usage.ErrorCode) != "" || strings.TrimSpace(usage.Error) != "" {
			if strings.EqualFold(strings.TrimSpace(usage.ErrorCode), errorCodeNetworkError) {
				fields := map[string]any{
					"error_code": strings.TrimSpace(usage.ErrorCode),
					"error":      strings.TrimSpace(usage.Error),
				}
				if attempt < openAIWarmPoolProbeNetworkRetryMaxAttempts {
					logNetworkRetry("network_error", "预热池探测检测到 network_error 用量响应，正在重试一次", attempt, fields)
					continue
				}
				return p.markNetworkErrorOrFailure(groupID, account, state, cfg, "network_error", "预热池探测检测到 network_error 用量响应，重试一次后仍失败，账号已移入网络异常池", withAttemptFields(fields, attempt, false))
			}
			now := time.Now()
			state.finishFailure(now, cfg.FailureCooldown)
			p.syncWarmAccountState(account.ID, state, now)
			p.writeProbeLog("warn", "probe_failed", "usage_invalid", "预热池探测失败：用量响应返回错误", groupID, account, mergeProbeFields(map[string]any{
				"error_code": strings.TrimSpace(usage.ErrorCode),
				"error":      strings.TrimSpace(usage.Error),
			}))
			return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "usage_invalid"}
		}
		break
	}
	var (
		fresh    *Account
		freshErr error
	)
	freshCtx := probeBaseCtx
	var freshCancel context.CancelFunc
	if cfg.ProbeTimeout > 0 {
		freshCtx, freshCancel = context.WithTimeout(freshCtx, cfg.ProbeTimeout)
	} else {
		freshCtx, freshCancel = context.WithCancel(freshCtx)
	}
	defer freshCancel()
	if p.service == nil {
		freshErr = errors.New("openai gateway service is nil")
	} else if p.service.accountRepo != nil {
		fresh, freshErr = p.service.accountRepo.GetByID(freshCtx, account.ID)
	} else {
		fresh, freshErr = p.service.getSchedulableAccount(freshCtx, account.ID)
	}
	if freshErr != nil {
		now := time.Now()
		state.finishFailure(now, cfg.FailureCooldown)
		p.syncWarmAccountState(account.ID, state, now)
		p.writeProbeLog("error", "probe_failed", "fresh_account_lookup_failed", "预热池探测刷新账号状态失败", groupID, account, mergeProbeFields(map[string]any{
			"error": freshErr.Error(),
		}))

		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "fresh_account_lookup_failed", Err: freshErr}
	}
	if fresh == nil || !fresh.IsSchedulable() || !fresh.IsOpenAI() {
		now := time.Now()
		state.finishFailure(now, cfg.FailureCooldown)
		p.syncWarmAccountState(account.ID, state, now)
		p.writeProbeLog("warn", "probe_failed", "account_not_schedulable", "预热池探测失败：刷新后的账号已不可调度", groupID, account, mergeProbeFields(nil))
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeFailedOutcome, Reason: "account_not_schedulable"}
	}
	readyAt := time.Now()
	if len(reservePromotion) > 0 && reservePromotion[0] != nil {
		if !reservePromotion[0]() {
			state.abortProbe()
			p.syncWarmAccountState(account.ID, state, time.Now())
			return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeSkippedOutcome, Reason: "target_reached"}
		}
	}
	state.finishSuccess(readyAt, cfg.GlobalEntryTTL)
	p.syncWarmAccountState(account.ID, state, readyAt)
	expiresAt := readyAt.Add(cfg.GlobalEntryTTL)

	if expiredReadyRecheck {
		p.writeProbeLog("info", "ready_recheck_done", "expired", "预热池过期就绪账号已通过复检，并刷新了全局有效期", groupID, account, mergeProbeFields(map[string]any{
			"expires_at": expiresAt.UTC().Format(time.RFC3339Nano),
		}))
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeReadyOutcome, Reason: "ready"}
	}
	if len(reservePromotion) == 0 && groupID > 0 {
		p.writeProbeLog("info", "bucket_member_refreshed", "ready", "分组 "+formatOpenAIWarmPoolGroupLabel(account, groupID)+" 预热池成员复检成功，账号 "+formatOpenAIWarmPoolAccountLabel(account)+" 已刷新就绪有效期", groupID, account, map[string]any{
			"expires_at": expiresAt.UTC().Format(time.RFC3339Nano),
		})
		return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeReadyOutcome, Reason: "ready"}
	}
	p.writeProbeLog("info", "probe_ready", "ready", "分组 "+formatOpenAIWarmPoolGroupLabel(account, groupID)+" 预热池探测成功，账号 "+formatOpenAIWarmPoolAccountLabel(account)+" 已进入全局池就绪状态", groupID, account, map[string]any{
		"expires_at": expiresAt.UTC().Format(time.RFC3339Nano),
	})
	return openAIWarmPoolProbeResult{Outcome: openAIWarmPoolProbeReadyOutcome, Reason: "ready"}
}
