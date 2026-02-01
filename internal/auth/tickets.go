package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Ticket represents a one-time connection token
type Ticket struct {
	TicketID  string
	UserID    string
	TaskID    int64
	ExpiresAt time.Time
}

// TicketStore defines the interface for ticket management
type TicketStore interface {
	// Generate creates a new ticket for a specific user and task
	Generate(userID string, taskID int64, ttl time.Duration) (*Ticket, error)

	// Exchange atomically validates and burns (deletes) a ticket.
	// Returns the ticket if valid, or an error if invalid/expired.
	Exchange(ticketID string) (*Ticket, error)

	// StartCleanupLoop starts a background goroutine to remove expired tickets.
	// Stops when context is cancelled.
	StartCleanupLoop(ctx context.Context, interval time.Duration)
}

// InMemoryTicketStore implements TicketStore using a map and RWMutex
type InMemoryTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]Ticket
}

// NewInMemoryTicketStore creates a new instance
func NewInMemoryTicketStore() *InMemoryTicketStore {
	return &InMemoryTicketStore{
		tickets: make(map[string]Ticket),
	}
}

// Generate creates a new ticket with cryptographic entropy
func (s *InMemoryTicketStore) Generate(userID string, taskID int64, ttl time.Duration) (*Ticket, error) {
	// Generate 16 bytes of entropy (128 bits)
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	ticketID := hex.EncodeToString(bytes)

	ticket := Ticket{
		TicketID:  ticketID,
		UserID:    userID,
		TaskID:    taskID,
		ExpiresAt: time.Now().Add(ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickets[ticketID] = ticket

	return &ticket, nil
}

// Exchange atomically validates and deletes the ticket (Check-and-Burn)
func (s *InMemoryTicketStore) Exchange(ticketID string) (*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket, exists := s.tickets[ticketID]
	if !exists {
		return nil, fmt.Errorf("ticket not found or already consumed")
	}

	// Always delete the ticket, regardless of expiry
	delete(s.tickets, ticketID)

	if time.Now().After(ticket.ExpiresAt) {
		return nil, fmt.Errorf("ticket expired")
	}

	return &ticket, nil
}

// StartCleanupLoop runs a background ticker to remove expired tickets
func (s *InMemoryTicketStore) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanup()
			}
		}
	}()
}

func (s *InMemoryTicketStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, ticket := range s.tickets {
		if now.After(ticket.ExpiresAt) {
			delete(s.tickets, id)
		}
	}
}
