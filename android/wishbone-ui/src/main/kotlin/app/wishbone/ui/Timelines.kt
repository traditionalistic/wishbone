package app.wishbone.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.setValue
import app.wishbone.core.Timeline
import app.wishbone.core.TimelineSampler

/** The transform layer timelines write to (spec §4.2): opacity/scale multiply, offsets/rotation add. */
internal data class LayerTransform(
    val opacity: Double = 1.0,
    val scale: Double = 1.0,
    val offsetX: Double = 0.0,
    val offsetY: Double = 0.0,
    val rotation: Double = 0.0,
) {
    fun with(prop: String, v: Double) = when (prop) {
        "opacity" -> copy(opacity = opacity * v)
        "scale" -> copy(scale = scale * v)
        "offsetX" -> copy(offsetX = offsetX + v)
        "offsetY" -> copy(offsetY = offsetY + v)
        "rotation" -> copy(rotation = rotation + v)
        else -> this
    }

    companion object {
        val IDENTITY = LayerTransform()
    }
}

/**
 * Active timelines `{name → start (frame-clock ns)}`: `autoplay` ones start with the registry,
 * `play` (re)starts, `stop` removes (values return to identity), finished ones hold their end values.
 */
internal class TimelineRegistry(private val timelines: Map<String, Timeline>, now: Long, previous: TimelineRegistry? = null) {
    private val active = mutableStateMapOf<String, Long>()

    /** Bumped on play/stop so the frame loop restarts. */
    var version by mutableIntStateOf(0)
        private set

    private val byTarget: Map<String, List<String>> = timelines.entries
        .flatMap { (name, t) -> t.tracks.map { it.target to name } }
        .groupBy({ it.first }, { it.second })
        .mapValues { it.value.distinct() }

    /** Materialized instance count per repeated target node (reported by the renderer), for stagger-aware durations. */
    val counts = HashMap<String, Int>()

    /** Autoplay timelines not yet rebased onto the first rendered frame. */
    private val pendingAutoplay = HashSet<String>()

    /** A rebuilt store (hot reload, SWR swap) keeps the timelines that were playing; autoplay only starts fresh ones. */
    init {
        for ((name, t) in timelines) {
            val kept = previous?.active?.get(name)
            if (kept != null) {
                active[name] = kept
            } else if (t.autoplay && previous?.timelines?.containsKey(name) != true) {
                active[name] = now
                pendingAutoplay += name
            }
        }
    }

    /** "autoplay starts at first render": the first frame restarts fresh autoplay timelines at its time. */
    fun firstFrame(now: Long) {
        for (name in pendingAutoplay) if (name in active) active[name] = now
        pendingAutoplay.clear()
    }

    fun isTarget(id: String) = id in byTarget
    fun isActive(name: String) = name in active

    fun play(name: String, now: Long) {
        if (name !in timelines) return
        active[name] = now
        version++
    }

    fun stop(name: String) {
        if (active.remove(name) != null) version++
    }

    /** ms until every instance has finished; +∞ for `repeat: -1`. */
    fun duration(name: String): Double {
        val t = timelines[name] ?: return 0.0
        return TimelineSampler.duration(t, t.tracks.maxOfOrNull { counts[it.target] ?: 1 } ?: 1)
    }

    /** Whether any active timeline still changes values at [now] (the frame loop runs only then). */
    fun running(now: Long): Boolean = active.any { (name, start) -> (now - start) / 1e6 < duration(name) }

    fun transform(id: String, index: Int, now: Long, reduceMotion: Boolean): LayerTransform {
        val names = byTarget[id] ?: return LayerTransform.IDENTITY
        var tr = LayerTransform.IDENTITY
        for (name in names) {
            val start = active[name] ?: continue
            val t = timelines.getValue(name)
            val ms = if (reduceMotion) finalMs(t, name) else (now - start) / 1e6
            for ((key, v) in TimelineSampler.sample(t, ms, index)) {
                if (key.substringBeforeLast('.') == id) tr = tr.with(key.substringAfterLast('.'), v)
            }
        }
        return tr
    }

    /** reduceMotion: the end state (infinite timelines: the end of one cycle). */
    private fun finalMs(t: Timeline, name: String): Double =
        if (t.repeat == -1) t.duration * (1 - 1e-9) else duration(name) + 1
}
