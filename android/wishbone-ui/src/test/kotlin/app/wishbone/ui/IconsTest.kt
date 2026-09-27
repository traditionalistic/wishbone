package app.wishbone.ui

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.io.File

class IconsTest {
    @Test
    fun tableMatchesContract() {
        val contract = Json.parseToJsonElement(File("../../contract/icons.json").readText()).jsonObject
        assertEquals(contract.keys, Icons.table.keys)
        for ((name, entry) in contract) {
            val drawable = entry.jsonObject["android"]!!.jsonPrimitive.content
            val id = R.drawable::class.java.getField(drawable).getInt(null)
            assertEquals(id, Icons.table[name], "icon $name → $drawable")
        }
    }
}
