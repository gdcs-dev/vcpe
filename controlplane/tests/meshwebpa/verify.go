package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/diagnostic"
	"github.com/xmidt-org/wrp-go/v3"
)

const maxResponseBytes = 1 << 20

var parodusClientNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func verifyDevices(ctx context.Context, client *http.Client, talariaURL, scytaleURL, talariaToken, scytaleToken string, deviceIDs []string) error {
	missing := ""
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, talariaURL+"/api/v2/devices", nil)
		if err != nil {
			return fmt.Errorf("inventory request: %w", err)
		}
		request.Header.Set("Authorization", "Basic "+talariaToken)
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				if missing != "" {
					return fmt.Errorf("device %s missing from inventory", missing)
				}
				return fmt.Errorf("inventory timeout")
			}
			return fmt.Errorf("inventory request failed")
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("inventory returned HTTP %d", response.StatusCode)
		}
		var inventory struct {
			Devices []struct {
				ID string `json:"id"`
			} `json:"devices"`
		}
		encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		response.Body.Close()
		if len(encoded) > maxResponseBytes {
			return fmt.Errorf("inventory exceeds limit")
		}
		if err != nil || json.Unmarshal(encoded, &inventory) != nil {
			return fmt.Errorf("invalid inventory")
		}
		registered := make(map[string]bool)
		for _, device := range inventory.Devices {
			registered[device.ID] = true
		}
		missing = ""
		for _, deviceID := range deviceIDs {
			if !registered[deviceID] {
				missing = deviceID
				break
			}
		}
		if missing == "" {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("device %s missing from inventory", missing)
		case <-time.After(10 * time.Millisecond):
		}
	}
	for _, deviceID := range deviceIDs {
		if err := retrieveClientList(ctx, client, scytaleURL, scytaleToken, deviceID); err != nil {
			return fmt.Errorf("device %s: %w", deviceID, err)
		}
	}
	return nil
}

func retrieveClientList(ctx context.Context, client *http.Client, baseURL, token, deviceID string) error {
	var transaction [16]byte
	if _, err := rand.Read(transaction[:]); err != nil {
		return fmt.Errorf("transaction identity: %w", err)
	}
	message := wrp.Message{
		Type: wrp.RetrieveMessageType, Source: "dns:webpa",
		Destination:     deviceID + "/parodus/client-list",
		TransactionUUID: fmt.Sprintf("%x-%x-%x-%x-%x", transaction[:4], transaction[4:6], transaction[6:8], transaction[8:10], transaction[10:]),
	}
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v3/device", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Basic "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/msgpack")
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("management timeout")
		}
		return fmt.Errorf("management request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return fmt.Errorf("management authentication rejected")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("management returned HTTP %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(encoded) > maxResponseBytes {
		return fmt.Errorf("invalid management response size")
	}
	var reply wrp.Message
	if err := wrp.NewDecoderBytes(encoded, wrp.Msgpack).Decode(&reply); err != nil {
		return fmt.Errorf("invalid management response")
	}
	if reply.Type != wrp.RetrieveMessageType || reply.Source != message.Destination || reply.Destination != message.Source || reply.TransactionUUID != message.TransactionUUID {
		return fmt.Errorf("mismatched management response")
	}
	var payload struct {
		Clients   []string `json:"client-list"`
		Truncated *bool    `json:"truncated"`
	}
	if err := json.Unmarshal(reply.Payload, &payload); err != nil || payload.Clients == nil || payload.Truncated == nil {
		return fmt.Errorf("invalid client list")
	}
	if len(payload.Clients) > diagnostic.MaxParodusClients {
		return fmt.Errorf("invalid client list")
	}
	for index, clientName := range payload.Clients {
		if len(clientName) > diagnostic.MaxIDLength || !parodusClientNamePattern.MatchString(clientName) || (index > 0 && payload.Clients[index-1] > clientName) {
			return fmt.Errorf("invalid client list")
		}
	}
	return nil
}

type containerTransport struct {
	container string
}

func containerClient(container string) *http.Client {
	return &http.Client{Transport: containerTransport{container: container}, Timeout: 40 * time.Second}
}

func (transport containerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var config strings.Builder
	fmt.Fprintf(&config, "url = %s\nrequest = %s\nsilent\nshow-error\nmax-time = 35\nmax-filesize = %d\ndump-header = /dev/stderr\n",
		strconv.Quote(request.URL.String()), strconv.Quote(request.Method), maxResponseBytes)
	for _, header := range []string{"Authorization", "Accept", "Content-Type"} {
		if value := request.Header.Get(header); value != "" {
			fmt.Fprintf(&config, "header = %s\n", strconv.Quote(header+": "+value))
		}
	}
	if request.Body != nil {
		body, err := io.ReadAll(io.LimitReader(request.Body, maxResponseBytes+1))
		if err != nil || len(body) > maxResponseBytes {
			return nil, fmt.Errorf("management request body exceeds limit")
		}
		fmt.Fprintf(&config, "data-binary = %s\n", strconv.Quote(string(body)))
	}
	command := exec.CommandContext(request.Context(), "podman", "exec", "-i", transport.container, "curl", "--config", "-")
	command.Stdin = strings.NewReader(config.String())
	var body, headers bytes.Buffer
	command.Stdout = &body
	command.Stderr = &headers
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("container-local request failed")
	}
	if body.Len() > maxResponseBytes {
		return nil, fmt.Errorf("container-local response exceeds limit")
	}
	status := 0
	for _, line := range strings.Split(headers.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], "HTTP/") {
			status, _ = strconv.Atoi(fields[1])
		}
	}
	if status == 0 {
		return nil, fmt.Errorf("container-local response has no HTTP status")
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body.Bytes())), Header: make(http.Header), Request: request}, nil
}
