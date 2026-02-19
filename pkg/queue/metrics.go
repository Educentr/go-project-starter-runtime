package queue

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type queueMetrics struct {
	cycleDuration        *prometheus.HistogramVec
	tasksInProgress      *prometheus.GaugeVec
	tasksProcessedTotal  *prometheus.CounterVec
	tasksErrorsTotal     *prometheus.CounterVec
	handlerErrorsTotal   *prometheus.CounterVec
	runningWorkers       prometheus.Gauge
	runningJobProcessors prometheus.Gauge
}

func newQueueMetrics(registry *prometheus.Registry, namespace string) *queueMetrics {
	if registry == nil {
		return nil
	}

	m := &queueMetrics{
		cycleDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "job_processor_cycle_duration_seconds",
				Help:      "Duration of one job processor cycle (get tasks + handle)",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"queue_num"},
		),
		tasksInProgress: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "tasks_in_progress",
				Help:      "Number of tasks currently being processed",
			},
			[]string{"queue_num"},
		),
		tasksProcessedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "tasks_processed_total",
				Help:      "Total number of successfully processed tasks",
			},
			[]string{"queue_num"},
		),
		tasksErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "tasks_errors_total",
				Help:      "Total number of task processing errors",
			},
			[]string{"queue_num"},
		),
		handlerErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "handler_errors_total",
				Help:      "Total number of handler-level errors (critical failures)",
			},
			[]string{"queue_num"},
		),
		runningWorkers: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "running_workers",
				Help:      "Number of running queue workers",
			},
		),
		runningJobProcessors: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "queue",
				Name:      "running_job_processors",
				Help:      "Number of running job processors",
			},
		),
	}

	registry.MustRegister(
		m.cycleDuration, m.tasksInProgress,
		m.tasksProcessedTotal, m.tasksErrorsTotal, m.handlerErrorsTotal,
		m.runningWorkers, m.runningJobProcessors,
	)

	return m
}

func queueNumLabel(queueNum int) string {
	return strconv.Itoa(queueNum)
}
