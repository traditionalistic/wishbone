package app.wishbone.playground

import android.content.Context
import android.graphics.Color as AndroidColor
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.enableEdgeToEdge
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.edit
import app.wishbone.ui.PreviewSession
import app.wishbone.ui.Wishbone
import app.wishbone.ui.WishboneDocument
import app.wishbone.ui.WishbonePreview
import app.wishbone.ui.WishboneScreen
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

private const val DEFAULT_URL = "http://10.0.2.2:8080"
private val SCREENS = listOf("showcase.onboarding", "showcase.promo", "showcase.form", "showcase.catalog")

/** `screen:<id>` | `preview:<code>` | `fixture:<name>` | `home`. Also accepted as the `route` intent extra (adb). */
@JvmInline
value class Route(val value: String) {
    val kind get() = value.substringBefore(':')
    val arg get() = value.substringAfter(':')
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun Playground(start: String? = null, startBrand: String? = null, startScheme: String? = null) {
    val context = LocalContext.current
    val prefs = remember { context.getSharedPreferences("playground", Context.MODE_PRIVATE) }
    var baseUrl by remember { mutableStateOf(prefs.getString("baseUrl", DEFAULT_URL)!!) }
    var brand by remember { mutableStateOf(startBrand ?: prefs.getString("brand", "korpu")!!) }
    var scheme by remember { mutableStateOf(startScheme ?: prefs.getString("scheme", "system")!!) }
    var route by remember { mutableStateOf(Route(start ?: "home")) }
    LaunchedEffect(baseUrl, brand, scheme) { prefs.edit { putString("baseUrl", baseUrl).putString("brand", brand).putString("scheme", scheme) } }
    val wishbone = remember(baseUrl, brand, scheme) {
        runCatching { Wishbone(context, baseUrl, brand, PlaygroundHost(context), scheme.takeIf { it != "system" }) }.getOrNull()
    }
    val dark = when (scheme) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }
    LaunchedEffect(dark) {
        val bars = if (dark) SystemBarStyle.dark(AndroidColor.TRANSPARENT) else SystemBarStyle.light(AndroidColor.TRANSPARENT, AndroidColor.TRANSPARENT)
        (context as? ComponentActivity)?.enableEdgeToEdge(bars, bars)
    }
    MaterialTheme(colorScheme = if (dark) darkColorScheme() else lightColorScheme()) {
        Surface {
            BackHandler(enabled = route.kind != "home") { route = Route("home") }
            val wb = wishbone
            when {
                wb == null || route.kind == "home" -> Home(
                    wb, baseUrl, { baseUrl = it }, brand, { brand = it }, scheme, { scheme = it }, fixtures(context),
                ) { route = it }
                route.kind == "screen" -> WishboneScreen(wb, route.arg, onExit = { route = Route("home") })
                route.kind == "preview" -> WishbonePreview(wb, route.arg)
                route.kind == "fixture" -> WishboneDocument(wb, loadFixture(context, route.arg))
            }
        }
    }
}

private fun fixtures(context: Context) = context.assets.list("fixtures").orEmpty().map { it.removeSuffix(".json") }.sorted()

private fun loadFixture(context: Context, name: String): JsonObject =
    Json.parseToJsonElement(context.assets.open("fixtures/$name.json").bufferedReader().use { it.readText() }).jsonObject

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Home(
    wishbone: Wishbone?,
    baseUrl: String,
    onBaseUrl: (String) -> Unit,
    brand: String,
    onBrand: (String) -> Unit,
    scheme: String,
    onScheme: (String) -> Unit,
    fixtures: List<String>,
    open: (Route) -> Unit,
) {
    var urlDraft by remember(baseUrl) { mutableStateOf(baseUrl) }
    var anyId by remember { mutableStateOf("") }
    var refresh by remember { mutableIntStateOf(0) }
    var sessions by remember(wishbone) { mutableStateOf<List<PreviewSession>>(emptyList()) }
    var sessionsError by remember(wishbone) { mutableStateOf<String?>(null) }
    LaunchedEffect(wishbone, refresh) {
        val wb = wishbone ?: return@LaunchedEffect
        runCatching { wb.client.previewSessions() }
            .onSuccess { sessions = it; sessionsError = null }
            .onFailure { sessionsError = it.message ?: it.toString() }
    }
    Scaffold(topBar = { TopAppBar(title = { Text("Wishbone Playground") }) }) { pad ->
        LazyColumn(contentPadding = PaddingValues(top = pad.calculateTopPadding(), bottom = pad.calculateBottomPadding() + 24.dp)) {
            item {
                Row(Modifier.padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                    OutlinedTextField(urlDraft, { urlDraft = it }, Modifier.weight(1f), label = { Text("Server") }, singleLine = true, isError = wishbone == null)
                    TextButton(onClick = { onBaseUrl(urlDraft.trim()) }, enabled = urlDraft.trim() != baseUrl) { Text("Apply") }
                }
            }
            item { Chips("Brand", listOf("korpu", "kz"), brand, onBrand) }
            item { Chips("Scheme", listOf("system", "light", "dark"), scheme, onScheme) }
            item {
                Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("Live preview", Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                    TextButton(onClick = { refresh++ }) { Text("Refresh") }
                }
            }
            sessionsError?.let { item { Text(it, Modifier.padding(horizontal = 16.dp), color = MaterialTheme.colorScheme.error) } }
            if (sessions.isEmpty() && sessionsError == null) item { Text("No live sessions", Modifier.padding(horizontal = 16.dp)) }
            items(sessions) { s ->
                ListItem(
                    headlineContent = { Text(s.title.ifEmpty { s.screen }) },
                    supportingContent = { Text("${s.code} · ${s.screen} · devices: ${s.devices}") },
                    modifier = Modifier.clickable { open(Route("preview:${s.code}")) },
                )
            }
            item { Section("Screens") }
            items(SCREENS) { id -> ListItem(headlineContent = { Text(id) }, modifier = Modifier.clickable(enabled = wishbone != null) { open(Route("screen:$id")) }) }
            item {
                Row(Modifier.padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(anyId, { anyId = it }, Modifier.weight(1f), label = { Text("Any screen id") }, singleLine = true)
                    Button(onClick = { open(Route("screen:${anyId.trim()}")) }, enabled = anyId.isNotBlank() && wishbone != null) { Text("Open") }
                }
            }
            if (fixtures.isNotEmpty()) {
                item { Section("Fixtures (offline)") }
                items(fixtures) { f -> ListItem(headlineContent = { Text(f) }, modifier = Modifier.clickable(enabled = wishbone != null) { open(Route("fixture:$f")) }) }
            }
        }
    }
}

@Composable
private fun Section(title: String) {
    Text(title, Modifier.padding(start = 16.dp, top = 24.dp, bottom = 4.dp), style = MaterialTheme.typography.titleMedium)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Chips(title: String, options: List<String>, selected: String, onSelect: (String) -> Unit) {
    Row(Modifier.padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(title, Modifier.padding(end = 8.dp))
        options.forEach { FilterChip(selected = it == selected, onClick = { onSelect(it) }, label = { Text(it) }) }
    }
}
