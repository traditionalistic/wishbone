package app.wishbone.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.VerticalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.key
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import app.wishbone.core.Node
import app.wishbone.core.Value

@Composable
private fun Children(items: List<Item>, parent: Parent) {
    for (item in items) key(item.key) { NodeView(item.node, item.ctx.withParent(parent)) }
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

@Composable
internal fun ScrollView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val items = ctx.materialize(node.children)
    val horizontal = p.str("axis") == "h"
    val spacing = p.spacing()
    // ponytail: `indicators` is ignored — Compose has no native scroll indicators
    when {
        p.bool("paging") == true -> {
            val state = rememberPagerState { items.size }
            val page: @Composable (Int) -> Unit = { NodeView(items[it].node, items[it].ctx.withParent(Parent.None)) }
            if (horizontal) {
                HorizontalPager(state, modifier, pageSpacing = spacing, key = { items[it].key }, verticalAlignment = vAlign(p.str("align"))) { page(it) }
            } else {
                VerticalPager(state, modifier, pageSpacing = spacing, key = { items[it].key }, horizontalAlignment = hAlign(p.str("align"))) { page(it) }
            }
        }
        p.bool("lazy") == true -> if (horizontal) {
            LazyRow(modifier, horizontalArrangement = Arrangement.spacedBy(spacing), verticalAlignment = vAlign(p.str("align"))) {
                items(items, key = { it.key }) { NodeView(it.node, it.ctx.withParent(Parent.Scroll(true))) }
            }
        } else {
            LazyColumn(modifier, verticalArrangement = Arrangement.spacedBy(spacing), horizontalAlignment = hAlign(p.str("align"))) {
                items(items, key = { it.key }) { NodeView(it.node, it.ctx.withParent(Parent.Scroll(false))) }
            }
        }
        horizontal -> Row(modifier.horizontalScroll(rememberScrollState()), Arrangement.spacedBy(spacing), vAlign(p.str("align"))) { Children(items, Parent.Scroll(true)) }
        else -> Column(modifier.verticalScroll(rememberScrollState()), Arrangement.spacedBy(spacing), hAlign(p.str("align"))) { Children(items, Parent.Scroll(false)) }
    }
}

/** Rows of `columns` equal cells (LazyVGrid with flexible columns: cells centered, rows as tall as their tallest cell). */
@Composable
internal fun GridView(node: Node, p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val items = ctx.materialize(node.children)
    val columns = (p.num("columns") ?: 1.0).toInt().coerceAtLeast(1)
    Column(modifier, Arrangement.spacedBy(p.spacing("rowSpacing"))) {
        for (row in items.chunked(columns)) {
            Row(Modifier.fillMaxWidth(), Arrangement.spacedBy(p.spacing()), Alignment.CenterVertically) {
                for (item in row) key(item.key) {
                    Box(Modifier.weight(1f), contentAlignment = Alignment.Center) { NodeView(item.node, item.ctx.withParent(Parent.Box(this))) }
                }
                repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

/** Takes the remaining space along the parent stack's axis (at least `min`); fills a z-stack. */
@Composable
internal fun SpacerView(p: Map<String, Value>, ctx: RenderContext, modifier: Modifier) {
    val min = (p.num("min") ?: 0.0).toFloat().dp
    val m = when (val parent = ctx.parent) {
        is Parent.Column -> with(parent.scope) { modifier.weight(1f).heightIn(min = min) }
        is Parent.Row -> with(parent.scope) { modifier.weight(1f).widthIn(min = min) }
        is Parent.Scroll -> if (parent.horizontal) modifier.widthIn(min = min) else modifier.heightIn(min = min)
        is Parent.Box, Parent.None -> modifier.fillMaxSize()
    }
    Spacer(m)
}
