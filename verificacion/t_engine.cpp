// Verificador del motor DualBrain de KAMPE IR (antes CORREAI).
// Mide dos cosas distintas y las reporta separadas:
//   INVARIANTES  : lo que debe seguir siendo cierto. Si se rompe, rojo.
//   DEFECTOS     : estado medido hoy. Si cambia, rojo, para forzar
//                  actualizar la bitacora en el mismo commit que arregla.
// Corrido: 2026-09-07, g++ (Debian 12.2.0) -std=c++17 -O2 -Wall -Wextra.
#include "../dualbrain/engine.hpp"
#include <cstdio>
#include <cmath>

static int fallos = 0;
static void invariante(const char* n, bool ok){
    printf("  [%s] INVARIANTE  %s\n", ok?"ok ":"ROJO", n);
    if(!ok) fallos++;
}
static void defecto(const char* n, bool sigue_presente){
    printf("  [%s] DEFECTO     %s\n", sigue_presente?"presente":"CAMBIO", n);
    if(!sigue_presente) fallos++;   // si se arreglo, hay que tocar la bitacora
}

int main(){
    const std::size_t sz = sizeof(dualbrain::Engine_v4_3);
    printf("sizeof(Engine_v4_3) = %zu bytes = %.2f KiB\n\n", sz, sz/1024.0);

    // ---------- A: baseline de norma 1 (lo que inject_baseline asume) ----------
    static dualbrain::Engine_v4_3 e;
    float v[64]; for(int i=0;i<64;i++) v[i]=1.0f/8.0f;      // ||v|| = 1 exacto
    e.inject_baseline(0, v, 1.0f);
    auto A = e.process_token(v, 3.0f);
    printf("A) ||v||=1   winner=%d distance=%.6f curvature=%.6f anomaly=%d\n",
           A.winning_centroid, A.distance, A.curvature, (int)A.anomaly);

    // ---------- B: MISMA direccion, norma 10 ----------
    static dualbrain::Engine_v4_3 e2;
    float w[64]; for(int i=0;i<64;i++) w[i]=10.0f/8.0f;     // ||w|| = 10
    e2.inject_baseline(0, w, 1.0f);
    auto B = e2.process_token(w, 3.0f);
    printf("B) ||w||=10  winner=%d distance=%.6f curvature=%.6f anomaly=%d"
           "   <-- token IDENTICO a su propio baseline\n",
           B.winning_centroid, B.distance, B.curvature, (int)B.anomaly);

    // ---------- C: control positivo, vector ortogonal al baseline de e ----------
    float z[64]; for(int i=0;i<64;i++) z[i]=(i%2? 1.0f/8.0f : -1.0f/8.0f); // z.v = 0
    auto C = e.process_token(z, 3.0f);
    printf("C) control positivo, z ortogonal a v: distance=%.6f anomaly=%d\n\n",
           C.distance, (int)C.anomaly);

    invariante("huella estatica == 262 KiB (268288 B)", sz == 262u*1024u);
    invariante("baseline de norma 1 reconoce su propio token (d < 1e-3)", A.distance < 1e-3f);
    invariante("control positivo: token distinto da mas distancia que el propio", C.distance > A.distance);

    defecto("D-03 process_token no normaliza: baseline de norma 10 marca "
            "ANOMALO su propio token (d=9 > umbral 3)", B.anomaly && B.distance > 3.0f);
    defecto("D-03b el umbral no es invariante de escala: d(identico,||10||) > "
            "d(ortogonal,||1||)", B.distance > C.distance);

    printf("\nfallos=%d\n", fallos);
    return fallos ? 1 : 0;
}
