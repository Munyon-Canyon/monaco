import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatPinListSheet: View {
    let pins: ChatPinsModel
    let cabalName: String
    let canPin: (ChatMessage) -> Bool
    let select: (String) -> Void
    let unpin: (ChatMessage) -> Void

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            ForEach(pins.pins, id: \.id) { pin in
                row(pin)
            }
        }
        .padding(.top, MonacoTheme.Space.s)
        .monacoSheet(title: "Pinned", detents: .review, showsClose: true)
        .accessibilityIdentifier("chat-pin-list")
        .onChange(of: pins.pins.isEmpty) { _, empty in
            if empty { dismiss() }
        }
    }

    private func row(_ pin: ChatMessage) -> some View {
        let line = pins.capsule(for: pin, cabalName: cabalName)
        return HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Button {
                dismiss()
                select(pin.id)
            } label: {
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    MonacoAvatar(
                        photoURL: pin.author.photoUrl, displayName: pin.author.displayName,
                        size: ChatBubbleMetrics.avatarSize, seed: pin.author.id)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        if !line.name.isEmpty {
                            Text(pin.author.displayName)
                                .font(MonacoTheme.Typo.headline)
                                .foregroundStyle(MonacoTheme.primaryText)
                        }
                        Text(line.text)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .lineLimit(3)
                            .multilineTextAlignment(.leading)
                        Text(pin.createdAt, format: .dateTime.month().day().hour().minute())
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            if canPin(pin) {
                Button("Unpin") { unpin(pin) }
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.tint)
                    .frame(minHeight: ChatPinMetrics.hitHeight)
                    .accessibilityIdentifier("chat-pin-unpin-\(pin.id)")
            }
        }
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: ChatPinMetrics.hitHeight)
        .accessibilityIdentifier("chat-pin-row-\(pin.id)")
    }
}
