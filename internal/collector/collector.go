package collector

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/chubchubsancho/prometheus-hydrao-exporter/internal/hydrao"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricPrefix = "hydrao_"
)

var (
	hydraoUpDesc = prometheus.NewDesc(metricPrefix+"up",
		"Zero if there was an error during the last refresh try.",
		nil, nil)

	refreshIntervalDesc = prometheus.NewDesc(
		metricPrefix+"refresh_interval_seconds",
		"Contains the configured refresh interval in seconds. This is provided as a convenience for calculations with the cache update time.",
		nil, nil)
	refreshPrefix        = metricPrefix + "last_refresh_"
	refreshTimestampDesc = prometheus.NewDesc(
		refreshPrefix+"time",
		"Contains the time of the last refresh try, successful or not.",
		nil, nil)
	refreshDurationDesc = prometheus.NewDesc(
		metricPrefix+refreshPrefix+"duration_seconds",
		"Contains the time it took for the last refresh to complete, even if it was unsuccessful.",
		nil, nil)

	cacheTimestampDesc = prometheus.NewDesc(
		metricPrefix+"cache_updated_time",
		"Contains the time of the cached data.",
		nil, nil)

	showerHeadsLabels = []string{
		"device_uuid",
		"type",
		"label",
	}

	showerHeadsPrefix = metricPrefix + "showersheads_"

	showerHeadDesc = prometheus.NewDesc(
		showerHeadsPrefix+"device",
		"Contains showerheads information",
		showerHeadsLabels,
		nil)

	updatedDesc = prometheus.NewDesc(
		showerHeadsPrefix+"updated",
		"Timestamp of last update",
		showerHeadsLabels,
		nil)

	showerHeadRefShowerDurationDesc = prometheus.NewDesc(
		showerHeadsPrefix+"ref_shower_duration",
		"Duration of reference shower",
		showerHeadsLabels,
		nil)

	showerHeadRefFlowDesc = prometheus.NewDesc(
		showerHeadsPrefix+"ref_flow",
		"Flow of reference shower",
		showerHeadsLabels,
		nil)

	showerHeadAvgFlowDesc = prometheus.NewDesc(
		showerHeadsPrefix+"avg_flow",
		"Average flow for the last 10 showers",
		showerHeadsLabels,
		nil)

	showerHeadLastSyncIsCompleteDesc = prometheus.NewDesc(
		showerHeadsPrefix+"last_sync_is_complete",
		"Last sync is complete",
		showerHeadsLabels,
		nil)

	showerHeadThresholdRequestDesc = prometheus.NewDesc(
		showerHeadsPrefix+"threshold_request",
		"Threshold request",
		append(showerHeadsLabels, "color"),
		nil)

	showersLabels = []string{
		"device_uuid",
		"shower_id",
	}

	showersPrefix = metricPrefix + "showers_"

	showerFlowDesc = prometheus.NewDesc(
		showersPrefix+"flow",
		"Flow of the shower",
		showersLabels,
		nil)

	showerDurationDesc = prometheus.NewDesc(
		showersPrefix+"duration",
		"Duration of the shower",
		showersLabels,
		nil)

	showerTemperatureDesc = prometheus.NewDesc(
		showersPrefix+"temperature",
		"Temperature of the shower",
		showersLabels,
		nil)

	showerVolumeDesc = prometheus.NewDesc(
		showersPrefix+"volume",
		"Volume of the shower",
		showersLabels,
		nil)

	showerSoapingTimeDesc = prometheus.NewDesc(
		showersPrefix+"soaping_time",
		"Soaping time of the shower",
		showersLabels,
		nil)
)

// Collector is the prometheus collector for the hydrao exporter
type HydraoCollector struct {
	logger              log.Logger
	getShowerheads      GetShowerheads
	clock               func() time.Time
	refreshInterval     time.Duration
	lastRefresh         time.Time
	lastRefreshError    error
	lastRefreshDuration time.Duration

	cacheLock             sync.RWMutex
	cacheTimestamp        time.Time
	cachedShowerHeadsData []*hydrao.ShowerHead
}

// GetShowerheads defines the interface for reading from the Hydrao API.
type GetShowerheads func() ([]*hydrao.ShowerHead, error)

// func New(client *hydrao.Client, logger log.Logger) (*HydraoCollector, error) {
func New(logger log.Logger, getShowerheads GetShowerheads) (*HydraoCollector, error) {
	level.Info(logger).Log("msg", "Setup Hydrao exporter")
	return &HydraoCollector{
		//hydrao:          client,
		getShowerheads:  getShowerheads,
		logger:          logger,
		clock:           time.Now,
		refreshInterval: 10,
	}, nil
}

// Describe implements prometheus.Collector
func (c *HydraoCollector) Describe(dChan chan<- *prometheus.Desc) {
	dChan <- hydraoUpDesc
	dChan <- refreshIntervalDesc
	dChan <- refreshTimestampDesc
	dChan <- refreshDurationDesc
	dChan <- cacheTimestampDesc
	dChan <- showerHeadDesc
	dChan <- updatedDesc
	dChan <- showerHeadRefShowerDurationDesc
	dChan <- showerHeadRefFlowDesc
	dChan <- showerHeadAvgFlowDesc
	dChan <- showerHeadLastSyncIsCompleteDesc
	dChan <- showerHeadThresholdRequestDesc
	dChan <- showerFlowDesc
	dChan <- showerDurationDesc
	dChan <- showerTemperatureDesc
	dChan <- showerVolumeDesc
	dChan <- showerSoapingTimeDesc
}

