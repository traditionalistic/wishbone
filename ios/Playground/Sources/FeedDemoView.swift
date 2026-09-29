import SwiftUI
import WishboneUI

/// Path value that opens `FeedDemoView`.
struct FeedDemo: Hashable {}

/// A native fake feed with the `slot.feed.top` slot as its first row (raffles spec §2). The slot pushes onto the
/// playground's stack; a `present` route is pushed too (the playground has no modals of its own).
struct FeedDemoView: View {
    let model: PlaygroundModel

    var body: some View {
        ScrollView {
            // No spacing: an empty slot is zero height and must leave no gap, so each post carries its own margin.
            LazyVStack(spacing: 0) {
                WishboneSlot(model.wishbone, id: "slot.feed.top") { model.path.append($0) }
                ForEach(1...6, id: \.self) { i in
                    VStack(alignment: .leading, spacing: 8) {
                        RoundedRectangle(cornerRadius: 12).fill(.gray.opacity(0.3)).frame(height: 200)
                        Text("Пост \(i)").font(.headline)
                        Text("Обычная нативная карточка ленты хоста").font(.subheadline).foregroundStyle(.secondary)
                    }
                    .padding([.horizontal, .top], 16)
                }
            }
            .padding(.bottom, 16)
        }
        .navigationTitle("Feed demo")
        .navigationBarTitleDisplayMode(.inline)
    }
}
