import Foundation
import Testing

@testable import Monaco

@MainActor
struct CallBoardDockedTests {
    @Test func theSharePayloadCarriesTheStandingAndTheInviteLink() throws {
        let url = try #require(URL(string: "https://monaco.xyz/i/abc"))
        let payload = CallBoardSharePayload.make(
            standing: "I'm #8 of 24 on Monaco this month · 78% right calls", inviteURL: url)
        #expect(payload.title == "I'm #8 of 24 on Monaco this month · 78% right calls")
        #expect(payload.url == url)
        #expect(payload.image == nil)
    }
}
