package service

import "github.com/Wei-Shaw/sub2api/internal/config"

func (s *OpenAIGatewayService) ResolveOpenAISchedulerBucket(groupID *int64) SchedulerBucket {
	if s != nil && s.schedulerSnapshot != nil {
		return s.schedulerSnapshot.bucketFor(groupID, PlatformOpenAI, SchedulerModeSingle)
	}

	normalizedGroupID := int64(0)
	if s == nil || s.cfg == nil || s.cfg.RunMode != config.RunModeSimple {
		if groupID != nil && *groupID > 0 {
			normalizedGroupID = *groupID
		}
	}

	return SchedulerBucket{
		GroupID:  normalizedGroupID,
		Platform: PlatformOpenAI,
		Mode:     SchedulerModeSingle,
	}
}

func (s *OpenAIGatewayService) ResolveOpenAISchedulerBucketString(groupID *int64) string {
	return s.ResolveOpenAISchedulerBucket(groupID).String()
}
