#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatThreadSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatThreadSampleQA")
    }

    @MainActor static func screen() -> some View {
        ChatThreadSampleHost(session: ChatSession.sample(ChatSampleScenario(threaded: true)) { Date() })
    }
}

private struct ChatThreadSampleHost: View {
    let session: ChatSession

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
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard ChatThreadSampleQA.matches(arguments) else { return nil }
        let route = SampleScreenRoute(screen: .chatThread, arguments: arguments)
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [route]))
    }
}
#endif
