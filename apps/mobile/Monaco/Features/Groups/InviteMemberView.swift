import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct InviteMemberRoute: AppRoute {
    let cabalID: String
    let standing: CabalInviteStanding

    @MainActor func destination() -> some View {
        InviteMemberView(cabalID: cabalID, standing: standing)
    }
}

struct InviteMemberView: View {
    let cabalID: String
    let standing: CabalInviteStanding
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: InviteMemberModel?

    var body: some View {
        Group {
            if let model {
                InviteMemberContent(model: model)
            } else {
                Color.clear
            }
        }
        .navigationTitle("Invite someone")
        .navigationBarTitleDisplayMode(.inline)
        .monacoCanvas()
        .task { await start() }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
        }
    }

    private func start() async {
        let model = preparedModel()
        if case .idle = model.state { await model.load() }
        await model.observe()
    }

    private func preparedModel() -> InviteMemberModel {
        if let model { return model }
        let created = InviteMemberModel(
            cabalID: cabalID, standing: standing, viewerID: environment.viewer?.userID,
            api: environment.api, hints: environment.hints, now: { Date.now }
        )
        model = created
        return created
    }
}

private struct InviteMemberContent: View {
    @Bindable var model: InviteMemberModel
    @FocusState private var fieldFocused: Bool

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                form
                pending
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .scrollDismissesKeyboard(.interactively)
        .refreshable { await model.load() }
    }

    private var form: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.xs) {
                Text("@")
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityHidden(true)
                TextField(
                    "",
                    text: $model.handle,
                    prompt: Text("handle").foregroundStyle(MonacoTheme.disabledLabel)
                )
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .tint(MonacoTheme.ink)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .submitLabel(.send)
                .focused($fieldFocused)
                .onSubmit { send() }
                .accessibilityLabel("Handle")
                .accessibilityIdentifier("invite-member-handle-field")
            }
            .monacoFieldChrome(isFocused: fieldFocused)
            .contentShape(Rectangle())
            .onTapGesture { fieldFocused = true }

            Button(action: send) {
                if model.isSending {
                    ProgressView()
                        .tint(MonacoTheme.primaryButtonLabel)
                } else {
                    Text("Send")
                }
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canSend)
            .accessibilityIdentifier("invite-member-send-button")
        }
    }

    @ViewBuilder
    private var pending: some View {
        switch model.state {
        case .idle, .loading:
            ProgressView()
                .frame(maxWidth: .infinity)
                .padding(.vertical, MonacoTheme.Space.l)
        case .failed(let error):
            EmptyState(
                title: "Pending invites didn't load",
                message: ToastCopy.message(for: error),
                actionTitle: "Try again"
            ) {
                Task { await model.load() }
            }
            .accessibilityIdentifier("invite-member-pending-failed")
        case .loaded(let invites):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Pending invites", count: invites.count)
                if invites.isEmpty {
                    Text("No pending invites.")
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityIdentifier("invite-member-pending-empty")
                } else {
                    MonacoGroupedList {
                        ForEach(Array(invites.enumerated()), id: \.element.id) { index, invite in
                            PendingInviteRow(
                                invite: invite,
                                isLast: index == invites.count - 1,
                                isRevoking: model.revoking.contains(invite.id)
                            ) {
                                Task { await model.revoke(invite) }
                            }
                        }
                    }
                }
            }
        }
    }

    private func send() {
        guard model.canSend else { return }
        fieldFocused = false
        Task { await model.send() }
    }
}

private struct PendingInviteRow: View {
    let invite: SentCabalInvite
    let isLast: Bool
    let isRevoking: Bool
    let revoke: () -> Void

    var body: some View {
        MonacoRow(
            title: invite.invitee,
            subtitle: "\(invite.invitedBy), \(invite.expiry)",
            isLast: isLast
        ) {
            Image(systemName: "envelope")
                .font(.title3)
                .foregroundStyle(MonacoTheme.muted)
                .frame(width: 44, height: 44)
                .accessibilityHidden(true)
        } trailing: {
            if invite.canRevoke {
                revokeLabel.hidden()
            }
        }
        .overlay(alignment: .trailing) {
            if invite.canRevoke {
                Button(action: revoke) {
                    if isRevoking {
                        ProgressView()
                    } else {
                        revokeLabel
                    }
                }
                .buttonStyle(.plain)
                .frame(minWidth: 44, minHeight: 44)
                .contentShape(Rectangle())
                .disabled(isRevoking)
                .padding(.trailing, MonacoTheme.Space.m)
                .accessibilityLabel("Revoke the invite to \(invite.invitee)")
                .accessibilityIdentifier("invite-member-revoke-button")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("invite-member-pending-row")
    }

    private var revokeLabel: some View {
        Text("Revoke")
            .font(MonacoTheme.Typo.calloutStrong)
            .foregroundStyle(MonacoTheme.destructive)
    }
}
