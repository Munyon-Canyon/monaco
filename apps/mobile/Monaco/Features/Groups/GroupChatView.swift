import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var session: ChatSession?
    @State private var cabal: CabalModel?
    @State private var reporter: ChatSeenReporter?

    var body: some View {
        GroupChatScreen(
            cabalID: cabalID,
            session: session,
            cabal: cabal?.cabal,
            reporter: reporter,
            openProfile: { userID in
                environment.navigator.open(UserProfileRoute(userID: userID), in: environment.navigator.selectedTab)
            },
            openThread: { parentID in
                environment.navigator.open(
                    ChatThreadRoute(cabalID: cabalID, parentID: parentID), in: environment.navigator.selectedTab)
            }
        )
        .task {
            let model = preparedCabal()
            _ = preparedSession()
            await model.load()
        }
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
            realtime: AblyChatRealtime(api: environment.api),
            now: { Date() },
            onArrival: { await seen.messageArrived() }
        )
        ChatSessionRegistry.register(created)
        session = created
        return created
    }
}
