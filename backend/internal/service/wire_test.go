//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/zeromicro/go-zero/core/collection"
)

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService()
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}

func TestProvideGroupCapacityService_WiresProviders(t *testing.T) {
	t.Parallel()

	accountRepo := &groupCapacityAccountRepoStub{}
	groupRepo := &groupCapacityGroupRepoStub{}
	snapshotAccountRepo := &groupCapacitySnapshotAccountRepoStub{}
	snapshotGroupRepo := &groupCapacitySnapshotGroupRepoStub{}
	concurrencyService := NewConcurrencyService(&stubConcurrencyCacheForTest{})
	sessionCache := &groupCapacitySessionLimitCacheStub{}
	rpmCache := &groupCapacityRPMCacheStub{}
	projector := NewGroupCapacitySnapshotProjector()
	snapshotProvider := newGroupCapacitySnapshotProviderService(snapshotAccountRepo, snapshotGroupRepo, projector, time.Minute)
	runtimeProvider := NewGroupCapacityRuntimeProviderService(concurrencyService, sessionCache, rpmCache, time.Second, 8)

	svc := ProvideGroupCapacityService(accountRepo, groupRepo, concurrencyService, sessionCache, rpmCache, snapshotProvider, runtimeProvider)
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	if svc.snapshotProvider != snapshotProvider {
		t.Fatalf("期望注入 snapshotProvider")
	}
	if svc.runtimeProvider != runtimeProvider {
		t.Fatalf("期望注入 runtimeProvider")
	}
}

func TestProvideSchedulerSnapshotService_RegistersGroupCapacityProjector(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	accountRepo := &groupCapacityAccountRepoStub{}
	opsCache := &stubOpsRealtimeCache{}
	projector := NewGroupCapacitySnapshotProjector()

	svc := ProvideSchedulerSnapshotService(nil, nil, accountRepo, nil, cfg, opsCache, projector)
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	defer svc.Stop()
	if len(svc.observers) != 2 {
		t.Fatalf("期望注册 2 个 observer，实际 %d", len(svc.observers))
	}
	if svc.observers[0] != projector && svc.observers[1] != projector {
		t.Fatalf("期望注册 group capacity projector observer")
	}
}

func TestProvideConcurrencyService_DoesNotWireGroupRuntimeCounter(t *testing.T) {
	t.Parallel()

	cache := &stubConcurrencyCacheForTest{acquireResult: true}
	accountRepo := &groupCapacityRuntimeAccountRepoStub{}

	svc := ProvideConcurrencyService(cache, accountRepo, nil)
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}

	result, err := svc.AcquireAccountSlot(context.Background(), 42, 1)
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if !result.Acquired {
		t.Fatalf("期望成功获取 slot")
	}
	if got := accountRepo.getByIDCalls.Load(); got != 0 {
		t.Fatalf("期望生产 wiring 不再查询 group runtime counter，实际调用 %d 次", got)
	}
	result.ReleaseFunc()
}
