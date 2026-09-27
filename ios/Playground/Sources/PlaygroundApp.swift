import SwiftUI
import WishboneUI

@main
struct PlaygroundApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
        }
    }
}

struct ContentView: View {
    var body: some View {
        VStack(spacing: 8) {
            Text("Wishbone Playground")
                .font(.title)
            Text("WishboneUI \(WishboneUIVersion.runtime)")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .padding()
    }
}
