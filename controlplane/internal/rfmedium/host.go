package rfmedium

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

const baselinePath = "/etc/vcpe/wmediumd.cfg"

type HostTransport interface {
	Execute(context.Context, []byte, ...string) (int, error)
}

type Host struct {
	Transport HostTransport
}

func NewHost() Host {
	return Host{Transport: machineTransport{}}
}

func (host Host) InstallBaseline(ctx context.Context, baseline string) error {
	if host.Transport == nil || baseline == "" {
		return fmt.Errorf("RF baseline transport and configuration are required")
	}
	staged := baselinePath + ".next"
	run := func(input []byte, args ...string) (int, error) {
		exit, err := host.Transport.Execute(ctx, input, args...)
		if err != nil {
			return exit, fmt.Errorf("RF baseline %s: %w", strings.Join(args, " "), err)
		}
		return exit, nil
	}
	require := func(input []byte, args ...string) error {
		exit, err := run(input, args...)
		if err != nil {
			return err
		}
		if exit != 0 {
			return fmt.Errorf("RF baseline %s exited %d", strings.Join(args, " "), exit)
		}
		return nil
	}
	if err := require([]byte(baseline), "install", "-m", "0600", "/dev/stdin", staged); err != nil {
		return err
	}
	stagedExists := true
	defer func() {
		if stagedExists {
			_, _ = run(nil, "rm", "-f", staged)
		}
	}()
	exit, err := run(nil, "cmp", "-s", staged, baselinePath)
	if err != nil {
		return err
	}
	switch exit {
	case 0:
		if err := require(nil, "rm", "-f", staged); err != nil {
			return err
		}
		stagedExists = false
		return require(nil, "systemctl", "start", "wmediumd.service")
	case 2:
		if err := require(nil, "test", "!", "-e", baselinePath); err != nil {
			return fmt.Errorf("RF baseline cmp failed on existing configuration: %w", err)
		}
	case 1:
	default:
		return fmt.Errorf("RF baseline cmp exited %d", exit)
	}
	if err := require(nil, "mv", "-f", staged, baselinePath); err != nil {
		return err
	}
	stagedExists = false
	return require(nil, "systemctl", "restart", "wmediumd.service")
}

func (host Host) ClearBaseline(ctx context.Context) error {
	if host.Transport == nil {
		return fmt.Errorf("RF host transport is not configured")
	}
	for _, command := range [][]string{{"systemctl", "stop", "wmediumd.service"}, {"rm", "-f", baselinePath}} {
		exit, err := host.Transport.Execute(ctx, nil, command...)
		if err != nil {
			return fmt.Errorf("RF baseline %s: %w", strings.Join(command, " "), err)
		}
		if exit != 0 {
			return fmt.Errorf("RF baseline %s exited %d", strings.Join(command, " "), exit)
		}
	}
	return nil
}

func (host Host) RestoreBaseline(ctx context.Context, baseline string) error {
	if err := host.InstallBaseline(ctx, baseline); err != nil {
		return err
	}
	if exit, err := host.Transport.Execute(ctx, nil, "systemctl", "restart", "wmediumd.service"); err != nil {
		return fmt.Errorf("restart RF baseline daemon: %w", err)
	} else if exit != 0 {
		return fmt.Errorf("restart RF baseline daemon exited %d", exit)
	}
	return nil
}

func (host Host) CheckHealthy(ctx context.Context) error {
	if host.Transport == nil {
		return fmt.Errorf("RF host transport is not configured")
	}
	if exit, err := host.Transport.Execute(ctx, nil, "systemctl", "is-active", "--quiet", "wmediumd.service"); err != nil {
		return fmt.Errorf("RF daemon health check: %w", err)
	} else if exit != 0 {
		return fmt.Errorf("RF daemon is not active (exit %d)", exit)
	}
	return nil
}

func (host Host) UpdateScenarioStep(ctx context.Context, deployment plan.Deployment, scenarioName string, index int) error {
	return host.updateScenarioLink(ctx, deployment, scenarioName, index, false)
}

