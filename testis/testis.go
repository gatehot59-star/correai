// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

// Package testis es el modulo IR de KAMPE IR: la cadena de veredictos con
// tamper-evidence y su ancla externa (el Sello).
//
// POR QUE ESTE PAQUETE EXISTE Y POR QUE ES UN PUERTO, NO UN DISENO NUEVO.
// La especificacion vivia entera en verificacion/t_testis.py, con 11 controles
// positivos y fallos=0, y CERO lineas de Go. Tres auditorias independientes
// (la mia, la de Tao y la de Tachi) coincidieron en el mismo punto: Testis es
// el diferencial del producto y no existia como codigo.
//
// EL CONTRATO NO NEGOCIABLE: este paquete tiene que producir los MISMOS BYTES
// que el Python, hash por hash. No "equivalente", identico. Si los dos
// difieren en un byte, el auditor que verifique una cadena con una
// implementacion y la rechace con la otra tiene razon las dos veces, y el
// producto no tiene un formato: tiene dos. Por eso el test trae vectores
// golden calculados por el Python y los compara contra el Go.
package testis

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

// CanonicalLen es el ancho FIJO del canonico, en bytes. Sale del formato de
// struct de Python ">Q32s32sqBQIII8dqiI":
//
//	seq       uint64   8
//	prevHash  [32]byte 32
//	agentID   [32]byte 32
//	observedAt int64   8
//	rule      uint8    1
//	packetTS  uint64   8
//	epoch     uint32   4
//	nonce     uint32   4
//	payloadLen uint32  4
//	context   [8]f64   64
//	backoffNS int64    8
//	streak    int32    4
//	droppedSince uint32 4
//	                 = 181
//
// Ancho fijo y CERO separadores es lo que hace que el canonico no colisione al
// correr la frontera entre dos campos contiguos. Custos Legis usa la version
// mala (valores unidos por '|' sin escapar) y ahi vive D-06.
const CanonicalLen = 181

// ContextSize es el largo del vector de contexto. Coincide con
// gateway.ContextVectorSize a proposito: el veredicto sella el mismo vector
// que el filtro evaluo.
const ContextSize = 8

// Cero32 es el prevHash del primer veredicto de toda cadena.
var Cero32 = [32]byte{}

// Reglas son los motivos de veredicto. 200 es Accepted; el resto, rechazos.
var Reglas = map[uint8]string{
	1: "FSMBlocked", 2: "FrameSize", 3: "DecodeFailed", 4: "AgentIDMismatch",
	5: "TimestampWindow", 6: "Replay", 7: "HMACInvalid", 8: "HuberTrip",
	9: "CoherenceTrip", 10: "Decayed", 200: "Accepted",
}

// Verdict es un eslabon de la cadena.
//
// Hash y Signature NO se calculan en el constructor: se calculan en Sellar().
// Que existan sin sellar es un estado valido y representable, porque el
// grabador construye el veredicto y despues lo sella; un tipo que no admita
// ese estado intermedio obliga a un constructor de 13 argumentos.
type Verdict struct {
	Seq          uint64
	PrevHash     [32]byte
	AgentID      [32]byte
	ObservedAt   int64
	Rule         uint8
	PacketTS     uint64
	Epoch        uint32
	Nonce        uint32
	PayloadLen   uint32
	Context      [ContextSize]float64
	BackoffNS    int64
	Streak       int32
	DroppedSince uint32

	Hash      [32]byte
	Signature [32]byte
	sellado   bool
}

