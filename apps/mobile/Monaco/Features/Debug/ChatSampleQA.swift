#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatSampleQA")
    }

    @MainActor static func screen(arguments: [String]) -> some View {
        var scenario = ChatSampleScenario(
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
            poll: arguments.contains("-MonacoChatSamplePoll"),
            pins: arguments.contains("-MonacoChatSamplePins")
        )
        let openVote = arguments.contains("-MonacoChatSampleOpenVote")
        let openVoteClosed = arguments.contains("-MonacoChatSampleOpenVoteClosed")
        if openVote || openVoteClosed { scenario = ChatSampleScenario(busy: true, proposalCard: true, pins: true) }
        if scenario == ChatSampleScenario() { scenario.pins = true }
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        let offline = arguments.contains("-MonacoChatSampleOffline")
        return OpensOnceActive {
            ChatSampleHost(scenario: scenario, acts: acts, offline: offline, arguments: arguments)
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
    @State private var openVotes: ChatOpenVotesModel?
    @State private var network: NetworkPathModel
    @State private var reactions = ReactionsModel.sample(reactions: [:], fails: false, clock: ContinuousClock())
    private let sendsOnLaunch: Bool
    private let scrollsPastCard: Bool
    private let arguments: [String]
    private let cabal: Components.Schemas.Cabal

    init(scenario: ChatSampleScenario, acts: Bool, offline: Bool, arguments: [String]) {
        self.arguments = arguments
        sendsOnLaunch =
            acts
            && (scenario.failSends || scenario.failFirstSend || scenario.closedOnSend || scenario.neverPosts)
        scrollsPastCard = acts && arguments.contains { $0.hasPrefix("-MonacoChatSampleOpenVote") }
        cabal = Self.cabal(startEmpty: scenario.startEmpty)
        _session = State(initialValue: ChatSession.sample(scenario) { Date() })
        _network = State(initialValue: NetworkPathModel(forcedOffline: offline))
        let closed = arguments.contains("-MonacoChatSampleOpenVoteClosed")
        if closed || arguments.contains("-MonacoChatSampleOpenVote") {
            _openVotes = State(
                initialValue: ChatOpenVotesModel(cabalID: ChatSession.sampleCabalID) { _ in
                    (items: Self.sampleVotes(closed: closed), nextCursor: nil)
                })
        }
    }

    private static let catchUpLines: [String] = [
        "I'd put the NVDA vote through before the print, who is in?",
        "Reading the thread now, the thesis holds up for me.",
        "Same here. Voting yes in a minute.",
        "Anyone seen the guidance from last quarter?",
        "Up eight percent on the call, so a small add looks fine.",
        "Fine by me, as long as the pot keeps its cash buffer.",
        "It does, the buffer stays above ten percent.",
        "Then I'm in. Voting now.",
    ]

    private nonisolated static func sampleVotes(closed: Bool) -> [ProposalSummary] {
        var sell = Components.Schemas.Proposal.sample(kind: .sell)
        sell.id = "00000000-0000-4000-8000-000000000502"
        sell.symbol = "AAPLx"
        sell.tokenAmount = 6_000_000
        sell.tally = .init(yes: 2, no: 0, voters: 8, needed: 5)
        sell.canVote = true
        sell.replyCount = 1
        sell.expiresAt = Date().addingTimeInterval(5 * 3_600)
        guard !closed else { return [ProposalSummary(sell)] }
        var buy = Components.Schemas.Proposal.sample()
        buy.id = ChatSession.sampleProposalID
        buy.symbol = "NVDAx"
        buy.usdcMicros = 400_000_000
        buy.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        buy.canVote = true
        buy.replyCount = 6
        buy.expiresAt = Date().addingTimeInterval(4 * 3_600)
        return [ProposalSummary(buy), ProposalSummary(sell)]
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
            openVotes: openVotes,
            network: network,
            gifs: SampleGifProvider(),
            openProfile: { openedProfile = $0 },
            openThread: { _ in },
            reactionsOverride: reactions
        )
        .environment(\.stockSearch, Self.stockSearch)
        .onAppear { SampleHarnessRegistry.markDrawn(arguments) }
        .task {
            guard scrollsPastCard, !sent else { return }
            sent = true
            for line in Self.catchUpLines { await session.send(body: line) }
        }
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
    @MainActor override class var marksOwnDraw: Bool { true }

    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard ChatSampleQA.matches(arguments) else { return nil }
        return AnyView(
            SampleAppFrame(auth: auth, tab: .cabals, root: { AnyView(ChatSampleQA.screen(arguments: arguments)) }))
    }
}
#endif
