package app.wishbone.playground

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import app.wishbone.lottie.WishboneLottie
import app.wishbone.rive.WishboneRive

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        WishboneLottie.install()
        WishboneRive.install(this)
        setContent { Playground(intent.getStringExtra("route"), intent.getStringExtra("brand"), intent.getStringExtra("scheme"), intent.getStringExtra("baseUrl")) }
    }
}
