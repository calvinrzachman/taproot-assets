//go:build loadtest

package loadtest

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/stretchr/testify/require"
)

var (
	testDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_duration_seconds",
			Help: "Duration of the test execution, in seconds",
		},
		[]string{"test_name"},
	)
)

func init() {
	// Register the metric with Prometheus's default registry.
	prometheus.MustRegister(testDuration)
}

type testCase struct {
	name string
	fn   func(t *testing.T, ctx context.Context, cfg *Config)
}

var loadTestCases = []testCase{
	{
		name: "mint",
		fn:   mintTest,
	},
	{
		name: "send",
		fn:   sendTest,
	},
}

// TestPerformance executes the configured performance tests.
func TestPerformance(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err, "unable to load main config")

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, cfg.TestSuiteTimeout)
	defer cancel()

	for _, tc := range loadTestCases {
		tc := tc

		if !shouldRunCase(tc.name, cfg.TestCases) {
			t.Logf("Not running test case '%s' as not configured",
				tc.name)

			continue
		}

		// Record the start time of the test case.
		startTime := time.Now()

		success := t.Run(tc.name, func(tt *testing.T) {
			ctxt, cancel := context.WithTimeout(
				ctxt, cfg.TestTimeout,
			)
			defer cancel()

			tc.fn(t, ctxt, cfg)
		})
		if !success {
			t.Fatalf("test case %v failed", tc.name)
		}

		// Calculate the test duration and push metrics if the test case succeeded.
		if cfg.PrometheusGateway.Enabled {
			duration := time.Since(startTime).Seconds()

			// Update the metric with the test duration.
			testDuration.WithLabelValues(tc.name).Set(duration)

			// Create a new pusher to push the metrics.
			pushURL := cfg.PrometheusGateway.Host + ":" +
				strconv.Itoa(cfg.PrometheusGateway.Port)

			pusher := push.New(pushURL, "load_test").
				Collector(testDuration).
				Grouping("test_case", tc.name)

			// Push the metrics to Prometheus PushGateway.
			if err := pusher.Push(); err != nil {
				t.Logf("Could not push metrics to Prometheus PushGateway: %v",
					err)
			} else {
				t.Logf("Metrics pushed for test case '%s': duration = %v seconds",
					tc.name, duration)
			}
		}
	}
}

// shouldRunCase returns true if the given test case should be run. This will
// return true if the config file does not specify any test cases. In that case
// we can select the test cases to run using the command line
// (-test.run="TestPerformance/test_case_name")
func shouldRunCase(name string, configuredCases []string) bool {
	if len(configuredCases) == 0 {
		return true
	}

	for _, c := range configuredCases {
		if c == name {
			return true
		}
	}

	return false
}
