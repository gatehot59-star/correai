// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// CLIENTE mTLS DE PRUEBA. Es el primero que existe en el proyecto.
//
// POR QUE IMPORTA: hasta este archivo, `handleConn`, `decodePerimeterPacket`, el
// anti-replay y `verifyPacketHMAC` NUNCA fueron ejercitados por un cliente. Todas
// las mediciones de concurrencia (D-26, el costo del candado, el p99) golpearon
// el `AgentState` directamente, que es un PROXY DECLARADO del callsite y no el
// callsite. Este cliente convierte ese proxy en la cosa real.
//
// TRES PIEZAS:
//   1. una PKI de prueba: CA, cert de servidor, y un cert POR AGENTE.
//   2. el CODIFICADOR del PerimeterPacket, escrito contra `decodePerimeterPacket`
//      campo por campo.
//   3. el cliente: handshake mTLS + frame + lectura de la respuesta de 1 byte.
//
// EL DETALLE QUE DECIDE COMO SE ESCRIBEN LOS TESTS: el agente se identifica con
// `sha256(cert.RawSubject)`, y `getOrCreateAgent` devuelve el MISMO *AgentState a
// todas las conexiones de ese certificado. Un `TriggerBlock` deja al agente
// bloqueado con backoff exponencial, asi que un caso que dispara un bloqueo
// contamina a cualquier caso posterior que use el mismo cert. Por eso cada caso
// emite su PROPIO certificado, con su propio CommonName.
//
// Y EL CODIFICADOR NO SE VALIDA CONTRA UNA SPEC, se valida contra el decoder del
// repo: si un campo, un wire type o un orden esta mal, el gateway rechaza el
// paquete y el test de ACK cae. Ese test es el control del arnes.

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// alpnDelGateway es el protocolo que declara NewTLSConfig. Si el cliente ofrece
// otro, TLS 1.3 corta el handshake con no_application_protocol.
const alpnDelGateway = "hipersec/perimeter/v1"

// respAck y respRechazo son los dos unicos bytes que el gateway sabe escribir.
// Los 9 caminos de rechazo comparten el 0xFF: por eso el cliente no puede
// distinguir POR QUE lo rechazaron, y por eso existe Testis.
const (
	respAck     = byte(0x00)
	respRechazo = byte(0xFF)
)

// ---------------------------------------------------------------------------
// PKI de prueba
// ---------------------------------------------------------------------------

type pki struct {
	dir    string
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte
}

func nuevaPKI(t *testing.T) *pki {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave de CA: %v", err)
	}

	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "KAMPE IR CA de prueba"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("crear cert de CA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsear cert de CA: %v", err)
	}

	p := &pki{
		dir:    t.TempDir(),
		caCert: cert,
		caKey:  key,
		caPEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
	return p
}

// emitir firma un cert con la CA. `servidor` decide el ExtKeyUsage y los SAN.
func (p *pki) emitir(t *testing.T, cn string, servidor bool) (certPEM, keyPEM []byte, parsed *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave de %q: %v", cn, err)
	}

	serie, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 96))
	if err != nil {
		t.Fatalf("serie: %v", err)
	}

	plantilla := &x509.Certificate{
		SerialNumber: serie,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"KAMPE IR"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if servidor {
		plantilla.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		plantilla.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		plantilla.DNSNames = []string{"localhost"}
	} else {
		plantilla.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	der, err := x509.CreateCertificate(rand.Reader, plantilla, p.caCert, &key.PublicKey, p.caKey)
	if err != nil {
		t.Fatalf("crear cert de %q: %v", cn, err)
	}
	parsed, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsear cert de %q: %v", cn, err)
	}

	derKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("serializar clave de %q: %v", cn, err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: derKey})
	return certPEM, keyPEM, parsed
}

// archivosDelServidor escribe CA, cert y clave a disco, porque NewTLSConfig
// recibe RUTAS. Se usa la funcion del repo a proposito: si NewTLSConfig estuviera
// mal, el handshake fallaria aca y no en produccion.
func (p *pki) archivosDelServidor(t *testing.T) (caPath, certPath, keyPath string) {
	t.Helper()

	certPEM, keyPEM, _ := p.emitir(t, "gateway de prueba", true)

	caPath = filepath.Join(p.dir, "ca.pem")
	certPath = filepath.Join(p.dir, "servidor.pem")
	keyPath = filepath.Join(p.dir, "servidor-key.pem")

	for ruta, datos := range map[string][]byte{
		caPath:   p.caPEM,
		certPath: certPEM,
		keyPath:  keyPEM,
	} {
		if err := os.WriteFile(ruta, datos, 0o600); err != nil {
			t.Fatalf("escribir %s: %v", ruta, err)
		}
	}
	return caPath, certPath, keyPath
}

