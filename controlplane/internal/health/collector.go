package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Target identifies one persisted, loopback-published health endpoint.
type Target struct {
	Deployment string
	Service    string
	Replica    int
	Host       string
	Port       int
}

// Observation is the collector's result for one expected instance.
type Observation struct {
	Target     Target
	State      string
	ObservedAt time.Time
	Response   *Response
	Error      string
}

// Collector retrieves health exclusively through HTTP.
type Collector struct {
	client      *http.Client
	concurrency int
}

// NewCollector creates a collector with bounded request duration and parallelism.
func NewCollector(timeout time.Duration, concurrency int) *Collector {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Collector{client: &http.Client{Timeout: timeout}, concurrency: concurrency}
}

// Collect observes every target, preserving target order even when requests
// complete out of order. Invalid responses and transport failures are unknown.
func (c *Collector) Collect(ctx context.Context, targets []Target) []Observation {
	observations := make([]Observation, len(targets))
	jobs := make(chan int)
	workers := c.concurrency
	if workers > len(targets) {
		workers = len(targets)
	}
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for index := range jobs {
				observations[index] = c.collectOne(ctx, targets[index])
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	waitGroup.Wait()
	sort.SliceStable(observations, func(left, right int) bool {
		if observations[left].Target.Service != observations[right].Target.Service {
			return observations[left].Target.Service < observations[right].Target.Service
		}
		return observations[left].Target.Replica < observations[right].Target.Replica
	})
	return observations
}

func (c *Collector) collectOne(ctx context.Context, target Target) Observation {
	observed := time.Now().UTC()
	observation := Observation{Target: target, State: "unknown", ObservedAt: observed}
	if target.Host != "127.0.0.1" || target.Port < 1 || target.Port > 65535 {
		observation.Error = "invalid persisted loopback endpoint"
		return observation
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(target.Host, fmt.Sprint(target.Port))+"/health", nil)
	if err != nil {
		observation.Error = "build health request: " + err.Error()
		return observation
	}
	response, err := c.client.Do(request)
	if err != nil {
		observation.Error = "health request: " + err.Error()
		return observation
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		observation.Error = fmt.Sprintf("health endpoint returned HTTP %d", response.StatusCode)
		return observation
	}
	var body Response
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		observation.Error = "decode health response: " + err.Error()
		return observation
	}
	if err := body.Validate(); err != nil {
		observation.Error = "invalid health response: " + err.Error()
		return observation
	}
	observation.Response = &body
	observation.State = string(body.Status)
	return observation
}
