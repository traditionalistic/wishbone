package app.wishbone.core

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Android's (ICU-based) `Pattern.compile` rejects `Pattern.UNICODE_CHARACTER_CLASS` with
 * `IllegalArgumentException` — desktop Java (where these tests run) accepts it, so this fallback
 * path is otherwise untested by every other suite in this module. `Pattern.UNICODE_CHARACTER_CLASS`
 * itself can't be used to force the branch on the JVM (it's valid here), so this uses a flag bit no
 * platform accepts — the exact same catch-and-retry code path `compiledPattern` relies on for real
 * on-device.
 */
class StdlibRegexFallbackTest {
    private val universallyInvalidFlag = 0x40000000 // not one of Pattern's defined flag bits, on any platform

    @Test
    fun `falls back to compiling without flags when the runtime rejects them`() {
        val pattern = compileWithFallback("[a-z]+\\d*", universallyInvalidFlag)
        assertTrue(pattern.matcher("abc123").matches())
        assertTrue(!pattern.matcher("ABC").matches())
    }

    @Test
    fun `a genuinely malformed regex still fails, not silently swallowed by the fallback`() {
        assertTrue(
            runCatching { compileWithFallback("[", universallyInvalidFlag) }.exceptionOrNull()
                is java.util.regex.PatternSyntaxException,
        )
    }

    @Test
    fun `matches() stdlib function works end to end via the same fallback-capable path`() {
        // Not forcing the fallback here (UNICODE_CHARACTER_CLASS is valid on the JVM) — this just
        // confirms compiledPattern -> compileWithFallback wiring still gives a working matcher.
        assertTrue((TestHarness.eval("""["f","matches",[["v","Привет"],["v","\\w+"]]]""") as Value.Bool).value)
    }
}
