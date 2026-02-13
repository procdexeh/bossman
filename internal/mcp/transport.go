package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

type Transport struct {
	scanner *bufio.Scanner
	writer  io.Writer
	mu      sync.Mutex
}

func NewTransport(r io.Reader, w io.Writer) *Transport {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 1<<20), 1<<20)
	return &Transport{scanner: s, writer: w}
}

func (t *Transport) ReadMessage() ([]Request, error) {
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	data := t.scanner.Bytes()

	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '[':
			var batch []Request
			if err := json.Unmarshal(data, &batch); err != nil {
				return nil, err
			}
			return batch, nil
		default:
			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			return []Request{req}, nil
		}
	}
	return nil, NewParseError("empty message")
}

func (t *Transport) WriteResponse(resp Response) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.writer.Write(data)
	return err
}

func (t *Transport) WriteBatchResponse(responses []Response) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(responses)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	_, err = t.writer.Write(data)
	return err
}
