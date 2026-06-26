package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
)

type Handler func(context.Context, CommandRequest) (CommandResponse, error)

func Serve(ctx context.Context, socketPath string, handler Handler) error {
	if err := os.RemoveAll(socketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.RemoveAll(socketPath)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			continue
		}

		go handleConn(ctx, conn, handler)
	}
}

func handleConn(ctx context.Context, conn net.Conn, handler Handler) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req CommandRequest
	if err := dec.Decode(&req); err != nil {
		_ = enc.Encode(CommandResponse{Error: fmt.Sprintf("decode request: %v", err)})
		return
	}

	resp, err := handler(ctx, req)
	if err != nil {
		_ = enc.Encode(CommandResponse{Error: err.Error()})
		return
	}

	_ = enc.Encode(resp)
}
