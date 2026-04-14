package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

type SchedulerSnapshotObserver interface {
	HandleSchedulerOutboxEvent(ctx context.Context, event SchedulerOutboxEvent) error
}

const (
	defaultOpsRealtimeReconcileInterval = 10 * time.Minute
	opsRealtimeReconcileTimeout         = 2 * time.Minute
	opsRealtimeReconcileLeaderTTL       = 3 * time.Minute
)

type OpsRealtimeProjector struct {
	cache       OpsRealtimeCache
	accountRepo AccountRepository
	instanceID  string

	reconcileInterval time.Duration
	startOnce         sync.Once
	skipLogMu         sync.Mutex
	skipLogAt         time.Time
}

func NewOpsRealtimeProjector(cache OpsRealtimeCache, accountRepo AccountRepository) *OpsRealtimeProjector {
	if cache == nil || accountRepo == nil {
		return nil
	}
	return &OpsRealtimeProjector{
		cache:             cache,
		accountRepo:       accountRepo,
		instanceID:        uuid.NewString(),
		reconcileInterval: defaultOpsRealtimeReconcileInterval,
	}
}

func (p *OpsRealtimeProjector) Start(stopCh <-chan struct{}) {
	if p == nil || p.cache == nil || p.accountRepo == nil || p.reconcileInterval <= 0 || stopCh == nil {
		return
	}
	p.startOnce.Do(func() {
		go p.runReconcileWorker(stopCh)
	})
}

func (p *OpsRealtimeProjector) RebuildAll(ctx context.Context) error {
	if p == nil || p.cache == nil || p.accountRepo == nil {
		return nil
	}
	lister, ok := p.accountRepo.(opsRealtimeAccountLister)
	if !ok {
		return nil
	}
	accounts, err := lister.ListOpsRealtimeAccounts(ctx, "", nil)
	if err != nil {
		return err
	}
	entries := make([]*OpsRealtimeAccountCacheEntry, 0, len(accounts))
	openAIAccountSet := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		entry := BuildOpsRealtimeAccountCacheEntry(&accounts[i])
		if entry == nil {
			continue
		}
		entries = append(entries, entry)
		if entry.Platform == PlatformOpenAI {
			openAIAccountSet[entry.AccountID] = struct{}{}
		}
	}
	if err := p.cache.ReplaceAccounts(ctx, entries); err != nil {
		return err
	}
	return p.reconcileWarmMirror(ctx, openAIAccountSet)
}

func (p *OpsRealtimeProjector) runReconcileWorker(stopCh <-chan struct{}) {
	ticker := time.NewTicker(p.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.reconcileOnce()
		case <-stopCh:
			return
		}
	}
}

func (p *OpsRealtimeProjector) reconcileOnce() {
	if p == nil || p.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), opsRealtimeReconcileTimeout)
	defer cancel()
	release, ok, err := p.cache.TryAcquireReconcileLeaderLock(ctx, p.instanceID, opsRealtimeReconcileLeaderTTL)
	if err != nil {
		logger.LegacyPrintf("service.ops_realtime_projector", "[OpsRealtimeProjector] acquire reconcile leader lock failed: %v", err)
		return
	}
	if !ok {
		p.maybeLogReconcileSkip()
		return
	}
	defer release()
	if err := p.RebuildAll(ctx); err != nil {
		logger.LegacyPrintf("service.ops_realtime_projector", "[OpsRealtimeProjector] reconcile rebuild failed: %v", err)
	}
}

func (p *OpsRealtimeProjector) maybeLogReconcileSkip() {
	if p == nil {
		return
	}
	p.skipLogMu.Lock()
	defer p.skipLogMu.Unlock()
	now := time.Now()
	if !p.skipLogAt.IsZero() && now.Sub(p.skipLogAt) < time.Minute {
		return
	}
	p.skipLogAt = now
	logger.LegacyPrintf("service.ops_realtime_projector", "[OpsRealtimeProjector] reconcile leader lock held by another instance; skipping")
}