// Collect implements prometheus.Collector
func (c *HydraoCollector) Collect(mChan chan<- prometheus.Metric) {
	level.Debug(c.logger).Log("msg", "Prometheus Collect")
	now := c.clock()
	if now.Sub(c.lastRefresh) >= c.refreshInterval {
		level.Debug(c.logger).Log("msg", "Refresh Datas")
		go c.RefreshData(now)
	}

	upValue := 1.0
	if c.lastRefresh.IsZero() || c.lastRefreshError != nil {
		upValue = 0
	}
	c.sendMetric(mChan, hydraoUpDesc, prometheus.GaugeValue, upValue)
	c.sendMetric(mChan, refreshIntervalDesc, prometheus.GaugeValue, c.refreshInterval.Seconds())
	c.sendMetric(mChan, refreshTimestampDesc, prometheus.GaugeValue, convertTime(c.lastRefresh))
	c.sendMetric(mChan, refreshDurationDesc, prometheus.GaugeValue, c.lastRefreshDuration.Seconds())

	c.cacheLock.RLock()
	defer c.cacheLock.RUnlock()

	c.sendMetric(mChan, cacheTimestampDesc, prometheus.GaugeValue, convertTime(c.cacheTimestamp))
	level.Debug(c.logger).Log("msg", "Cached ShowerHeads Data %s", c.cachedShowerHeadsData)
	if c.cachedShowerHeadsData != nil {
		for _, showerHead := range c.cachedShowerHeadsData {
			deviceUUID := showerHead.DeviceUUID
			showerHeadType := showerHead.Type
			label := showerHead.Label
			c.collectShowerHeadData(mChan, showerHead, deviceUUID, showerHeadType, label)
		}
	}
}

// RefreshData causes the collector to try to refresh the cached data.
func (c *HydraoCollector) RefreshData(now time.Time) {
	level.Debug(c.logger).Log("Refreshing data. Time since last refresh: %s", now.Sub(c.lastRefresh))
	c.lastRefresh = now

	defer func(start time.Time) {
		c.lastRefreshDuration = c.clock().Sub(start)
	}(c.clock())

	showerHeads, err := c.getShowerheads()
	c.lastRefreshError = err

	if err != nil {
		level.Error(c.logger).Log("Error during showerheads refresh: %s", err)
		return
	}

	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	c.cacheTimestamp = now
	c.cachedShowerHeadsData = showerHeads
}

func (c *HydraoCollector) collectShowerHeadData(ch chan<- prometheus.Metric, showerhead *hydrao.ShowerHead, device_uuid, showerHeadType string, label string) {
	c.sendMetric(ch, showerHeadDesc, prometheus.GaugeValue, 1, device_uuid, showerHeadType, label)
	c.sendMetric(ch, updatedDesc, prometheus.GaugeValue, convertTime(showerhead.LastSyncDate), device_uuid, showerHeadType, label)
	c.sendMetric(ch, showerHeadRefShowerDurationDesc, prometheus.GaugeValue, float64(showerhead.RefShowerDuration), device_uuid, showerHeadType, label)
	c.sendMetric(ch, showerHeadRefFlowDesc, prometheus.GaugeValue, float64(showerhead.PreviousFlow), device_uuid, showerHeadType, label)
	c.sendMetric(ch, showerHeadAvgFlowDesc, prometheus.GaugeValue, float64(showerhead.Flow), device_uuid, showerHeadType, label)
	c.sendMetric(ch, showerHeadLastSyncIsCompleteDesc, prometheus.GaugeValue, float64(showerhead.IsLastSyncComplete), device_uuid, showerHeadType, label)

	var thresholds []hydrao.Threshold
	err := json.Unmarshal([]byte(showerhead.ThresholdRequest), &thresholds)
	if err != nil {
		level.Error(c.logger).Log("msg", fmt.Sprintf("Error unmarshalling thresholds: %s", err))
	}

	for _, threshold := range thresholds {
		c.sendMetric(ch, showerHeadThresholdRequestDesc, prometheus.GaugeValue, float64(threshold.Liter), device_uuid, showerHeadType, label, threshold.Color)
	}

	for _, shower := range showerhead.Showers {
		c.sendMetric(ch, showerFlowDesc, prometheus.GaugeValue, float64(shower.Flow), shower.DeviceUUID, fmt.Sprint(shower.ShowerID))
		c.sendMetric(ch, showerDurationDesc, prometheus.GaugeValue, float64(shower.Duration), shower.DeviceUUID, fmt.Sprint(shower.ShowerID))
		c.sendMetric(ch, showerTemperatureDesc, prometheus.GaugeValue, float64(shower.Temperature), shower.DeviceUUID, fmt.Sprint(shower.ShowerID))
		c.sendMetric(ch, showerVolumeDesc, prometheus.GaugeValue, float64(shower.Volume), shower.DeviceUUID, fmt.Sprint(shower.ShowerID))
		c.sendMetric(ch, showerSoapingTimeDesc, prometheus.GaugeValue, float64(shower.SoapingTime), shower.DeviceUUID, fmt.Sprint(shower.ShowerID))
	}
}

func (c *HydraoCollector) sendMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues ...string) {
	m, err := prometheus.NewConstMetric(desc, valueType, value, labelValues...)
	if err != nil {
		level.Error(c.logger).Log("msg", fmt.Sprintf("Error creating %s metric: %s", updatedDesc.String(), err))
		return
	}
	ch <- m
}

func convertTime(t time.Time) float64 {
	if t.IsZero() {
		return 0.0
	}

	return float64(t.Unix())
}
