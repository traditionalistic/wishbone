package app.wishbone.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.VerticalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.key
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.unit.dp
import app.wishbone.core.Node
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope

@Composable
private fun Children(items: List<Item>, parent: Parent) {
    val (presence, merged) = rememberPresence(items)
    for (item in merged) key(item.key) { ItemView(item, parent, presence) }
}

private fun hAlign(a: String?) = when (a) {
    "center" -> Alignment.CenterHorizontally
    "end" -> Alignment.End
    else -> Alignment.Start
}

private fun vAlign(a: String?) = when (a) {
    "center" -> Alignment.CenterVertically
    "end" -> Alignment.Bottom
    else -> Alignment.Top
}

private fun zAlign(a: String?) = when (a) {
    "topStart" -> Alignment.TopStart
    "top" -> Alignment.TopCenter
    "topEnd" -> Alignment.TopEnd
    "start" -> Alignment.CenterStart
    "end" -> Alignment.CenterEnd
    "bottomStart" -> Alignment.BottomStart
    "bottom" -> Alignment.BottomCenter
    "bottomEnd" -> Alignment.BottomEnd
    else -> Alignment.Center
}

private fun Map<String, Value>.spacing(k: String = "spacing") = (num(k) ?: 0.0).toFloat().dp

@Composable
internal fun StackView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val items = ctx.materialize(node.children)
    when (p.str("axis")) {
        "h" -> Row(modifier, Arrangement.spacedBy(p.spacing()), vAlign(p.str("align"))) { Children(items, Parent.Row(this)) }
        "z" -> Box(modifier, zAlign(p.str("align"))) { Children(items, Parent.Box(this)) }
        else -> Column(modifier, Arrangement.spacedBy(p.spacing()), hAlign(p.str("align"))) { Children(items, Parent.Column(this)) }
    }
}

/**
 * Exposes `scroll.<id>.x|y` (content offset from the resting top/left, dp) as driver cells, written on
 * every scroll frame. Lazy lists: offset estimated from the average measured item size.
 */
@Composable
internal fun ScrollView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier, contentPadding: PaddingValues) {
    val items = ctx.materialize(node.children)
    val horizontal = p.str("axis") == "h"
    val spacing = p.spacing()
    val cell = ctx.store.driverCell("scroll", node.id, if (horizontal) "x" else "y")
    val density = LocalDensity.current.density
    val window = LocalWindowInfo.current.containerSize
    val bounded = Modifier.boundAxis(horizontal, if (horizontal) window.width else window.height)
    fun drive(offsetPx: () -> Float): suspend CoroutineScope.() -> Unit = { snapshotFlow(offsetPx).collect { cell.doubleValue = (it / density).toDouble() } }
    // ponytail: `indicators` is ignored — Compose has no native scroll indicators
    when {
        p.bool("paging") == true -> {
            val pages = items.filter { it.visible }
            val state = rememberPagerState { pages.size }
            LaunchedEffect(state) { drive { (state.currentPage + state.currentPageOffsetFraction) * (state.layoutInfo.pageSize + state.layoutInfo.pageSpacing) }() }
            val page: @Composable (Int) -> Unit = { NodeView(pages[it].node, pages[it].ctx.withParent(Parent.None)) }
            if (horizontal) {
                HorizontalPager(state, modifier.then(bounded), contentPadding = contentPadding, pageSpacing = spacing, key = { pages[it].key }, verticalAlignment = vAlign(p.str("align"))) { page(it) }
            } else {
                VerticalPager(state, modifier.then(bounded), contentPadding = contentPadding, pageSpacing = spacing, key = { pages[it].key }, horizontalAlignment = hAlign(p.str("align"))) { page(it) }
            }
        }
        p.bool("lazy") == true -> {
            val state = rememberLazyListState()
            // ponytail: lazy content offset = index × average visible item size + in-item offset (exact for uniform items)
            LaunchedEffect(state) {
                drive {
                    val visible = state.layoutInfo.visibleItemsInfo
                    val avg = if (visible.isEmpty()) 0f else visible.sumOf { it.size }.toFloat() / visible.size + state.layoutInfo.mainAxisItemSpacing
                    state.firstVisibleItemIndex * avg + state.firstVisibleItemScrollOffset
                }()
            }
            val (presence, merged) = rememberPresence(items)
            val parent = Parent.Scroll(horizontal)
            if (horizontal) {
                LazyRow(modifier.then(bounded), state, contentPadding, horizontalArrangement = Arrangement.spacedBy(spacing), verticalAlignment = vAlign(p.str("align"))) {
                    items(merged, key = { it.key }) { ItemView(it, parent, presence) }
                }
            } else {
                LazyColumn(modifier.then(bounded), state, contentPadding, verticalArrangement = Arrangement.spacedBy(spacing), horizontalAlignment = hAlign(p.str("align"))) {
                    items(merged, key = { it.key }) { ItemView(it, parent, presence) }
                }
            }
        }
                else -> {
            val state = rememberScrollState()
            LaunchedEffect(state) { drive { state.value.toFloat() }() }
            if (horizontal) {
                Row(modifier.then(bounded).horizontalScroll(state).padding(contentPadding), Arrangement.spacedBy(spacing), vAlign(p.str("align"))) { Children(items, Parent.Scroll(true)) }
            } else {
                Column(modifier.then(bounded).verticalScroll(state).padding(contentPadding), Arrangement.spacedBy(spacing), hAlign(p.str("align"))) { Children(items, Parent.Scroll(false)) }
            }
        }
    }
}

/** A scroll measured with an unbounded main axis (nested same-axis scroll) is capped at the window size instead of crashing. */
private fun Modifier.boundAxis(horizontal: Boolean, limit: Int) = layout { measurable, c ->
    val capped = when {
        horizontal && !c.hasBoundedWidth -> c.copy(minWidth = minOf(c.minWidth, limit), maxWidth = limit)
        !horizontal && !c.hasBoundedHeight -> c.copy(minHeight = minOf(c.minHeight, limit), maxHeight = limit)
        else -> c
    }
    val placeable = measurable.measure(capped)
    layout(placeable.width, placeable.height) { placeable.place(0, 0) }
}

/** Rows of `columns` equal cells (LazyVGrid with flexible columns: cells centered, rows as tall as their tallest cell). */
@Composable
internal fun GridView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val (presence, merged) = rememberPresence(ctx.materialize(node.children))
    val columns = (p.num("columns") ?: 1.0).toInt().coerceAtLeast(1)
    Column(modifier, Arrangement.spacedBy(p.spacing("rowSpacing"))) {
        for (row in merged.chunked(columns)) {
            Row(Modifier.fillMaxWidth(), Arrangement.spacedBy(p.spacing()), Alignment.CenterVertically) {
                for (item in row) key(item.key) {
                    Box(Modifier.weight(1f), contentAlignment = Alignment.Center) { ItemView(item, Parent.Box(this), presence) }
                }
                repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

/** Takes the remaining space along the parent stack's axis (at least `min`); fills a z-stack. */
@Composable
internal fun SpacerView(p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val min = (p.num("min") ?: 0.0).coerceAtLeast(0.0).toFloat().dp
    val m = when (val parent = ctx.parent) {
        is Parent.Column -> with(parent.scope) { modifier.weight(1f).heightIn(min = min) }
        is Parent.Row -> with(parent.scope) { modifier.weight(1f).widthIn(min = min) }
        is Parent.Scroll -> if (parent.horizontal) modifier.widthIn(min = min) else modifier.heightIn(min = min)
        is Parent.Box, Parent.None -> modifier.fillMaxSize()
    }
    Spacer(m)
}
