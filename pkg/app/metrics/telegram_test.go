package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTelegramMetrics_WithNilRegistry(t *testing.T) {
	m := NewTelegramMetrics(nil, "test")
	assert.Nil(t, m, "metrics should be nil when registry is nil")
}

func TestNewTelegramMetrics_WithRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewTelegramMetrics(registry, "test")

	require.NotNil(t, m)
	assert.NotNil(t, m.CommandsTotal)
	assert.NotNil(t, m.CommandDuration)
	assert.NotNil(t, m.ErrorsTotal)
	assert.NotNil(t, m.PaymentsTotal)
	assert.NotNil(t, m.UpdatesReceivedTotal)
	assert.NotNil(t, m.HiddenMessagesQueue)
	assert.NotNil(t, m.APILatency)
	assert.NotNil(t, m.UserStateChanges)
}

func TestTelegramMetrics_RecordCommand(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewTelegramMetrics(registry, "test")

	m.CommandsTotal.WithLabelValues("start", "success").Inc()
	m.CommandDuration.WithLabelValues("start").Observe(0.5)

	mfs, err := registry.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, mfs)
}
