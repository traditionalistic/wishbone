package app.wishbone.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** Vectors transcribed from contract/conformance/timelines.json (named eases resolved to their contract/stdlib.json `easings` arrays — the compiler's job on the real wire, done by hand here). */
class TimelineSamplerTest {
    private val EASE_IN = listOf(0.42, 0.0, 1.0, 1.0)
    private val EASE_OUT = listOf(0.0, 0.0, 0.58, 1.0)
    private val EASE_IN_OUT = listOf(0.42, 0.0, 0.58, 1.0)
    private val SPRING = listOf(0.38, 0.7, 0.125, 1.0)
    private val TOL = 1e-6

    private fun tl(
        duration: Double,
        repeat: Int = 1,
        autoreverse: Boolean = false,
        tracks: List<Track>,
    ) = Timeline(duration, repeat, autoreverse, autoplay = false, tracks = tracks)

    private fun track(target: String, prop: String, stagger: Double = 0.0, vararg keys: Key) =
        Track(target, prop, stagger, keys.toList())

    @Test
    fun `linear track`() {
        val t = tl(400.0, tracks = listOf(track("a", "opacity", 0.0, Key(0.0, 0.0, null), Key(400.0, 1.0, null))))
        assertEquals(0.0, TimelineSampler.sample(t, 0.0, 0)["a.opacity"]!!, TOL)
        assertEquals(0.25, TimelineSampler.sample(t, 100.0, 0)["a.opacity"]!!, TOL)
        assertEquals(1.0, TimelineSampler.sample(t, 400.0, 0)["a.opacity"]!!, TOL)
        assertEquals(1.0, TimelineSampler.sample(t, 1000.0, 0)["a.opacity"]!!, TOL)
    }

    @Test
    fun `easeOut track`() {
        val t = tl(1000.0, tracks = listOf(track("a", "offsetY", 0.0, Key(0.0, 0.0, null), Key(1000.0, 100.0, EASE_OUT))))
        assertEquals(37.813813083, TimelineSampler.sample(t, 250.0, 0)["a.offsetY"]!!, TOL)
        assertEquals(68.464318743, TimelineSampler.sample(t, 500.0, 0)["a.offsetY"]!!, TOL)
        assertEquals(90.653534928, TimelineSampler.sample(t, 750.0, 0)["a.offsetY"]!!, TOL)
    }

    @Test
    fun `easeIn then easeInOut track`() {
        val t = tl(
            200.0,
            tracks = listOf(
                track(
                    "a", "scale", 0.0,
                    Key(0.0, 0.0, null),
                    Key(100.0, 1.0, EASE_IN),
                    Key(200.0, 0.0, EASE_IN_OUT),
                ),
            ),
        )
        assertEquals(0.315356813, TimelineSampler.sample(t, 50.0, 0)["a.scale"]!!, TOL)
        assertEquals(0.5, TimelineSampler.sample(t, 150.0, 0)["a.scale"]!!, TOL)
        assertEquals(0.129161931, TimelineSampler.sample(t, 175.0, 0)["a.scale"]!!, TOL)
    }

    @Test
    fun `spring ease track`() {
        val t = tl(200.0, tracks = listOf(track("a", "offsetY", 0.0, Key(0.0, 40.0, null), Key(200.0, 0.0, SPRING))))
        assertEquals(15.333559586, TimelineSampler.sample(t, 50.0, 0)["a.offsetY"]!!, TOL)
        assertEquals(2.7664857, TimelineSampler.sample(t, 100.0, 0)["a.offsetY"]!!, TOL)
        assertEquals(0.000131286, TimelineSampler.sample(t, 199.0, 0)["a.offsetY"]!!, TOL)
    }

    @Test
    fun `custom bezier track`() {
        val t = tl(1000.0, tracks = listOf(track("a", "rotation", 0.0, Key(0.0, 0.0, null), Key(1000.0, 90.0, listOf(0.25, 0.1, 0.25, 1.0)))))
        assertEquals(46.198364488, TimelineSampler.sample(t, 300.0, 0)["a.rotation"]!!, TOL)
        assertEquals(72.216304883, TimelineSampler.sample(t, 500.0, 0)["a.rotation"]!!, TOL)
    }

    @Test
    fun `stagger delays by instance index`() {
        val t = tl(200.0, tracks = listOf(track("item", "opacity", 100.0, Key(0.0, 0.0, null), Key(200.0, 1.0, null))))
        assertEquals(0.75, TimelineSampler.sample(t, 150.0, 0)["item.opacity"]!!, TOL)
        assertEquals(0.25, TimelineSampler.sample(t, 150.0, 1)["item.opacity"]!!, TOL)
        assertEquals(0.0, TimelineSampler.sample(t, 150.0, 2)["item.opacity"]!!, TOL)
        assertEquals(1.0, TimelineSampler.sample(t, 500.0, 2)["item.opacity"]!!, TOL)
    }

    @Test
    fun `repeat with autoreverse`() {
        val t = tl(100.0, repeat = 2, autoreverse = true, tracks = listOf(track("a", "offsetX", 0.0, Key(0.0, 0.0, null), Key(100.0, 10.0, null))))
        assertEquals(5.0, TimelineSampler.sample(t, 50.0, 0)["a.offsetX"]!!, TOL)
        assertEquals(7.0, TimelineSampler.sample(t, 130.0, 0)["a.offsetX"]!!, TOL)
        assertEquals(0.0, TimelineSampler.sample(t, 250.0, 0)["a.offsetX"]!!, TOL)
    }

    @Test
    fun `infinite repeat`() {
        val t = tl(100.0, repeat = -1, tracks = listOf(track("a", "rotation", 0.0, Key(0.0, 0.0, null), Key(100.0, 360.0, null))))
        assertEquals(180.0, TimelineSampler.sample(t, 1050.0, 0)["a.rotation"]!!, TOL)
        assertEquals(356.4, TimelineSampler.sample(t, 99.0, 0)["a.rotation"]!!, TOL)
    }

    @Test
    fun `multiple tracks sampled together`() {
        val t = tl(
            400.0,
            tracks = listOf(
                track("a", "opacity", 0.0, Key(0.0, 0.0, null), Key(400.0, 1.0, null)),
                track("a", "offsetY", 0.0, Key(0.0, 40.0, null), Key(200.0, 0.0, SPRING)),
            ),
        )
        val s1 = TimelineSampler.sample(t, 100.0, 0)
        assertEquals(0.25, s1["a.opacity"]!!, TOL)
        assertEquals(2.7664857, s1["a.offsetY"]!!, TOL)
        val s2 = TimelineSampler.sample(t, 300.0, 0)
        assertEquals(0.75, s2["a.opacity"]!!, TOL)
        assertEquals(0.0, s2["a.offsetY"]!!, TOL)
    }

    @Test
    fun `keys starting after t=0 hold the first value`() {
        val t = tl(200.0, tracks = listOf(track("a", "scale", 0.0, Key(100.0, 5.0, null), Key(200.0, 10.0, null))))
        assertEquals(5.0, TimelineSampler.sample(t, 50.0, 0)["a.scale"]!!, TOL)
        assertEquals(7.5, TimelineSampler.sample(t, 150.0, 0)["a.scale"]!!, TOL)
    }

    @Test
    fun `zero duration holds the single key constant`() {
        val t = tl(0.0, tracks = listOf(track("a", "opacity", 0.0, Key(0.0, 3.0, null))))
        assertEquals(3.0, TimelineSampler.sample(t, 0.0, 0)["a.opacity"]!!, TOL)
        assertEquals(3.0, TimelineSampler.sample(t, 500.0, 0)["a.opacity"]!!, TOL)
    }

    @Test
    fun `interior duplicate key times step to the later key (contract README - equal key times step to the later key)`() {
        // keys at t=0(v=0), t=5(v=1), t=5(v=9), t=10(v=20): sampling exactly at t=5 must use the LATER
        // of the two t=5 keys (v=9), not the earlier one (v=1).
        val t = tl(10.0, tracks = listOf(track("a", "opacity", 0.0, Key(0.0, 0.0, null), Key(5.0, 1.0, null), Key(5.0, 9.0, null), Key(10.0, 20.0, null))))
        assertEquals(9.0, TimelineSampler.sample(t, 5.0, 0)["a.opacity"]!!, TOL)
        // interpolation onward from t=5 uses the later key's value (9) as its left edge.
        assertEquals(14.5, TimelineSampler.sample(t, 7.5, 0)["a.opacity"]!!, TOL)
    }

    @Test
    fun `sampleTrack equals the matching sample entry (staggered, repeat, autoreverse)`() {
        val tracks = listOf(
            track("a", "opacity", 0.0, Key(0.0, 0.0, null), Key(100.0, 1.0, EASE_IN_OUT)),
            track("b", "offsetX", 30.0, Key(0.0, 0.0, null), Key(50.0, 10.0, SPRING), Key(100.0, -4.0, null)),
        )
        val timelines = listOf(
            tl(100.0, repeat = 1, tracks = tracks),
            tl(100.0, repeat = 3, autoreverse = true, tracks = tracks),
            tl(100.0, repeat = 2, autoreverse = true, tracks = tracks),
            tl(100.0, repeat = -1, autoreverse = true, tracks = tracks),
            tl(0.0, tracks = tracks.map { it.copy(keys = listOf(Key(0.0, 7.0, null))) }),
        )
        for (t in timelines) for (index in 0..3) for (ms in listOf(-10.0, 0.0, 25.0, 99.0, 100.0, 130.0, 250.0, 1000.0)) {
            val all = TimelineSampler.sample(t, ms, index)
            for (track in t.tracks) {
                assertEquals(all["${track.target}.${track.prop}"]!!, TimelineSampler.sampleTrack(t, track, ms, index), 0.0)
            }
        }
    }
}
