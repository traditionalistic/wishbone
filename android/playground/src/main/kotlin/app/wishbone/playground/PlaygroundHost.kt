package app.wishbone.playground

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.util.Log
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import app.wishbone.ui.WishboneHost

/** Logs track/host/open, toasts host actions, renders a stub for any `native` view. */
class PlaygroundHost(private val context: Context) : WishboneHost {
    override fun open(url: Uri) {
        Log.i(TAG, "open $url")
        runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, url).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
    }

    override fun track(event: String, params: Map<String, Any?>) {
        Log.i(TAG, "track $event $params")
    }

    override suspend fun action(name: String, args: Map<String, Any?>) {
        Log.i(TAG, "host $name $args")
        Toast.makeText(context, "$name: ${args["message"] ?: args["text"] ?: args}", Toast.LENGTH_SHORT).show()
    }

    override fun nativeView(name: String, props: Map<String, Any?>): (@androidx.compose.runtime.Composable () -> Unit) = {
        Box(Modifier.fillMaxSize().background(Color(0x3300A0FF), RoundedCornerShape(8)), contentAlignment = Alignment.Center) {
            Text("native \"$name\" $props")
        }
    }

    private companion object {
        const val TAG = "WishbonePlayground"
    }
}
