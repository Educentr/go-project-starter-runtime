package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// TelegramMetrics holds all Telegram bot metrics
type TelegramMetrics struct {
	CommandsTotal        *prometheus.CounterVec
	CommandDuration      *prometheus.HistogramVec
	ErrorsTotal          *prometheus.CounterVec
	PaymentsTotal        *prometheus.CounterVec
	UpdatesReceivedTotal *prometheus.CounterVec
	HiddenMessagesQueue  *prometheus.GaugeVec
	APILatency           *prometheus.HistogramVec
	UserStateChanges     *prometheus.CounterVec
}

// NewTelegramMetrics creates and registers all Telegram metrics
// Returns nil if registry is nil (metrics disabled)
func NewTelegramMetrics(registry *prometheus.Registry, namespace string) *TelegramMetrics {
	if registry == nil {
		return nil
	}

	m := &TelegramMetrics{
		CommandsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "commands_total",
				Help:      "Total number of bot commands processed",
			},
			[]string{"command", "status"},
		),
		CommandDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "command_duration_seconds",
				Help:      "Command processing duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"command"},
		),
		ErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "errors_total",
				Help:      "Total number of errors by type",
			},
			[]string{"error_type"},
		),
		PaymentsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "payments_total",
				Help:      "Total number of payments by status",
			},
			[]string{"status"},
		),
		UpdatesReceivedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "updates_received_total",
				Help:      "Total updates received by type",
			},
			[]string{"update_type"},
		),
		HiddenMessagesQueue: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "hidden_messages_queue",
				Help:      "Current hidden messages queue size",
			},
			[]string{"status"},
		),
		APILatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "api_latency_seconds",
				Help:      "Telegram API call latency in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"method"},
		),
		UserStateChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "telegram",
				Name:      "user_state_changes_total",
				Help:      "Total user state changes",
			},
			[]string{"from_state", "to_state"},
		),
	}

	registry.MustRegister(
		m.CommandsTotal,
		m.CommandDuration,
		m.ErrorsTotal,
		m.PaymentsTotal,
		m.UpdatesReceivedTotal,
		m.HiddenMessagesQueue,
		m.APILatency,
		m.UserStateChanges,
	)

	return m
}
