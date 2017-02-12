// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package prometheus provides functions that are useful to control and manage
// the build-in prometheus instance.
package prometheus

import (
	"errors"
	"net/http"

	errwrap "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DefaultPrometheusListen is registered in
// https://github.com/prometheus/prometheus/wiki/Default-port-allocations
const DefaultPrometheusListen = "127.0.0.1:9233"

// Restate represents the status of a resource.
type ResState int

const (
	ResStateOK       ResState = iota // Working resource
	ResStateSoftFail                 // Resource in soft fail (will be retried)
	ResStateHardFail                 // Resource in hard fail (will NOT be retries)
)

// Prometheus is the struct that contains information about the
// prometheus instance. Run Init() on it.
type Prometheus struct {
	Listen string // the listen specification for the net/http server

	managedResources     *prometheus.GaugeVec   // Resources we manage now
	checkApplyTotal      *prometheus.CounterVec // TODO: total of CheckApplies that have been triggered
	failedResourcesTotal *prometheus.CounterVec // Total of failures since mgmt has started
	failedResources      *prometheus.GaugeVec   // Number of current resources

	resourcesState map[string]resStateWithKind // Maps the resources with their current kind/state
}

// resStateWithKind is used to count the failures by kind
type resStateWithKind struct {
	state ResState
	kind  string
}

// Init some parameters - currently the Listen address.
func (obj *Prometheus) Init() error {
	if len(obj.Listen) == 0 {
		obj.Listen = DefaultPrometheusListen
	}

	obj.resourcesState = make(map[string]resStateWithKind)

	// Create the Gauges and Counter for prometheus
	obj.managedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_resources",
			Help: "Number of managed resources.",
		},
		[]string{"type"}, // File, Svc, ...
	)
	prometheus.MustRegister(obj.managedResources)

	obj.checkApplyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_checkapply_total",
			Help: "Number of CheckApply that have run.",
		},
		[]string{"type"}, // File, Svc, ...
	)
	prometheus.MustRegister(obj.checkApplyTotal)

	obj.failedResourcesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_failures_total",
			Help: "Total of failed resources.",
		},
		[]string{"type", "kind"}, // File, Svc, ... |  soft, hard
	)
	prometheus.MustRegister(obj.failedResourcesTotal)

	obj.failedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_failures",
			Help: "Number of failing resources.",
		},
		[]string{"type", "kind"}, // File, Svc, ... | soft, hard
	)
	prometheus.MustRegister(obj.failedResources)

	return nil
}

// Start runs a http server in a go routine, that responds to /metrics
// as prometheus would expect.
func (obj *Prometheus) Start() error {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(obj.Listen, nil)
	return nil
}

// Stop the http server.
func (obj *Prometheus) Stop() error {
	// TODO: There is no way in go < 1.8 to stop a http server.
	// https://stackoverflow.com/questions/39320025/go-how-to-stop-http-listenandserve/41433555#41433555
	return nil
}

// AddManagedResource increments the Managed Resource counter and updates the resource status.
func (obj *Prometheus) AddManagedResource(resUuid string, rtype string) error {
	obj.managedResources.With(prometheus.Labels{"type": rtype}).Inc()
	if err := obj.UpdateState(resUuid, rtype, ResStateOK); err != nil {
		return errwrap.Wrapf(err, "Can't update the resource status in the map!")
	}
	return nil
}

// RemoveManagedResource decrements the Managed Resource counter and updates the resource status.
func (obj *Prometheus) RemoveManagedResource(resUuid string, rtype string) error {
	obj.managedResources.With(prometheus.Labels{"type": rtype}).Dec()
	if err := obj.deleteState(resUuid); err != nil {
		return errwrap.Wrapf(err, "Can't remove the resource status from the map!")
	}
	return nil
}

// deleteState removes the resources for the state map and re-populates the failing gauge.
func (obj *Prometheus) deleteState(resUuid string) error {
	delete(obj.resourcesState, resUuid)
	if err := obj.updateFailingGauge(); err != nil {
		return errwrap.Wrapf(err, "Can't update the failing gauge!")
	}
	return nil
}

// UpdateState updates the state of the resources in our internal state map
// then triggers a refresh of the failing gauge.
func (obj *Prometheus) UpdateState(resUuid string, rtype string, newState ResState) error {
	obj.resourcesState[resUuid] = resStateWithKind{state: newState, kind: rtype}
	if newState != ResStateOK {
		var strState string
		if newState == ResStateSoftFail {
			strState = "soft"
		} else if newState == ResStateHardFail {
			strState = "hard"
		} else {
			return errors.New("State should be Soft or Hard failure!")
		}
		obj.failedResourcesTotal.With(prometheus.Labels{"type": rtype, "kind": strState}).Inc()
	}
	if err := obj.updateFailingGauge(); err != nil {
		return errwrap.Wrapf(err, "Can't update the failing gauge!")
	}
	return nil
}

// updateFailingGauge refreshes the failing gauge by parsking the internal
// state map.
func (obj *Prometheus) updateFailingGauge() error {
	var softFails, hardFails map[string]float64
	softFails = make(map[string]float64)
	hardFails = make(map[string]float64)
	for _, v := range obj.resourcesState {
		if v.state == ResStateSoftFail {
			if _, ok := softFails[v.kind]; ok {
				softFails[v.kind] = 0
			}
			softFails[v.kind] += 1
		} else if v.state == ResStateHardFail {
			if _, ok := hardFails[v.kind]; ok {
				hardFails[v.kind] = 0
			}
			hardFails[v.kind] += 1
		}
	}
	// TODO: we might want to Zero the metrics we are not using
	// because in prometheus design the metrics keep living for some time
	// even after they are removed.
	obj.failedResources.Reset()
	for k, v := range softFails {
		obj.failedResources.With(prometheus.Labels{"type": k, "kind": "soft"}).Set(v)
	}
	for k, v := range hardFails {
		obj.failedResources.With(prometheus.Labels{"type": k, "kind": "hard"}).Set(v)
	}
	return nil
}
