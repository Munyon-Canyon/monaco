#if DEBUG
import Foundation
@testable import MonacoCore
import XCTest

final class SampleThreadTogglePathTests: XCTestCase {
    private func channel(_ session: ChatSession) async -> [ChatMessage] {
        for await state in await session.states() { return state.timeline.messages }
        preconditionFailure("states ended")
    }

    private func openedThread(_ session: ChatSession) async -> ThreadSession {
        await session.open()
        let thread = await session.thread(parentId: "root-1")
        await thread.open()
        return thread
    }

    func testAReplySentWithTheSwitchOnReachesTheChannelAsAThreadReply() async {
        let session = ChatSession.sample(ChatSampleScenario(threaded: true)) { Date(timeIntervalSince1970: 1_000) }
        let thread = await openedThread(session)

        _ = await thread.send(body: "agree", alsoInChannel: true)

        let reply = await channel(session).first { $0.body == "agree" }
        XCTAssertEqual(reply?.parentId, "root-1")
        XCTAssertEqual(reply?.alsoInChannel, true)
    }

    func testAReplySentWithTheSwitchOffStaysOutOfTheChannel() async {
        let session = ChatSession.sample(ChatSampleScenario(threaded: true)) { Date(timeIntervalSince1970: 1_000) }
        let thread = await openedThread(session)

        _ = await thread.send(body: "thread only", alsoInChannel: false)

        let messages = await channel(session)
        XCTAssertNil(messages.first { $0.body == "thread only" })
        XCTAssertEqual(messages.first { $0.id == "root-1" }?.replyCount, 1)
    }
}
#endif
