package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// MessageHandler processes raw JSON-RPC bytes and returns a response.
type MessageHandler interface {
	HandleMessage(ctx context.Context, raw []byte) ([]byte, error)
}

// StdioTransport implements MCP over stdin/stdout using newline-delimited JSON-RPC.
type StdioTransport struct {
	reader  *bufio.Reader
	writer  io.Writer
	handler MessageHandler
	mu      sync.Mutex
	logger  *slog.Logger
}

func NewStdioTransport(handler MessageHandler, logger *slog.Logger) *StdioTransport {
	if logger == nil {
		logger = slog.Default()
	}
	return &StdioTransport{
		reader:  bufio.NewReader(os.Stdin),
		writer:  os.Stdout,
		handler: handler,
		logger:  logger,
	}
}

func NewStdioTransportWith(reader io.Reader, writer io.Writer, handler MessageHandler, logger *slog.Logger) *StdioTransport {
	if logger == nil {
		logger = slog.Default()
	}
	return &StdioTransport{
		reader:  bufio.NewReader(reader),
		writer:  writer,
		handler: handler,
		logger:  logger,
	}
}

func (t *StdioTransport) Serve(ctx context.Context) error {
	t.logger.Info("mcp stdio transport started")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("stdin read: %w", err)
			}
		}
		if len(line) <= 1 {
			continue
		}

		response, err := t.handler.HandleMessage(ctx, line)
		if err != nil {
			t.logger.Error("mcp handler error", "error", err)
			continue
		}
		if response != nil {
			if err := t.write(response); err != nil {
				return fmt.Errorf("stdout write: %w", err)
			}
		}
	}
}

func (t *StdioTransport) write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	_, err := t.writer.Write([]byte("\n"))
	return err
}

func (t *StdioTransport) SendNotification(method string, params interface{}) error {
	notif := map[string]interface{}{"jsonrpc": JSONRPCVersion, "method": method}
	if params != nil {
		notif["params"] = params
	}
	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	return t.write(data)
}
