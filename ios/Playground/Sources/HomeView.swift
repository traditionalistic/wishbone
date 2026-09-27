import SwiftUI
import WishboneUI

struct HomeView: View {
    @Bindable var model: PlaygroundModel
    @State private var customID = ""

    private static let showcase = ["showcase.onboarding", "showcase.promo", "showcase.form", "showcase.catalog"]

    var body: some View {
        NavigationStack {
            Form {
                Section("Server") {
                    TextField("Base URL", text: $model.serverURL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .onSubmit { model.applyServer() }
                    Toggle("Serve bundled fixtures (offline)", isOn: $model.offline)
                }
                Section("Theme") {
                    Picker("Brand", selection: $model.brand) {
                        Text("korpu").tag("korpu")
                        Text("kz").tag("kz")
                    }
                    Picker("Scheme", selection: $model.scheme) {
                        ForEach(SchemeChoice.allCases) { Text($0.rawValue).tag($0) }
                    }
                    .pickerStyle(.segmented)
                }
                Section {
                    if model.sessions.isEmpty {
                        Text(model.sessionsError ?? "No live sessions — start one in the admin")
                            .font(.footnote).foregroundStyle(.secondary)
                    }
                    ForEach(model.sessions) { s in
                        Button { model.opened = .preview(s.code) } label: {
                            LabeledContent("\(s.code) · \(s.title)", value: "\(s.screen) · \(s.devices)📱")
                        }
                    }
                } header: {
                    HStack {
                        Text("Live preview")
                        Spacer()
                        Button("Refresh") { Task { await model.loadSessions() } }.font(.caption)
                    }
                }
                Section("Screens") {
                    ForEach(Self.showcase, id: \.self) { id in
                        Button(id) { model.opened = .screen(id) }
                    }
                    HStack {
                        TextField("any screen id", text: $customID)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                        Button("Open") { model.opened = .screen(customID) }.disabled(customID.isEmpty)
                    }
                }
                Section("Fixtures") {
                    ForEach(PlaygroundModel.fixtures.keys.sorted(), id: \.self) { id in
                        Button(id) { model.opened = .screen(id) }
                    }
                }
            }
            .navigationTitle("Wishbone \(WishboneUIVersion.runtime)")
            .task { await model.loadSessions() }
            .refreshable { await model.loadSessions() }
        }
        .fullScreenCover(item: $model.opened) { opened in
            OpenedView(model: model, opened: opened)
                .overlay(alignment: .top) { BannerView(host: model.host) }
                .preferredColorScheme(model.scheme.colorScheme)
        }
    }
}

/// A screen or a live preview, full screen, with a close button.
struct OpenedView: View {
    let model: PlaygroundModel
    let opened: Opened
    @State private var preview: PreviewClient?

    var body: some View {
        content
            .overlay(alignment: .bottomTrailing) {
                Button { model.opened = nil } label: {
                    Image(systemName: "xmark").font(.footnote.bold()).padding(10)
                        .background(.thinMaterial, in: Circle())
                }
                .padding(16)
                .accessibilityIdentifier("playground.close")
            }
    }

    @ViewBuilder
    private var content: some View {
        switch opened {
        case let .screen(id):
            WishboneScreen(model.wishbone, id: id)
        case let .preview(code):
            ZStack(alignment: .bottomLeading) {
                if let preview {
                    WishbonePreviewScreen(model.wishbone, preview: preview)
                    Text(verbatim: "\(code) · \(status(preview.status))")
                        .font(.caption2.monospaced()).padding(6)
                        .background(.thinMaterial, in: Capsule()).padding(16)
                }
            }
            .onAppear {
                let client = PreviewClient(model.wishbone, code: code)
                client.connect()
                preview = client
            }
            .onDisappear { preview?.disconnect() }
        }
    }

    private func status(_ s: PreviewClient.Status) -> String {
        switch s {
        case .connecting: "connecting…"
        case .connected: "live"
        case let .disconnected(e): "offline: \(e)"
        }
    }
}
