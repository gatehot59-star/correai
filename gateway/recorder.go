// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

package gateway

import (
	"sync"

	"github.com/gatehot59-star/kampe-ir/testis"
)

// Recorder recibe los veredictos que emite handleConn.
//
// ES UNA INTERFAZ Y NO UNA IMPLEMENTACION A PROPOSITO. El destino real es
// Postgres, y las 5 tablas de audit/schema.sql NO EXISTEN en ninguna base
// (medido). Con una interfaz el cableado se puede escribir y medir hoy; con una
// implementacion concreta habria que esperar la base, y el cable quedaria como
// otro modulo sin llamador.
//
// Grabar NO devuelve error a proposito: el grabador no puede vetar una decision
// de seguridad ya tomada. Si la grabacion falla, eso es un hueco DECLARADO
// (dropped_since), no un motivo para cambiar el veredicto del gateway.
type Recorder interface {
	Grabar(v *testis.Verdict)
}

// NopRecorder descarta todo y cuenta los descartes.
//
// Es el default cuando no hay grabador configurado, y CUENTA en vez de ignorar:
// un gateway que perdio 10.000 veredictos y no lo sabe es peor que uno que no
// graba. El contador alimenta dropped_since del veredicto siguiente, que es
// exactamente la regla D-21: un descarte NO consume seq, se declara.
type NopRecorder struct {
	mu        sync.Mutex
	Descartes uint32
}

func (n *NopRecorder) Grabar(_ *testis.Verdict) {
	n.mu.Lock()
	n.Descartes++
	n.mu.Unlock()
}

// TomarDescartes devuelve el contador y lo resetea, para que el veredicto
// siguiente declare cuantos se perdieron desde el ultimo grabado.
func (n *NopRecorder) TomarDescartes() uint32 {
	n.mu.Lock()
	d := n.Descartes
	n.Descartes = 0
	n.mu.Unlock()
	return d
}

// MemRecorder guarda los veredictos en memoria. Es el grabador de los tests: sin
// el, un test del cableado no puede afirmar QUE se emitio, solo que no explota.
type MemRecorder struct {
	mu       sync.Mutex
	Cadenas  map[[32]byte][]*testis.Verdict
}

func NuevoMemRecorder() *MemRecorder {
	return &MemRecorder{Cadenas: make(map[[32]byte][]*testis.Verdict)}
}

func (m *MemRecorder) Grabar(v *testis.Verdict) {
	m.mu.Lock()
	m.Cadenas[v.AgentID] = append(m.Cadenas[v.AgentID], v)
	m.mu.Unlock()
}

// Cadena devuelve una copia de la cadena de un agente.
func (m *MemRecorder) Cadena(agentID [32]byte) []*testis.Verdict {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*testis.Verdict, len(m.Cadenas[agentID]))
	copy(out, m.Cadenas[agentID])
	return out
}

// Reglas del veredicto, alineadas con testis.Reglas. Son los MISMOS numeros que
// la spec de Python: si divergen, dos implementaciones nombran distinto el mismo
// rechazo y la evidencia deja de ser comparable.
const (
	ReglaFSMBlocked      uint8 = 1
	ReglaFrameSize       uint8 = 2
	ReglaDecodeFailed    uint8 = 3
	ReglaAgentIDMismatch uint8 = 4
	ReglaTimestampWindow uint8 = 5
	ReglaReplay          uint8 = 6
	ReglaHMACInvalid     uint8 = 7
	ReglaHuberTrip       uint8 = 8
	ReglaCoherenceTrip   uint8 = 9
	ReglaDecayed         uint8 = 10
	ReglaAccepted        uint8 = 200
)

// emitir construye el veredicto, lo encadena al anterior de ESTE agente y lo
// manda al grabador.
//
// El estado de la cadena (seq, prevHash) vive en AgentState bajo su mutex,
// porque un agente puede tener varias conexiones concurrentes con el mismo
// certificado y la cadena es UNA por agente, no una por conexion. Si el seq se
// llevara en la pila de handleConn, dos conexiones del mismo cert emitirian dos
// cadenas con seq 1, y el validador leeria un salto de seq: manipulacion donde
// solo hubo concurrencia.
func (g *Gateway) emitir(agent *AgentState, agentID [32]byte, rule uint8,
	pkt *PerimeterPacket, observedAtNs int64, backoffNS int64) {
	if g.recorder == nil {
		return
	}

	agent.mu.Lock()
	agent.chainSeq++
	v := &testis.Verdict{
		Seq:        agent.chainSeq,
		PrevHash:   agent.chainPrev,
		AgentID:    agentID,
		ObservedAt: observedAtNs,
		Rule:       rule,
		BackoffNS:  backoffNS,
	}
	// El paquete puede ser nil: los rechazos tempranos (frame invalido, decode
	// fallido) ocurren ANTES de que exista un paquete parseado. En ese caso los
	// campos del paquete quedan en cero y el veredicto igual se emite: que el
	// rechazo no tenga payload no lo hace menos evidencia.
	if pkt != nil {
		v.PacketTS = pkt.Timestamp
		v.Epoch = pkt.Epoch
		v.Nonce = pkt.Nonce
		v.PayloadLen = uint32(len(pkt.Ciphertext))
		n := len(pkt.Context)
		if n > testis.ContextSize {
			n = testis.ContextSize
		}
		copy(v.Context[:n], pkt.Context[:n])
	}
	if nop, ok := g.recorder.(*NopRecorder); ok {
		v.DroppedSince = nop.TomarDescartes()
	}
	v.Sellar(g.testisKey)
	agent.chainPrev = v.Hash
	agent.mu.Unlock()

	g.recorder.Grabar(v)
}
