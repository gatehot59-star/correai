// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sync"
	"time"
)

const (
	frameHeaderSize     = 4
	maxBufferedBytes    = 64 * 1024
	maxPacketSize       = maxBufferedBytes - frameHeaderSize
	numberOfMapShards   = 64
	defaultReadDeadline = 2 * time.Second

	// FIX anti-replay: ventana de tiempo aceptable para timestamps.
	maxTimestampDriftNs = int64(30 * time.Second)
)

var (
	bufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, maxBufferedBytes)
			return &buf
		},
	}

	packetPool = sync.Pool{
		New: func() interface{} {
			return &PerimeterPacket{}
		},
	}

	ackBytes    = []byte{0x00}
	rejectBytes = []byte{0xFF}
)

type agentShard struct {
	mu     sync.RWMutex
	agents map[[32]byte]*AgentState
}

type PerimeterPacket struct {
	AgentID    [32]byte
	AgentIDLen uint8
	Timestamp  uint64
	Epoch      uint32
	Nonce      uint32
	HMAC       []byte

	Context    [ContextVectorSize]float64
	ContextLen int

	Ciphertext []byte
	Flags      uint32
	Padding    uint32
}

func (p *PerimeterPacket) Reset() {
	p.AgentID = [32]byte{}
	p.AgentIDLen = 0
	p.Timestamp = 0
	p.Epoch = 0
	p.Nonce = 0
	p.HMAC = nil
	p.Context = [ContextVectorSize]float64{}
	p.ContextLen = 0
	p.Ciphertext = nil
	p.Flags = 0
	p.Padding = 0
}

// AgentState contiene todos los filtros y el estado de un agente.
// FIX anti-replay: se agrega lastTimestampNs para detectar replays.
type AgentState struct {
	FSM             *AgentFSM
	Huber           *HuberFilter
	Coherence       *CoherenceFilter
	mu              sync.Mutex
	lastTimestampNs uint64 // FIX: para anti-replay monotónico
}

// Gateway es el punto de entrada TLS mTLS del sistema.
// FIX: incorpora hmacKey cargada desde variable de entorno
// para verificar la firma de cada PerimeterPacket.
type Gateway struct {
	addr           string
	tlsConfig      *tls.Config
	shards         []*agentShard
	initialBackoff time.Duration
	hmacKey        []byte // FIX: clave para verificar HMAC de paquetes
}

// loadHMACKey carga la clave desde la variable de entorno GATEWAY_HMAC_KEY
// (32 bytes en hexadecimal). Falla explícitamente si no está definida.
func loadHMACKey() ([]byte, error) {
	hexKey := os.Getenv("GATEWAY_HMAC_KEY")
	if hexKey == "" {
		return nil, errors.New("hipersec: GATEWAY_HMAC_KEY no definida")
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("hipersec: GATEWAY_HMAC_KEY inválida: %w", err)
	}

	if len(key) != 32 {
		return nil, errors.New("hipersec: GATEWAY_HMAC_KEY debe tener 32 bytes")
	}

	return key, nil
}

func NewGateway(addr string, tlsConfig *tls.Config) (*Gateway, error) {
	// FIX: la clave HMAC es obligatoria desde la construcción.
	key, err := loadHMACKey()
	if err != nil {
		return nil, err
	}

	shards := make([]*agentShard, numberOfMapShards)
	for i := 0; i < numberOfMapShards; i++ {
		shards[i] = &agentShard{
			agents: make(map[[32]byte]*AgentState),
		}
	}

	return &Gateway{
		addr:           addr,
		tlsConfig:      tlsConfig,
		shards:         shards,
		initialBackoff: 100 * time.Millisecond,
		hmacKey:        key,
	}, nil
}

func NewTLSConfig(caFile, certFile, keyFile string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("hipersec: read ca: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("hipersec: load server cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("hipersec: append ca certificate")
	}

	return &tls.Config{
		Certificates:           []tls.Certificate{cert},
		ClientCAs:              pool,
		ClientAuth:             tls.RequireAndVerifyClientCert,
		MinVersion:             tls.VersionTLS13,
		NextProtos:             []string{"hipersec/perimeter/v1"},
		SessionTicketsDisabled: true,
		Renegotiation:          tls.RenegotiateNever,
	}, nil
}