func (host Host) RestoreScenarioLinks(ctx context.Context, deployment plan.Deployment, scenarioName string) error {
	for _, scenario := range deployment.WirelessScenarios {
		if scenario.Name != scenarioName {
			continue
		}
		restored := map[string]bool{}
		for index, step := range scenario.Steps {
			if restored[step.AP.BSSID] {
				continue
			}
			if err := host.updateScenarioLink(ctx, deployment, scenarioName, index, true); err != nil {
				return fmt.Errorf("restore scenario RF links: %w", err)
			}
			restored[step.AP.BSSID] = true
		}
		return nil
	}
	return fmt.Errorf("RF scenario %q is not planned for deployment %q", scenarioName, deployment.Name)
}

func (host Host) updateScenarioLink(ctx context.Context, deployment plan.Deployment, scenarioName string, index int, restore bool) error {
	if host.Transport == nil {
		return fmt.Errorf("RF host transport is not configured")
	}
	for _, scenario := range deployment.WirelessScenarios {
		if scenario.Name != scenarioName {
			continue
		}
		if index < 0 || index >= len(scenario.Steps) {
			return fmt.Errorf("RF scenario %q has no step %d", scenarioName, index)
		}
		step := scenario.Steps[index]
		if !restore && (step.SNRDb < -100 || step.SNRDb > 100) {
			return fmt.Errorf("RF scenario %q step %d SNR is out of range", scenarioName, index)
		}
		for _, address := range []string{scenario.Station.MAC, step.AP.BSSID} {
			parsed, err := net.ParseMAC(address)
			if err != nil || len(parsed) != 6 || parsed[0]&3 != 2 {
				return fmt.Errorf("RF scenario %q step %d has invalid planned MAC", scenarioName, index)
			}
		}
		snr := step.SNRDb
		if restore {
			snr = 35
		}
		exit, err := host.Transport.Execute(ctx, []byte(snrUpdateScript), "python3", "-", scenario.Station.MAC, step.AP.BSSID, strconv.Itoa(snr))
		if err != nil {
			return fmt.Errorf("RF scenario %q step %d link update: %w", scenarioName, index, err)
		}
		if exit != 0 {
			return fmt.Errorf("RF scenario %q step %d link update was not confirmed (exit %d)", scenarioName, index, exit)
		}
		return nil
	}
	return fmt.Errorf("RF scenario %q is not planned for deployment %q", scenarioName, deployment.Name)
}

const snrUpdateScript = `import socket, struct, sys
source = bytes.fromhex(sys.argv[1].replace(':', ''))
target = bytes.fromhex(sys.argv[2].replace(':', ''))
request = b'\x01' + source + target + struct.pack('!i', int(sys.argv[3]))
def read_exact(channel, length):
    result = b''
    while len(result) < length:
        chunk = channel.recv(length - len(result))
        if not chunk:
            raise RuntimeError('wmediumd disconnected before responding')
        result += chunk
    return result
with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as channel:
    channel.settimeout(3)
    channel.connect('/run/wmediumd.sock')
    channel.sendall(request)
    response = read_exact(channel, len(request) + 2)
if response != b'\x02' + request + b'\x00':
    raise RuntimeError('wmediumd did not confirm the requested SNR')
`

type machineTransport struct{}

func (machineTransport) Execute(ctx context.Context, input []byte, args ...string) (int, error) {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "podman"
		args = append([]string{"machine", "ssh", "--", "sudo", "-n"}, args...)
	case "linux":
		if os.Geteuid() != 0 {
			return 0, fmt.Errorf("RF baseline installation requires root")
		}
		name = args[0]
		args = args[1:]
	default:
		return 0, fmt.Errorf("RF baseline installation is unsupported on %s", runtime.GOOS)
	}
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if len(output) != 0 {
			return exitError.ExitCode(), fmt.Errorf("host command: %s", strings.TrimSpace(string(output)))
		}
		return exitError.ExitCode(), nil
	}
	return 0, fmt.Errorf("host command failed: %w (%s)", err, strings.TrimSpace(string(output)))
}