func (p *OpsRealtimeProjector) reconcileWarmMirror(ctx context.Context, openAIAccountSet map[int64]struct{}) error {
	if p == nil || p.cache == nil {
		return nil
	}
	warmStates, err := p.cache.GetWarmAccountStates(ctx, nil)
	if err != nil {
		return err
	}
	if len(warmStates) == 0 {
		return nil
	}
	staleAccountIDs := make([]int64, 0)
	for accountID := range warmStates {
		if _, ok := openAIAccountSet[accountID]; ok {
			continue
		}
		staleAccountIDs = append(staleAccountIDs, accountID)
	}
	if len(staleAccountIDs) == 0 {
		return nil
	}
	for _, accountID := range staleAccountIDs {
		if err := p.cache.DeleteWarmAccountState(ctx, accountID); err != nil {
			return err
		}
	}
	groupIDs, err := p.cache.ListWarmBucketGroupIDs(ctx)
	if err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		for _, accountID := range staleAccountIDs {
			if err := p.cache.RemoveWarmBucketAccount(ctx, groupID, accountID); err != nil {
				logger.LegacyPrintf("service.ops_realtime_projector", "[OpsRealtimeProjector] reconcile stale warm membership cleanup failed: group=%d account=%d err=%v", groupID, accountID, err)
			}
		}
	}
	return nil
}

func (p *OpsRealtimeProjector) HandleSchedulerOutboxEvent(ctx context.Context, event SchedulerOutboxEvent) error {
	if p == nil || p.cache == nil || p.accountRepo == nil {
		return nil
	}
	switch event.EventType {
	case SchedulerOutboxEventAccountChanged, SchedulerOutboxEventAccountGroupsChanged:
		return p.handleAccountEvent(ctx, event.AccountID)
	case SchedulerOutboxEventAccountBulkChanged:
		return p.handleBulkAccountEvent(ctx, event.Payload)
	case SchedulerOutboxEventGroupChanged:
		return p.handleGroupEvent(ctx, event.GroupID)
	case SchedulerOutboxEventFullRebuild:
		return p.RebuildAll(ctx)
	default:
		return nil
	}
}

func (p *OpsRealtimeProjector) handleAccountEvent(ctx context.Context, accountID *int64) error {
	if accountID == nil || *accountID <= 0 {
		return nil
	}
	currentEntries, err := p.cache.GetAccounts(ctx, []int64{*accountID})
	if err != nil {
		return err
	}
	previous := currentEntries[*accountID]

	account, err := p.accountRepo.GetByID(ctx, *accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			if previous != nil {
				p.cleanupWarmMembership(ctx, *accountID, previous.GroupIDs)
				if err := p.cache.DeleteWarmAccountState(ctx, *accountID); err != nil {
					return err
				}
			}
			return p.cache.DeleteAccount(ctx, *accountID)
		}
		return err
	}
	entry := BuildOpsRealtimeAccountCacheEntry(account)
	if entry == nil {
		return p.cache.DeleteAccount(ctx, *accountID)
	}
	if previous != nil {
		p.cleanupRemovedWarmMemberships(ctx, *accountID, previous.GroupIDs, entry.GroupIDs)
	}
	if entry.Platform != PlatformOpenAI {
		p.cleanupWarmMembership(ctx, *accountID, entry.GroupIDs)
		if err := p.cache.DeleteWarmAccountState(ctx, *accountID); err != nil {
			return err
		}
	}
	return p.cache.UpsertAccount(ctx, entry)
}

