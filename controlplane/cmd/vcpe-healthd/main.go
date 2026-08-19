package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/gdcs-dev/vcpe/controlplane/internal/health"
)

type probeFlags []string

func (probes *probeFlags) String() string { return strings.Join(*probes, ",") }

func (probes *probeFlags) Set(value string) error {
	*probes = append(*probes, value)
	return nil
}

type httpProbeFlags []string

func (probes *httpProbeFlags) String() string { return strings.Join(*probes, ",") }

func (probes *httpProbeFlags) Set(value string) error {
	*probes = append(*probes, value)
	return nil
}

func main() {
	listen := flag.String("listen", ":9878", "health server listen address")
	command := flag.String("command", "", "optional readiness command")
	timeout := flag.Duration("timeout", 2*time.Second, "readiness command timeout")
	run := flag.String("run", "", "optional workload command to supervise")
	check := flag.Bool("check", false, "exit successfully only when the local health endpoint is healthy")
	checkURL := flag.String("check-url", "http://127.0.0.1:9878/health", "health endpoint URL for --check")
	var probes probeFlags
	flag.Var(&probes, "probe", "named readiness probe in name=command form; may be repeated")
	var httpProbes httpProbeFlags
	flag.Var(&httpProbes, "http-probe", "named HTTP readiness probe in name=url[|status] form; may be repeated")
	var diagnosticJourneys probeFlags
	flag.Var(&diagnosticJourneys, "diagnostic", "supported active diagnostic journey; may be repeated")
	subscriberDiagnosticURL := flag.String("diagnostic-subscriber-url", "", "optional loopback webhook subscriber diagnostic endpoint")
	flag.Parse()
	if *check {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		if err := checkEndpoint(ctx, *checkURL); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	probe := func(context.Context) health.Response {
		return health.Response{SchemaVersion: health.SchemaVersion, Status: health.StatusStarting, ObservedAt: time.Now().UTC()}
	}
	if strings.TrimSpace(*command) != "" {
		probe = commandProbe(*command, *timeout)
	}
	if len(probes) > 0 {
		var err error
		probe, err = namedCommandProbe(probes, *timeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	if len(httpProbes) > 0 {
		var err error
		probe, err = namedHTTPProbe(httpProbes, *timeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	journeys, err := buildDiagnosticJourneys(diagnosticJourneys, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	passiveRoutes, err := buildPassiveDiagnosticRoutes(*subscriberDiagnosticURL, *timeout, diagnosticJourneys)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := serve(ctx, *listen, probe, *run, journeys, passiveRoutes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, listen string, probe health.Probe, workload string, journeys map[string]diagnostic.JourneyHandler, passiveRoutes map[string]http.Handler) error {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	handler := diagnostic.Server{Journeys: journeys, PassiveRoutes: passiveRoutes}.Handler(health.Server{Probe: probe}.Handler())
	server := &http.Server{Handler: handler}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	if strings.TrimSpace(workload) == "" {
		select {
		case err := <-serveErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			return server.Shutdown(context.Background())
		}
	}

	workloadCommand := exec.CommandContext(ctx, "/bin/sh", "-c", workload)
	workloadCommand.Stdout = os.Stdout
	workloadCommand.Stderr = os.Stderr
	if err := workloadCommand.Start(); err != nil {
		_ = server.Close()
		return err
	}
	workloadErr := make(chan error, 1)
	go func() { workloadErr <- workloadCommand.Wait() }()

	select {
	case err := <-workloadErr:
		_ = server.Shutdown(context.Background())
		return err
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		_ = workloadCommand.Process.Signal(syscall.SIGTERM)
		<-workloadErr
		return err
	case <-ctx.Done():
		_ = workloadCommand.Process.Signal(syscall.SIGTERM)
		<-workloadErr
		return server.Shutdown(context.Background())
	}
}

func buildPassiveDiagnosticRoutes(rawURL string, timeout time.Duration, journeys []string) (map[string]http.Handler, error) {
	routes := make(map[string]http.Handler, 2)
	if strings.TrimSpace(rawURL) != "" {
		proxy, err := newSubscriberDiagnosticProxy(rawURL, timeout)
		if err != nil {
			return nil, err
		}
		routes[diagnostic.JourneyWebhookSubscriber] = proxy
	}
	for _, journey := range journeys {
		if journey != diagnostic.JourneyCPEWebPACallback {
			continue
		}
		probe, err := diagnostic.NewCaduceusRoutingProbeFromEnvironment(timeout)
		if err != nil {
			return nil, err
		}
		routes[diagnostic.JourneyCPEWebPACallback] = probe.Handler()
		break
	}
	if len(routes) == 0 {
		return nil, nil
	}
	return routes, nil
}

type subscriberDiagnosticProxy struct {
	endpoint *url.URL
	client   *http.Client
}

func newSubscriberDiagnosticProxy(rawURL string, timeout time.Duration) (http.Handler, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("invalid webhook subscriber diagnostic endpoint")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid webhook subscriber diagnostic endpoint")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return subscriberDiagnosticProxy{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (proxy subscriberDiagnosticProxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	maximum, ok := subscriberDiagnosticMaximum(request.URL.Path)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	endpoint := *proxy.endpoint
	endpoint.Path = request.URL.Path
	upstreamRequest, err := http.NewRequestWithContext(request.Context(), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		http.Error(writer, "subscriber diagnostic endpoint unavailable", http.StatusBadGateway)
		return
	}
	response, err := proxy.client.Do(upstreamRequest)
	if err != nil {
		http.Error(writer, "subscriber diagnostic endpoint unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.ContentLength > maximum {
		http.Error(writer, "subscriber diagnostic response exceeds limit", http.StatusBadGateway)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil || int64(len(body)) > maximum {
		http.Error(writer, "subscriber diagnostic response exceeds limit", http.StatusBadGateway)
		return
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		writer.Header().Set("Content-Type", contentType)
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = writer.Write(body)
}

func subscriberDiagnosticMaximum(path string) (int64, bool) {
	switch {
	case path == "/diagnostics/webhook-subscriber/intent":
		return diagnostic.MaxWebhookIntentBodySize, true
	case strings.HasPrefix(path, "/diagnostics/webhook-subscriber/receipts/") && strings.TrimPrefix(path, "/diagnostics/webhook-subscriber/receipts/") != "" && !strings.Contains(strings.TrimPrefix(path, "/diagnostics/webhook-subscriber/receipts/"), "/"):
		return diagnostic.MaxInvocationBodySize, true
	default:
		return 0, false
	}
}

func buildDiagnosticJourneys(values []string, timeout time.Duration) (map[string]diagnostic.JourneyHandler, error) {
	journeys := make(map[string]diagnostic.JourneyHandler, len(values))
	for _, value := range values {
		if value != diagnostic.JourneyCPEWebPA && value != diagnostic.JourneyCPEWebPACallback && value != diagnostic.JourneyParodusClients && value != diagnostic.JourneyArgusWebhooks && value != diagnostic.JourneyWebhook {
			return nil, fmt.Errorf("unsupported diagnostic journey %q", value)
		}
		if _, duplicate := journeys[value]; duplicate {
			return nil, fmt.Errorf("duplicate diagnostic journey %q", value)
		}
		switch value {
		case diagnostic.JourneyCPEWebPA:
			probe := diagnostic.NewCPEWebPAProbeFromEnvironment(timeout)
			journeys[value] = probe.RunWithInvocation
		case diagnostic.JourneyCPEWebPACallback:
			probe := diagnostic.NewCPECallbackProbeFromEnvironment(timeout)
			journeys[value] = probe.RunWithInvocation
		case diagnostic.JourneyParodusClients:
			probe := diagnostic.NewCPEWebPAProbeFromEnvironment(timeout)
			journeys[value] = probe.RunParodusClients
		case diagnostic.JourneyWebhook:
			probe := diagnostic.NewWebhookProbeFromEnvironment(timeout)
			journeys[value] = probe.RunWithInvocation
		case diagnostic.JourneyArgusWebhooks:
			probe := diagnostic.NewWebhookProbeFromEnvironment(timeout)
			journeys[value] = probe.RunInventory
		}
	}
	return journeys, nil
}

func commandProbe(command string, timeout time.Duration) health.Probe {
	return func(parent context.Context) health.Response {
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		err := exec.CommandContext(ctx, "/bin/sh", "-c", command).Run()
		status := health.StatusHealthy
		message := "ready"
		if err != nil {
			status = health.StatusUnhealthy
			message = "readiness command failed"
		}
		return health.Response{SchemaVersion: health.SchemaVersion, Status: status, ObservedAt: time.Now().UTC(), Checks: []health.Check{{Name: "readiness-command", Status: status, Message: message}}}
	}
}

func namedCommandProbe(values []string, timeout time.Duration) (health.Probe, error) {
	type namedCommand struct {
		name    string
		command string
	}
	commands := make([]namedCommand, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name, command, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		command = strings.TrimSpace(command)
		if !ok || name == "" || command == "" {
			return nil, fmt.Errorf("invalid --probe %q: use name=command", value)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate --probe name %q", name)
		}
		seen[name] = struct{}{}
		commands = append(commands, namedCommand{name: name, command: command})
	}
	return func(parent context.Context) health.Response {
		checks := make([]health.Check, 0, len(commands))
		overall := health.StatusHealthy
		for _, command := range commands {
			ctx, cancel := context.WithTimeout(parent, timeout)
			err := exec.CommandContext(ctx, "/bin/sh", "-c", command.command).Run()
			cancel()
			status := health.StatusHealthy
			message := "ready"
			if err != nil {
				status = health.StatusUnhealthy
				message = "readiness check failed"
				overall = health.StatusUnhealthy
			}
			checks = append(checks, health.Check{Name: command.name, Status: status, Message: message})
		}
		return health.Response{SchemaVersion: health.SchemaVersion, Status: overall, ObservedAt: time.Now().UTC(), Checks: checks}
	}, nil
}

func namedHTTPProbe(values []string, timeout time.Duration) (health.Probe, error) {
	type namedHTTP struct {
		name     string
		url      string
		expected int
	}
	probes := make([]namedHTTP, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name, value, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || value == "" {
			return nil, fmt.Errorf("invalid --http-probe %q: use name=url[|status]", value)
		}
		url := value
		expected := http.StatusOK
		if index := strings.LastIndex(value, "|"); index > 0 {
			url = value[:index]
			if _, err := fmt.Sscanf(value[index+1:], "%d", &expected); err != nil || expected < 100 || expected > 599 {
				return nil, fmt.Errorf("invalid --http-probe expected status in %q", value)
			}
		}
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil || request.URL.Scheme == "" || request.URL.Host == "" {
			return nil, fmt.Errorf("invalid --http-probe URL %q", url)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate health probe name %q", name)
		}
		seen[name] = struct{}{}
		probes = append(probes, namedHTTP{name: name, url: url, expected: expected})
	}
	return func(parent context.Context) health.Response {
		checks := make([]health.Check, 0, len(probes))
		overall := health.StatusHealthy
		for _, probe := range probes {
			ctx, cancel := context.WithTimeout(parent, timeout)
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, probe.url, nil)
			response, err := http.DefaultClient.Do(request)
			cancel()
			status := health.StatusHealthy
			message := "ready"
			if err != nil || response == nil || response.StatusCode != probe.expected {
				status = health.StatusUnhealthy
				message = "HTTP readiness check failed"
				overall = health.StatusUnhealthy
			}
			if response != nil {
				response.Body.Close()
			}
			checks = append(checks, health.Check{Name: probe.name, Status: status, Message: message})
		}
		return health.Response{SchemaVersion: health.SchemaVersion, Status: overall, ObservedAt: time.Now().UTC(), Checks: checks}
	}, nil
}

func checkEndpoint(ctx context.Context, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", response.StatusCode)
	}
	var payload health.Response
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode health response: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return err
	}
	if payload.Status != health.StatusHealthy {
		return fmt.Errorf("health endpoint status is %q", payload.Status)
	}
	return nil
}
