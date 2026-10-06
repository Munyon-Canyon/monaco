#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatSampleQA {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoChatSampleQA")
    }

    @MainActor static func rootView(arguments: [String]) -> some View {
        let scenario = ChatSampleScenario(
            startEmpty: arguments.contains("-MonacoChatSampleEmpty"),
            failSends: arguments.contains("-MonacoChatSampleOffline"),
            busy: arguments.contains("-MonacoChatSampleBusy"),
            closedFirstLoad: arguments.contains("-MonacoChatSampleClosedFirstLoad"),
            neverAnswers: arguments.contains("-MonacoChatSampleLoading"),
            failFirstSend: arguments.contains("-MonacoChatSampleFlaky"),
            closedOnSend: arguments.contains("-MonacoChatSampleClosedOnSend")
        )
        return NavigationStack {
            OpensOnceActive {
                ChatSampleHost(session: ChatSession.sample(scenario) { Date() })
            }
        }
    }
}

private struct ChatSampleHost: View {
    let session: ChatSession

    @State private var openedProfile: String?

    var body: some View {
        GroupChatScreen(
            cabalID: ChatSession.sampleCabalID,
            session: session,
            cabal: .sample(role: "member"),
            openProfile: { openedProfile = $0 }
        )
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
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard ChatSampleQA.matches(arguments) else { return nil }
        return AnyView(ChatSampleQA.rootView(arguments: arguments))
    }
}
#endif