func (p *OpsRealtimeProjector) handleBulkAccountEvent(ctx context.Context, payload map[string]any) error {
	ids := parseInt64Slice(nil)
	if payload != nil {
		ids = parseInt64Slice(payload["account_ids"])
	}
	if len(ids) == 0 {
		return nil
	}
	currentEntries, err := p.cache.GetAccounts(ctx, ids)
	if err != nil {
		return err
	}
	accounts, err := p.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}
	found := make(map[int64]*Account, len(accounts))
	entries := make([]*OpsRealtimeAccountCacheEntry, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		found[account.ID] = account
		entry := BuildOpsRealtimeAccountCacheEntry(account)
		if entry == nil {
			continue
		}
		entries = append(entries, entry)
		if previous := currentEntries[account.ID]; previous != nil {
			p.cleanupRemovedWarmMemberships(ctx, account.ID, previous.GroupIDs, entry.GroupIDs)
		}
		if entry.Platform != PlatformOpenAI {
			p.cleanupWarmMembership(ctx, entry.AccountID, entry.GroupIDs)
			if err := p.cache.DeleteWarmAccountState(ctx, entry.AccountID); err != nil {
				return err
			}
		}
	}
	for _, id := range ids {
		if _, ok := found[id]; ok {
			continue
		}
		if previous := currentEntries[id]; previous != nil {
			p.cleanupWarmMembership(ctx, id, previous.GroupIDs)
			if err := p.cache.DeleteWarmAccountState(ctx, id); err != nil {
				return err
			}
		}
		if err := p.cache.DeleteAccount(ctx, id); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := p.cache.UpsertAccount(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func (p *OpsRealtimeProjector) handleGroupEvent(ctx context.Context, groupID *int64) error {
	if groupID == nil || *groupID <= 0 {
		return nil
	}
	lister, ok := p.accountRepo.(opsRealtimeAccountLister)
	if !ok {
		return nil
	}
	previousIDs, err := p.cache.ListAccountIDs(ctx, "", groupID)
	if err != nil {
		return err
	}
	accounts, err := lister.ListOpsRealtimeAccounts(ctx, "", groupID)
	if err != nil {
		return err
	}
	entries := make([]*OpsRealtimeAccountCacheEntry, 0, len(accounts))
	currentIDSet := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		entry := BuildOpsRealtimeAccountCacheEntry(&accounts[i])
		if entry == nil {
			continue
		}
		currentIDSet[entry.AccountID] = struct{}{}
		entries = append(entries, entry)
	}
	staleIDs := make([]int64, 0, len(previousIDs))
	for _, accountID := range previousIDs {
		if _, exists := currentIDSet[accountID]; exists {
			continue
		}
		staleIDs = append(staleIDs, accountID)
	}
	if len(staleIDs) > 0 {
		staleAccounts, err := p.accountRepo.GetByIDs(ctx, staleIDs)
		if err != nil {
			return err
		}
		for _, account := range staleAccounts {
			entry := BuildOpsRealtimeAccountCacheEntry(account)
			if entry == nil {
				continue
			}
			entries = append(entries, entry)
			currentIDSet[entry.AccountID] = struct{}{}
		}
		seenStale := make(map[int64]struct{}, len(staleAccounts))
		for _, account := range staleAccounts {
			if account != nil && account.ID > 0 {
				seenStale[account.ID] = struct{}{}
			}
		}
		for _, accountID := range staleIDs {
			if _, exists := seenStale[accountID]; exists {
				continue
			}
			if err := p.cache.DeleteAccount(ctx, accountID); err != nil {
				return err
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].AccountID < entries[j].AccountID })
	for _, entry := range entries {
		if err := p.cache.UpsertAccount(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func (p *OpsRealtimeProjector) cleanupRemovedWarmMemberships(ctx context.Context, accountID int64, previousGroupIDs, currentGroupIDs []int64) {
	removed := diffInt64Slice(previousGroupIDs, currentGroupIDs)
	p.cleanupWarmMembership(ctx, accountID, removed)
}

func (p *OpsRealtimeProjector) cleanupWarmMembership(ctx context.Context, accountID int64, groupIDs []int64) {
	if p == nil || p.cache == nil || accountID <= 0 || len(groupIDs) == 0 {
		return
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if err := p.cache.RemoveWarmBucketAccount(ctx, groupID, accountID); err != nil {
			logger.LegacyPrintf("service.ops_realtime_projector", "[OpsRealtimeProjector] cleanup warm membership failed: group=%d account=%d err=%v", groupID, accountID, err)
		}
	}
}

func diffInt64Slice(oldValues, newValues []int64) []int64 {
	if len(oldValues) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(newValues))
	for _, value := range newValues {
		if value > 0 {
			seen[value] = struct{}{}
		}
	}
	out := make([]int64, 0, len(oldValues))
	for _, value := range oldValues {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}
