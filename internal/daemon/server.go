package daemon

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/albachteng/provenance/internal/config"
	"github.com/albachteng/provenance/internal/session"
	"github.com/albachteng/provenance/internal/storage"
)

// ErrDaemonStopped is returned when the daemon is stopped gracefully
var ErrDaemonStopped = errors.New("daemon stopped")

// Daemon represents the provenance event collection daemon
type Daemon struct {
	db         *sql.DB
	socketPath string
	listener   net.Listener
	ready      chan struct{}
	shutdown   chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	stopped    bool

	// Session management
	sessionMgr *session.Manager
	cfg        *config.Config
	ticker     *time.Ticker
}

// SessionEvent represents a session lifecycle event
type SessionEvent struct {
	Type    string          `json:"type"` // "session_start" | "session_end"
	Session storage.Session `json:"session"`
}

// NewDaemon creates a new daemon instance
func NewDaemon(db *sql.DB, socketPath string, sessionMgr *session.Manager, cfg *config.Config) (*Daemon, error) {
	if db == nil {
		return nil, errors.New("database cannot be nil")
	}
	if socketPath == "" {
		return nil, errors.New("socket path cannot be empty")
	}
	if sessionMgr == nil {
		return nil, errors.New("session manager cannot be nil")
	}
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	return &Daemon{
		db:         db,
		socketPath: socketPath,
		sessionMgr: sessionMgr,
		cfg:        cfg,
		ready:      make(chan struct{}),
		shutdown:   make(chan struct{}),
	}, nil
}

// Ready returns a channel that closes when the daemon is ready to accept connections
func (d *Daemon) Ready() <-chan struct{} {
	return d.ready
}

// Start starts the daemon and begins accepting connections
func (d *Daemon) Start() error {
	if err := os.Remove(d.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	d.listener = listener

	close(d.ready)

	// Start session boundary checking
	d.ticker = time.NewTicker(d.cfg.Daemon.SessionCheckInterval.Duration)
	d.wg.Add(1)
	go d.sessionCheckLoop()

	for {
		select {
		case <-d.shutdown:
			return ErrDaemonStopped
		default:
			// Set a deadline so Accept doesn't block forever
			// This is a workaround since we can't interrupt Accept on Unix sockets
			conn, err := listener.Accept()
			if err != nil {
				// Check if we're shutting down
				select {
				case <-d.shutdown:
					return ErrDaemonStopped
				default:
					// Log error but continue accepting
					log.Printf("failed to accept connection: %v", err)
					continue
				}
			}

			// Handle connection in background
			d.mu.Lock()
			if d.stopped {
				d.mu.Unlock()
				if err := conn.Close(); err != nil {
					log.Printf("failed to close rejected connection: %v", err)
				}
				continue
			}
			d.wg.Add(1)
			d.mu.Unlock()
			go d.handleConnection(conn)
		}
	}
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() error {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return nil
	}
	d.stopped = true
	close(d.shutdown)
	d.mu.Unlock()

	if d.ticker != nil {
		d.ticker.Stop()
	}

	if d.listener != nil {
		if err := d.listener.Close(); err != nil {
			log.Printf("error closing listener: %v", err)
		}
	}

	d.wg.Wait()

	if err := os.Remove(d.socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to remove socket file: %v", err)
	}

	return nil
}

// sessionCheckLoop periodically checks if sessions should end
func (d *Daemon) sessionCheckLoop() {
	defer d.wg.Done()

	for {
		select {
		case <-d.shutdown:
			return
		case <-d.ticker.C:
			d.sessionMgr.CheckSessionBoundaries()
		}
	}
}

// handleConnection processes a single connection
func (d *Daemon) handleConnection(conn net.Conn) {
	defer d.wg.Done()
	defer conn.Close() //nolint:errcheck

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				log.Printf("error reading from connection: %v", err)
			}
			break
		}
		buf = append(buf, tmp[:n]...)
		if n < len(tmp) {
			break
		}
	}

	if len(buf) == 0 {
		return
	}

	// Try to determine message type by checking for "type" field (SessionEvent)
	// If it has a "type" field, it's a SessionEvent, otherwise PromptEvent
	var typeCheck struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(buf, &typeCheck); err != nil {
		log.Printf("failed to parse JSON: %v", err)
		return
	}

	if typeCheck.Type != "" {
		var sessionEvent SessionEvent
		if err := json.Unmarshal(buf, &sessionEvent); err != nil {
			log.Printf("failed to decode session event: %v", err)
			return
		}

		if err := d.handleSessionEvent(&sessionEvent); err != nil {
			log.Printf("failed to handle session event: %v", err)
		}
	} else {
		var event storage.PromptEvent
		if err := json.Unmarshal(buf, &event); err != nil {
			log.Printf("failed to decode prompt event: %v", err)
			return
		}

		if err := storage.StorePromptEvent(d.db, &event); err != nil {
			log.Printf("failed to store prompt event: %v", err)
		}
	}
}

// handleSessionEvent handles session lifecycle events
func (d *Daemon) handleSessionEvent(event *SessionEvent) error {
	switch event.Type {
	case "session_start":
		err := storage.CreateSession(d.db, &event.Session)
		// Ignore duplicate session errors (idempotent session creation)
		if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil
		}
		return err
	case "session_end":
		if event.Session.EndTime != nil {
			return storage.EndSession(d.db, event.Session.ID, *event.Session.EndTime, event.Session.EndedBy)
		}
		return errors.New("session_end event missing EndTime")
	default:
		return fmt.Errorf("unknown session event type: %s", event.Type)
	}
}