// Canonical serializa el veredicto en big-endian, ancho fijo, sin separadores.
//
// Devuelve un array y no un slice a proposito: el largo es parte del contrato,
// y un [CanonicalLen]byte lo hace imposible de violar sin que el compilador lo
// vea. El Python lo verifica en runtime con un invariante; en Go lo verifica el
// tipo.
func (v *Verdict) Canonical() [CanonicalLen]byte {
	var b [CanonicalLen]byte
	o := 0
	binary.BigEndian.PutUint64(b[o:], v.Seq)
	o += 8
	copy(b[o:], v.PrevHash[:])
	o += 32
	copy(b[o:], v.AgentID[:])
	o += 32
	binary.BigEndian.PutUint64(b[o:], uint64(v.ObservedAt))
	o += 8
	b[o] = v.Rule
	o += 1
	binary.BigEndian.PutUint64(b[o:], v.PacketTS)
	o += 8
	binary.BigEndian.PutUint32(b[o:], v.Epoch)
	o += 4
	binary.BigEndian.PutUint32(b[o:], v.Nonce)
	o += 4
	binary.BigEndian.PutUint32(b[o:], v.PayloadLen)
	o += 4
	for i := 0; i < ContextSize; i++ {
		binary.BigEndian.PutUint64(b[o:], math.Float64bits(v.Context[i]))
		o += 8
	}
	binary.BigEndian.PutUint64(b[o:], uint64(v.BackoffNS))
	o += 8
	binary.BigEndian.PutUint32(b[o:], uint32(v.Streak))
	o += 4
	binary.BigEndian.PutUint32(b[o:], v.DroppedSince)
	o += 4
	if o != CanonicalLen {
		// Inalcanzable si el tipo no cambio. Si alguien agrega un campo y se
		// olvida de este metodo, el panic es mejor que un hash silenciosamente
		// distinto entre implementaciones.
		panic(fmt.Sprintf("testis: el canonico escribio %d B, no %d", o, CanonicalLen))
	}
	return b
}

// Sellar calcula Hash = sha256(canonico) y Signature = HMAC(key, Hash).
//
// Se firma el HASH y no el canonico, igual que el Python: asi el verificador
// puede chequear el eslabon sin recomputar la firma, y las dos fallas quedan
// distinguibles ("hash no coincide" vs "firma invalida").
func (v *Verdict) Sellar(key []byte) *Verdict {
	c := v.Canonical()
	v.Hash = sha256.Sum256(c[:])
	m := hmac.New(sha256.New, key)
	m.Write(v.Hash[:])
	copy(v.Signature[:], m.Sum(nil))
	v.sellado = true
	return v
}

// Sellado dice si Sellar() ya corrio sobre este veredicto.
func (v *Verdict) Sellado() bool { return v.sellado }

// Hueco es un descarte DECLARADO por el grabador: en el veredicto Seq se
// perdieron Perdidos veredictos anteriores.
type Hueco struct {
	Seq      uint64
	Perdidos uint32
}

// Info son los datos que el validador devuelve mas alla del veredicto.
type Info struct {
	N                 int
	HuecosDeclarados  []Hueco
	Perdidos          uint32
}

// ValidarCadena verifica la integridad de la cadena de un agente.
//
// REGLA DE HUECOS (fix D-21): un descarte del grabador NO consume Seq. El
// veredicto siguiente declara cuantos se perdieron en DroppedSince. Un salto
// de Seq es SIEMPRE manipulacion, no perdida: si el descarte consumiera Seq,
// un borrado seria indistinguible de una perdida legitima y el grabador
// fabricaria evidencia falsa a su favor.
//
// sello puede ser nil. Sin sello, una cadena vacia es INDECIDIBLE y no verde
// (ese es D-20), y el truncamiento de cola NO se detecta: el ancla externa es
// lo unico que lo cierra.
func ValidarCadena(vs []*Verdict, agentID [32]byte, key []byte, sello *Sello) (bool, string, Info) {
	info := Info{N: len(vs)}

	if sello != nil {
		if ok, motivo := validarContraSello(vs, agentID, sello); !ok {
			return false, motivo, info
		}
	} else if len(vs) == 0 {
		return true, "cadena vacia y SIN SELLO: indecidible, no verde. Este es D-20", info
	}

	if len(vs) == 0 {
		return false, "cadena vacia con sello vigente: truncamiento total", info
	}

	for i, v := range vs {
		if v.AgentID != agentID {
			return false, fmt.Sprintf("agent_id ajeno en pos %d", i), info
		}
		if v.Seq != uint64(i)+1 {
			return false, fmt.Sprintf("salto de seq en pos %d (esperaba %d, vino %d)",
				i, i+1, v.Seq), info
		}
		esperado := Cero32
		if i > 0 {
			esperado = vs[i-1].Hash
		}
		if v.PrevHash != esperado {
			return false, fmt.Sprintf("prev_hash roto en seq %d", v.Seq), info
		}
		c := v.Canonical()
		if sha256.Sum256(c[:]) != v.Hash {
			return false, fmt.Sprintf("hash no coincide en seq %d", v.Seq), info
		}
		m := hmac.New(sha256.New, key)
		m.Write(v.Hash[:])
		if !hmac.Equal(m.Sum(nil), v.Signature[:]) {
			return false, fmt.Sprintf("firma invalida en seq %d", v.Seq), info
		}
		if v.DroppedSince != 0 {
			info.HuecosDeclarados = append(info.HuecosDeclarados,
				Hueco{Seq: v.Seq, Perdidos: v.DroppedSince})
			info.Perdidos += v.DroppedSince
		}
	}

	return true, "cadena integra", info
}

