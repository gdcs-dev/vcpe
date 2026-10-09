package hwsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const APIVersion = "vcpe.dev/hwsim/v1"

type ExitCategory int

const (
	ExitOK ExitCategory = iota
	ExitRuntime
	ExitUsage
	ExitPermission
	ExitConflict
	ExitQuota
	ExitNotManaged
	ExitNotReady
)

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Transport interface {
	Execute(ctx context.Context, args ...string) (CommandResult, error)
}

type CommandError struct {
	ExitCode ExitCategory
	Code     string
	Message  string
	Details  map[string]any
}

func (e *CommandError) Error() string { return e.Message }

func IsErrorCode(err error, code string) bool {
	var commandErr *CommandError
	return errors.As(err, &commandErr) && commandErr.Code == code
}

type Client struct {
	transport Transport
}

func NewClient(transport Transport) *Client {
	return &Client{transport: transport}
}

type VersionResult struct {
	Version string `json:"version"`
}

type DoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type DoctorResult struct {
	Healthy        bool          `json:"healthy"`
	Checks         []DoctorCheck `json:"checks"`
	MeshCapability string        `json:"meshCapability"`
}

type NamespaceIdentity struct {
	PID   int    `json:"pid"`
	Inode uint64 `json:"inode"`
}

type KernelIdentity struct {
	RadioID uint32 `json:"radioId"`
	PHYName string `json:"phyName"`
}

type Radio struct {
	Name          string            `json:"name"`
	PermanentMAC  string            `json:"permanentMac"`
	GroupMask     uint64            `json:"groupMask"`
	InterfaceName string            `json:"interfaceName"`
	RadioType     string            `json:"radioType"`
	Namespace     NamespaceIdentity `json:"namespace"`
	Kernel        *KernelIdentity   `json:"kernel,omitempty"`
	Lifecycle     string            `json:"lifecycle"`
	LastError     string            `json:"lastError,omitempty"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

type EnsureRequest struct {
	Name          string
	MAC           string
	GroupMask     uint64
	NetNSPID      int
	InterfaceName string
	RadioType     string
}

type EnsureResult struct {
	Created bool  `json:"created"`
	Reused  bool  `json:"reused"`
	Radio   Radio `json:"radio"`
}

type ContainerPIDInspector interface {
	ContainerPID(context.Context, string) (int, error)
}

type ListEntry struct {
	Record            Radio              `json:"record"`
	ResolvedKernel    *KernelIdentity    `json:"resolvedKernel,omitempty"`
	ResolvedNamespace *NamespaceIdentity `json:"resolvedNamespace,omitempty"`
	Live              bool               `json:"live"`
}

type ReleaseResult struct {
	Name          string `json:"name"`
	Released      bool   `json:"released"`
	AlreadyAbsent bool   `json:"alreadyAbsent"`
}

type GCResult struct {
	Released  []string `json:"released"`
	Preserved []string `json:"preserved"`
}

func (c *Client) Version(ctx context.Context) (VersionResult, error) {
	var result VersionResult
	if err := c.execute(ctx, "version", nil, &result); err != nil {
		return VersionResult{}, err
	}
	if result.Version == "" {
		return VersionResult{}, fmt.Errorf("hwsim version response is missing version")
	}
	return result, nil
}

func (c *Client) Doctor(ctx context.Context, probe bool) (DoctorResult, error) {
	args := []string(nil)
	if probe {
		args = append(args, "--probe")
	}
	var result DoctorResult
	if err := c.execute(ctx, "doctor", args, &result); err != nil {
		return DoctorResult{}, err
	}
	return result, nil
}

func (c *Client) MediumHealth(ctx context.Context) (DoctorResult, error) {
	var result DoctorResult
	if err := c.execute(ctx, "medium-health", nil, &result); err != nil {
		return DoctorResult{}, err
	}
	return result, nil
}

func (c *Client) Ensure(ctx context.Context, request EnsureRequest) (EnsureResult, error) {
	args := []string{
		"--name", request.Name,
		"--mac", request.MAC,
		"--group-mask", strconv.FormatUint(request.GroupMask, 10),
		"--netns-pid", strconv.Itoa(request.NetNSPID),
		"--ifname", request.InterfaceName,
		"--type", request.RadioType,
	}
	var result EnsureResult
	if err := c.execute(ctx, "ensure", args, &result); err != nil {
		return EnsureResult{}, err
	}
	if err := validateEnsureResult(request, result); err != nil {
		return EnsureResult{}, err
	}
	return result, nil
}

// EnsureForContainer resolves a fresh VM-side container PID and retries once
// when the manager reports that the inspected namespace disappeared.
func (c *Client) EnsureForContainer(ctx context.Context, inspector ContainerPIDInspector, containerName string, request EnsureRequest) (EnsureResult, error) {
	if inspector == nil {
		return EnsureResult{}, fmt.Errorf("container PID inspector is not configured")
	}
	if strings.TrimSpace(containerName) == "" {
		return EnsureResult{}, fmt.Errorf("container name is required")
	}
	for attempt := 0; attempt < 2; attempt++ {
		pid, err := inspector.ContainerPID(ctx, containerName)
		if err != nil {
			return EnsureResult{}, fmt.Errorf("inspect container %s for radio %s: %w", containerName, request.Name, err)
		}
		request.NetNSPID = pid
		result, err := c.Ensure(ctx, request)
		if err == nil {
			return result, nil
		}
		if attempt == 0 && IsErrorCode(err, "namespace_not_found") {
			continue
		}
		return EnsureResult{}, err
	}
	panic("unreachable")
}

func (c *Client) List(ctx context.Context) ([]ListEntry, error) {
	var result []ListEntry
	if err := c.execute(ctx, "list", nil, &result); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(result))
	for _, entry := range result {
		if entry.Record.Name == "" {
			return nil, fmt.Errorf("hwsim list response contains an empty radio name")
		}
		if _, exists := seen[entry.Record.Name]; exists {
			return nil, fmt.Errorf("hwsim list response contains duplicate radio %q", entry.Record.Name)
		}
		seen[entry.Record.Name] = struct{}{}
	}
	return result, nil
}

func (c *Client) Release(ctx context.Context, name string) (ReleaseResult, error) {
	var result ReleaseResult
	if err := c.execute(ctx, "release", []string{"--name", name}, &result); err != nil {
		return ReleaseResult{}, err
	}
	if result.Name != name {
		return ReleaseResult{}, fmt.Errorf("hwsim release returned name %q, want %q", result.Name, name)
	}
	if result.Released == result.AlreadyAbsent {
		return ReleaseResult{}, fmt.Errorf("hwsim release returned invalid outcome for %q", name)
	}
	return result, nil
}

func (c *Client) GC(ctx context.Context, keep []string, allowEmpty bool) (GCResult, error) {
	names := append([]string(nil), keep...)
	sort.Strings(names)
	args := make([]string, 0, len(names)*2+1)
	for _, name := range names {
		args = append(args, "--keep", name)
	}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	var result GCResult
	if err := c.execute(ctx, "gc", args, &result); err != nil {
		return GCResult{}, err
	}
	return result, nil
}

type envelope struct {
	APIVersion string          `json:"apiVersion"`
	Command    string          `json:"command"`
	Success    bool            `json:"success"`
	Data       json.RawMessage `json:"data"`
	Error      *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error,omitempty"`
}

