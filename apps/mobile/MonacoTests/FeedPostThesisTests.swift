import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct FeedPostThesisTests {
    private static let oneLine = "Small add before earnings."
    private static let fourLines = String(
        repeating: "Earnings next week and I want more before, so we can add on a dip. ", count: 6)

    private static func proposal(body: String?) throws -> Components.Schemas.FeedItem {
        var item = try #require(Components.Schemas.FeedItem.samples.first { $0.kind == "proposal" })
        item.body = body
        return item
    }

    private static func height(of item: Components.Schemas.FeedItem) throws -> CGFloat {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), hints: FakeHintSource(),
            isAuthenticated: { true }, endAuthSession: {})
        let host = UIHostingController(
            rootView: FeedPost(item: item, now: Date(), opensPost: false) { EmptyView() }
                .environment(environment)
                .environment(ToastCenter())
        )
        let window = UIWindow(windowScene: scene)
        window.rootViewController = host
        window.makeKeyAndVisible()
        defer { TestWindow.close(window) }
        return host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func aProposalShowsItsTrimmedThesis() throws {
        let item = try Self.proposal(body: "  \(Self.oneLine)\n")
        #expect(FeedPostThesis.text(of: item) == Self.oneLine)
    }

    @Test func aProposalWithoutWordsShowsNoThesis() throws {
        for body in [nil, "", "  \n "] as [String?] {
            #expect(FeedPostThesis.text(of: try Self.proposal(body: body)) == nil)
        }
    }

    @Test func onlyAProposalShowsAThesis() throws {
        var item = try Self.proposal(body: Self.oneLine)
        item.kind = "reply"
        #expect(FeedPostThesis.text(of: item) == nil)
    }

    @Test func theThesisIsReadAfterTheHeader() {
        #expect(
            FeedPostThesis.spoken("Mia proposed, 2h", thesis: Self.oneLine, joint: ".")
                == "Mia proposed, 2h. \(Self.oneLine)")
        #expect(FeedPostThesis.spoken("Mia proposed, 2h", thesis: nil, joint: ".") == "Mia proposed, 2h")
    }

    @Test func aFourLineThesisStopsAtThreeLines() throws {
        let none = try Self.height(of: Self.proposal(body: nil))
        let one = try Self.height(of: Self.proposal(body: Self.oneLine))
        let four = try Self.height(of: Self.proposal(body: Self.fourLines))
        let capped = try Self.height(of: Self.proposal(body: Self.fourLines + Self.fourLines))
        #expect(one > none)
        #expect(four > one)
        #expect(capped == four)
    }
}
