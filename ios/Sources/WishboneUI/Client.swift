import Foundation
import WishboneCore

public enum WishboneError: Error, Equatable, Sendable, CustomStringConvertible {
    /// Non-200 answer with the contract error body `{error: {code, message, retryable}}`.
    case server(status: Int, code: String, message: String, retryable: Bool)
    case network(String)
    case decode(String)

    public var description: String {
        switch self {
        case let .server(status, code, message, _): "\(status) \(code): \(message)"
        case let .network(m): "network: \(m)"
        case let .decode(m): "decode: \(m)"
        }
    }
}

/// `GET /v1/screens/{id}` answer (contract/api.md). `document` is the raw wire document, nil when
/// `If-None-Match` matched; `data` maps source name → `{ok, value?, error?}`.
struct ScreenResponse: Sendable {
    var etag: String
    var revision: Int
    var document: Data?
    var data: [String: Value]
    /// `data` as received (re-encoded by JSONSerialization) — cached without number rounding.
    var rawData: Data

    /// Value visible to the DSL: the source's value when ok, else null.
    static func dataValue(_ result: Value) -> Value {
        guard case let .record(r) = result, r["ok"] == .bool(true) else { return .null }
        return r["value"] ?? .null
    }
}

/// Everything a request needs besides its own arguments.
struct RequestInfo: Sendable {
    var baseURL: URL
    var brand: String
    var locale: String
    var headers: [String: String] = [:]
}

/// Request building and response parsing (contract/api.md), kept pure for tests; `send` does I/O.
enum API {
    /// RFC 3986 unreserved, ASCII only (`.alphanumerics` would let Cyrillic through and crash URLComponents).
    static let queryAllowed = CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~")

    static func request(_ info: RequestInfo, _ path: String, query: [(String, String)] = [], method: String = "GET", body: Data? = nil) -> URLRequest {
        var comps = URLComponents(url: info.baseURL, resolvingAgainstBaseURL: false)!
        comps.percentEncodedPath = comps.percentEncodedPath.trimmingSuffix("/") + path
        if !query.isEmpty {
            comps.percentEncodedQuery = query.map {
                "\($0.0)=\($0.1.addingPercentEncoding(withAllowedCharacters: queryAllowed) ?? "")"
            }.joined(separator: "&")
        }
        var req = URLRequest(url: comps.url!)
        req.httpMethod = method
        req.setValue(WishboneCoreVersion.runtime, forHTTPHeaderField: "X-Wishbone-Runtime")
        req.setValue("ios", forHTTPHeaderField: "X-Wishbone-Platform")
        req.setValue(info.brand, forHTTPHeaderField: "X-Wishbone-Brand")
        req.setValue(info.locale, forHTTPHeaderField: "Accept-Language")
        for (k, v) in info.headers { req.setValue(v, forHTTPHeaderField: k) }
        if let body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return req
    }

