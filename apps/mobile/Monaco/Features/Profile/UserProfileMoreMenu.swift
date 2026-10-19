import MonacoAPI
import MonacoCore
import SwiftUI

struct UserProfileMoreMenu: View {
    enum SafetyAction: CaseIterable {
        case report
        case block
        case unblock

        var title: String {
            switch self {
            case .report: "Report"
            case .block: "Block"
            case .unblock: "Unblock"
            }
        }

        var systemImage: String {
            switch self {
            case .report: "exclamationmark.bubble"
            case .block: "nosign"
            case .unblock: "person.crop.circle.badge.checkmark"
            }
        }

        var role: ButtonRole? { self == .block ? .destructive : nil }
    }

    @Environment(UserProfileModel.self) private var profile: UserProfileModel?
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var reporting: ReportTarget?
    @State private var confirmingBlock = false
    @State private var actions: Flow20aReportBlockModel?

    private var handle: String { profile?.handle ?? "" }
    private var blockPrompt: String { handle.isEmpty ? "Block this member?" : "Block @\(handle)?" }

    var body: some View {
        menu
            .reportSheet($reporting)
            .confirmationDialog(blockPrompt, isPresented: $confirmingBlock, titleVisibility: .visible) {
                Button(SafetyAction.block.title, role: .destructive) { Task { await run(.block) } }
                    .accessibilityIdentifier("user-profile-block-confirm")
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("They can't follow you, and you won't see their messages or comments.")
            }
    }

    private func run(_ action: SafetyAction) async {
        let flow = actions ?? Flow20aReportBlockModel(api: environment.api, hints: environment.hints)
        actions = flow
        guard let profile else { return }
        await UserProfileBlockRunner.run(action == .block, flow: flow, model: profile, toasts: toasts)
    }

    private var menu: some View {
        Menu {
            Button {
                if let profile { reporting = ReportTarget(kind: .user, targetID: profile.userID) }
            } label: {
                Label(SafetyAction.report.title, systemImage: SafetyAction.report.systemImage)
            }
            .accessibilityIdentifier("user-profile-report")
            if profile?.blockedByMe == true {
                Button {
                    Task { await run(.unblock) }
                } label: {
                    Label(SafetyAction.unblock.title, systemImage: SafetyAction.unblock.systemImage)
                }
                .accessibilityIdentifier("user-profile-unblock")
            } else {
                Button(role: SafetyAction.block.role) {
                    confirmingBlock = true
                } label: {
                    Label(SafetyAction.block.title, systemImage: SafetyAction.block.systemImage)
                }
                .accessibilityIdentifier("user-profile-block")
            }
        } label: {
            Image(systemName: "ellipsis")
                .monacoToolbarIcon()
                .frame(minWidth: 44, minHeight: 44)
        }
        .accessibilityLabel("More")
        .accessibilityIdentifier("user-profile-more")
    }
}

enum UserProfileBlockRunner {
    @MainActor static func run(
        _ block: Bool, flow: Flow20aReportBlockModel, model: UserProfileModel, toasts: ToastCenter
    ) async {
        let handle = model.handle
        let outcome = block ? await flow.block(userID: model.userID) : await flow.unblock(userID: model.userID)
        let message = flow.toast(for: outcome, after: block ? .block(handle: handle) : .unblock(handle: handle))
        if outcome == .ok {
            toasts.show(success: message)
            await model.refresh()
        } else {
            toasts.current = MonacoToast(message: message)
        }
    }
}
