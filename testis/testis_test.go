// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

package testis

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// Claves FIJAS, iguales a las del Python. Con claves aleatorias la evidencia
// no es recomputable y un auditor no puede rehacer los hashes de una corrida
// commiteada.
var (
	kVerdict = seq32(0)  // bytes(range(32))
	kSello   = seq32(32) // bytes(range(32, 64))
	a1       = padID("agente-uno")
	a2       = padID("agente-dos")
)

func seq32(desde int) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(desde + i)
	}
	return b
}

func padID(s string) [32]byte {
	var id [32]byte
	copy(id[:], s)
	return id
}

func hx(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("hex golden invalido: %v", err)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

// construir replica verificacion/t_testis.py:construir(). Si esta funcion se
// desvia del Python, los vectores golden de abajo se ponen rojos.
func construir(agentID [32]byte, n int, descartes map[int]uint32, consumirSeq bool) []*Verdict {
	vs := make([]*Verdict, 0, n)
	prev := Cero32
	seq := uint64(1)
	for i := 1; i <= n; i++ {
		perdidos := descartes[i]
		if perdidos != 0 && consumirSeq {
			seq += uint64(perdidos) // politica MALA de la spec 6.2
			perdidos = 0
		}
		rule := uint8(200)
		if i%7 == 0 {
			rule = 9
		}
		var ctx [ContextSize]float64
		for k := range ctx {
			ctx[k] = float64(i) / 8.0
		}
		v := &Verdict{
			Seq: seq, PrevHash: prev, AgentID: agentID,
			ObservedAt: 1_788_800_000_000_000_000 + int64(i)*1_000_000,
			Rule:       rule,
			PacketTS:   1_788_800_000_000_000_000 + uint64(i)*900_000,
			Epoch:      1, Nonce: uint32(i), PayloadLen: uint32(512 + i),
			Context: ctx, BackoffNS: 100_000_000 * int64(1+i%5),
			Streak: int32(i % 11), DroppedSince: perdidos,
		}
		v.Sellar(kVerdict)
		vs = append(vs, v)
		prev = v.Hash
		seq++
	}
	return vs
}

func selloDe(cadenas map[[32]byte][]*Verdict, anchorSeq uint64, prev [32]byte) *Sello {
	inst := make(map[[32]byte]Ancla, len(cadenas))
	for aid, vs := range cadenas {
		inst[aid] = Ancla{Seq: uint64(len(vs)), Head: vs[len(vs)-1].Hash}
	}
	return NuevoSello(anchorSeq, prev, 1_788_800_100_000_000_000, inst, kSello)
}

func init() { UsarKeySello(kSello) }

// =====================================================================
// EL TEST QUE DECIDE SI ESTE PUERTO VALE: los vectores golden del Python.
//
// Un test que solo compara Go contra Go probaria que Go es consistente consigo
// mismo, y eso ya lo sabemos. Estos hashes los calculo CPython 3.12 con
// struct.pack, y estan pegados como constantes: si el canonico de Go difiere en
// UN byte, el sha256 cambia entero y este test cae. Es el unico control que
// distingue "porte correcto" de "reimplementacion parecida".
// =====================================================================

func TestGolden_CanonicoIdenticoAlPython(t *testing.T) {
	vs := construir(a1, 1, nil, false)
	v := vs[0]

	if got := len(v.Canonical()); got != 181 {
		t.Fatalf("el canonico mide %d B, el Python mide 181", got)
	}

	// struct.calcsize(">Q32s32sqBQIII8dqiI") == 181, medido en CPython 3.12.
	golden := hx(t, "678fcd4b0384fe96981cd52401f6eda382eb414647a1a909178294f1bc23e8f5")
	if v.Hash != golden {
		t.Fatalf("HASH DISTINTO DEL PYTHON\n  go     = %x\n  python = %x", v.Hash, golden)
	}

	goldenSig := hx(t, "55d9a0cb6027e2f69b5d3f2893082655b051babfb9e6d7894894dd712cd26aed")
	if v.Signature != goldenSig {
		t.Fatalf("FIRMA DISTINTA DEL PYTHON\n  go     = %x\n  python = %x", v.Signature, goldenSig)
	}
}

func TestGolden_PacketTSAlMaximo(t *testing.T) {
	// packet_ts = 2^64-1. Es el caso que obliga a numeric(20) en la columna, no
	// bigint: un int64 con signo no lo representa.
	v := &Verdict{Seq: 1, PrevHash: Cero32, AgentID: a1, ObservedAt: 1, Rule: 5,
		PacketTS: math.MaxUint64, Epoch: 1, Nonce: 1, PayloadLen: 1}
	v.Sellar(kVerdict)
	golden := hx(t, "ea11173e8036842873e2cecc498010e19b041ee6e53e6a0b059b33b95bc2f760")
	if v.Hash != golden {
		t.Fatalf("packet_ts=2^64-1 da hash distinto\n  go     = %x\n  python = %x", v.Hash, golden)
	}
}

func TestGolden_HojaYRaizMerkle(t *testing.T) {
	h := Hoja(a1, 1, Cero32)
	if g := hx(t, "3cd1ed0f4a41c9ae2b5691aa9c0cd923c8532818430844f44226a9253567570f"); h != g {
		t.Fatalf("hoja distinta\n  go     = %x\n  python = %x", h, g)
	}
	// Tres hojas: ejercita la duplicacion del ultimo nodo con cantidad impar.
	r := RaizMerkle([][32]byte{Hoja(a1, 1, Cero32), Hoja(a1, 2, Cero32), Hoja(a1, 3, Cero32)})
	if g := hx(t, "32c203487271fe7344a21ee3999e87fe41c3ad916ac4294733aa19b715924637"); r != g {
		t.Fatalf("raiz con 3 hojas distinta\n  go     = %x\n  python = %x", r, g)
	}
}

func TestGolden_SelloHashYFirma(t *testing.T) {
	s := NuevoSello(1, Cero32, 1_788_800_100_000_000_000,
		map[[32]byte]Ancla{a1: {Seq: 1, Head: Cero32}}, kSello)
	if g := hx(t, "7fedeaf1fd1f56cc9bb2ab224c709048974ff72b1839c9ab3f6a8d7106fdcafa"); s.Hash != g {
		t.Fatalf("hash del sello distinto\n  go     = %x\n  python = %x", s.Hash, g)
	}
	if g := hx(t, "6bbbbf4caf9ce01c122f973d3fdab0decf83e877b727908f5ea2d0e085d84466"); s.Signature != g {
		t.Fatalf("firma del sello distinta\n  go     = %x\n  python = %x", s.Signature, g)
	}
	if !s.FirmaValida(kSello) {
		t.Fatal("el sello recien construido no se valida a si mismo")
	}
}

// =====================================================================
// INVARIANTES
// =====================================================================

func TestCadenaLimpiaVerifica(t *testing.T) {
	base := construir(a1, 100, nil, false)
	ok, motivo, info := ValidarCadena(base, a1, kVerdict, nil)
	if !ok {
		t.Fatalf("cadena integra de 100 dio ROJO: %s", motivo)
	}
	if info.N != 100 {
		t.Fatalf("info.N = %d, esperaba 100", info.N)
	}
	if base[0].PrevHash != Cero32 {
		t.Fatal("el primer prev_hash no son 32 ceros")
	}
	// Ancho fijo REAL, no declarado: los 100 miden lo mismo.
	for i, v := range base {
		if len(v.Canonical()) != CanonicalLen {
			t.Fatalf("canonico de largo distinto en pos %d", i)
		}
	}
}

func TestSelloNoDaFalsoPositivo(t *testing.T) {
	base := construir(a1, 100, nil, false)
	s := selloDe(map[[32]byte][]*Verdict{a1: base, a2: construir(a2, 40, nil, false)}, 1, Cero32)
	if ok, motivo, _ := ValidarCadena(base, a1, kVerdict, s); !ok {
		t.Fatalf("el Sello rechazo la cadena que sello: %s", motivo)
	}
}

func TestSelloPermiteCrecimiento(t *testing.T) {
	// La regla implementada fija un PUNTO, no compara heads. Este test es el
	// guard de regresion: si alguien "simplifica" a head == ancla, toda cadena
	// viva entre dos checkpoints daria manipulacion.
	base := construir(a1, 100, nil, false)
	s := selloDe(map[[32]byte][]*Verdict{a1: base, a2: construir(a2, 40, nil, false)}, 1, Cero32)

	nuevo := &Verdict{Seq: 101, PrevHash: base[len(base)-1].Hash, AgentID: a1,
		ObservedAt: 1_788_800_200_000_000_000, Rule: 7, PacketTS: 1, Epoch: 1,
		Nonce: 101, PayloadLen: 9, BackoffNS: 200_000_000}
	for i := range nuevo.Context {
		nuevo.Context[i] = 0.5
	}
	nuevo.Sellar(kVerdict)
	crecida := append(append([]*Verdict{}, base...), nuevo)

	ok, motivo, _ := ValidarCadena(crecida, a1, kVerdict, s)
	if !ok {
		t.Fatalf("el Sello rechazo crecimiento legitimo: %s", motivo)
	}
	// Y la regla INGENUA tiene que fallar sobre la misma cadena sana. Sin este
	// contraste, el test de arriba no prueba que la regla implementada sea
	// mejor: prueba solo que no explota.
	ancla := s.Instantanea[a1]
	if crecida[len(crecida)-1].Hash == ancla.Head {
		t.Fatal("la regla ingenua deberia diferir aca; el caso no discrimina")
	}
}

func TestCanonicoNoColisionaAlCorrerLaFrontera(t *testing.T) {
	// El ataque de D-06 aplicado al canonico REAL, no a un helper de juguete:
	// se corre la frontera entre dos campos contiguos (agentID y prevHash).
	// Con un canonico naive de "valores unidos por |" los dos darian lo mismo.
	b1 := &Verdict{Seq: 1, PrevHash: padID("c"), AgentID: padID("a|b"), ObservedAt: 1, Rule: 7}
	b2 := &Verdict{Seq: 1, PrevHash: padID("b|c"), AgentID: padID("a"), ObservedAt: 1, Rule: 7}
	b1.Sellar(kVerdict)
	b2.Sellar(kVerdict)
	if b1.Hash == b2.Hash {
		t.Fatal("COLISION del canonico real al correr la frontera")
	}

	// Control del propio control: el canonico naive SI tiene que colisionar. Si
	// no colisiona, el test de arriba no prueba nada.
	naive := func(campos ...string) string {
		out := ""
		for i, c := range campos {
			if i > 0 {
				out += "|"
			}
			out += c
		}
		return out
	}
	if naive("a|b", "c") != naive("a", "b|c") {
		t.Fatal("el canonico naive NO colisiona: el control no discrimina")
	}
}

// =====================================================================
// CONTROLES POSITIVOS: cada uno TIENE que dar ROJO.
// Un validador que nunca dice no, no valida.
// =====================================================================

func TestControlesPositivos(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func() ([]*Verdict, *Sello)
	}{
		{"borrar el veredicto 50 (del medio)", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			return append(append([]*Verdict{}, b[:49]...), b[50:]...), nil
		}},
		{"voltear 1 bit del Context del veredicto 50", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			bits := math.Float64bits(b[49].Context[3]) ^ 1
			b[49].Context[3] = math.Float64frombits(bits)
			return b, nil
		}},
		{"reordenar los veredictos 50 y 51", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			b[49], b[50] = b[50], b[49]
			return b, nil
		}},
		{"refirmar el veredicto 71 con otra clave HMAC", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			k := append([]byte("clave-del-atacante"), make([]byte, 14)...)
			m := hmac.New(sha256.New, k)
			m.Write(b[70].Hash[:])
			copy(b[70].Signature[:], m.Sum(nil))
			return b, nil
		}},
		// CP-5, el que salio de la matriz de mutacion (E-006). Sin este, apagar
		// el chequeo de prev_hash dejaba el script en verde: el control de seq
		// agarraba el borrado y el reordenamiento primero, o sea que la cadena
		// de hash, que ES el producto, estaba sin probar. El ataque que lo aisla
		// es el que haria un atacante competente: borra el 50 y RENUMERA, asi
		// seq queda contiguo 1..99 y lo unico roto es el eslabon. Renumerar es
		// un UPDATE.
		{"borrar el 50 y RENUMERAR: solo se rompe el eslabon de hash", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			renum := make([]*Verdict, 0, 99)
			j := uint64(1)
			for k, v := range b {
				if k == 49 {
					continue
				}
				w := *v
				w.Seq = j
				// El atacante NO recalcula el eslabon: reusa el prev_hash viejo.
				w.PrevHash = v.PrevHash
				w.Sellar(kVerdict)
				renum = append(renum, &w)
				j++
			}
			return renum, nil
		}},
		{"borrar los ULTIMOS 10 (truncamiento de cola), con Sello", func() ([]*Verdict, *Sello) {
			s := selloDe(map[[32]byte][]*Verdict{
				a1: construir(a1, 100, nil, false),
				a2: construir(a2, 40, nil, false)}, 1, Cero32)
			return construir(a1, 100, nil, false)[:90], s
		}},
		{"borrar la cadena ENTERA del agente, con Sello", func() ([]*Verdict, *Sello) {
			s := selloDe(map[[32]byte][]*Verdict{
				a1: construir(a1, 100, nil, false),
				a2: construir(a2, 40, nil, false)}, 1, Cero32)
			return nil, s
		}},
		{"adulterar la raiz Merkle del Sello", func() ([]*Verdict, *Sello) {
			b := construir(a1, 100, nil, false)
			s := selloDe(map[[32]byte][]*Verdict{a1: b,
				a2: construir(a2, 40, nil, false)}, 1, Cero32)
			for i := range s.MerkleRoot {
				s.MerkleRoot[i] ^= 1
			}
			return b, s
		}},
		{"truncar por DEBAJO del punto sellado (seq 99 < 100)", func() ([]*Verdict, *Sello) {
			larga := construir(a1, 120, nil, false)
			s := NuevoSello(1, Cero32, 1_788_800_100_000_000_000, map[[32]byte]Ancla{
				a1: {Seq: 100, Head: larga[99].Hash},
				a2: {Seq: 40, Head: construir(a2, 40, nil, false)[39].Hash},
			}, kSello)
			return larga[:99], s
		}},
		{"cadena de OTRO agente presentada como propia", func() ([]*Verdict, *Sello) {
			return construir(a2, 10, nil, false), nil
		}},
		{"descarte que CONSUME seq (politica mala de la spec 6.2)", func() ([]*Verdict, *Sello) {
			return construir(a1, 30, map[int]uint32{21: 2}, true), nil
		}},
	}

	for _, c := range casos {
		vs, s := c.armar()
		ok, motivo, _ := ValidarCadena(vs, a1, kVerdict, s)
		if ok {
			t.Errorf("CP NO RECHAZO: %s  -> el validador dijo VERDE. No sirve.", c.nombre)
			continue
		}
		t.Logf("CP ok (rechazo): %-58s motivo: %s", c.nombre, motivo)
	}
}

