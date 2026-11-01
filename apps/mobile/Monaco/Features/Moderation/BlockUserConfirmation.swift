import MonacoCore
import SwiftUI

struct BlockTarget: Identifiable, Equatable {
    let userID: String
    let name: String
    var handle = ""

    var id: String { userID }

    var prompt: String { name.isEmpty ? "Block this member?" : "Block \(name)?" }
}

private struct BlockUserConfirmation: ViewModifier {
    @Binding var target: BlockTarget?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var actions: Flow20aReportBlockModel?

    func body(content: Content) -> some View {
        content.confirmationDialog(
            target?.prompt ?? "Block this member?",
            isPresented: Binding(get: { target != nil }, set: { if !$0 { target = nil } }),
            titleVisibility: .visible,
            presenting: target
        ) { person in
            Button("Block", role: .destructive) { Task { await block(person) } }
                .accessibilityIdentifier("block-confirm")
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text("They can't follow you, and you won't see their posts, messages or comments.")
        }
    }

    private func block(_ person: BlockTarget) async {
        let flow = actions ?? Flow20aReportBlockModel(api: environment.api, hints: environment.hints)
        actions = flow
        let outcome = await flow.block(userID: person.userID)
        let message = flow.toast(for: outcome, after: .block(handle: person.handle))
        if outcome == .ok {
            toasts.show(success: message)
        } else {
            toasts.current = MonacoToast(message: message)
        }
    }
}

extension View {
    func blockConfirmation(_ target: Binding<BlockTarget?>) -> some View {
        modifier(BlockUserConfirmation(target: target))
    }
}
