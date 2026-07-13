package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) Execute(ctx context.Context, req CommandRequest) (CommandResponse, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return CommandResponse{}, fmt.Errorf("connect daemon: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(req); err != nil {
		return CommandResponse{}, fmt.Errorf("send request: %w", err)
	}

	var resp CommandResponse
	if err := dec.Decode(&resp); err != nil {
		return CommandResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
