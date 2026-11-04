import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ChatPinsModelHidingTests: XCTestCase {
    private let cabalID = ChatFixtures.cabalID

    private func model(_ transport: StubTransport) -> ChatPinsModel {
        ChatPinsModel(
            cabalID: cabalID, viewerID: ChatFixtures.viewerID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            makeKey: { "key-1" })
    }

    private func message(_ id: String, author: String = ChatFixtures.otherID, body: String? = nil) -> ChatMessage {
        ChatFixtures.message(id, author: author, body: body)
    }

    private func loaded(_ messages: [ChatMessage]) async throws -> ChatPinsModel {
        let json = try messages.map(ChatFixtures.json).joined(separator: ",")
        let model = model(StubTransport(scripted: [.json(.ok, #"{"messages":[\#(json)]}"#)]))
        await model.load()
        return model
    }

    func testLoadSkipsBlockedAuthorsAndMaskedPlaceholdersBeforeTakingThree() async throws {
        let blocked = message("a", author: "blocked-1")
        var masked = message("b")
        masked.deleted = true
        let all = [blocked, masked] + ["c", "d", "e", "f"].map { message($0) }
        let json = try all.map(ChatFixtures.json).joined(separator: ",")
        let model = model(StubTransport(scripted: [.json(.ok, #"{"messages":[\#(json)]}"#)]))

        await model.load(hiding: ["blocked-1"])

        XCTAssertEqual(model.pins.map(\.id), ["c", "d", "e"])
    }

    func testDropRemovesAPinAtOnceAndClampsCurrent() async throws {
        let model = try await loaded(["a", "b"].map { message($0) })
        model.next()
        XCTAssertEqual(model.current, 1)

        model.drop(messageID: "b")

        XCTAssertEqual(model.pins.map(\.id), ["a"])
        XCTAssertEqual(model.current, 0)
        model.drop(messageID: "missing")
        XCTAssertEqual(model.pins.map(\.id), ["a"])
    }
}
