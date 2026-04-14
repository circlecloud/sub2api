package service

import "context"

const defaultOpsOpenAIWarmPoolPageSize = 20

func (s *OpsService) GetOpenAIWarmPoolStatsWithPage(ctx context.Context, groupID *int64, includeAccounts bool, accountStateFilter string, accountsOnly bool, page, pageSize int) (*OpsOpenAIWarmPoolStats, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if normalizeWarmPoolStateFilter(accountStateFilter) == "ready" && includeAccounts && accountsOnly && s != nil && s.openAIGatewayService != nil {
		pool := s.openAIGatewayService.getOpenAIWarmPool()
		if pool != nil {
			if page < 1 {
				page = 1
			}
			if pageSize < 1 {
				pageSize = defaultOpsOpenAIWarmPoolPageSize
			}
			stats, err := s.buildOpenAIWarmPoolReadyListStats(ctx, pool, groupID, page, pageSize)
			if err != nil {
				return nil, err
			}
			return stats, nil
		}
	}
	stats, err := s.GetOpenAIWarmPoolStatsWithOptions(ctx, groupID, includeAccounts, accountStateFilter, accountsOnly)
	if err != nil {
		return nil, err
	}
	return paginateOpsOpenAIWarmPoolStats(stats, page, pageSize), nil
}

func paginateOpsOpenAIWarmPoolStats(stats *OpsOpenAIWarmPoolStats, page, pageSize int) *OpsOpenAIWarmPoolStats {
	if stats == nil {
		return nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultOpsOpenAIWarmPoolPageSize
	}

	total := len(stats.Accounts)
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	cloned := *stats
	cloned.Page = page
	cloned.PageSize = pageSize
	cloned.Total = total
	cloned.Pages = pages
	if start >= total {
		cloned.Accounts = []*OpsOpenAIWarmPoolAccount{}
	} else {
		cloned.Accounts = append([]*OpsOpenAIWarmPoolAccount(nil), stats.Accounts[start:end]...)
	}
	return &cloned
}
