import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatBlockedAuthorsTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    private func blocks(_ ids: String...) -> StubTransport.Reply {
        let users = ids.map { #"{"user_id":"\#($0)","handle":"h","display_name":"H","photo_url":null}"# }
        return .json(.ok, #"{"users":[\#(users.joined(separator: ","))]}"#)
    }

    private func transport(_ blocks: StubTransport.Reply..., then page: StubTransport.Reply) -> StubTransport {
        StubTransport(routes: [
            "/v1/me/blocks": blocks,
            "/v1/cabals/\(Fixtures.cabalID)/messages?limit=50": [page],
        ])
    }

    func testALiveMessageFromABlockedAuthorIsDropped() async throws {
        let transport = transport(blocks(Fixtures.otherID), then: try Fixtures.page([]))
        let session = Fixtures.session(transport, blocked: true)
        await session.open()

        await session.apply(.messageCreated(Fixtures.message("m1", author: Fixtures.otherID, minutes: 1)))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), [])
    }

    func testALiveMessageFromAnUnblockedAuthorIsShown() async throws {
        let transport = transport(blocks("someone-else"), then: try Fixtures.page([]))
        let session = Fixtures.session(transport, blocked: true)
        await session.open()

        await session.apply(.messageCreated(Fixtures.message("m1", author: Fixtures.otherID, minutes: 1)))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
    }

    func testAfterUnblockingNewMessagesShow() async throws {
        let transport = transport(
            blocks(Fixtures.otherID), blocks(), then: try Fixtures.page([]))
        let session = Fixtures.session(transport, blocked: true)
        await session.open()
        await session.apply(.messageCreated(Fixtures.message("m1", minutes: 1)))

        await session.refreshBlocked()
        await session.apply(.messageCreated(Fixtures.message("m2", minutes: 2)))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m2"])
    }

    func testABlockedAuthorsReplyNeverReachesAnOpenThread() async throws {
        let parent = Fixtures.message("p1", author: "someone-else", minutes: 1, replyCount: 0)
        let transport = StubTransport(routes: [
            "/v1/me/blocks": [blocks(Fixtures.otherID)],
            "/v1/cabals/\(Fixtures.cabalID)/messages?limit=50": [try Fixtures.page([parent])],
            "/v1/cabals/\(Fixtures.cabalID)/messages/p1/thread?limit=50": [
                try Fixtures.thread(parent: parent, replies: [])
            ],
        ])
        let session = Fixtures.session(transport, blocked: true)
        await session.open()
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        await session.apply(.messageCreated(Fixtures.message("r1", minutes: 2, parentID: "p1")))
        await session.apply(
            .messageCreated(Fixtures.message("r2", author: "someone-else", minutes: 3, parentID: "p1")))

        for await state in await thread.states() {
            XCTAssertEqual(state.timeline.rows.map(\.id), ["r2"])
            break
        }
    }

    func testBlockingAnAuthorRemovesTheirLoadedMessagesBeforeTheReload() async throws {
        let loaded = (0..<10).map { Fixtures.message("x\($0)", minutes: Double($0)) }
        let keep = Fixtures.message("keep", author: "someone-else", minutes: 20)
        let transport = transport(
            blocks(), blocks(Fixtures.otherID), then: try Fixtures.page(loaded + [keep]))
        let session = Fixtures.session(transport, blocked: true)
        await session.open()
        let before = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(before).count, 11)

        await session.refreshBlocked()

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["keep"])
    }

    func testAMessageDeletedWhileAwayDisappearsOnReload() async throws {
        let m0 = Fixtures.message("m0", minutes: 0)
        let m1 = Fixtures.message("m1", minutes: 1)
        let m2 = Fixtures.message("m2", minutes: 2)
        let transport = StubTransport(routes: [
            "/v1/cabals/\(Fixtures.cabalID)/messages?limit=50": [
                try Fixtures.page([m0, m1, m2]), try Fixtures.page([m0, m2]),
            ]
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.reload()

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m0", "m2"])
    }
}
