package app.wishbone.playground

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import app.wishbone.ui.Wishbone
import app.wishbone.ui.WishboneRoute
import app.wishbone.ui.WishboneSlot

/**
 * A native fake feed with the `slot.feed.top` slot as its first item (raffles spec §2). The slot pushes onto the
 * playground's stack; a `present` route is pushed too (the playground has no modals of its own).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FeedDemo(wishbone: Wishbone, onNavigate: (WishboneRoute) -> Unit) {
    Scaffold(topBar = { CenterAlignedTopAppBar(title = { Text("Feed demo") }) }) { pad ->
        LazyColumn(contentPadding = PaddingValues(top = pad.calculateTopPadding(), bottom = pad.calculateBottomPadding() + 16.dp)) {
            // No spacing: an empty slot is zero height and must leave no gap, so each post carries its own margin.
            item { WishboneSlot(wishbone, "slot.feed.top", onNavigate = onNavigate) }
            items(6) { i ->
                Column(Modifier.padding(start = 16.dp, end = 16.dp, top = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Box(Modifier.fillMaxWidth().height(200.dp).background(Color.Gray.copy(alpha = 0.3f), RoundedCornerShape(12.dp)))
                    Text("Пост ${i + 1}", style = MaterialTheme.typography.titleMedium)
                    Text("Обычная нативная карточка ленты хоста", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}
