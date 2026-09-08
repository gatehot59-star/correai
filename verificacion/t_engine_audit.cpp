// Instrumento de auditoria de dualbrain/engine.hpp, blob 53327c6a.
// Verifica D-40, D-41, D-49, D-50 y D-51 del auditor externo.
// Lee campos privados con `#define private public`: es un instrumento de
// medicion, no codigo de produccion.
//
// Compilacion medida (g++ 12.2.0, Debian):
//   g++ -std=c++17 -fsyntax-only engine.hpp        -> exit 0  (D-50 NO reproduce)
//   g++ -std=c++17 -O2 -o t_engine_audit t_engine_audit.cpp -> exit 0
//
// Defectos propios corregidos durante la construccion de este instrumento:
//   1. El primer control positivo usaba `static Engine_v4_3` dentro de la
//      funcion, compartiendo estado entre amplitudes, y llamaba run_amp DOS
//      veces por linea. Un control contaminado no es un control.
//   2. El primer control (amplitud 1.0) NO PODIA dar verde: el ruido nunca
//      superaba el umbral de escape. Un instrumento que solo puede dar rojo
//      no mide nada.
#define private public
#include "engine.hpp"
#undef private
#include <cstdio>
#include <cmath>

using dualbrain::Engine_v4_3;

static int run_amp(float amp) {
    Engine_v4_3* e = new Engine_v4_3();
    float base[64];
    for (int d = 0; d < 64; ++d) base[d] = 0.125f;
    e->inject_baseline(0, base, 1.0f);
    unsigned int seed = 999u;
    for (int it = 1; it <= 4000; ++it) {
        float tok[64];
        for (int d = 0; d < 64; ++d) {
            seed = seed * 1103515245u + 12345u;
            float u = (float)((seed >> 16) & 0x7FFF) / 32767.0f;
            tok[d] = 0.125f + (u - 0.5f) * amp;
        }
        e->process_token(tok, 1e9f);
    }
    int v = (int)e->sigma_sq_[0][0];
    delete e;
    return v;
}

int main() {
    printf("== D-51: huella real reportada por el compilador ==\n");
    printf("sizeof(Engine_v4_3)    = %zu bytes\n", sizeof(Engine_v4_3));
    printf("datos utiles (sin pad) = %zu bytes\n", Engine_v4_3::kDataSize);
    printf("L2 de 256 KiB          = %d bytes\n", 256 * 1024);
    printf("cabe en 256 KiB?       = %s\n",
           Engine_v4_3::kDataSize <= 256u * 1024u ? "SI" : "NO");
    printf("kSigmaScale            = %.8f\n", Engine_v4_3::kSigmaScale);
    printf("kMinSigmaSq            = %.8f\n", Engine_v4_3::kMinSigmaSq);
    printf("kMinSigmaSq cuantizado = %.6f -> int16 %d\n",
           Engine_v4_3::kMinSigmaSq / Engine_v4_3::kSigmaScale,
           (int)(Engine_v4_3::kMinSigmaSq / Engine_v4_3::kSigmaScale));

    printf("\n== D-41: colapso de varianza en dimension estable ==\n");
    Engine_v4_3* eng = new Engine_v4_3();
    float base[64];
    for (int d = 0; d < 64; ++d) base[d] = 0.125f;
    eng->inject_baseline(0, base, 1.0f);
    printf("tras inject: sigma_crudo=%d kappa=%.8f\n",
           (int)eng->sigma_sq_[0][0], eng->kappa_[0]);
    float tok[64];
    for (int d = 0; d < 64; ++d) tok[d] = 0.125f;
    int cps[] = {1, 10, 50, 100, 300, 600, 1200, 2000};
    int ci = 0;
    for (int it = 1; it <= 2000; ++it) {
        Engine_v4_3::TokenResult r = eng->process_token(tok, 5.0f);
        if (ci < 8 && it == cps[ci]) {
            printf("it=%5d sigma_crudo=%6d var_efectiva=%.8f kappa=%.8f dist=%.6f\n",
                   it, (int)eng->sigma_sq_[0][0],
                   (float)eng->sigma_sq_[0][0] * Engine_v4_3::kSigmaScale,
                   r.curvature, r.distance);
            ++ci;
        }
    }
    printf("VEREDICTO D-41: %s\n",
           eng->sigma_sq_[0][0] == 0 ? "COLAPSO A 0 (reproducido)" : "sobrevivio");
    delete eng;

    printf("\n== D-41b: el cero es estado ABSORBENTE, no piso ==\n");
    printf("escape teorico: kEwmaAlpha*sq >= kSigmaScale -> sq >= %.6f\n",
           Engine_v4_3::kSigmaScale / Engine_v4_3::kEwmaAlpha);
    float amps[] = {0.5f, 1.0f, 1.5f, 2.0f, 4.0f, 8.0f};
    for (int i = 0; i < 6; ++i) {
        int v = run_amp(amps[i]);
        printf("amplitud %5.2f sq_max~%8.4f sigma_final=%6d -> %s\n",
               amps[i], (amps[i] / 2) * (amps[i] / 2), v,
               v > 0 ? "VIVA (control puede dar verde)" : "CERO (absorbente)");
    }

    printf("\n== D-41c: amplificacion de la distancia ==\n");
    float amp_min = (0.01f * 0.01f) / Engine_v4_3::kMinSigmaSq;
    float amp_ok  = (0.01f * 0.01f) / Engine_v4_3::kSigmaScale;
    printf("perturbacion 0.01 con var colapsada aporta %.4f\n", amp_min);
    printf("con la varianza minima representable aporta %.4f (factor %.1fx)\n",
           amp_ok, amp_min / amp_ok);

    printf("\n== D-49: process_token normaliza el token? ==\n");
    Engine_v4_3* e3 = new Engine_v4_3();
    Engine_v4_3* e4 = new Engine_v4_3();
    e3->inject_baseline(0, base, 1.0f);
    e4->inject_baseline(0, base, 1.0f);
    float t1[64], t2[64];
    for (int d = 0; d < 64; ++d) { t1[d] = 0.125f; t2[d] = 0.250f; }
    Engine_v4_3::TokenResult r1 = e3->process_token(t1, 1e9f);
    Engine_v4_3::TokenResult r2 = e4->process_token(t2, 1e9f);
    printf("dist(norma N)=%.6f dist(norma 2N)=%.6f -> %s\n",
           r1.distance, r2.distance,
           std::fabs(r1.distance - r2.distance) > 1e-4f
             ? "la norma CAMBIA la distancia (D-49 confirmado)"
             : "invariante (D-49 refutado)");
    delete e3; delete e4;
    return 0;
}
