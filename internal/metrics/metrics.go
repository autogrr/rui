// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/rs/zerolog/log"

	"github.com/autogrr/rui/internal/database"
	"github.com/autogrr/rui/internal/metrics/collector"
	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/qbittorrent"
)

type MetricsManager struct {
	registry         *prometheus.Registry
	torrentCollector *collector.TorrentCollector
}

func NewMetricsManager(syncManager *qbittorrent.SyncManager, clientPool *qbittorrent.ClientPool, trackerCustomizationStore *models.TrackerCustomizationStore) *MetricsManager {
	registry := prometheus.NewRegistry()

	// Register standard Go collectors like autobrr does
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Register custom collectors
	torrentCollector := collector.NewTorrentCollector(syncManager, clientPool, trackerCustomizationStore)
	registry.MustRegister(torrentCollector)
	registry.MustRegister(database.NewMetricsCollector())

	log.Info().Msg("Metrics manager initialized with collectors")

	return &MetricsManager{
		registry:         registry,
		torrentCollector: torrentCollector,
	}
}

func (m *MetricsManager) GetRegistry() *prometheus.Registry {
	return m.registry
}
