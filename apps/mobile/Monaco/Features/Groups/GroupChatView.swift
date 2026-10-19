import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

struct GroupChatView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var session: ChatSession?
    @State private var cabal: CabalModel?
    @State private var reporter: ChatSeenReporter?
    @State private var proposals: ChatProposalStore?
    @State private var network = NetworkPathModel()

    var body: some View {
        GroupChatScreen(
            cabalID: cabalID,
            session: session,
            cabal: cabal?.cabal,
            reporter: reporter,
            viewerID: environment.viewer?.userID ?? "",
            proposals: preparedProposals(),
            network: network,
            openProfile: { userID in
                environment.navigator.open(UserProfileRoute(userID: userID), in: environment.navigator.selectedTab)
            },
            openThread: { parentID in
                environment.navigator.open(
                    ChatThreadRoute(cabalID: cabalID, parentID: parentID), in: environment.navigator.selectedTab)
            },
            openProposal: { proposalID in
                environment.navigator.open(
                    ProposalRoute(proposalID: proposalID), in: environment.navigator.selectedTab)
            }
        )
        .analyticsScreen("chat", step: .chat(.chatOpened))
        .task {
            let model = preparedCabal()
            _ = preparedSession()
            await model.load()
        }
    }

    private func preparedProposals() -> ChatProposalStore {
        if let proposals { return proposals }
        let created = ChatProposalStore(api: environment.api)
        proposals = created
        return created
    }

    private func preparedCabal() -> CabalModel {
        if let cabal { return cabal }
        let created = CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        cabal = created
        return created
    }

    private func preparedSession() -> ChatSession {
        if let session { return session }
        let seen = ChatSeenReporter(cabalID: cabalID, api: environment.api, clock: ContinuousClock())
        reporter = seen
        let created = ChatSession(
            cabalID: cabalID,
            viewerID: environment.viewer?.userID ?? "",
            api: environment.api,
            realtime: environment.chatRealtime,
            now: { Date() },
            onArrival: { await seen.messageArrived() },
            blocked: BlockedAuthors(api: environment.api)
        )
        ChatSessionRegistry.register(created)
        session = created
        return created
    }
}

private final class ChatDetailsModels {
    private var cabal: CabalModel?
    private var pot: CabalPotModel?

    func prepared(cabalID: String, environment: AppEnvironment) -> (CabalModel, CabalPotModel) {
        let cabal = self.cabal ?? CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        let pot =
            self.pot
            ?? CabalPotModel(
                cabalID: cabalID, api: environment.api, hints: environment.hints, logoStore: environment.assetLogos)
        self.cabal = cabal
        self.pot = pot
        return (cabal, pot)
    }
}

struct ChatCabalDetailsSheet: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var models = ChatDetailsModels()

    var body: some View {
        let (cabal, pot) = models.prepared(cabalID: cabalID, environment: environment)
        NavigationStack {
            SectionStack(
                context: CabalContext(cabalID: cabalID), sections: CabalScreen.detailsSections.map { $0.erased }
            )
            .environment(\.cabalModel, cabal)
            .environment(\.cabalPotModel, pot)
            .monacoSheet(title: "Cabal details")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("chat-details-done")
                }
            }
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .presentationDetents([.medium, .large])
        .monacoToastCenter(toasts)
        .task {
            async let loadedCabal: Void = cabal.load()
            async let loadedPot: Void = pot.load()
            _ = await (loadedCabal, loadedPot)
        }
    }
}
