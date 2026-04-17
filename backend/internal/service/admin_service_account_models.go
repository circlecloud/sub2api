package service

import "context"

type accountModelProjectionRepository interface {
	GetModelProjectionByIDs(ctx context.Context, ids []int64) ([]*Account, error)
}

// GetAccountsModelProjectionByIDs returns a lightweight account projection that only contains
// fields required to derive available model lists. It falls back to the regular GetAccountsByIDs
// path when the repository does not provide a lighter projection.
func (s *adminServiceImpl) GetAccountsModelProjectionByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	if len(ids) == 0 {
		return []*Account{}, nil
	}
	if reader, ok := s.accountRepo.(accountModelProjectionRepository); ok {
		return reader.GetModelProjectionByIDs(ctx, ids)
	}
	return s.GetAccountsByIDs(ctx, ids)
}