// =====================================================================
// LO QUE EL VALIDADOR NO CUBRE, Y QUEDA MEDIDO COMO PROPIEDAD
// =====================================================================

func TestD20_SinSelloElTruncamientoDeColaPasaEnVerde(t *testing.T) {
	// D-20 no es un bug del validador: es el limite de una cadena sin ancla.
	// Este test AFIRMA el limite, para que si alguien "arregla" el validador
	// sin agregar ancla, se entere de que no alcanza.
	cola := construir(a1, 100, nil, false)[:90]
	ok, _, _ := ValidarCadena(cola, a1, kVerdict, nil)
	if !ok {
		t.Fatal("D-20 cambio: sin Sello el truncamiento de cola ahora se detecta. " +
			"Si es real, hay que actualizar este test y el Python.")
	}
	t.Log("D-20 reproducido: sin Sello, borrar los ultimos 10 verifica en VERDE")
}

func TestD28_ElSelloNoCubreLoPosteriorAlCheckpoint(t *testing.T) {
	// La ventana de exposicion es exactamente T, el intervalo de sellado, y T
	// es un parametro de RIESGO del producto, no de performance.
	larga := construir(a1, 120, nil, false)
	s := NuevoSello(1, Cero32, 1_788_800_100_000_000_000, map[[32]byte]Ancla{
		a1: {Seq: 100, Head: larga[99].Hash},
		a2: {Seq: 40, Head: construir(a2, 40, nil, false)[39].Hash},
	}, kSello)
	truncada := larga[:105] // el atacante borra 15 de los 20 posteriores
	ok, motivo, _ := ValidarCadena(truncada, a1, kVerdict, s)
	if !ok {
		t.Fatalf("D-28 cambio: el Sello ahora cubre lo posterior (%s)", motivo)
	}
	t.Log("D-28 medido: sello en seq 100, cadena en 120, atacante trunca a 105 y da VERDE")
}

