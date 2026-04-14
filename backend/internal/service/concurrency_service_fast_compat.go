package service

import "context"

func (s *ConcurrencyService) GetAccountsLoadBatchFast(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}, nil
	}
	if s == nil || s.cache == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}
	if fastCache, ok := s.cache.(interface {
		GetAccountsLoadBatchFast(context.Context, []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
	}); ok {
		return fastCache.GetAccountsLoadBatchFast(ctx, accounts)
	}
	return s.cache.GetAccountsLoadBatch(ctx, accounts)
}
