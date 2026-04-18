package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIChatCompletionsURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "base root appends v1 chat completions",
			base: "https://example.com",
			want: "https://example.com/v1/chat/completions",
		},
		{
			name: "v1 base appends chat completions",
			base: "https://example.com/v1",
			want: "https://example.com/v1/chat/completions",
		},
		{
			name: "existing chat completions endpoint preserved",
			base: "https://example.com/v1/chat/completions",
			want: "https://example.com/v1/chat/completions",
		},
		{
			name: "responses endpoint rewrites to chat completions",
			base: "https://example.com/v1/responses",
			want: "https://example.com/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, buildOpenAIChatCompletionsURL(tt.base))
		})
	}
}
