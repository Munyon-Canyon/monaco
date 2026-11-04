#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatSampleQA")
    }

    @MainActor static func screen(arguments: [String]) -> some View {
        let scenario = ChatSampleScenario(
            startEmpty: arguments.contains("-MonacoChatSampleEmpty"),
            failSends: arguments.contains("-MonacoChatSampleFailed"),
            busy: arguments.contains("-MonacoChatSampleBusy"),
            closedFirstLoad: arguments.contains("-MonacoChatSampleClosedFirstLoad"),
            neverAnswers: arguments.contains("-MonacoChatSampleLoading"),
            failFirstSend: arguments.contains("-MonacoChatSampleFlaky"),
            closedOnSend: arguments.contains("-MonacoChatSampleClosedOnSend"),
            neverPosts: arguments.contains("-MonacoChatSampleSending"),
            proposalCard: arguments.contains("-MonacoChatSampleProposal"),
            moments: arguments.contains("-MonacoChatSampleMoments"),
            poll: arguments.contains("-MonacoChatSamplePoll")
        )
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        let offline = arguments.contains("-MonacoChatSampleOffline")
        return OpensOnceActive {
            ChatSampleHost(scenario: scenario, acts: acts, offline: offline)
        }
    }
}

private struct ChatSampleHost: View {
    @State private var session: ChatSession
    @State private var openedProfile: String?
    @State private var sent = false
    @State private var proposals = ChatProposalStore { id in
        var proposal = Components.Schemas.Proposal.sample()
        proposal.id = id
        proposal.symbol = "NVDAx"
        proposal.usdcMicros = 400_000_000
        proposal.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        proposal.canVote = true
        proposal.replyCount = 6
        proposal.expiresAt = Date().addingTimeInterval(4 * 3_600)
        return ChatProposalFacts(summary: ProposalSummary(proposal), asset: nil)
    }
    @State private var network: NetworkPathModel
    @State private var reactions = ReactionsModel.sample(reactions: [:], fails: false, clock: ContinuousClock())
    private let sendsOnLaunch: Bool
    private let cabal: Components.Schemas.Cabal

    init(scenario: ChatSampleScenario, acts: Bool, offline: Bool) {
        sendsOnLaunch =
            acts
            && (scenario.failSends || scenario.failFirstSend || scenario.closedOnSend || scenario.neverPosts)
        cabal = Self.cabal(startEmpty: scenario.startEmpty)
        _session = State(initialValue: ChatSession.sample(scenario) { Date() })
        _network = State(initialValue: NetworkPathModel(forcedOffline: offline))
    }

    private static func cabal(startEmpty: Bool) -> Components.Schemas.Cabal {
        guard startEmpty else { return .sample(role: "member") }
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        cabal.name = "Big Ballers"
        cabal.openProposalCount = 0
        return cabal
    }

    private static let stockSearch: StockSearch = { _ in
        [
            Components.Schemas.AssetSummary(
                id: "01890a5d-ac96-774b-bcce-b302099a8070",
                symbol: "AAPLx", displayName: "Apple", issuer: .xstocks, kind: .equity, logoUrl: nil,
                priceMicros: 180_000_000, priceAsOf: Date(timeIntervalSince1970: 0), changeBps: -80,
                sparklineMicros: [181_000_000, 180_000_000], quotable: true, session: .open, tradable: true)
        ]
    }

    var body: some View {
        GroupChatScreen(
            cabalID: ChatSession.sampleCabalID,
            session: session,
            cabal: cabal,
            proposals: proposals,
            network: network,
            gifs: SampleGifProvider(),
            openProfile: { openedProfile = $0 },
            openThread: { _ in },
            reactionsOverride: reactions
        )
        .environment(\.stockSearch, Self.stockSearch)
        .task {
            guard sendsOnLaunch, !sent else { return }
            sent = true
            await session.send(body: "Anyone looking at NVDA before Friday?")
        }
        .overlay(alignment: .top) {
            if let openedProfile {
                Text("Opened profile \(openedProfile)")
                    .font(MonacoTheme.Typo.caption)
                    .accessibilityIdentifier("chat-sample-opened-profile")
            }
        }
    }
}

private struct OpensOnceActive<Content: View>: View {
    @ViewBuilder let content: () -> Content

    @Environment(\.scenePhase) private var scenePhase
    @State private var isOpen = false

    var body: some View {
        Group {
            if isOpen {
                content()
            } else {
                MonacoTheme.canvas.ignoresSafeArea()
            }
        }
        .onChange(of: scenePhase, initial: true) { _, phase in
            if phase == .active { isOpen = true }
        }
    }
}

final class ChatSampleQAEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard ChatSampleQA.matches(arguments) else { return nil }
        return AnyView(
            SampleAppFrame(auth: auth, tab: .cabals, routes: [SampleScreenRoute(screen: .chat, arguments: arguments)]))
    }
}
#endif
