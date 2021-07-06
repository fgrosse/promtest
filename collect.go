package promtest

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Reporter has methods matching go's testing.T to avoid importing `testing` in
// the main part of the library.
type Reporter interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Error(...interface{})
	Errorf(string, ...interface{})
	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Helper()
	FailNow()
}

// AssertEquals checks if the value of a given counter or gauge metric is equal
// to an expected value. Other than promtest.GetMetric(…), this function
// aggregates the value from all label combinations that match the given label
// set.
//
// Example usage:
//   promtest.AssertEquals(t, 5, requestMethodMetric, "method=GET")
func AssertEquals(t Reporter, expected float64, metric prometheus.Collector, labels ...string) {
	t.Helper()
	allLabels := CollectMetrics(t, metric)

	var filteredLabels []*dto.Metric
	for _, m := range allLabels {
		if matches(t, m, labels) {
			filteredLabels = append(filteredLabels, m)
		}
	}

	var actualValue float64
	for _, m := range filteredLabels {
		switch {
		case m.Counter != nil:
			a := m.GetCounter().GetValue()
			actualValue += a
		case m.Gauge != nil:
			a := m.GetGauge().GetValue()
			actualValue += a
		default:
			t.Fatal("neither a counter nor a gauge")
		}
	}

	if expected != actualValue {
		t.Errorf("Expected metric with labels=%q to have a value of %v but we got %v", labels, expected, actualValue)
	}
}

// AssertSummarySampleCount checks if the sample count of a given summary metric
// is equal to an expected value. Other than promtest.GetMetric(…), this function
// aggregates the value from all label combinations that match the given label
// set.
//
// Example usage:
//   promtest.AssertSummarySampleCount(t, 5, requestDurationsMetric, "method=GET")
func AssertSummarySampleCount(t Reporter, expected int, metric prometheus.Collector, labels ...string) {
	t.Helper()
	allLabels := CollectMetrics(t, metric)

	var filteredLabels []*dto.Metric
	for _, m := range allLabels {
		if matches(t, m, labels) {
			filteredLabels = append(filteredLabels, m)
		}
	}

	var actualValue int
	for _, m := range filteredLabels {
		if m.Summary == nil {
			t.Fatal("metric is not a summary")
		}

		actualValue += int(m.Summary.GetSampleCount())
	}

	if expected != actualValue {
		t.Errorf("Expected metric with labels=%q to have a sample count of %v but we got %v", labels, expected, actualValue)
	}
}

// GetMetric extracts a metric from a prometheus collector that has *exactly*
// the given labels. If you want to make assertions on the numeric metric value
// you should probably use promtest.AssertEquals(…).
//
// Example usage:
//   m := promtest.GetMetric(t, postTxnReturnCode, "code=200")
//   assert.EqualValues(t, 3, m.GetCounter().GetValue())
func GetMetric(t Reporter, metric prometheus.Collector, expectedLabels ...string) *dto.Metric {
	metrics := CollectMetrics(t, metric)

	for _, m := range metrics {
		if matches(t, m, expectedLabels) {
			return m
		}
	}

	return nil
}

func matches(t Reporter, m *dto.Metric, expectedLabels []string) bool {
	for _, expected := range expectedLabels {
		parts := strings.SplitN(expected, "=", 2)
		if len(parts) != 2 {
			t.Error("metrics labels should have two parts, e.g. key=value")
			return false
		}

		expectedName, expectedValue := parts[0], parts[1]
		found := false

		for _, l := range m.Label {
			if l.Name == nil || l.Value == nil {
				continue
			}

			if *l.Name == expectedName && *l.Value == expectedValue {
				found = true
				break
			}
		}

		if !found {
			t.Logf("label %q not found in %q", expected, m.Label)
			return false
		}
	}

	return true
}

// CollectMetrics extracts all values from the given prometheus metric.
// Consider using promtest.GetMetric() instead of this function.
func CollectMetrics(t Reporter, metric prometheus.Collector) []*dto.Metric {
	var result []*dto.Metric

	metrics := make(chan prometheus.Metric)
	done := make(chan bool, 1)
	go func() {
		for m := range metrics {
			metric := CollectMetric(t, m)
			if metric != nil {
				result = append(result, metric)
			}
		}
		done <- true
	}()

	metric.Collect(metrics)
	close(metrics)
	<-done // wait until goroutine returns

	return result
}

// CollectMetric extracts a single value from a given prometheus metric.
// Consider using promtest.GetMetric() instead of this function.
func CollectMetric(t Reporter, m prometheus.Metric) *dto.Metric {
	metric := new(dto.Metric)
	err := m.Write(metric)
	if err != nil {
		t.Errorf("Failed to collect metric: %v", err)
	}

	return metric
}
