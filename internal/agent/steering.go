package agent

import (
	"sync"

	"github.com/nigelpepper/attractor/internal/llm"
)

// SteeringManager holds steering and follow-up messages injected between rounds.
type SteeringManager struct {
	mu        sync.Mutex
	steering  []string
	followups []string
}

// NewSteeringManager returns an empty manager.
func NewSteeringManager() *SteeringManager { return &SteeringManager{} }

// Steer queues a message to inject between tool rounds.
func (s *SteeringManager) Steer(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steering = append(s.steering, message)
}

// FollowUp queues a message to process after the current turn.
func (s *SteeringManager) FollowUp(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followups = append(s.followups, message)
}

// DrainSteering returns and clears pending steering messages.
func (s *SteeringManager) DrainSteering() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]llm.Message, 0, len(s.steering))
	for _, t := range s.steering {
		msgs = append(msgs, llm.UserMessage(t))
	}
	s.steering = nil
	return msgs
}

// DrainFollowups returns and clears pending follow-up messages.
func (s *SteeringManager) DrainFollowups() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]llm.Message, 0, len(s.followups))
	for _, t := range s.followups {
		msgs = append(msgs, llm.UserMessage(t))
	}
	s.followups = nil
	return msgs
}
