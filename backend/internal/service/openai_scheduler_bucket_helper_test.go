package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceResolveOpenAISchedulerBucketString_SimpleModeNormalizesGroupID(t *testing.T) {
	groupID := int64(42)
	svc := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeSimple}}

	require.Equal(t, "0:openai:single", svc.ResolveOpenAISchedulerBucketString(&groupID))
}

func TestOpenAIGatewayServiceResolveOpenAISchedulerBucketString_StandardModeKeepsGroupID(t *testing.T) {
	groupID := int64(42)
	svc := &OpenAIGatewayService{cfg: &config.Config{RunMode: config.RunModeStandard}}

	require.Equal(t, "42:openai:single", svc.ResolveOpenAISchedulerBucketString(&groupID))
	require.Equal(t, "0:openai:single", svc.ResolveOpenAISchedulerBucketString(nil))
}
