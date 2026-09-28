import Foundation
import Testing
@testable import WishboneUI

@Suite struct ClientTests {
    let info = RequestInfo(baseURL: URL(string: "http://localhost:8080")!, brand: "kz", locale: "ru-ru", headers: ["Authorization": "Bearer t"])

    private func body(_ req: URLRequest) throws -> Value { try Value(json: req.httpBody!) }

    @Test func commonHeaders() {
        let req = API.screen(info, id: "showcase.promo", params: [:], etag: nil)
        #expect(req.value(forHTTPHeaderField: "X-Wishbone-Runtime") == "1.0.0")
        #expect(req.value(forHTTPHeaderField: "X-Wishbone-Platform") == "ios")
        #expect(req.value(forHTTPHeaderField: "X-Wishbone-Brand") == "kz")
        #expect(req.value(forHTTPHeaderField: "Accept-Language") == "ru-ru")
        #expect(req.value(forHTTPHeaderField: "Authorization") == "Bearer t")
        #expect(req.value(forHTTPHeaderField: "If-None-Match") == nil)
        #expect(req.url?.absoluteString == "http://localhost:8080/v1/screens/showcase.promo")
    }

    @Test func screenIDsFollowTheServerRule() {
        for id in ["host.hello", "showcase.promo", "a-b_c9"] { #expect(API.isScreenID(id), "\(id)") }
        for id in ["..", ".", "", "a/b", "Showcase", String(repeating: "a", count: 65)] { #expect(!API.isScreenID(id), "\(id)") }
    }

    @Test func screenParamsAreStrictlyEncoded() throws {
        let req = API.screen(info, id: "a b", params: ["q": .string("1+1 & x=y")], etag: "\"4-abc\"")
        #expect(req.url?.absoluteString == "http://localhost:8080/v1/screens/a%20b?params=%7B%22q%22%3A%221%2B1%20%26%20x%3Dy%22%7D")
        #expect(req.value(forHTTPHeaderField: "If-None-Match") == "\"4-abc\"")
        let items = URLComponents(url: req.url!, resolvingAgainstBaseURL: false)!.queryItems!
        #expect(try Value(json: items[0].value!) == .record(["q": .string("1+1 & x=y")]))
    }

    @Test func nonASCIIIdsAndParamsArePercentEncoded() throws {
        let req = API.screen(info, id: "экран", params: ["name": .string("Иван")], etag: nil)
        #expect(req.url?.absoluteString == "http://localhost:8080/v1/screens/%D1%8D%D0%BA%D1%80%D0%B0%D0%BD?params=%7B%22name%22%3A%22%D0%98%D0%B2%D0%B0%D0%BD%22%7D")
        let items = URLComponents(url: req.url!, resolvingAgainstBaseURL: false)!.queryItems!
        #expect(try Value(json: items[0].value!) == .record(["name": .string("Иван")]))
    }

    @Test func baseURLWithPathPrefix() {
        let prefixed = RequestInfo(baseURL: URL(string: "https://h.io/api/")!, brand: "korpu", locale: "en")
        #expect(API.theme(prefixed, etag: nil).url?.absoluteString == "https://h.io/api/v1/theme?brand=korpu")
        #expect(API.previewSocket(prefixed, code: "K7Q2").url?.absoluteString == "wss://h.io/api/v1/preview/ws?session=K7Q2")
        #expect(API.previewSocket(info, code: "K7Q2").url?.absoluteString == "ws://localhost:8080/v1/preview/ws?session=K7Q2")
    }

    @Test func callAndDataBodies() throws {
        let call = API.call(info, screen: "s", revision: 4, op: "demo.subscribe", args: .record(["email": .string("a@b")]), key: "k1")
        #expect(call.httpMethod == "POST" && call.url?.path == "/v1/call")
        #expect(call.value(forHTTPHeaderField: "Content-Type") == "application/json")
        #expect(try body(call) == .record([
            "screen": .string("s"), "revision": .number(4), "op": .string("demo.subscribe"),
            "args": .record(["email": .string("a@b")]), "idempotencyKey": .string("k1"),
        ]))
        let data = API.data(info, screen: "s", revision: 4, params: ["id": .string("1")], sources: ["me"])
        #expect(try body(data) == .record([
            "screen": .string("s"), "revision": .number(4), "params": .record(["id": .string("1")]), "sources": .list([.string("me")]),
        ]))
    }

    @Test func parseScreenWithDocumentAndData() throws {
        let json = #"{"etag":"4-ab","revision":4,"document":{"format":1,"screen":"s","root":{"t":"stack","id":"root"},"n":0.125,"b":true},"data":{"me":{"ok":true,"value":{"name":"Ann"}},"offers":{"ok":false,"error":{"code":"X","message":"m","retryable":false}}}}"#
        let r = try API.parseScreen(Data(json.utf8))
        #expect(r.etag == "4-ab" && r.revision == 4)
        let doc = try Value(json: r.document!)
        guard case let .record(d) = doc else { Issue.record("document not a record"); return }
        #expect(d["n"] == .number(0.125) && d["b"] == .bool(true))
        #expect(ScreenResponse.dataValue(r.data["me"]!) == .record(["name": .string("Ann")]))
        #expect(ScreenResponse.dataValue(r.data["offers"]!) == .null)
        // cache round trip keeps everything
        let back = try API.parseScreen(try API.encodeScreen(r))
        #expect(back.etag == r.etag && back.revision == 4 && back.data == r.data)
        #expect(try Value(json: back.document!) == doc)
    }

    @Test func parseScreenNotModified() throws {
        let r = try API.parseScreen(Data(#"{"etag":"4-ab","revision":4,"document":null,"data":{}}"#.utf8))
        #expect(r.document == nil && r.data.isEmpty)
    }

    @Test func contractErrors() {
        let e = API.error(status: 404, body: Data(#"{"error":{"code":"SCREEN_NOT_FOUND","message":"no","retryable":false}}"#.utf8))
        #expect(e == .server(status: 404, code: "SCREEN_NOT_FOUND", message: "no", retryable: false))
        guard case let .server(503, code, _, retryable) = API.error(status: 503, body: Data("oops".utf8)) else {
            Issue.record("expected a server error"); return
        }
        #expect(code == "HTTP_503" && retryable)
    }

    @Test func callFailuresBecomeResults() {
        #expect(API.callFailure(WishboneError.network("offline")) == .record([
            "ok": .bool(false), "error": .record(["code": .string("NETWORK"), "message": .string("offline")]),
        ]))
        #expect(API.callFailure(WishboneError.server(status: 403, code: "OPERATION_NOT_BOUND", message: "m", retryable: false)) == .record([
            "ok": .bool(false), "error": .record(["code": .string("OPERATION_NOT_BOUND"), "message": .string("m")]),
        ]))
    }

    @MainActor @Test func cacheScopeSeparatesServersAccountsAndBrands() {
        struct Account: WishboneHost { var cacheScope: String }
        let a = URL(string: "http://a")!, b = URL(string: "http://b")!
        let scopes = [
            Wishbone.cacheScope(a, DefaultHost(), "korpu"), Wishbone.cacheScope(b, DefaultHost(), "korpu"),
            Wishbone.cacheScope(a, DefaultHost(), "kz"), Wishbone.cacheScope(a, Account(cacheScope: "user-2"), "korpu"),
        ]
        #expect(Set(scopes).count == 4)
    }

    @Test func diskCache() {
        let cache = DiskCache(dir: tempDir())
        let k1 = DiskCache.screenKey(scope: "korpu", id: "s", params: [:])
        let k2 = DiskCache.screenKey(scope: "korpu", id: "s", params: ["id": .string("1")])
        let k3 = DiskCache.screenKey(scope: "kz", id: "s", params: [:])
        #expect(Set([k1, k2, k3]).count == 3)
        #expect(cache.read(k1) == nil)
        cache.write(k1, Data("one".utf8))
        cache.write(k2, Data("two".utf8))
        #expect(cache.read(k1) == Data("one".utf8))
        #expect(cache.read(k2) == Data("two".utf8))
        cache.write(k1, Data("uno".utf8))
        #expect(cache.read(k1) == Data("uno".utf8))
        #expect(cache.read(k3) == nil)
    }

    @Test func previewMessages() async throws {
        let json = #"{"type":"document","screen":"s","document":{"format":1,"screen":"s","revision":2,"root":{"t":"text","id":"t"}},"data":{"me":{"ok":true,"value":1}}}"#
        await MainActor.run {
            let client = PreviewClient(testWishbone(), code: "K7Q2")
            client.handle(Data(json.utf8))
            #expect(client.revision == 1)
            #expect(client.latest?.revision == 2)
            #expect(client.latest?.data["me"] == .record(["ok": .bool(true), "value": .number(1)]))
            client.handle(Data(#"{"type":"play","timeline":"intro"}"#.utf8))
            #expect(client.play?.timeline == "intro" && client.play?.seq == 1)
        }
    }
}