func (g *Gateway) Run() error {
	listener, err := tls.Listen("tcp", g.addr, g.tlsConfig)
	if err != nil {
		return fmt.Errorf("hipersec gateway listen: %w", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("hipersec gateway accept: %w", err)
		}

		go g.handleConn(conn)
	}
}

func (g *Gateway) getShard(agentID [32]byte) *agentShard {
	idx := int(agentID[0]) % numberOfMapShards
	return g.shards[idx]
}

func (g *Gateway) handleConn(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			_ = conn.Close()
		}
	}()
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return
	}

	if err := tlsConn.Handshake(); err != nil {
		return
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return
	}

	agentID := deriveAgentID(state.PeerCertificates[0])
	agent := g.getOrCreateAgent(agentID)

	bufPtr := bufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer bufferPool.Put(bufPtr)

	pkt := packetPool.Get().(*PerimeterPacket)
	defer packetPool.Put(pkt)

	lastSeen := time.Now()

	for {
		now := time.Now()

		if !agent.FSM.Allow(now) {
			writeReject(conn)
			return
		}

		_ = conn.SetReadDeadline(now.Add(defaultReadDeadline))

		if _, err := io.ReadFull(conn, buf[:frameHeaderSize]); err != nil {
			return
		}

		size := binary.BigEndian.Uint32(buf[:frameHeaderSize])
		if size == 0 || size > uint32(maxPacketSize) {
			writeReject(conn)
			return
		}

		bodyEnd := frameHeaderSize + int(size)
		if _, err := io.ReadFull(conn, buf[frameHeaderSize:bodyEnd]); err != nil {
			return
		}

		body := buf[frameHeaderSize:bodyEnd]

		// FIX E0. La llegada REAL del paquete. El `now` de arriba se capturo
		// ANTES del io.ReadFull, asi que no sirve ni para dt ni para el reloj
		// del anti-replay. `now` se sigue usando para FSM.Allow y para el read
		// deadline, que SI deben mirar el inicio de la iteracion.
		llegada := time.Now()

		pkt.Reset()
		if !decodePerimeterPacket(body, pkt) {
			writeReject(conn)
			agent.FSM.TriggerBlock(now)
			return
		}

		// FIX 1: Verificar que el AgentID del paquete coincide
		// con el derivado del certificado TLS.
		if pkt.AgentID != agentID {
			writeReject(conn)
			agent.FSM.TriggerBlock(now)
			return
		}

		// FIX 2: Anti-replay — verificar ventana de timestamp.
		// El timestamp del paquete no debe diferir más de ±30s del reloj
		// del servidor, y debe ser mayor al último timestamp aceptado.
		nowNs := uint64(llegada.UnixNano())
		if !validateTimestamp(nowNs, pkt.Timestamp, maxTimestampDriftNs) {
			writeReject(conn)
			agent.FSM.TriggerBlock(now)
			return
		}

		agent.mu.Lock()
		if pkt.Timestamp <= agent.lastTimestampNs {
			// Replay detectado: timestamp no es estrictamente creciente.
			agent.mu.Unlock()
			writeReject(conn)
			agent.FSM.TriggerBlock(now)
			return
		}
		agent.lastTimestampNs = pkt.Timestamp
		agent.mu.Unlock()

		// FIX 3: Verificar HMAC del paquete.
		// El HMAC cubre los campos de identidad: AgentID + Timestamp +
		// Epoch + Nonce, que son los que un atacante querría manipular.
		if !g.verifyPacketHMAC(pkt) {
			writeReject(conn)
			agent.FSM.TriggerBlock(now)
			return
		}

		// FIX 4: Pasar bytes reales al HuberFilter en lugar del
		// valor fijo 1.0. Usamos el tamaño del ciphertext normalizado
		// a KB como métrica de volumen real.
		dt := llegada.Sub(lastSeen).Seconds()
		if dt <= 0 {
			dt = 1e-9
		}
		lastSeen = llegada

		// Volumen normalizado: bytes del payload en KB.
		volumeKB := float64(len(pkt.Ciphertext)) / 1024.0
		if volumeKB <= 0 {
			volumeKB = 1e-9
		}

		if agent.Huber.Update(volumeKB, dt) {
			agent.FSM.TriggerBlock(now)
			writeReject(conn)
			return
		}

		if agent.Coherence.Update(pkt.Context) {
			agent.FSM.TriggerBlock(now)
			writeReject(conn)
			return
		}

		// FIX 5: Registrar éxito para que DecayBackoff se active
		// automáticamente tras éxitos consecutivos.
		agent.FSM.RecordSuccess()

		writeAck(conn)
	}
}

