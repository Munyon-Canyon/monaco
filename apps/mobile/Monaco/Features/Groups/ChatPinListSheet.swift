import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatPinListSheet: View {
    let pins: ChatPinsModel
    let cabalName: String
    let viewerID: String
    let controls: ChatPinControls
    let open: (String) -> Void

    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 0) {
                    ForEach(pins.pins, id: \.id) { message in
                        row(message)
                    }
                }
            }
            .monacoSheet(title: ChatPinCopy.sheetTitle, titleIdentifier: "chat-pin-list-title", detents: .votes)
            .toolbar(.hidden, for: .navigationBar)
        }
        .accessibilityIdentifier("chat-pin-list")
    }

    private func row(_ message: ChatMessage) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Button {
                open(message.id)
            } label: {
                content(message)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("chat-pin-row-\(message.id)")
            if controls.canPin(message) {
                Button(ChatPinCopy.unpin) { controls.unpin(message, toasts: toasts) }
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .buttonStyle(.plain)
                    .frame(minHeight: ChatPinMetrics.minTarget)
                    .accessibilityIdentifier("chat-pin-unpin-\(message.id)")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
    }

    private func content(_ message: ChatMessage) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            MonacoAvatar(
                photoURL: message.author.photoUrl, displayName: message.author.displayName, size: 36,
                seed: message.author.id
            )
            .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Text(ChatPinCopy.firstName(message.author.displayName))
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(message.createdAt.formatted(.dateTime.month(.abbreviated).day().hour().minute()))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                Text(ChatPinCopy.text(of: message, cabalName: cabalName, viewerID: viewerID))
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(ChatPinMetrics.listRowLines)
                    .multilineTextAlignment(.leading)
            }
        }
    }
}
