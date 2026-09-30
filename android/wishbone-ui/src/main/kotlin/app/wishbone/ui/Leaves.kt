package app.wishbone.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import app.wishbone.core.Format
import app.wishbone.core.Node
import app.wishbone.core.Value
import coil.compose.AsyncImage

private fun RenderContext.ink(v: Value?) = store.theme.color(v ?: DEFAULT_INK, store.dark) ?: store.theme.color(DEFAULT_INK, store.dark)!!

@Composable
internal fun TextView(p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val style = ctx.store.theme.textStyle(p["style"], ctx.ink(p["color"]), p.str("textAlign"))
    val maxLines = p.num("maxLines")?.toInt()?.takeIf { it > 0 } ?: Int.MAX_VALUE
    BasicText(p["text"]?.let { Format.str(it) } ?: "", modifier, style, overflow = TextOverflow.Ellipsis, maxLines = maxLines)
}

/** Image loads in flight, counted per image node; a node that leaves the composition mid-load leaves the count. */
internal class ImageLoads {
    private val loading = HashSet<Any>()
    val inFlight: Int get() = loading.size
    fun start(node: Any) { loading += node }
    fun end(node: Any) { loading -= node }
}

/** Set only inside a capture layer (spec 2026-09-30 §2.2); null on a live screen, whose loads are not counted. */
internal val LocalImageLoads = staticCompositionLocalOf<ImageLoads?> { null }

@Composable
internal fun ImageView(p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val placeholder = ColorPainter(ctx.ink(p["placeholder"] ?: Value.rec("\$color" to Value.Str("bg2"))))
    val url = p.str("url")
    val loads = LocalImageLoads.current
    val node = remember { Any() }
    if (loads != null) DisposableEffect(loads, node) { onDispose { loads.end(node) } }
    AsyncImage(
        model = url?.let { LocalWishbone.current?.resolve(it) ?: it },
        contentDescription = null,
        modifier = modifier.clipToBounds(),
        placeholder = placeholder,
        error = placeholder,
        onLoading = loads?.let { l -> { l.start(node) } },
        onSuccess = loads?.let { l -> { l.end(node) } },
        onError = loads?.let { l -> { l.end(node) } },
        contentScale = if (p.str("fit") == "fit") ContentScale.Fit else ContentScale.Crop,
    )
}

@Composable
internal fun IconView(p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val name = (p.rec("name")?.get("\$icon") as? Value.Str)?.value
    val res = Icons.table[name] ?: return ctx.store.warnOnce("icon:$name", "unknown icon '$name'")
    Image(painterResource(res), null, modifier.size((p.num("size") ?: 24.0).coerceAtLeast(0.0).toFloat().dp), colorFilter = ColorFilter.tint(ctx.ink(p["color"])))
}

@Composable
internal fun ShapeView(p: Map<String, Value>, box: BoxProps, ctx: RenderContext, modifier: Modifier) {
    val shape = when (p.str("kind")) {
        "circle" -> CenteredCircle
        "capsule" -> RoundedCornerShape(50)
        else -> cornerShape(box.corner)
    }
    var m = modifier
    ctx.store.theme.brush(p["fill"], ctx.store.dark)?.let { m = m.background(it, shape) }
    val stroke = p.rec("stroke")
    val strokeWidth = (stroke?.get("width") as? Value.Num)?.value
    val strokeColor = stroke?.get("color")?.let { ctx.store.theme.color(it, ctx.store.dark) }
    val arc = if (p.str("kind") == "circle") trimArc(p.rec("trim")) else null
    if (strokeWidth != null && strokeColor != null && strokeWidth > 0) {
        if (arc == null) {
            m = m.border(strokeWidth.toFloat().dp, strokeColor, shape)
        } else if (arc.second > 0f) {
            // Stroke inside the bounds, on CenteredCircle's circle (diameter min(w, h)).
            m = m.drawBehind {
                val w = strokeWidth.toFloat().dp.toPx()
                val d = size.minDimension - w
                if (d > 0f) {
                    drawArc(strokeColor, arc.first, arc.second, useCenter = false, topLeft = Offset((size.width - d) / 2, (size.height - d) / 2),
                        size = Size(d, d), style = Stroke(w, cap = StrokeCap.Round))
                }
            }
        }
    }
    Box(m)
}

/**
 * `shape.trim` (circle only) → drawArc (startAngle, sweepAngle) in degrees, 0° = 3 o'clock, clockwise: fractions are
 * clamped to 0…1 and start at 12 o'clock; sweep 0 = no stroke. null when absent or malformed → the whole stroke.
 */
internal fun trimArc(trim: Map<String, Value>?): Pair<Float, Float>? {
    val from = (trim?.get("from") as? Value.Num)?.value?.takeIf { it.isFinite() } ?: return null
    val to = (trim["to"] as? Value.Num)?.value?.takeIf { it.isFinite() } ?: return null
    val f = from.coerceIn(0.0, 1.0)
    return (-90 + f * 360).toFloat() to ((to.coerceIn(0.0, 1.0) - f).coerceAtLeast(0.0) * 360).toFloat()
}

/**
 * Controlled when `value` is present (always shows it; an edit the screen doesn't store is reverted),
 * uncontrolled otherwise. Edits fire `change`, the IME action fires `submit`, both with `{value}`.
 */
@Composable
internal fun TextFieldView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val controlled = p["value"]
    var local by rememberSaveable(stateSaver = TextFieldValue.Saver) { mutableStateOf(TextFieldValue((controlled as? Value.Str)?.value ?: "")) }
    val text = (controlled as? Value.Str)?.value ?: local.text
    val shown = if (local.text == text) local else TextFieldValue(text, TextRange(text.length))
    val color = ctx.ink(p["color"])
    val style = ctx.store.theme.textStyle(p["style"], color, null)
    val keyboard = when (p.str("keyboard")) {
        "email" -> KeyboardType.Email
        "number" -> KeyboardType.Decimal
        "phone" -> KeyboardType.Phone
        else -> if (p.bool("secure") == true) KeyboardType.Password else KeyboardType.Text
    }
    fun payload(s: String) = Value.rec("value" to Value.Str(s))
    BasicTextField(
        value = shown,
        onValueChange = {
            local = it
            if (it.text != text) node.on["change"]?.let { s -> ctx.fire(s, payload(it.text)) }
        },
        modifier = modifier,
        textStyle = style,
        singleLine = true,
        cursorBrush = SolidColor(color),
        visualTransformation = if (p.bool("secure") == true) PasswordVisualTransformation() else VisualTransformation.None,
        keyboardOptions = KeyboardOptions(keyboardType = keyboard, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { node.on["submit"]?.let { ctx.fire(it, payload(shown.text)) } }),
        decorationBox = { inner ->
            Box {
                if (shown.text.isEmpty()) BasicText(p.str("placeholder") ?: "", style = style.copy(color = color.copy(alpha = color.alpha * 0.4f)), maxLines = 1)
                inner()
            }
        },
    )
}

@Composable
internal fun NativeView(p: Map<String, Value>, modifier: Modifier) {
    val host = LocalWishbone.current?.host ?: return
    val content = host.nativeView(p.str("name") ?: return, p.rec("props")?.mapValues { it.value.toAny() } ?: emptyMap()) ?: return
    Box(modifier) { content() }
}
