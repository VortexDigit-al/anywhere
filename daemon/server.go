package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Protocol (newline-delimited JSON, one exchange per connection):
//
//	→  {"q":"query","max":500}
//	←  {"results":["/path/a","/path/b"],"truncated":false}

type searchRequest struct {
	Q   string `json:"q"`
	Max int    `json:"max"`
}

type searchResponse struct {
	Results   []string `json:"results"`
	Truncated bool     `json:"truncated"`
}

// Server listens on a Unix domain socket and answers search queries.
type Server struct {
	listener net.Listener
	idx      *Index
}

func newServer(socketPath string, idx *Index) (*Server, error) {
	// Remove a stale socket from a previous run.
	os.Remove(socketPath)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	// Only the current user may connect.
	os.Chmod(socketPath, 0600)
	return &Server{listener: l, idx: idx}, nil
}

func (s *Server) Run() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed by Stop
		}
		go s.handle(conn)
	}
}

func (s *Server) Stop() {
	s.listener.Close()
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req searchRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	if req.Max <= 0 {
		req.Max = 500
	}

	results := s.idx.Search(req.Q, req.Max)
	resp := searchResponse{
		Results:   results,
		Truncated: len(results) == req.Max,
	}
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		log.Printf("encode response: %v", err)
	}
}
