// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE

package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"
)

// tpsCounter reports the transaction throughput of the node.
//
// Two observables: a periodic log line, and a counter in the node's configured
// telemetry sink (e.g. Prometheus when prometheus-retention-time > 0).
type tpsCounter struct {
	nSuccessful, NFailed uint64
	reportPeriod         time.Duration
	logger               log.Logger
	doneCloseOnce        sync.Once
	doneCh               chan bool
}

func newTPSCounter(logger log.Logger) *tpsCounter {
	return &tpsCounter{logger: logger, doneCh: make(chan bool, 1)}
}

func (tpc *tpsCounter) incrementSuccess() { atomic.AddUint64(&tpc.nSuccessful, 1) }
func (tpc *tpsCounter) incrementFailure() { atomic.AddUint64(&tpc.NFailed, 1) }

const defaultTPSReportPeriod = 10 * time.Second

func (tpc *tpsCounter) start(ctx context.Context) error {
	tpsReportPeriod := defaultTPSReportPeriod
	if tpc.reportPeriod > 0 {
		tpsReportPeriod = tpc.reportPeriod
	}
	ticker := time.NewTicker(tpsReportPeriod)
	defer ticker.Stop()
	defer tpc.doneCloseOnce.Do(func() {
		close(tpc.doneCh)
	})

	var lastNSuccessful, lastNFailed uint64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			// Report the number of transactions seen in the designated period of time.
			latestNSuccessful := atomic.LoadUint64(&tpc.nSuccessful)
			latestNFailed := atomic.LoadUint64(&tpc.NFailed)

			var nTxn int64
			nSuccess := tpc.recordValue(latestNSuccessful, lastNSuccessful, statusSuccess)
			nTxn += nSuccess
			nFailed := tpc.recordValue(latestNFailed, lastNFailed, statusFailure)
			nTxn += nFailed

			if nTxn != 0 {
				// Record to our logger for easy examination in the logs.
				secs := float64(tpsReportPeriod) / float64(time.Second)
				tpc.logger.Info("Transactions per second", "tps", float64(nTxn)/secs)
			}

			lastNFailed = latestNFailed
			lastNSuccessful = latestNSuccessful
		}
	}
}

type status string

const (
	statusSuccess = "success"
	statusFailure = "failure"
)

// recordValue emits the delta since the previous report and returns it.
func (tpc *tpsCounter) recordValue(latest, previous uint64, status status) int64 {
	if latest < previous {
		return 0
	}

	n := int64(latest - previous) //nolint:gosec
	if n < 0 {
		// Perhaps we exceeded the uint64 limits then wrapped around, for the latest value.
		// TODO: Perhaps log this?
		return 0
	}

	statusValue := "OK"
	if status == statusFailure {
		statusValue = "ERR"
	}

	telemetry.IncrCounterWithLabels(
		[]string{"transactions", "processed", "total"},
		float32(n),
		[]metrics.Label{telemetry.NewLabel("status", statusValue)},
	)

	return n
}