// validateTimestamp verifica que el timestamp del paquete esté dentro
// de la ventana de deriva aceptable respecto al reloj del servidor.
func validateTimestamp(serverNs uint64, packetNs uint64, maxDriftNs int64) bool {
	var diff int64
	if packetNs > serverNs {
		diff = int64(packetNs - serverNs)
	} else {
		diff = int64(serverNs - packetNs)
	}

	return diff <= maxDriftNs
}

// verifyPacketHMAC verifica la firma HMAC del paquete usando la clave
// del gateway. El mensaje a firmar es: AgentID || Timestamp || Epoch || Nonce.
// FIX CRÍTICO: antes el HMAC nunca se verificaba.
func (g *Gateway) verifyPacketHMAC(pkt *PerimeterPacket) bool {
	if len(pkt.HMAC) == 0 {
		return false
	}

	// Construir el mensaje canónico: campos de identidad y tiempo.
	msg := make([]byte, 32+8+4+4) // AgentID(32) + Timestamp(8) + Epoch(4) + Nonce(4)
	copy(msg[0:32], pkt.AgentID[:])
	binary.BigEndian.PutUint64(msg[32:40], pkt.Timestamp)
	binary.BigEndian.PutUint32(msg[40:44], pkt.Epoch)
	binary.BigEndian.PutUint32(msg[44:48], pkt.Nonce)

	mac := hmac.New(sha256.New, g.hmacKey)
	mac.Write(msg)
	expected := mac.Sum(nil)

	// Comparación en tiempo constante para evitar timing attacks.
	return hmac.Equal(pkt.HMAC, expected)
}

func (g *Gateway) getOrCreateAgent(id [32]byte) *AgentState {
	shard := g.getShard(id)

	shard.mu.RLock()
	agent, exists := shard.agents[id]
	shard.mu.RUnlock()
	if exists {
		return agent
	}

	fsm := NewAgentFSM(g.initialBackoff)
	huber := NewHuberFilter(DefaultAlpha, DefaultSigma)
	coherence := NewCoherenceFilter(DefaultFastAlpha, DefaultMediumAlpha, DefaultCoherenceThreshold)

	agent = &AgentState{
		FSM:       fsm,
		Huber:     huber,
		Coherence: coherence,
	}

	shard.mu.Lock()
	if existing, ok := shard.agents[id]; ok {
		agent = existing
	} else {
		shard.agents[id] = agent
	}
	shard.mu.Unlock()

	return agent
}

func deriveAgentID(cert *x509.Certificate) [32]byte {
	return sha256.Sum256(cert.RawSubject)
}

func writeAck(conn net.Conn) {
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(ackBytes)
}

func writeReject(conn net.Conn) {
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(rejectBytes)
}

func consumeVarint(buf []byte) (uint64, []byte, bool) {
	var x uint64
	var shift uint

	for i, b := range buf {
		if i >= 10 {
			return 0, buf, false
		}

		if b < 0x80 {
			x |= uint64(b) << shift
			return x, buf[i+1:], true
		}

		x |= uint64(b&0x7f) << shift
		shift += 7
	}

	return 0, buf, false
}

