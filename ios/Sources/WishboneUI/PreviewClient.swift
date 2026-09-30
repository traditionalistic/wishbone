import Foundation
import Observation
import WishboneCore
#if canImport(UIKit)
import UIKit
#endif

/// A live admin preview session (`GET /v1/preview/sessions`).
public struct PreviewSession: Identifiable, Decodable, Sendable, Equatable {
    public let code: String
    public let screen: String
    public let title: String
    public let devices: Int
    public let updatedAt: String
    public var id: String { code }
}

/// Device side of the preview protocol (spec §5.4, 2026-09-30 §3): `hello` on connect, `document` → render in
/// place, `play` → start a timeline, `render` → a shot from the capture layer, runtime errors → `log`. Reconnects
/// with backoff.
@MainActor @Observable
public final class PreviewClient {
    public enum Status: Equatable, Sendable {
        case connecting, connected
        case disconnected(String)
    }

    public private(set) var status: Status = .connecting
    /// The last document pushed by the session (with its data).
    private(set) var latest: ScreenResponse?
    /// Its screen id: the live tree's identity (spec 2026-09-30 §1.1 — another screen starts fresh).
    private(set) var screen: String?
    private(set) var revision = 0
    /// Last `play` request: (timeline, sequence number) — consumed by motion (phase b).
    public private(set) var play: (timeline: String, seq: Int)?
    /// The render job in progress (spec 2026-09-30 §2): `WishbonePreviewScreen` draws it in its capture layer and answers
    /// through `finish`. A newer job replaces it unanswered — the hub waits for the latest only.
    private(set) var job: RenderJob?
    /// The platform can draw shots (§2.3): told in `hello`, so the hub sends jobs only where a capture layer exists.
    #if canImport(UIKit)
    static let renders = true
    #else
    static let renders = false
    #endif

    public let code: String
    @ObservationIgnored let wishbone: Wishbone
    @ObservationIgnored private var socket: URLSessionWebSocketTask?
    @ObservationIgnored private var loop: Task<Void, Never>?

    public init(_ wishbone: Wishbone, code: String) {
        self.wishbone = wishbone
        self.code = code
    }

    /// A dropped client stops reconnecting and closes its socket.
    isolated deinit {
        loop?.cancel()
        socket?.cancel(with: .goingAway, reason: nil)
    }

    public static func sessions(_ wishbone: Wishbone) async throws -> [PreviewSession] {
        let (body, _) = try await API.send(API.sessions(await wishbone.requestInfo()))
        do {
            return try JSONDecoder().decode([PreviewSession].self, from: body ?? Data("[]".utf8))
        } catch {
            throw WishboneError.decode("sessions: \(error)")
        }
    }

    public func connect() {
        guard loop == nil else { return }
        wishbone.previewLog = { [weak self] level, message in self?.send(["type": "log", "level": level, "message": message]) }
        let hello = Self.json(["type": "hello", "platform": "ios", "device": Self.deviceName, "runtime": WishboneCoreVersion.runtime,
                               "render": Self.renders])
        // Holds self weakly across awaits so a dropped client stops reconnecting.
        loop = Task { [weak self] in
            var delay = 1.0
            while !Task.isCancelled {
                guard let req = await self?.socketRequest() else { return }
                let ws = URLSession.shared.webSocketTask(with: req)
                ws.maximumMessageSize = 4 << 20
                self?.socket = ws
                self?.status = .connecting
                ws.resume()
                do {
                    try await ws.send(.string(hello))
                    self?.status = .connected
                    delay = 1 // connected: the next drop retries quickly again
                    while !Task.isCancelled {
                        switch try await ws.receive() {
                        case let .string(s): self?.handle(Data(s.utf8))
                        case let .data(d): self?.handle(d)
                        @unknown default: break
                        }
                    }
                } catch {
                    self?.status = .disconnected(String(describing: error))
                }
                ws.cancel()
                try? await Task.sleep(for: .seconds(delay))
                delay = min(delay * 2, 10)
            }
        }
    }

    public func disconnect() {
        loop?.cancel()
        loop = nil
        socket?.cancel(with: .goingAway, reason: nil)
        socket = nil
        wishbone.previewLog = nil
    }

    private func socketRequest() async -> URLRequest {
        API.previewSocket(await wishbone.requestInfo(), code: code)
    }

    func handle(_ message: Data) {
        guard let obj = try? API.object(message) else { return }
        switch obj["type"] as? String {
        case "document":
            guard let r = Self.screen(obj) else { return wishbone.log("warn", "preview: malformed document message") }
            latest = r
            screen = obj["screen"] as? String
            revision += 1
        case "render":
            guard let id = obj["job"] as? Int else { return wishbone.log("warn", "preview: render without a job") }
            // SwiftUI mounts no layer while the app is in the background: answered here at once (§2.3).
            guard !Self.inBackground else { return send(Shot.error("app is in the background").message(job: id)) }
            guard let r = Self.screen(obj), let json = r.document, let document = try? Document(json: json) else {
                wishbone.log("warn", "preview: malformed render message")
                return send(Shot.error("malformed render message").message(job: id))
            }
            job = RenderJob(id: id, document: document, data: r.data)
        case "play":
            if let name = obj["timeline"] as? String { play = (name, (play?.seq ?? 0) + 1) }
        default:
            break
        }
    }

    /// The shot of job `id`, sent once; the job goes with it (and its capture layer). A stale id — the job was replaced
    /// meanwhile — is dropped.
    func finish(_ id: Int, _ shot: Shot) {
        guard job?.id == id else { return }
        job = nil
        send(shot.message(job: id))
    }

    /// The screen of a `document`/`render` message (nil: malformed).
    private static func screen(_ obj: [String: Any]) -> ScreenResponse? {
        guard let doc = obj["document"] as? [String: Any],
              let docData = try? JSONSerialization.data(withJSONObject: doc),
              let rawData = try? JSONSerialization.data(withJSONObject: obj["data"] as? [String: Any] ?? [:]),
              let data = try? API.parseData(rawData)
        else { return nil }
        return ScreenResponse(etag: "", revision: doc["revision"] as? Int ?? 0, document: docData, data: data, rawData: rawData)
    }

    private func send(_ obj: [String: Any]) {
        guard let socket, status == .connected else { return }
        socket.send(.string(Self.json(obj))) { _ in }
    }

    private static func json(_ obj: [String: Any]) -> String {
        String(decoding: (try? JSONSerialization.data(withJSONObject: obj)) ?? Data(), as: UTF8.self)
    }

    private static var inBackground: Bool {
        #if canImport(UIKit)
        UIApplication.shared.applicationState == .background
        #else
        false
        #endif
    }

    private static var deviceName: String {
        #if canImport(UIKit)
        ProcessInfo.processInfo.environment["SIMULATOR_DEVICE_NAME"] ?? UIDevice.current.model
        #else
        Host.current().localizedName ?? "mac"
        #endif
    }
}
