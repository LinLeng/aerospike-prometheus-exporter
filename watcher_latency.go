package main

import (
	"github.com/prometheus/client_golang/prometheus"

	log "github.com/sirupsen/logrus"
)

type LatencyWatcher struct {
}

func (lw *LatencyWatcher) describe(ch chan<- *prometheus.Desc) {}

func (lw *LatencyWatcher) passOneKeys() []string {
	// return []string{"build"}
	return nil
}

func (lw *LatencyWatcher) passTwoKeys(rawMetrics map[string]string) (latencyCommands []string) {

	// return if this feature is disabled.
	if config.Aerospike.DisableLatenciesMetrics {
		// disabled
		return nil
	}

	latencyCommands = []string{"latencies:", "latency:"}

	ok, err := buildVersionGreaterThanOrEqual(rawMetrics, "5.1.0.0")
	if err != nil {
		log.Warn(err)
		return latencyCommands
	}

	if ok {
		return []string{"latencies:"}
	}

	return []string{"latency:"}
}

func (lw *LatencyWatcher) refresh(o *Observer, infoKeys []string, rawMetrics map[string]string, ch chan<- prometheus.Metric) error {

	allowedLatenciesList := make(map[string]struct{})
	blockedLatenciessList := make(map[string]struct{})

	if config.Aerospike.LatenciesMetricsAllowlistEnabled {
		for _, allowedLatencies := range config.Aerospike.LatenciesMetricsAllowlist {
			allowedLatenciesList[allowedLatencies] = struct{}{}
		}
	}

	if len(config.Aerospike.LatenciesMetricsBlocklist) > 0 {
		for _, blockedLatencies := range config.Aerospike.LatenciesMetricsBlocklist {
			blockedLatenciessList[blockedLatencies] = struct{}{}
		}
	}

	var latencyStats map[string]StatsMap

	if rawMetrics["latencies:"] != "" {
		latencyStats = parseLatencyInfo(rawMetrics["latencies:"], int(config.Aerospike.LatencyBucketsCount))
	} else {
		latencyStats = parseLatencyInfoLegacy(rawMetrics["latency:"], int(config.Aerospike.LatencyBucketsCount))
	}

	log.Tracef("latency-stats:%+v", latencyStats)

	for namespaceName, nsLatencyStats := range latencyStats {
		for operation, opLatencyStats := range nsLatencyStats {

			// operation comes from server as histogram-names
			if config.Aerospike.LatenciesMetricsAllowlistEnabled {
				if _, ok := allowedLatenciesList[operation]; !ok {
					continue
				}
			}

			if len(config.Aerospike.LatenciesMetricsBlocklist) > 0 {
				if _, ok := blockedLatenciessList[operation]; ok {
					continue
				}
			}

			for i, labelValue := range opLatencyStats.(StatsMap)["bucketLabels"].([]string) {
				// aerospike_latencies_<operation>_<timeunit>_bucket metric - Less than or equal to histogram buckets

				pm := makeMetric("aerospike_latencies", operation+"_"+opLatencyStats.(StatsMap)["timeUnit"].(string)+"_bucket", mtGauge, config.AeroProm.MetricLabels, METRIC_LABEL_CLUSTER_NAME, METRIC_LABEL_SERVICE, METRIC_LABEL_NS, METRIC_LABEL_LE)
				ch <- prometheus.MustNewConstMetric(pm.desc, pm.valueType, opLatencyStats.(StatsMap)["bucketValues"].([]float64)[i], rawMetrics[ikClusterName], rawMetrics[ikService], namespaceName, labelValue)

				// aerospike_latencies_<operation>_<timeunit>_count metric
				if i == 0 {
					pm = makeMetric("aerospike_latencies", operation+"_"+opLatencyStats.(StatsMap)["timeUnit"].(string)+"_count", mtGauge, config.AeroProm.MetricLabels, METRIC_LABEL_CLUSTER_NAME, METRIC_LABEL_SERVICE, METRIC_LABEL_NS)
					ch <- prometheus.MustNewConstMetric(pm.desc, pm.valueType, opLatencyStats.(StatsMap)["bucketValues"].([]float64)[i], rawMetrics[ikClusterName], rawMetrics[ikService], namespaceName)
				}
			}
		}
	}

	return nil
}