    static func pathSegment(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: queryAllowed) ?? s
    }

    static func screen(_ info: RequestInfo, id: String, params: [String: Value], etag: String?) -> URLRequest {
        let query = params.isEmpty ? [] : [("params", jsonString(.record(params)))]
        var req = request(info, "/v1/screens/\(pathSegment(id))", query: query)
        if let etag { req.setValue(etag, forHTTPHeaderField: "If-None-Match") }
        return req
    }

    static func data(_ info: RequestInfo, screen: String, revision: Int, params: [String: Value], sources: [String]) -> URLRequest {
        let body: Value = .record([
            "screen": .string(screen), "revision": .number(Double(revision)), "params": .record(params),
            "sources": .list(sources.map(Value.string)),
        ])
        return request(info, "/v1/data", method: "POST", body: body.toJSON())
    }

    static func call(_ info: RequestInfo, screen: String, revision: Int, op: String, args: Value, key: String) -> URLRequest {
        // ponytail: Value.toJSON prints non-integers with ≤6 decimals (DSL rule) — fine for call args.
        let body: Value = .record([
            "screen": .string(screen), "revision": .number(Double(revision)), "op": .string(op),
            "args": args, "idempotencyKey": .string(key),
        ])
        return request(info, "/v1/call", method: "POST", body: body.toJSON())
    }

    static func theme(_ info: RequestInfo, etag: String?) -> URLRequest {
        var req = request(info, "/v1/theme", query: [("brand", info.brand)])
        if let etag { req.setValue(etag, forHTTPHeaderField: "If-None-Match") }
        return req
    }

    static func sessions(_ info: RequestInfo) -> URLRequest {
        request(info, "/v1/preview/sessions")
    }

    /// `ws(s)://…/v1/preview/ws?session=CODE`.
    static func previewSocket(_ info: RequestInfo, code: String) -> URLRequest {
        var req = request(info, "/v1/preview/ws", query: [("session", code)])
        var comps = URLComponents(url: req.url!, resolvingAgainstBaseURL: false)!
        comps.scheme = comps.scheme == "https" ? "wss" : "ws"
        req.url = comps.url
        return req
    }

    static func jsonString(_ v: Value) -> String { String(decoding: v.toJSON(), as: UTF8.self) }

    // MARK: parsing

    static func object(_ data: Data) throws -> [String: Any] {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw WishboneError.decode("expected a JSON object")
        }
        return obj
    }

    /// Re-encodes a JSONSerialization fragment and parses it with the core's exact JSON reader.
    static func value(_ any: Any) throws -> Value {
        try Value(json: JSONSerialization.data(withJSONObject: any, options: .fragmentsAllowed))
    }

    static func error(status: Int, body: Data) -> WishboneError {
        guard let obj = try? object(body), let e = obj["error"] as? [String: Any] else {
            return .server(status: status, code: "HTTP_\(status)", message: HTTPURLResponse.localizedString(forStatusCode: status), retryable: status >= 500)
        }
        return .server(status: status, code: e["code"] as? String ?? "HTTP_\(status)", message: e["message"] as? String ?? "", retryable: e["retryable"] as? Bool ?? false)
    }

    static func parseScreen(_ body: Data) throws -> ScreenResponse {
        let obj = try object(body)
        let rawData = try JSONSerialization.data(withJSONObject: obj["data"] as? [String: Any] ?? [:])
        let doc: Data? = if let d = obj["document"] as? [String: Any] { try JSONSerialization.data(withJSONObject: d) } else { nil }
        return ScreenResponse(etag: obj["etag"] as? String ?? "", revision: obj["revision"] as? Int ?? 0, document: doc, data: try parseData(rawData), rawData: rawData)
    }

    /// Cache form of a screen answer: the same JSON shape with a non-null document.
    static func encodeScreen(_ r: ScreenResponse) throws -> Data {
        var obj: [String: Any] = ["etag": r.etag, "revision": r.revision, "data": try JSONSerialization.jsonObject(with: r.rawData)]
        if let doc = r.document { obj["document"] = try JSONSerialization.jsonObject(with: doc) }
        return try JSONSerialization.data(withJSONObject: obj)
    }

    static func parseData(_ body: Data) throws -> [String: Value] {
        guard case let .record(r) = try Value(json: body) else { throw WishboneError.decode("data: expected an object") }
        return r
    }

    /// Protocol errors and transport failures become a failed call result, never a throw.
    static func callFailure(_ error: Error) -> Value {
        let (code, message): (String, String) = switch error {
        case let WishboneError.server(_, code, message, _): (code, message)
        case let WishboneError.network(m): ("NETWORK", m)
        default: ("NETWORK", String(describing: error))
        }
        return .record(["ok": .bool(false), "error": .record(["code": .string(code), "message": .string(message)])])
    }

    // MARK: I/O

    /// Sends a request; 200 → body, 304 → nil, anything else throws.
    static func send(_ req: URLRequest, session: URLSession = .shared) async throws -> (Data?, HTTPURLResponse) {
        let data: Data, response: URLResponse
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            throw WishboneError.network(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw WishboneError.network("not an HTTP response") }
        switch http.statusCode {
        case 200: return (data, http)
        case 304: return (nil, http)
        default: throw error(status: http.statusCode, body: data)
        }
    }
}

extension String {
    func trimmingSuffix(_ s: String) -> String { hasSuffix(s) ? String(dropLast(s.count)) : self }
}
