#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatThreadSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatThreadSampleQA")
    }

    @MainActor static func rootView() -> some View {
        ChatThreadSampleHost(session: ChatSession.threadSample { Date() })
    }
}

private struct ChatThreadSampleHost: View {
    let session: ChatSession

    @State private var path: [String] = []

    var body: some View {
        NavigationStack(path: $path) {
            GroupChatScreen(
                cabalID: ChatSession.sampleCabalID,
                session: session,
                cabal: .sample(role: "member"),
                openProfile: { _ in },
                openThread: { path.append($0) }
            )
            .navigationDestination(for: String.self) { parentID in
                ChatThreadSampleScreen(session: session, parentID: parentID)
            }
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
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard ChatThreadSampleQA.matches(arguments) else { return nil }
        return AnyView(ChatThreadSampleQA.rootView())
    }
}
#endif