func TestD21_ElHuecoDeclaradoNoRompeLaCadena(t *testing.T) {
	buena := construir(a1, 30, map[int]uint32{21: 2}, false)
	ok, motivo, info := ValidarCadena(buena, a1, kVerdict, nil)
	if !ok {
		t.Fatalf("el descarte declarado dio ROJO: %s", motivo)
	}
	if len(info.HuecosDeclarados) != 1 || info.HuecosDeclarados[0].Seq != 21 ||
		info.HuecosDeclarados[0].Perdidos != 2 {
		t.Fatalf("el hueco no quedo declarado: %+v", info.HuecosDeclarados)
	}
	if info.Perdidos != 2 {
		t.Fatalf("perdidos = %d, esperaba 2", info.Perdidos)
	}
}

func TestD25_HMACNoEsNoRepudio(t *testing.T) {
	// Propiedad de HMAC, no defecto del codigo: quien tiene la clave refabrica
	// la cadena entera y verifica en verde. Es tamper-EVIDENCE, no no-repudio
	// frente a terceros. Cerrarlo pide ed25519, y eso es una decision de
	// producto que este puerto NO toma.
	refabricada := construir(a1, 100, nil, false)
	ok, _, _ := ValidarCadena(refabricada, a1, kVerdict, nil)
	if !ok {
		t.Fatal("la cadena refabricada no verifico: el modelo de amenaza cambio")
	}
	t.Log("D-25 medido: con la clave se refabrica la cadena y verifica en VERDE")
}

// =====================================================================
// GUARD DEL PROPIO CANONICO: el panic de longitud es alcanzable?
// =====================================================================

func TestCanonicoEscribeExactamente181(t *testing.T) {
	// Recomputa el largo desde los tamanos de cada campo, en vez de repetir la
	// constante: si alguien cambia un tipo, este numero se mueve y el test cae
	// aunque CanonicalLen siga diciendo 181.
	suma := 8 + 32 + 32 + 8 + 1 + 8 + 4 + 4 + 4 + (ContextSize * 8) + 8 + 4 + 4
	if suma != CanonicalLen {
		t.Fatalf("la suma de campos da %d y CanonicalLen dice %d", suma, CanonicalLen)
	}
	// Y que el canonico no sea todo ceros por un bug de offsets.
	v := construir(a1, 1, nil, false)[0]
	c := v.Canonical()
	if bytes.Equal(c[:], make([]byte, CanonicalLen)) {
		t.Fatal("el canonico salio todo ceros")
	}
	// El seq va en los primeros 8 bytes, big-endian.
	if binary.BigEndian.Uint64(c[0:8]) != 1 {
		t.Fatal("el seq no quedo en los primeros 8 bytes big-endian")
	}
}
