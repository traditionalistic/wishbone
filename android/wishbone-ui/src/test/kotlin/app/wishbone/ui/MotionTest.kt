package app.wishbone.ui

import androidx.compose.animation.core.SpringSpec
import androidx.compose.animation.core.TweenSpec
import app.wishbone.core.CubicBezier
import app.wishbone.core.Document
import app.wishbone.core.Value
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File
import kotlin.math.PI

class MotionTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())

    @Test
    fun springMapsResponseToStiffness() {
        val s = MotionSpec.Spring(280.0, 0.86)
        assertEquals(((2 * PI / 0.28) * (2 * PI / 0.28)).toFloat(), s.stiffness, 0.01f)
        val spec = s.animationSpec<Float>() as SpringSpec<Float>
        assertEquals(0.86f, spec.dampingRatio, 1e-6f)
        assertEquals(s.stiffness, spec.stiffness, 1e-3f)
    }

    @Test
    fun curveMapsToTweenWithCssBezier() {
        val c = MotionSpec.Curve(300.0, listOf(0.38, 0.7, 0.125, 1.0))
        val spec = c.animationSpec<Float>() as TweenSpec<Float>
        assertEquals(300, spec.durationMillis)
        for (x in listOf(0.1, 0.5, 0.9)) {
            assertEquals(CubicBezier.solve(0.38, 0.7, 0.125, 1.0, x).toFloat(), spec.easing.transform(x.toFloat()), 1e-3f)
        }
    }

    @Test
    fun tokensAndInlineMotionResolve() {
        assertEquals(MotionSpec.Curve(300.0, listOf(0.38, 0.7, 0.125, 1.0)), theme.motionOf(Value.rec("\$motion" to Value.Str("smooth"))))
        assertEquals(MotionSpec.Spring(280.0, 0.86), theme.motionOf(Value.rec("\$motion" to Value.Str("interactive"))))
        val inline = Value.rec("\$motion" to Value.rec("kind" to Value.Str("spring"), "response" to Value.Num(400.0), "damping" to Value.Num(0.7)))
        assertEquals(MotionSpec.Spring(400.0, 0.7), theme.motionOf(inline))
        assertNull(theme.motionOf(Value.rec("\$motion" to Value.Str("nope"))))
        assertNull(theme.motionOf(Value.Num(1.0)))
    }

    @Test
    fun transitionPartsAndReduceMotion() {
        val parts = TransitionPart.list(Json.parseToJsonElement("""[{"k":"slide","edge":"end"},{"k":"fade"},{"k":"scale","from":0.6},{"k":"offset","x":0,"y":8}]""") as JsonArray)
        assertEquals(listOf("slide", "fade", "scale", "offset"), parts.map { it.kind })
        assertEquals("end", parts[0].edge)
        assertEquals(0.6, parts[2].from)
        assertEquals(8.0, parts[3].y)
        assertEquals(parts, parts.reduced(false))
        assertEquals(listOf("fade"), parts.reduced(true).map { it.kind })
        assertTrue(emptyList<TransitionPart>().reduced(true).isEmpty())
    }

    @Test
    fun snapPicksNearestPointElseZero() {
        val snaps = listOf(-420.0, 0.0, 420.0)
        assertEquals(-420.0, nearestSnap(-300.0, snaps))
        assertEquals(0.0, nearestSnap(150.0, snaps))
        assertEquals(420.0, nearestSnap(260.0, snaps))
        assertEquals(0.0, nearestSnap(90.0, emptyList()))
    }

    @Test
    fun layerTransformComposes() {
        val t = LayerTransform.IDENTITY.with("opacity", 0.5).with("opacity", 0.5).with("scale", 2.0).with("offsetX", 3.0).with("offsetX", 4.0).with("rotation", 10.0).with("width", 9.0)
        assertEquals(LayerTransform(opacity = 0.25, scale = 2.0, offsetX = 7.0, rotation = 10.0), t)
    }

    private val ms = 1_000_000L

    private fun registry() = TimelineRegistry(
        Document.parse(
            """{"screen":"t","root":{"t":"stack","id":"root"},"timelines":{
              "fade":{"duration":400,"repeat":1,"autoreverse":false,"autoplay":false,"tracks":[
                {"target":"a","prop":"opacity","stagger":0,"keys":[{"t":0,"v":0},{"t":400,"v":1}]},
                {"target":"a","prop":"offsetY","stagger":0,"keys":[{"t":0,"v":20},{"t":400,"v":0}]}]},
              "dim":{"duration":100,"repeat":1,"autoreverse":false,"autoplay":true,"tracks":[
                {"target":"a","prop":"opacity","stagger":0,"keys":[{"t":0,"v":1},{"t":100,"v":0.5}]}]},
              "spin":{"duration":1000,"repeat":-1,"autoreverse":false,"autoplay":false,"tracks":[
                {"target":"s","prop":"rotation","stagger":0,"keys":[{"t":0,"v":0},{"t":1000,"v":360}]}]},
              "list":{"duration":200,"repeat":1,"autoreverse":false,"autoplay":false,"tracks":[
                {"target":"item","prop":"scale","stagger":100,"keys":[{"t":0,"v":0},{"t":200,"v":1}]}]}}}""",
        ).timelines,
        now = 0,
    )

    @Test
    fun registryLifecycle() {
        val r = registry()
        assertTrue(r.isActive("dim")) // autoplay starts with the registry…
        assertFalse(r.isActive("fade"))
        r.firstFrame(0) // …and is rebased onto the first rendered frame (here: the same instant)
        assertTrue(r.running(50 * ms))
        assertFalse(r.running(150 * ms)) // dim finished, holds its end value
        assertEquals(0.5, r.transform("a", 0, 150 * ms, false).opacity, 1e-9)

        r.play("fade", 1000 * ms)
        assertTrue(r.running(1200 * ms))
        val mid = r.transform("a", 0, 1200 * ms, false)
        assertEquals(0.25, mid.opacity, 1e-9) // 0.5 (fade) × 0.5 (dim end)
        assertEquals(10.0, mid.offsetY, 1e-9)

        r.play("fade", 2000 * ms) // restarts
        assertEquals(0.0, r.transform("a", 0, 2000 * ms, false).opacity, 1e-9)

        r.stop("fade")
        r.stop("dim")
        assertEquals(LayerTransform.IDENTITY, r.transform("a", 0, 3000 * ms, false))
        assertFalse(r.running(3000 * ms))
        assertEquals(LayerTransform.IDENTITY, r.transform("unrelated", 0, 0, false))
    }

    @Test
    fun infiniteStaggerAndReduceMotion() {
        val r = registry()
        r.play("spin", 0)
        assertTrue(r.running(100_000 * ms))
        assertFalse(r.running(100_000 * ms, reduceMotion = true)) // reduceMotion: end states, no frame loop
        assertEquals(Double.POSITIVE_INFINITY, r.duration("spin"))
        assertEquals(90.0, r.transform("s", 0, 250 * ms, false).rotation, 1e-6)
        assertEquals(360.0, r.transform("s", 0, 250 * ms, true).rotation, 1e-3) // reduceMotion: end state

        r.play("list", 0)
        r.counts["item"] = 4
        assertEquals(200.0 + 3 * 100.0, r.duration("list"))
        assertEquals(0.5, r.transform("item", 2, 300 * ms, false).scale, 1e-9) // u = 300 − 2·100
        assertEquals(0.0, r.transform("item", 3, 250 * ms, false).scale, 1e-9)
        assertEquals(1.0, r.transform("item", 3, 0, true).scale, 1e-9)
    }

    @Test
    fun autoplayRebasesOnFirstFrameAndSurvivesRebuild() {
        val r = registry()
        r.firstFrame(500 * ms)
        assertEquals(1.0, r.transform("a", 0, 500 * ms, false).opacity, 1e-9) // dim starts at the first frame
        r.play("fade", 600 * ms)
        val rebuilt = TimelineRegistry(registryTimelines(), now = 900 * ms, previous = r)
        assertTrue(rebuilt.isActive("fade")) // keeps playing across a store rebuild
        assertEquals(0.25, rebuilt.transform("a", 0, 800 * ms, false).opacity, 1e-9) // fade 0.5 × dim held at 0.5
        r.stop("dim")
        val again = TimelineRegistry(registryTimelines(), now = 900 * ms, previous = r)
        assertFalse(again.isActive("dim")) // a stopped autoplay timeline is not restarted by a rebuild
    }

    private fun registryTimelines() = Document.parse(
        """{"screen":"t","root":{"t":"stack","id":"root"},"timelines":{
          "fade":{"duration":400,"repeat":1,"autoreverse":false,"autoplay":false,"tracks":[{"target":"a","prop":"opacity","stagger":0,"keys":[{"t":0,"v":0},{"t":400,"v":1}]}]},
          "dim":{"duration":100,"repeat":1,"autoreverse":false,"autoplay":true,"tracks":[{"target":"a","prop":"opacity","stagger":0,"keys":[{"t":0,"v":1},{"t":100,"v":0.5}]}]}}}""",
    ).timelines
}
