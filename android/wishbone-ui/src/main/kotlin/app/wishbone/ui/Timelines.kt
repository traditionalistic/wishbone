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

    /** Bumped on play/stop and when a target first goes live, so the frame loop restarts. */
    var version by mutableIntStateOf(0)
        private set

    /** Per target node: (timeline name, prop, single-track timeline) — precomputed so a frame samples without lookups. */
    private class TargetTrack(val name: String, val prop: String, val timeline: Timeline)

    private val byTarget: Map<String, List<TargetTrack>> = timelines.entries
        .flatMap { (name, t) -> t.tracks.map { tr -> tr.target to TargetTrack(name, tr.prop, t.copy(tracks = listOf(tr))) } }
        .groupBy({ it.first }, { it.second })

    /** Live materialized instances per target node id: instance key (id + key path) → repeat index. */
    private val live = HashMap<String, HashMap<String, Int>>()

    /** A target's first live instance restarts the frame loop: an infinite timeline on it may have been idle. */
    fun register(id: String, instance: String, index: Int) {
        val instances = live.getOrPut(id) { HashMap() }
        if (instances.isEmpty()) version++
        instances[instance] = index
    }

    fun unregister(id: String, instance: String) {
        live[id]?.remove(instance)
    }

    /** Instances for stagger-aware durations: highest live repeat index + 1 (1 when none is composed). */
    fun count(id: String): Int = live[id]?.values?.maxOrNull()?.plus(1) ?: 1

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
        return TimelineSampler.duration(t, t.tracks.maxOfOrNull { count(it.target) } ?: 1)
    }

    /**
     * Whether any active timeline still changes values at [now] (the frame loop runs only then; never under reduceMotion).
     * An infinite one counts only while one of its targets is composed: off screen (another tab, a lazy row scrolled
     * away) it would redraw nothing, every frame.
     */
    fun running(now: Long, reduceMotion: Boolean = false): Boolean = finiteRunning(now, reduceMotion) || !reduceMotion && active.any { (name, _) ->
        val t = timelines.getValue(name)
        t.repeat == -1 && t.tracks.any { live[it.target]?.isNotEmpty() == true }
    }

    /** A finite active timeline still short of its end at [now]: what a capture waits for (spec 2026-09-30 §2.2); spinners never end. */
    fun finiteRunning(now: Long, reduceMotion: Boolean): Boolean = !reduceMotion && active.any { (name, start) ->
        timelines.getValue(name).repeat != -1 && (now - start) / 1e6 < duration(name)
    }

    fun transform(id: String, index: Int, now: Long, reduceMotion: Boolean): LayerTransform {
        val tracks = byTarget[id] ?: return LayerTransform.IDENTITY
        var tr = LayerTransform.IDENTITY
        for (t in tracks) {
            val start = active[t.name] ?: continue
            val ms = if (reduceMotion) finalMs(t.timeline, t.name) else (now - start) / 1e6
            tr = tr.with(t.prop, TimelineSampler.sampleTrack(t.timeline, t.timeline.tracks[0], ms, index))
        }
        return tr
    }

    /** reduceMotion: the end state (infinite timelines: the end of one cycle). */
    private fun finalMs(t: Timeline, name: String): Double =
        if (t.repeat == -1) t.duration * (1 - 1e-9) else duration(name) + 1
}
