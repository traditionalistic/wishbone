package app.wishbone.core

import kotlin.math.floor

/**
 * Pure timeline sampling per contract/README.md "Семантика сэмплинга таймлайна (нормативно)".
 * `opacity`/`scale` compose with node props by multiplication, `offsetX`/`offsetY`/`rotation` by
 * addition — that composition happens in the renderer, not here; this just samples raw track
 * values.
 */
object TimelineSampler {

    /** `target.prop` -> sampled value, for every track, at time [tMs] for repeat-instance [index]. */
    fun sample(timeline: Timeline, tMs: Double, index: Int): Map<String, Double> {
        val result = LinkedHashMap<String, Double>(timeline.tracks.size)
        for (track in timeline.tracks) {
            result["${track.target}.${track.prop}"] = sampleTrack(timeline, track, tMs, index)
        }
        return result
    }

    private fun sampleTrack(timeline: Timeline, track: Track, tMs: Double, index: Int): Double {
        val keys = track.keys
        val d = timeline.duration
        val u = tMs - track.stagger * index

        if (d == 0.0) return keys.last().v
        if (u < 0.0) return keys.first().v

        val c = floor(u / d)
        var ph = u - c * d

        if (timeline.repeat != -1 && c >= timeline.repeat) {
            ph = if (timeline.autoreverse && isOdd(timeline.repeat - 1)) 0.0 else d
        } else if (timeline.autoreverse && isOdd(c.toLong())) {
            ph = d - ph
        }

        return valueAt(keys, ph)
    }

    private fun isOdd(n: Long): Boolean = n % 2 != 0L
    private fun isOdd(n: Int): Boolean = n % 2 != 0

    /** ph <= k0.t -> k0.v; ph >= kn.t -> kn.v; between -> eased lerp; equal key times step to the later key. */
    private fun valueAt(keys: List<Key>, ph: Double): Double {
        val first = keys.first()
        if (ph <= first.t) return first.v
        val last = keys.last()
        if (ph >= last.t) return last.v

        // Latest key index whose time is <= ph — resolves duplicate-time ties to the later key.
        var lo = 0
        for (j in keys.indices) {
            if (keys[j].t <= ph) lo = j else break
        }
        val kj = keys[lo]
        val kj1 = keys[lo + 1]
        val span = kj1.t - kj.t
        val progress = if (span <= 0.0) 1.0 else (ph - kj.t) / span
        val eased = kj1.ease?.let { CubicBezier.solve(it[0], it[1], it[2], it[3], progress) } ?: progress
        return kj.v + (kj1.v - kj.v) * eased
    }

    /** Total wall-clock span of `instanceCount` staggered instances; infinite when repeat == -1 (used by `await play`). */
    fun duration(timeline: Timeline, instanceCount: Int): Double {
        if (timeline.repeat == -1) return Double.POSITIVE_INFINITY
        val maxStagger = timeline.tracks.maxOfOrNull { it.stagger } ?: 0.0
        return timeline.duration * timeline.repeat + maxStagger * (instanceCount - 1).coerceAtLeast(0)
    }
}
