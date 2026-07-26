package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mycodex-dev/dashdrop/internal/config"
	"github.com/mycodex-dev/dashdrop/internal/storage"
)

// Server is a Streamable HTTP MCP endpoint that exposes dashboard tools.
type Server struct {
	store *storage.Store
	cfg   config.Config

	mu       sync.Mutex
	sessions map[string]time.Time
}

// New creates an MCP server bound to the given store and config.
func New(store *storage.Store, cfg config.Config) *Server {
	return &Server{
		store:    store,
		cfg:      cfg,
		sessions: make(map[string]time.Time),
	}
}

// Handler returns the HTTP handler for the MCP endpoint.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.ServeHTTP)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MCPToken != "" {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// CORS for browser-based MCP clients
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id, Mcp-Protocol-Version")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleGet(w, r)
	case http.MethodDelete:
		s.handleDelete(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		token := strings.TrimSpace(auth[7:])
		return token != "" && token == s.cfg.MCPToken
	}
	// Also accept token query for simple agent wiring
	if q := r.URL.Query().Get("token"); q != "" && q == s.cfg.MCPToken {
		return true
	}
	return false
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxUploadRequestBytes()+64<<10))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, errorResponse(nil, errParse, "parse error", nil))
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != jsonRPCVersion {
		writeJSON(w, http.StatusOK, errorResponse(req.ID, errInvalidReq, "invalid jsonrpc version", nil))
		return
	}
	if req.Method == "" {
		writeJSON(w, http.StatusOK, errorResponse(req.ID, errInvalidReq, "method is required", nil))
		return
	}

	// Notifications have no id and must return 202 with empty body.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	sessionID := r.Header.Get(headerSessionID)
	if req.Method != "initialize" && sessionID != "" && !s.sessionExists(sessionID) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	resp, newSession := s.dispatch(r, req)
	if isNotification {
		if newSession != "" {
			w.Header().Set(headerSessionID, newSession)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if newSession != "" {
		w.Header().Set(headerSessionID, newSession)
	}
	if v := r.Header.Get(headerProtocol); v != "" {
		w.Header().Set(headerProtocol, negotiateProtocol(v))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(headerSessionID)
	if sessionID != "" && !s.sessionExists(sessionID) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	accept := strings.Join(r.Header.Values("Accept"), ",")
	if !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Accept must contain text/event-stream", http.StatusBadRequest)
		return
	}

	// No server-initiated notifications; keep an idle SSE stream briefly then end.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if sessionID != "" {
		w.Header().Set(headerSessionID, sessionID)
	}
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(headerSessionID)
	if sessionID == "" {
		http.Error(w, "Mcp-Session-Id required", http.StatusBadRequest)
		return
	}
	s.deleteSession(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dispatch(r *http.Request, req jsonRPCRequest) (jsonRPCResponse, string) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized", "initialized":
		return jsonRPCResponse{}, ""
	case "ping":
		return successResponse(req.ID, map[string]any{}), ""
	case "tools/list":
		return successResponse(req.ID, map[string]any{"tools": toolDefinitions()}), ""
	case "tools/call":
		params, err := parseCallToolParams(req.Params)
		if err != nil {
			return errorResponse(req.ID, errInvalidParams, err.Error(), nil), ""
		}
		result := s.callTool(r, params)
		return successResponse(req.ID, result), ""
	default:
		return errorResponse(req.ID, errMethod, "method not found: "+req.Method, nil), ""
	}
}

func (s *Server) handleInitialize(req jsonRPCRequest) (jsonRPCResponse, string) {
	var params initializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, errInvalidParams, "invalid initialize params", nil), ""
		}
	}

	version := negotiateProtocol(params.ProtocolVersion)
	sessionID := newSessionID()
	s.putSession(sessionID)

	result := map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": serverVersion,
		},
		"instructions": "Dashdrop MCP server. Use upload_dashboard to publish a single HTML file and receive a live URL. Optional title, slug, tags, and expires_at are supported. Thumbnails are generated by the server.",
	}
	return successResponse(req.ID, result), sessionID
}

func (s *Server) putSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneSessionsLocked(time.Now())
	s.sessions[id] = time.Now().Add(24 * time.Hour)
}

func (s *Server) sessionExists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneSessionsLocked(time.Now())
	exp, ok := s.sessions[id]
	return ok && time.Now().Before(exp)
}

func (s *Server) deleteSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Server) pruneSessionsLocked(now time.Time) {
	for id, exp := range s.sessions {
		if now.After(exp) {
			delete(s.sessions, id)
		}
	}
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("mcp: write response: %v", err)
	}
}

func readFileLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if max > 0 && int64(len(data)) > max {
		return nil, storage.ErrTooLarge
	}
	return data, nil
}
