package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAggregateFetchedUpstreamModelIDsUsesIntersection(t *testing.T) {
	result := aggregateFetchedUpstreamModelIDs([][]string{
		{"gpt-4.1", "gpt-4o", "shared"},
		{"shared", "gpt-4.1-mini", "gpt-4o"},
		{"gpt-4o", "shared"},
	})

	require.Equal(t, []string{"gpt-4o", "shared"}, result)
}

func TestSplitFetchModelsKeysUnquotesJSONStringArray(t *testing.T) {
	result := splitFetchModelsKeys(`["sk-one","sk-two"]`)

	require.Equal(t, []string{"sk-one", "sk-two"}, result)
}
