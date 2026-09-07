// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

package gateway

import (
	"sync"
	"time"
)

type GateState int

const (
	StateActive GateState = iota
	StateBlocked
)

// AgentFSM implementa una máquina de estados finitos por agente con
// Circuit Breaker de timeout exponencial.
// FIX: DecayBackoff ahora se llama correctamente desde el path de ACK exitoso.
type AgentFSM struct {
	mu            sync.Mutex
	state         GateState
	initialBackoff time.Duration
	maxBackoff    time.Duration
	backoff       time.Duration
	blockUntil    time.Time
	successStreak int // FIX: contador de éxitos consecutivos para decay
}

const decayAfterSuccesses = 10 // decae tras 10 éxitos consecutivos

func NewAgentFSM(initialBackoff time.Duration) *AgentFSM {
	if initialBackoff <= 0 {
		initialBackoff = 100 * time.Millisecond
	}

	return &AgentFSM{
		state:          StateActive,
		initialBackoff: initialBackoff,
		maxBackoff:     30 * time.Second,
		backoff:        initialBackoff,
	}
}

func (f *AgentFSM) Allow(now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch f.state {
	case StateActive:
		return true
	case StateBlocked:
		if now.After(f.blockUntil) || now.Equal(f.blockUntil) {
			f.state = StateActive
			f.successStreak = 0
			return true
		}
		return false
	default:
		return false
	}
}

func (f *AgentFSM) TriggerBlock(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.successStreak = 0
	f.backoff *= 2
	if f.backoff > f.maxBackoff {
		f.backoff = f.maxBackoff
	}

	f.blockUntil = now.Add(f.backoff)
	f.state = StateBlocked
}

// RecordSuccess registra un paquete exitoso y dispara DecayBackoff
// automáticamente tras decayAfterSuccesses consecutivos.
// FIX: antes DecayBackoff nunca era llamado.
func (f *AgentFSM) RecordSuccess() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.successStreak++
	if f.successStreak >= decayAfterSuccesses {
		f.backoff = f.initialBackoff
		f.successStreak = 0
	}
}

// DecayBackoff resetea el backoff al valor inicial (API pública conservada).
func (f *AgentFSM) DecayBackoff() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.backoff = f.initialBackoff
	f.successStreak = 0
}