func decodePerimeterPacket(data []byte, pkt *PerimeterPacket) bool {
	for len(data) > 0 {
		tag, rest, ok := consumeVarint(data)
		if !ok {
			return false
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		if wireType != 2 {
			return false
		}

		length, rest, ok := consumeVarint(rest)
		if !ok || length > uint64(len(rest)) {
			return false
		}

		fieldData := rest[:length]
		data = rest[length:]

		switch fieldNum {
		case 1:
			if !decodeHeader(fieldData, pkt) {
				return false
			}
		case 2:
			if !decodeContext(fieldData, pkt) {
				return false
			}
		case 3:
			if !decodePayload(fieldData, pkt) {
				return false
			}
		default:
			// Campo desconocido: se ignora para compatibilidad futura.
		}
	}

	return pkt.AgentIDLen > 0 &&
		pkt.ContextLen == ContextVectorSize &&
		len(pkt.Ciphertext) > 0
}

func decodeHeader(data []byte, pkt *PerimeterPacket) bool {
	for len(data) > 0 {
		tag, rest, ok := consumeVarint(data)
		if !ok {
			return false
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // agent_id
			if wireType != 2 {
				return false
			}
			length, rest2, ok := consumeVarint(rest)
			if !ok || length > uint64(len(rest2)) || length > uint64(len(pkt.AgentID)) {
				return false
			}
			copy(pkt.AgentID[:], rest2[:length])
			pkt.AgentIDLen = uint8(length)
			data = rest2[length:]

		case 2: // timestamp_ns
			if wireType != 0 {
				return false
			}
			value, rest2, ok := consumeVarint(rest)
			if !ok {
				return false
			}
			pkt.Timestamp = value
			data = rest2

		case 3: // epoch
			if wireType != 0 {
				return false
			}
			value, rest2, ok := consumeVarint(rest)
			if !ok {
				return false
			}
			pkt.Epoch = uint32(value)
			data = rest2

		case 4: // nonce
			if wireType != 0 {
				return false
			}
			value, rest2, ok := consumeVarint(rest)
			if !ok {
				return false
			}
			pkt.Nonce = uint32(value)
			data = rest2

		case 5: // hmac
			if wireType != 2 {
				return false
			}
			length, rest2, ok := consumeVarint(rest)
			if !ok || length > uint64(len(rest2)) {
				return false
			}
			pkt.HMAC = rest2[:length]
			data = rest2[length:]

		default:
			// FIX: antes esto hacía return false, rompiendo compatibilidad.
			// Ahora se salta el campo desconocido según su wire type.
			if err := skipField(wireType, rest, &data); err != nil {
				return false
			}
		}
	}

	return pkt.AgentIDLen > 0
}

// skipField avanza data más allá de un campo desconocido.
// FIX: decodeHeader antes rechazaba cualquier campo desconocido,
// ahora los salta para mantener compatibilidad futura.
func skipField(wireType uint64, rest []byte, data *[]byte) error {
	switch wireType {
	case 0: // varint
		_, remaining, ok := consumeVarint(rest)
		if !ok {
			return errors.New("varint inválido")
		}
		*data = remaining
	case 2: // length-delimited
		length, remaining, ok := consumeVarint(rest)
		if !ok || length > uint64(len(remaining)) {
			return errors.New("campo length-delimited inválido")
		}
		*data = remaining[length:]
	default:
		return errors.New("wire type desconocido")
	}

	return nil
}

func decodeContext(data []byte, pkt *PerimeterPacket) bool {
	for len(data) > 0 {
		tag, rest, ok := consumeVarint(data)
		if !ok {
			return false
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		if fieldNum != 1 || wireType != 2 {
			return false
		}

		length, rest, ok := consumeVarint(rest)
		if !ok || length > uint64(len(rest)) {
			return false
		}

		if length%8 != 0 {
			return false
		}

		n := int(length / 8)
		if n != ContextVectorSize {
			return false
		}

		for i := 0; i < n; i++ {
			bits := binary.LittleEndian.Uint64(rest[i*8 : (i+1)*8])
			pkt.Context[i] = math.Float64frombits(bits)
		}

		pkt.ContextLen = n
		data = rest[length:]
	}

	return pkt.ContextLen == ContextVectorSize
}

func decodePayload(data []byte, pkt *PerimeterPacket) bool {
	for len(data) > 0 {
		tag, rest, ok := consumeVarint(data)
		if !ok {
			return false
		}

		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 1: // ciphertext
			if wireType != 2 {
				return false
			}
			length, rest2, ok := consumeVarint(rest)
			if !ok || length > uint64(len(rest2)) {
				return false
			}
			pkt.Ciphertext = rest2[:length]
			data = rest2[length:]

		case 2: // flags
			if wireType != 0 {
				return false
			}
			value, rest2, ok := consumeVarint(rest)
			if !ok {
				return false
			}
			pkt.Flags = uint32(value)
			data = rest2

		case 3: // padding
			if wireType != 0 {
				return false
			}
			value, rest2, ok := consumeVarint(rest)
			if !ok {
				return false
			}
			pkt.Padding = uint32(value)
			data = rest2

		default:
			return false
		}
	}

	return len(pkt.Ciphertext) > 0
}