// ---------------------------------------------------------------------
// El Sello: el ancla externa
// ---------------------------------------------------------------------

// Hoja calcula la hoja Merkle de un agente. El prefijo 0x00 separa hojas de
// nodos internos (que llevan 0x01): sin ese dominio, un atacante puede pasar
// un nodo interno por hoja y viceversa.
func Hoja(agentID [32]byte, seq uint64, head [32]byte) [32]byte {
	buf := make([]byte, 0, 1+32+8+32)
	buf = append(buf, 0x00)
	buf = append(buf, agentID[:]...)
	var s [8]byte
	binary.BigEndian.PutUint64(s[:], seq)
	buf = append(buf, s[:]...)
	buf = append(buf, head[:]...)
	return sha256.Sum256(buf)
}

// RaizMerkle calcula la raiz. Con cantidad impar de nodos DUPLICA el ultimo,
// igual que el Python. Es la convencion de Bitcoin y tiene una debilidad
// conocida (dos arboles distintos pueden dar la misma raiz); se replica a
// proposito para no divergir del formato, y queda declarado como NO MEDIDO.
func RaizMerkle(hojas [][32]byte) [32]byte {
	if len(hojas) == 0 {
		return Cero32
	}
	n := make([][32]byte, len(hojas))
	copy(n, hojas)
	for len(n) > 1 {
		if len(n)%2 == 1 {
			n = append(n, n[len(n)-1])
		}
		sig := make([][32]byte, 0, len(n)/2)
		for i := 0; i < len(n); i += 2 {
			buf := make([]byte, 0, 1+64)
			buf = append(buf, 0x01)
			buf = append(buf, n[i][:]...)
			buf = append(buf, n[i+1][:]...)
			sig = append(sig, sha256.Sum256(buf))
		}
		n = sig
	}
	return n[0]
}

// Ancla es el punto que el Sello fija para un agente.
type Ancla struct {
	Seq  uint64
	Head [32]byte
}

// Sello es un checkpoint firmado.
//
// DONDE VIVE, y es la mitad del valor: FUERA del alcance del DBA. Archivo
// append-only en otro host, S3 Object Lock o syslog remoto. Un Sello guardado
// en la misma base que los veredictos no ancla NADA: quien puede borrar la
// cadena puede borrar su ancla.
type Sello struct {
	AnchorSeq   uint64
	PrevHash    [32]byte
	CreatedAt   int64
	Instantanea map[[32]byte]Ancla

	MerkleRoot [32]byte
	Hash       [32]byte
	Signature  [32]byte
}

// cuerpo arma los bytes que se hashean. ">QqI" + prevHash + merkleRoot.
func (s *Sello) cuerpo() []byte {
	buf := make([]byte, 0, 8+8+4+32+32)
	var b8 [8]byte
	binary.BigEndian.PutUint64(b8[:], s.AnchorSeq)
	buf = append(buf, b8[:]...)
	binary.BigEndian.PutUint64(b8[:], uint64(s.CreatedAt))
	buf = append(buf, b8[:]...)
	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], uint32(len(s.Instantanea)))
	buf = append(buf, b4[:]...)
	buf = append(buf, s.PrevHash[:]...)
	buf = append(buf, s.MerkleRoot[:]...)
	return buf
}

