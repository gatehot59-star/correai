# 2026-09-07 · 02 · Bautismo: KAMPE IR

## 1. Pedido

Dos mensajes. Primero "Bautizado. KAMPE IR", con la jerarquia oficial del
ecosistema y los cuatro subsistemas con sus roles. Despues, explicito:
"KAMPE IR ES EL NUEVO NOMBRE DE COORREAI".

## 2. Herramientas declaradas

| Herramienta | Escribio | Cuota ajena |
| --- | --- | --- |
| busqueda web (mitologia + colision de marca) | no | no |
| GitHub API: `push_files` x1, `delete_file` x1 | si, este repo | no |
| ClickUp: creacion de un Doc publico | si | no |

**No hay tool de rename de repositorio** en el conjunto que tengo. Verificado
buscando en el catalogo: existen `create_repository`, `create_or_update_file`,
`delete_file`, `push_files`, `fork_repository`, y ninguna que edite settings del
repo. Ese click es de Abraham.

## 3. Lo que el nombre resuelve

En E-002 deje un choque abierto: el `go.mod` decia `correai`, el proto decia
`go_package = "hipersec/gateway/v1"` y los errores decian `hipersec:`. Dos
nombres para el mismo sistema.

La jerarquia oficial lo cierra: **KAMPE IR es el ecosistema matriz, HiperSec es
el Gateway de Contencion Perimetral.** O sea que el proto **nunca estuvo mal**:
nombra el componente. El que estaba mal era mi `go.mod`, y ya esta corregido.

## 4. Lo que se verifico del nombre

Instrumento: fuentes primarias, no memoria del modelo.

> "Zeus waged the war against Cronus and the Titans. They fought for ten years,
> and Earth prophesied victory to Zeus if he should have as allies those who had
> been hurled down to Tartarus. **So he slew their jailoress Campe, and loosed
> their bonds.**"
> — Pseudo-Apolodoro, *Biblioteca* 1.2.1

Confirmado de forma independiente en Nonno (*Dionisiaca* 18.237 ss.), Diodoro
Siculo 3.72, el *Dictionary of Greek and Roman Biography* de Smith, y la entrada
de Perseus ("Campe, gaoleress of the Titans"). Traduccion del nombre: "torcida,
curvada", de *kampsos / kamptô*.

**Kampe existe en las fuentes unicamente como la carcelera.** No tiene origen
propio, ni aventuras, ni simbolismo separado de la custodia. Para un producto
cuyo unico trabajo es contener, eso es una puntada exacta: el nombre **es** la
funcion, no una decoracion mitologica pegada arriba.

Y encaja con tu propio protocolo: TITAN pide medir; Kampe es la guardiana de la
Titanomaquia. No lo elegiste por eso, pero cierra.

## 5. Los cuatro problemas del nombre, que no te los voy a esconder

1. **Kampe pierde.** Zeus la mata y **esa muerte es lo que habilita la victoria
   olimpica**. Es la guardiana cuyo fallo es el nudo de la trama. Si le vendes
   "contencion absoluta" a un CISO que leyo a Apolodoro, el nombre dice "la
   contencion que fue sorteada". Es un riesgo chico y real.
2. **La designa Kronos**, el regimen que cae.
3. El sustantivo comun κάμπη, segun el Brill, es **"larva, oruga, gusano de
   seda"**. "La Torcida" y "La Oruga" son lecturas disponibles para quien sepa
   griego.
4. **"IR" no es fonetica libre: ya tiene dueno.** En el mercado de seguridad,
   IR = **Incident Response**, y es una **categoria de producto** con
   expectativas concretas: triage, case management, forense, playbooks,
   timeline. KAMPE IR, tal como esta construido, hace **prevencion, contencion y
   no-repudio**. Lo unico IR-adyacente que existe es la alerta de deriva kappa
   publicada por MQTT. Vender IR es prometer un modulo que no existe, y en B2B
   de seguridad eso se descubre en la demo tecnica, no en la venta. Aparte,
   fuera de seguridad "IR" es la abreviatura estandar de *Investor Relations*:
   buscar "KAMPE IR" en Google va a competir con paginas de accionistas.

Ninguno de los cuatro me parece motivo para cambiarlo. El 4 si me parece motivo
para **decidir por escrito** que significa IR antes del primer pitch, porque de
las tres lecturas que propusiste, una promete producto.

## 6. El hallazgo del mito que va a favor

Lo que Zeus hace **despues** de matarla: no repone una guardiana monstruosa
unica. Pone a los **Hecatonquiros**, ex prisioneros convertidos en guardianes.
Guardia distribuida, con interes alineado, en lugar de un unico cuello de
botella obediente.

Eso describe mejor el `AgentState` por certificado de HiperSec (un FSM, un
Huber y un Coherence **por agente**) que un gateway como punto unico de paso. Y
sugiere donde esta la ventaja arquitectonica real del sistema: en el estado por
agente, no en el perimetro. El perimetro lo tiene cualquiera.

## 7. Archivos generados en este mismo commit

- `respuestas/2026-09-07-02-bautismo-kampe-ir.md` (este)
- `README.md` (reescrito con KAMPE IR y los cuatro roles oficiales)
- `CONTEXTO-KAMPE-IR.md` (nuevo, reemplaza al de CORREAI)
- `02-BITACORA.md` (entrada E-003; E-001 y E-002 **intactas**)
- `go.mod` (module `github.com/gatehot59-star/kampe-ir`)
- `verificacion/t_engine.cpp`, `t_ts.c`, `t_custos.py`, `t_fleet.py`
  (solo encabezados: decian CORREAI)

Borrado en un commit aparte: `CONTEXTO-CORREAI.md`.

**No toque** los 9 archivos de codigo: el rename del producto no cambia una
linea de logica, y tocarlos habria invalidado los blob SHA verificados en E-002
sin ganar nada.

## 8. NO MEDIDO

1. **Si el CI acepta el `go.mod` nuevo.** El path `kampe-ir` no resuelve hasta
   que Abraham renombre el slug. No deberia romper `go build ./...` porque no
   hay dependencias externas ni imports remotos, pero **no hay Go en el sandbox
   para comprobarlo** y el resultado del CI no fue leido.
2. **La marca.** Una busqueda de texto no encontro ninguna empresa de seguridad
   llamada Kampe. Eso **no es** una busqueda de marca: no consulte USPTO, EUIPO
   ni INPI, que es donde vive la respuesta. Antes de vender licencias, esto lo
   mira un abogado, no yo.
3. **El dominio.** No verifique si `kampe.ir`, `kampeir.com` o similares estan
   libres. Nota aparte: `.ir` es el TLD de Iran, asi que un dominio
   `kampe.ir` para un producto de seguridad B2B occidental trae friccion de
   sanciones y de percepcion. No lo mediste ni lo pediste; lo dejo anotado.
4. Todo lo de E-002 sigue abierto: el gateway sin compilar, el data race, el SQL
   sin Postgres, cero integracion mTLS.
