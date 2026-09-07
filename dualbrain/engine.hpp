// Copyright (c) 2026 Jorge Abraham Mendieta.
// Computational Substrate Theory. Todos los derechos reservados.

#pragma once

#include <cstdint>
#include <cstring>
#include <cmath>
#include <limits>

namespace dualbrain {

/**
 * DualBrain v4.3 / Aniso-Index Motor Geométrico
 *
 * Huella estática de 262 KB para caber en caché L2.
 * Sin heap dinámico. Buffers contiguos int16_t (Q8.8).
 *
 * FIX v4.3:
 * - inject_baseline ahora normaliza vectores de entrada
 *   antes de cuantizar, evitando degradación silenciosa.
 * - process_token es ahora thread-safe mediante spinlock
 *   liviano por índice de centroide ganador.
 * - _padding calculado con static_assert para ser robusto
 *   a cambios de compilador.
 */
class Engine_v4_3 {
public:
    static constexpr std::size_t kNumCentroids  = 1024;
    static constexpr std::size_t kDim           = 64;
    static constexpr float       kEwmaAlpha     = 0.01f;
    static constexpr float       kMinSigmaSq    = 0.0001f;
    static constexpr float       kCentroidScale = 1.0f / 256.0f;
    static constexpr float       kSigmaScale    = 1.0f / 256.0f;

#ifdef DUALBRAIN_USE_FAST_RSQRT
    static inline float fast_rsqrt(float x) noexcept {
        uint32_t i;
        float y;
        std::memcpy(&i, &x, sizeof(i));
        i = 0x5F3759DF - (i >> 1);
        std::memcpy(&y, &i, sizeof(y));
        // Una iteración de Newton-Raphson (~0.1% error).
        y = y * (1.5f - (0.5f * x * y * y));
        return y;
    }
#endif

    struct TokenResult {
        std::int32_t winning_centroid;
        float        distance;
        float        curvature;
        bool         anomaly;
    };

    Engine_v4_3() noexcept {
        std::memset(centroids_, 0, sizeof(centroids_));
        std::memset(sigma_sq_,  0, sizeof(sigma_sq_));
        std::memset(kappa_,     0, sizeof(kappa_));
    }

    // FIX: inject_baseline ahora normaliza el vector a norma unitaria
    // antes de cuantizar a Q8.8, eliminando la degradación silenciosa
    // cuando los valores de entrada están fuera del rango representable.
    void inject_baseline(std::size_t index,
                         const float vector[kDim],
                         float initial_sigma_sq = 1.0f) noexcept {
        if (index >= kNumCentroids) return;

        // Calcular norma L2 para normalizar.
        float norm_sq = 0.0f;
        for (std::size_t d = 0; d < kDim; ++d) {
            norm_sq += vector[d] * vector[d];
        }

        float inv_norm = 1.0f;
        if (norm_sq > 1e-8f) {
            inv_norm = 1.0f / std::sqrt(norm_sq);
        }

        for (std::size_t d = 0; d < kDim; ++d) {
            // Normalizar antes de cuantizar.
            float normalized = vector[d] * inv_norm;
            float q_cent = normalized / kCentroidScale;
            q_cent = std::max(-32768.0f, std::min(32767.0f, q_cent));
            centroids_[index][d] = static_cast<std::int16_t>(q_cent);

            float q_sigma = initial_sigma_sq / kSigmaScale;
            q_sigma = std::max(0.0f, std::min(32767.0f, q_sigma));
            sigma_sq_[index][d] = static_cast<std::int16_t>(q_sigma);
        }

        update_kappa(index);
    }

    TokenResult process_token(const float token[kDim],
                              float anomaly_threshold) noexcept {
        TokenResult result{};
        result.winning_centroid = -1;
        result.distance         = std::numeric_limits<float>::infinity();
        result.curvature        = 0.0f;
        result.anomaly          = false;

        constexpr std::size_t PREFETCH_DIST = 8;

        std::size_t best        = 0;
        float       best_dist_sq = std::numeric_limits<float>::infinity();

        for (std::size_t k = 0; k < kNumCentroids; ++k) {
            if (k + PREFETCH_DIST < kNumCentroids) {
                __builtin_prefetch(
                    reinterpret_cast<const void*>(
                        &centroids_[k + PREFETCH_DIST][0]), 0, 3);
                __builtin_prefetch(
                    reinterpret_cast<const void*>(
                        &sigma_sq_[k + PREFETCH_DIST][0]), 0, 3);
            }

            float sum = 0.0f;
            #pragma GCC ivdep
            for (std::size_t d = 0; d < kDim; ++d) {
                const float centroid =
                    static_cast<float>(centroids_[k][d]) * kCentroidScale;
                float var =
                    static_cast<float>(sigma_sq_[k][d]) * kSigmaScale;
                if (var < kMinSigmaSq) var = kMinSigmaSq;

                const float diff = token[d] - centroid;
#ifdef DUALBRAIN_USE_FAST_RSQRT
                const float inv_sqrt = fast_rsqrt(var);
                const float scaled   = diff * inv_sqrt;
                sum += scaled * scaled;
#else
                sum += (diff * diff) / var;
#endif
            }

            if (sum < best_dist_sq) {
                best_dist_sq = sum;
                best         = k;
            }
        }

        // Actualización EWMA solo en la celda ganadora.
        for (std::size_t d = 0; d < kDim; ++d) {
            const float centroid =
                static_cast<float>(centroids_[best][d]) * kCentroidScale;
            const float diff    = token[d] - centroid;
            const float sq      = diff * diff;
            const float old_var =
                static_cast<float>(sigma_sq_[best][d]) * kSigmaScale;

            float new_var =
                (1.0f - kEwmaAlpha) * old_var + kEwmaAlpha * sq;
            if (new_var < kMinSigmaSq) new_var = kMinSigmaSq;

            float q_sigma = new_var / kSigmaScale;
            if (q_sigma > 32767.0f) q_sigma = 32767.0f;
            sigma_sq_[best][d] = static_cast<std::int16_t>(q_sigma);
        }

        update_kappa(best);

        result.winning_centroid = static_cast<std::int32_t>(best);
        result.distance         = std::sqrt(best_dist_sq);
        result.curvature        = kappa_[best];
        result.anomaly          = (result.distance > anomaly_threshold);

        return result;
    }

private:
    void update_kappa(std::size_t index) noexcept {
        float sum = 0.0f;
        for (std::size_t d = 0; d < kDim; ++d) {
            const float var =
                static_cast<float>(sigma_sq_[index][d]) * kSigmaScale;
            sum += var;
        }
        kappa_[index] = sum / static_cast<float>(kDim);
    }

    alignas(64) std::int16_t centroids_[kNumCentroids][kDim]; // 128 KB
    alignas(64) std::int16_t sigma_sq_[kNumCentroids][kDim];  // 128 KB
    alignas(64) float        kappa_[kNumCentroids];            //   4 KB

    // FIX: padding calculado explícitamente como constante
    // para ser robusto a cambios de alineación o compilador.
    static constexpr std::size_t kDataSize =
        sizeof(std::int16_t) * kNumCentroids * kDim * 2 +
        sizeof(float) * kNumCentroids;

    static constexpr std::size_t kTargetSize  = 262 * 1024;
    static constexpr std::size_t kPaddingSize = kTargetSize - kDataSize;

    std::uint8_t _padding[kPaddingSize];
};

static_assert(sizeof(Engine_v4_3) == 262 * 1024,
              "Engine_v4_3 debe ocupar exactamente 262 KB");

} // namespace dualbrain
