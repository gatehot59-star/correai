/* Verificador del anti-replay del gateway de CORREAI.
   Reimplementacion LITERAL de validateTimestamp (gateway/gateway.go).
   Instrumento: C, no Go. Go define la conversion uint64->int64 como
   truncamiento a la misma representacion en dos complementos, identica a C99,
   asi que la aritmetica es la misma; el veredicto sobre el binario Go real
   queda NO MEDIDO hasta que corra en Actions.
   Corrido: 2026-09-07, gcc -O0 -Wall -Wextra. */
#include <stdio.h>
#include <stdint.h>

static int validateTimestamp(uint64_t serverNs, uint64_t packetNs, int64_t maxDriftNs){
    int64_t diff;
    if (packetNs > serverNs) diff = (int64_t)(packetNs - serverNs);
    else                     diff = (int64_t)(serverNs - packetNs);
    return diff <= maxDriftNs;
}

int main(void){
    const uint64_t now   = 1788784000000000000ULL;   /* ~2026-09-07 */
    const int64_t  drift = 30LL*1000*1000*1000;      /* maxTimestampDriftNs */
    struct { const char* n; uint64_t ts; int debe_aceptar; } c[] = {
        {"legitimo now",            now,                    1},
        {"legitimo +10s",           now + 10000000000ULL,   1},
        {"viejo -60s",              now - 60000000000ULL,   0},
        {"futuro +60s",             now + 60000000000ULL,   0},
        {"ATAQUE ts=2^64-1",        0xFFFFFFFFFFFFFFFFULL,  0},
        {"ATAQUE ts=2^63",          0x8000000000000000ULL,  0},
    };
    int cambios = 0, ataque_pasa = 0;
    for (unsigned i=0;i<sizeof(c)/sizeof(c[0]);++i){
        int got = validateTimestamp(now, c[i].ts, drift);
        int64_t diff = c[i].ts > now ? (int64_t)(c[i].ts-now) : (int64_t)(now-c[i].ts);
        printf("  %-18s ts=%20llu int64(diff)=%21lld acepta=%d espera=%d %s\n",
               c[i].n,(unsigned long long)c[i].ts,(long long)diff,got,c[i].debe_aceptar,
               got==c[i].debe_aceptar ? "ok" : "<<< ACEPTA LO QUE NO DEBE");
        if (i < 4 && got != c[i].debe_aceptar) cambios++;      /* invariantes */
        if (i == 4 && got == 1) ataque_pasa = 1;               /* defecto D-01 */
    }
    printf("\n  [%s] INVARIANTE  la ventana de +-30s funciona para ts normales\n",
           cambios? "ROJO":"ok ");
    printf("  [%s] DEFECTO     D-01 int64(diff) desborda a negativo y ts=2^64-1 "
           "PASA la ventana\n", ataque_pasa? "presente":"CAMBIO");
    /* rojo si se rompe un invariante, o si el defecto desaparecio sin
       actualizar la bitacora en el mismo commit */
    int fallos = cambios + (ataque_pasa?0:1);
    printf("\nfallos=%d\n", fallos);
    return fallos?1:0;
}
