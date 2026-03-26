// Package syncer polls the tinykube API to synchronise Running pod IPs into
// the tinydns service registry.
//
// Pull model: tinydns polls tinykube /pods and /services, builds the set of
// Running pods matching each Service's selector, then rebuilds the registry.
// This avoids the need for tinykube to push events to tinydns.
package syncer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/krapi0314/tinybox/tinydns/registry"
)

const defaultTTL = 30 // seconds

// Syncer periodically polls tinykube and updates the registry.
type Syncer struct {
	reg       *registry.Registry
	baseURL   string
	namespace string
	interval  time.Duration
	stop      chan struct{}
}

// New creates a Syncer. Call Start to begin polling.
func New(reg *registry.Registry, tinykubeURL, namespace string, interval time.Duration) *Syncer {
	return &Syncer{
		reg:       reg,
		baseURL:   tinykubeURL,
		namespace: namespace,
		interval:  interval,
		stop:      make(chan struct{}),
	}
}

// Start begins the background sync loop.
func (s *Syncer) Start() {
	go s.loop()
}

// Stop halts the sync loop.
func (s *Syncer) Stop() {
	close(s.stop)
}

func (s *Syncer) loop() {
	s.sync()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.sync()
		case <-s.stop:
			return
		}
	}
}

// sync fetches pods and services from tinykube and rebuilds the registry.
func (s *Syncer) sync() {
	pods, err := s.fetchPods()
	if err != nil {
		return
	}
	services, err := s.fetchServices()
	if err != nil {
		return
	}

	// Build label-indexed pod map: label key=value → [podIP, ...]
	// Only Running pods are included.
	type podInfo struct {
		ip     string
		labels map[string]interface{}
	}
	var runningPods []podInfo
	for _, pod := range pods {
		status, _ := pod["status"].(string)
		if status != "Running" {
			continue
		}
		ip, _ := pod["podIP"].(string)
		labels, _ := pod["labels"].(map[string]interface{})
		if ip != "" {
			runningPods = append(runningPods, podInfo{ip: ip, labels: labels})
		}
	}

	// For each service, find matching pods and register them.
	// We rebuild the registry from scratch on each sync: deregister existing
	// entries then re-register the current state.
	for _, svc := range services {
		name, _ := svc["name"].(string)
		ns, _ := svc["namespace"].(string)
		if ns == "" {
			ns = s.namespace
		}
		spec, _ := svc["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}
		selector, _ := spec["selector"].(map[string]interface{})

		fqdn := fmt.Sprintf("%s.%s.svc.cluster.local.", name, ns)
		s.reg.Deregister(fqdn)

		for _, pod := range runningPods {
			if matchesSelector(pod.labels, selector) {
				s.reg.Register(registry.ServiceRecord{
					Name: fqdn,
					IP:   pod.ip,
					TTL:  defaultTTL,
				})
			}
		}
	}
}

// matchesSelector returns true if all selector key=value pairs are present in
// labels.
func matchesSelector(labels, selector map[string]interface{}) bool {
	for k, v := range selector {
		lv, ok := labels[k]
		if !ok {
			return false
		}
		if fmt.Sprint(lv) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func (s *Syncer) fetchPods() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/apis/v1/namespaces/%s/pods", s.baseURL, s.namespace)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var pods []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pods); err != nil {
		return nil, err
	}
	return pods, nil
}

func (s *Syncer) fetchServices() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/apis/v1/namespaces/%s/services", s.baseURL, s.namespace)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var svcs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&svcs); err != nil {
		return nil, err
	}
	return svcs, nil
}
