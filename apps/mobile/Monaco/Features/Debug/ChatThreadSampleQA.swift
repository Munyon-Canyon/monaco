#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatThreadSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatThreadSampleQA")
    }

    static func opensProposal(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatThreadSampleProposal")
    }

    @MainActor @ViewBuilder static func screen(arguments: [String]) -> some View {
        if opensProposal(arguments) {
            ChatThreadSampleProposalHost(
                session: ChatSession.sample(ChatSampleScenario(proposalThread: true)) { Date() },
                arguments: arguments)
        } else {
            ChatThreadSampleHost(
                session: ChatSession.sample(ChatSampleScenario(threaded: true)) { Date() }, arguments: arguments)
        }
    }
}

private struct ChatThreadSampleProposalHost: View {
    let session: ChatSession
    let arguments: [String]

    @State private var thread: ThreadSession?
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

    var body: some View {
        ChatThreadScreen(
            thread: thread, cabalName: "AC Capital", proposals: proposals,
            deleteMessage: { _ in nil }, openProfile: { _ in }, openProposal: { _ in }
        )
        .task { thread = await session.thread(parentId: ChatSession.sampleThreadRootID) }
        .onAppear { SampleHarnessRegistry.markDrawn(arguments) }
    }
}

private struct ChatThreadSampleHost: View {
    let session: ChatSession
    let arguments: [String]

    @State private var openedThread: String?

    var body: some View {
        GroupChatScreen(
            cabalID: ChatSession.sampleCabalID,
            session: session,
            cabal: .sample(role: "member"),
            openProfile: { _ in },
            openThread: { openedThread = $0 }
        )
        .navigationDestination(item: $openedThread) { parentID in
            ChatThreadSampleScreen(session: session, parentID: parentID)
        }
        .onAppear { SampleHarnessRegistry.markDrawn(arguments) }
    }
}

private struct ChatThreadSampleScreen: View {
    let session: ChatSession
    let parentID: String

    @State private var thread: ThreadSession?

    var body: some View {
        ChatThreadScreen(thread: thread, deleteMessage: { _ in nil }, openProfile: { _ in })
            .task { thread = await session.thread(parentId: parentID) }
    }
}

final class ChatThreadSampleQAEntry: SampleHarnessEntry {
    @MainActor override class var marksOwnDraw: Bool { true }

    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard ChatThreadSampleQA.matches(arguments) else { return nil }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: .cabals, root: { AnyView(ChatThreadSampleQA.screen(arguments: arguments)) }))
    }
}
#endif
