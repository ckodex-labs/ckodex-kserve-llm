package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMLoraAdapterCoveragePostJSONRejectsInvalidURL(t *testing.T) {
	response, err := postJSON(context.Background(), nil, "://invalid", nil)
	if response != nil && response.Body != nil {
		require.NoError(t, response.Body.Close())
	}
	assert.Nil(t, response)
	assert.Error(t, err)
}
