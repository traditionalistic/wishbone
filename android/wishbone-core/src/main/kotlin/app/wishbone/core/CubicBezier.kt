package app.wishbone.core

import kotlin.math.abs

/**
 * CSS-style cubic-bezier easing with control points (0,0), (x1,y1), (x2,y2), (1,1). Solver exactly
 * per contract/README.md: Newton from t = x, at most 8 iterations, stop early if |x'(t)| < 1e-6;
 * then bisection on [0, 1], at most 64 iterations, both converging on |Δx| < 1e-12.
 */
object CubicBezier {
    private const val EPSILON = 1e-12
    private const val DERIVATIVE_EPSILON = 1e-6

    private fun sample(t: Double, a1: Double, a2: Double): Double {
        val u = 1 - t
        return 3 * u * u * t * a1 + 3 * u * t * t * a2 + t * t * t
    }

    private fun sampleDerivative(t: Double, a1: Double, a2: Double): Double {
        val u = 1 - t
        return 3 * u * u * a1 + 6 * u * t * (a2 - a1) + 3 * t * t * (1 - a2)
    }

    private fun solveT(x1: Double, x2: Double, x: Double): Double {
        var t = x
        for (i in 0 until 8) {
            val error = sample(t, x1, x2) - x
            if (abs(error) < EPSILON) return t
            val derivative = sampleDerivative(t, x1, x2)
            if (abs(derivative) < DERIVATIVE_EPSILON) break
            t -= error / derivative
        }

        var lo = 0.0
        var hi = 1.0
        t = x.coerceIn(0.0, 1.0)
        for (i in 0 until 64) {
            if (lo >= hi) break
            val current = sample(t, x1, x2)
            if (abs(current - x) < EPSILON) return t
            if (x > current) lo = t else hi = t
            t = (hi - lo) * 0.5 + lo
        }
        return t
    }

    /** `x` is the normalized progress in [0, 1]; returns the eased progress y. */
    fun solve(x1: Double, y1: Double, x2: Double, y2: Double, x: Double): Double {
        val t = solveT(x1, x2, x)
        return sample(t, y1, y2)
    }
}