// agente es un cliente con su certificado y el AgentID que el gateway va a
// derivar de el. Ese AgentID se calcula con la MISMA funcion del repo
// (deriveAgentID), no con una reimplementacion: si cambiara la derivacion, este
// arnes cambia con ella.
type agente struct {
	nombre  string
	agentID [32]byte
	tlsCfg  *tls.Config
}

func (p *pki) nuevoAgente(t *testing.T, cn string) *agente {
	t.Helper()

	certPEM, keyPEM, parsed := p.emitir(t, cn, false)

	par, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("par de claves de %q: %v", cn, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(p.caPEM) {
		t.Fatal("no se pudo cargar la CA en el pool del cliente")
	}

	return &agente{
		nombre:  cn,
		agentID: deriveAgentID(parsed),
		tlsCfg: &tls.Config{
			Certificates: []tls.Certificate{par},
			RootCAs:      pool,
			ServerName:   "127.0.0.1",
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{alpnDelGateway},
		},
	}
}

// ---------------------------------------------------------------------------
// CODIFICADOR del PerimeterPacket
// ---------------------------------------------------------------------------

// paquete es lo que el cliente va a mandar. Los campos son deliberadamente
// crudos para poder forjar cada uno por separado en los tests.
type paquete struct {
	agentID    [32]byte
	timestamp  uint64
	epoch      uint32
	nonce      uint32
	hmac       []byte
	contexto   [ContextVectorSize]float64
	ciphertext []byte
	flags      uint32
	padding    uint32
}

func ponerVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

// campoBytes emite un campo length-delimited (wire type 2).
func campoBytes(dst []byte, num int, val []byte) []byte {
	dst = ponerVarint(dst, uint64(num)<<3|2)
	dst = ponerVarint(dst, uint64(len(val)))
	return append(dst, val...)
}

// campoVarint emite un campo varint (wire type 0).
func campoVarint(dst []byte, num int, v uint64) []byte {
	dst = ponerVarint(dst, uint64(num)<<3|0)
	return ponerVarint(dst, v)
}

// firmarCabeceraDelPaquete arma el mensaje canonico que verifyPacketHMAC espera:
// AgentID(32) || Timestamp(8 BE) || Epoch(4 BE) || Nonce(4 BE).
//
// Ojo: NO cubre Context ni Ciphertext. Eso es D-47, y este cliente lo puede
// demostrar sobre el camino real.
func firmarCabeceraDelPaquete(clave []byte, p *paquete) []byte {
	msg := make([]byte, 32+8+4+4)
	copy(msg[0:32], p.agentID[:])
	binary.BigEndian.PutUint64(msg[32:40], p.timestamp)
	binary.BigEndian.PutUint32(msg[40:44], p.epoch)
	binary.BigEndian.PutUint32(msg[44:48], p.nonce)

	mac := hmac.New(sha256.New, clave)
	mac.Write(msg)
	return mac.Sum(nil)
}

// codificar arma el frame completo: 4 bytes big-endian de largo + cuerpo.
// El cuerpo son tres campos externos, TODOS wire type 2, porque
// decodePerimeterPacket rechaza cualquier otro wire type en el nivel de arriba.
func (p *paquete) codificar() []byte {
	var cabecera []byte
	cabecera = campoBytes(cabecera, 1, p.agentID[:])
	cabecera = campoVarint(cabecera, 2, p.timestamp)
	cabecera = campoVarint(cabecera, 3, uint64(p.epoch))
	cabecera = campoVarint(cabecera, 4, uint64(p.nonce))
	cabecera = campoBytes(cabecera, 5, p.hmac)

	// El contexto va como 8 float64 LITTLE-endian, aunque el resto del
	// protocolo sea big-endian. No es un error mio: es lo que hace
	// decodeContext con binary.LittleEndian.Uint64.
	crudo := make([]byte, 0, 8*ContextVectorSize)
	for _, v := range p.contexto {
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(v))
		crudo = append(crudo, tmp[:]...)
	}
	var contexto []byte
	contexto = campoBytes(contexto, 1, crudo)

	var carga []byte
	carga = campoBytes(carga, 1, p.ciphertext)
	carga = campoVarint(carga, 2, uint64(p.flags))
	carga = campoVarint(carga, 3, uint64(p.padding))

	var cuerpo []byte
	cuerpo = campoBytes(cuerpo, 1, cabecera)
	cuerpo = campoBytes(cuerpo, 2, contexto)
	cuerpo = campoBytes(cuerpo, 3, carga)

	frame := make([]byte, frameHeaderSize+len(cuerpo))
	binary.BigEndian.PutUint32(frame[:frameHeaderSize], uint32(len(cuerpo)))
	copy(frame[frameHeaderSize:], cuerpo)
	return frame
}

