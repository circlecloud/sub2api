//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAIAuthUnavailableReason_DetectsForbiddenVariants(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Access forbidden (403): workspace forbidden",
		"Validation required (403): account needs Google verification",
		"Account violation (403): terms of service violation",
	}

	for _, reason := range cases {
		reason := reason
		t.Run(reason, func(t *testing.T) {
			require.True(t, isOpenAIAuthUnavailableReason(reason))
		})
	}
}
