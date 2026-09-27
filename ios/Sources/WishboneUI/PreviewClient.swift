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

/// Device side of the preview protocol (spec §5.4): `hello` on connect, `document` → render in
/// place, `play` → start a timeline, runtime errors → `log`. Reconnects with backoff.
@MainActor @Observable
public final class PreviewClient {
    public enum Status: Equatable, Sendable {
        case connecting, connected
        case disconnected(String)
    }

    public private(set) var status: Status = .connecting
    /// The last document pushed by the session (with its data).
    private(set) var latest: ScreenResponse?
    private(set) var revision = 0
    /// Last `play` request: (timeline, sequence number) — consumed by motion (phase b).
    public private(set) var play: (timeline: String, seq: Int)?

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
        let hello = Self.json(["type": "hello", "platform": "ios", "device": Self.deviceName, "runtime": WishboneCoreVersion.runtime])
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
            guard let doc = obj["document"] as? [String: Any],
                  let docData = try? JSONSerialization.data(withJSONObject: doc),
                  let rawData = try? JSONSerialization.data(withJSONObject: obj["data"] as? [String: Any] ?? [:]),
                  let data = try? API.parseData(rawData)
            else { return wishbone.log("warn", "preview: malformed document message") }
            latest = ScreenResponse(etag: "", revision: doc["revision"] as? Int ?? 0, document: docData, data: data, rawData: rawData)
            revision += 1
        case "play":
            if let name = obj["timeline"] as? String { play = (name, (play?.seq ?? 0) + 1) }
        default:
            break
        }
    }

    private func send(_ obj: [String: String]) {
        guard let socket, status == .connected else { return }
        socket.send(.string(Self.json(obj))) { _ in }
    }

    private static func json(_ obj: [String: String]) -> String {
        String(decoding: (try? JSONSerialization.data(withJSONObject: obj)) ?? Data(), as: UTF8.self)
    }

    private static var deviceName: String {
        #if canImport(UIKit)
        ProcessInfo.processInfo.environment["SIMULATOR_DEVICE_NAME"] ?? UIDevice.current.model
        #else
        Host.current().localizedName ?? "mac"
        #endif
    }
}