// paqueteValido devuelve un paquete bien formado y bien firmado para `ag`.
func paqueteValido(clave []byte, ag *agente, ts uint64, nonce uint32) *paquete {
	p := &paquete{
		agentID:    ag.agentID,
		timestamp:  ts,
		epoch:      1,
		nonce:      nonce,
		ciphertext: []byte("carga-de-prueba-de-kampe-ir"),
		flags:      0,
		padding:    0,
	}
	for i := range p.contexto {
		p.contexto[i] = 0.1 * float64(i+1)
	}
	p.hmac = firmarCabeceraDelPaquete(clave, p)
	return p
}

// ---------------------------------------------------------------------------
// EL CLIENTE
// ---------------------------------------------------------------------------

type conexion struct {
	conn net.Conn
}

// conectar hace el handshake mTLS contra el gateway.
func (ag *agente) conectar(addr string) (*conexion, error) {
	conn, err := tls.Dial("tcp", addr, ag.tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("handshake de %q: %w", ag.nombre, err)
	}
	return &conexion{conn: conn}, nil
}

func (c *conexion) cerrar() { _ = c.conn.Close() }

// enviar manda un paquete y devuelve el byte de respuesta.
// Un error de lectura significa que el gateway cerro sin responder.
func (c *conexion) enviar(p *paquete) (byte, error) {
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(p.codificar()); err != nil {
		return 0, fmt.Errorf("escribir frame: %w", err)
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp [1]byte
	if _, err := io.ReadFull(c.conn, resp[:]); err != nil {
		return 0, fmt.Errorf("leer respuesta: %w", err)
	}
	return resp[0], nil
}

// enviarCrudo manda bytes arbitrarios, para forjar frames que el codificador no
// puede producir (tamano cero, tamano gigante, basura).
func (c *conexion) enviarCrudo(datos []byte) (byte, error) {
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(datos); err != nil {
		return 0, fmt.Errorf("escribir crudo: %w", err)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp [1]byte
	if _, err := io.ReadFull(c.conn, resp[:]); err != nil {
		return 0, fmt.Errorf("leer respuesta: %w", err)
	}
	return resp[0], nil
}

// ---------------------------------------------------------------------------
// ARRANQUE DEL GATEWAY
// ---------------------------------------------------------------------------

// claveHMACDePrueba es la clave del gateway, en hex, como la espera
// GATEWAY_HMAC_KEY. Fija a proposito: la evidencia tiene que ser recomputable.
func claveHMACDePrueba() ([]byte, string) {
	clave := make([]byte, 32)
	for i := range clave {
		clave[i] = byte(i*7 + 3)
	}
	return clave, hex.EncodeToString(clave)
}

// puertoLibre pide un puerto al SO y lo suelta.
//
// LIMITACION DECLARADA: entre el Close y el Listen del gateway hay una ventana
// en la que otro proceso podria tomar el puerto. Si eso pasa, Run() falla y
// esperarAlGateway corta el test con un mensaje claro. No se enmascara.
func puertoLibre(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pedir puerto libre: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func esperarAlGateway(t *testing.T, addr string) {
	t.Helper()
	fin := time.Now().Add(5 * time.Second)
	for time.Now().Before(fin) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("el gateway no escucha en %s despues de 5 s", addr)
}

// arrancarGateway levanta un gateway REAL con NewGateway + NewTLSConfig + Run.
// Devuelve la direccion, la clave HMAC y la PKI.
//
// LIMITACION DECLARADA: Run() no tiene forma de apagarse (no hay Shutdown en el
// codigo de produccion), asi que su goroutine y su listener quedan vivos hasta
// que termina el proceso de test. Es una fuga por turno de test, acotada, y es
// un hueco del gateway, no del arnes.
func arrancarGateway(t *testing.T) (addr string, clave []byte, p *pki) {
	t.Helper()

	p = nuevaPKI(t)
	caPath, certPath, keyPath := p.archivosDelServidor(t)

	cfg, err := NewTLSConfig(caPath, certPath, keyPath)
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}

	clave, hexClave := claveHMACDePrueba()
	t.Setenv("GATEWAY_HMAC_KEY", hexClave)

	addr = puertoLibre(t)
	g, err := NewGateway(addr, cfg)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	go func() { _ = g.Run() }()
	esperarAlGateway(t, addr)

	return addr, clave, p
}

// ahoraNs es el reloj que usa el gateway para la ventana de deriva.
func ahoraNs() uint64 { return uint64(time.Now().UnixNano()) }