func (c *Client) execute(ctx context.Context, command string, args []string, destination any) error {
	if c == nil || c.transport == nil {
		return fmt.Errorf("hwsim transport is not configured")
	}
	commandArgs := append([]string{command}, args...)
	result, err := c.transport.Execute(ctx, commandArgs...)
	if err != nil {
		return fmt.Errorf("execute hwsim %s: %w", command, err)
	}
	var response envelope
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode hwsim %s response (exit %d): %w (%s)", command, result.ExitCode, err, strings.TrimSpace(string(result.Stderr)))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("decode hwsim %s response: expected one JSON document", command)
	}
	if response.APIVersion != APIVersion {
		return fmt.Errorf("incompatible hwsim API version %q, want %q", response.APIVersion, APIVersion)
	}
	if response.Command != command {
		return fmt.Errorf("hwsim response command %q does not match request %q", response.Command, command)
	}
	if result.ExitCode < int(ExitOK) || result.ExitCode > int(ExitNotReady) {
		return fmt.Errorf("hwsim %s returned unsupported exit code %d", command, result.ExitCode)
	}
	if response.Success {
		if result.ExitCode != int(ExitOK) || response.Error != nil {
			return fmt.Errorf("hwsim %s returned an inconsistent success envelope", command)
		}
		if destination != nil {
			if len(response.Data) == 0 {
				return fmt.Errorf("hwsim %s response is missing data", command)
			}
			if err := json.Unmarshal(response.Data, destination); err != nil {
				return fmt.Errorf("decode hwsim %s data: %w", command, err)
			}
		}
		return nil
	}
	if result.ExitCode == int(ExitOK) || response.Error == nil || response.Error.Code == "" || response.Error.Message == "" {
		return fmt.Errorf("hwsim %s returned an inconsistent error envelope", command)
	}
	return &CommandError{
		ExitCode: ExitCategory(result.ExitCode),
		Code:     response.Error.Code,
		Message:  response.Error.Message,
		Details:  response.Error.Details,
	}
}

func validateEnsureResult(request EnsureRequest, result EnsureResult) error {
	if result.Created == result.Reused {
		return fmt.Errorf("hwsim ensure returned invalid created/reused outcome")
	}
	radio := result.Radio
	if radio.Name != request.Name ||
		!strings.EqualFold(radio.PermanentMAC, request.MAC) ||
		radio.GroupMask != request.GroupMask ||
		radio.InterfaceName != request.InterfaceName ||
		radio.RadioType != request.RadioType ||
		radio.Namespace.PID != request.NetNSPID ||
		radio.Namespace.Inode == 0 ||
		radio.Kernel == nil || radio.Kernel.PHYName == "" ||
		radio.Lifecycle != "ready" {
		return fmt.Errorf("hwsim ensure returned identity that does not match radio %q", request.Name)
	}
	return nil
}
