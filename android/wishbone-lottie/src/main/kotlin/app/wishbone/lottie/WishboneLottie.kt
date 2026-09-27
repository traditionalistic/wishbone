package app.wishbone.lottie

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import app.wishbone.core.Value
import app.wishbone.ui.WishboneRenderers
import com.airbnb.lottie.compose.LottieAnimation
import com.airbnb.lottie.compose.LottieCompositionSpec
import com.airbnb.lottie.compose.LottieConstants
import com.airbnb.lottie.compose.animateLottieCompositionAsState
import com.airbnb.lottie.compose.rememberLottieComposition

/** `lottie` primitive (spec §4.4) on lottie-compose: call [install] once, before rendering screens. */
object WishboneLottie {
    fun install() = WishboneRenderers.register("lottie") { props, modifier, emit -> LottieNode(props, modifier, emit) }
}

/** `playing`, `loop`, `speed`; `progress` (0…1) drives the frame instead when set; `end` after the last iteration. */
@Composable
private fun LottieNode(p: Map<String, Value>, modifier: Modifier, emit: (String, Value) -> Unit) {
    val url = (p["url"] as? Value.Str)?.value ?: return
    val composition by rememberLottieComposition(LottieCompositionSpec.Url(url))
    val progress = (p["progress"] as? Value.Num)?.value
    if (progress != null) {
        LottieAnimation(composition, { progress.toFloat().coerceIn(0f, 1f) }, modifier)
        return
    }
    val loop = (p["loop"] as? Value.Bool)?.value ?: true
    val state = animateLottieCompositionAsState(
        composition,
        isPlaying = (p["playing"] as? Value.Bool)?.value ?: true,
        iterations = if (loop) LottieConstants.IterateForever else 1,
        speed = ((p["speed"] as? Value.Num)?.value ?: 1.0).toFloat(),
    )
    LaunchedEffect(state.isAtEnd) { if (!loop && composition != null && state.isAtEnd) emit("end", Value.rec()) }
    LottieAnimation(composition, { state.progress }, modifier)
}