// hojasOrdenadas replica el `sorted(instantanea.items())` del Python: las hojas
// van ordenadas por agentID. Sin orden determinista la raiz no es reproducible
// y el Sello no verifica dos veces igual.
func (s *Sello) hojasOrdenadas() [][32]byte {
	ids := make([][32]byte, 0, len(s.Instantanea))
	for id := range s.Instantanea {
		ids = append(ids, id)
	}
	// Orden lexicografico por bytes, que es el mismo que usa Python al ordenar
	// bytes objects.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && menor(ids[j], ids[j-1]); j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	hojas := make([][32]byte, 0, len(ids))
	for _, id := range ids {
		a := s.Instantanea[id]
		hojas = append(hojas, Hoja(id, a.Seq, a.Head))
	}
	return hojas
}

func menor(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// NuevoSello construye el checkpoint y lo firma.
func NuevoSello(anchorSeq uint64, prevHash [32]byte, createdAt int64,
	instantanea map[[32]byte]Ancla, keySello []byte) *Sello {
	s := &Sello{
		AnchorSeq:   anchorSeq,
		PrevHash:    prevHash,
		CreatedAt:   createdAt,
		Instantanea: make(map[[32]byte]Ancla, len(instantanea)),
	}
	for k, v := range instantanea {
		s.Instantanea[k] = v
	}
	s.MerkleRoot = RaizMerkle(s.hojasOrdenadas())
	s.Hash = sha256.Sum256(s.cuerpo())
	m := hmac.New(sha256.New, keySello)
	m.Write(s.Hash[:])
	copy(s.Signature[:], m.Sum(nil))
	return s
}

// FirmaValida chequea las TRES cosas, no una: que el hash recompute, que la
// raiz Merkle recompute DESDE la instantanea publicada, y que la firma valga.
// Chequear solo la firma dejaria pasar un Sello con instantanea adulterada y
// raiz vieja.
func (s *Sello) FirmaValida(keySello []byte) bool {
	if sha256.Sum256(s.cuerpo()) != s.Hash {
		return false
	}
	if RaizMerkle(s.hojasOrdenadas()) != s.MerkleRoot {
		return false
	}
	m := hmac.New(sha256.New, keySello)
	m.Write(s.Hash[:])
	return hmac.Equal(m.Sum(nil), s.Signature[:])
}

// keySelloDe permite que ValidarCadena verifique el Sello. Se guarda al
// construirlo para no cambiar la firma de ValidarCadena respecto del Python.
var keySelloActiva []byte

// UsarKeySello fija la clave con la que se verifican los Sellos.
func UsarKeySello(k []byte) { keySelloActiva = k }

// validarContraSello: el sello fija un PUNTO de la cadena.
//
// La cadena PUEDE crecer despues del sello; lo que no puede es encogerse por
// debajo del punto sellado ni cambiar lo que ya quedo sellado. Comparar
// head == ancla daria ROJO FALSO en toda cadena viva que recibio un veredicto
// nuevo despues del checkpoint, y hay un test que lo prueba.
func validarContraSello(vs []*Verdict, agentID [32]byte, sello *Sello) (bool, string) {
	if !sello.FirmaValida(keySelloActiva) {
		return false, "SELLO adulterado (hash, raiz Merkle o firma)"
	}
	a, hay := sello.Instantanea[agentID]
	if !hay {
		return false, "el agente no figura en el sello"
	}
	if uint64(len(vs)) < a.Seq {
		return false, fmt.Sprintf("truncamiento de cola: el sello fija seq %d y la cadena tiene %d",
			a.Seq, len(vs))
	}
	if vs[a.Seq-1].Hash != a.Head {
		return false, fmt.Sprintf("el veredicto seq %d no es el sellado", a.Seq)
	}
	return true, "ok"
}
