import SwiftUI

extension MonacoAvatar {
    struct Source: Identifiable, Equatable {
        let id: String
        let displayName: String
        let photoURL: String?
    }
}

struct VoteSidesRow: View {
    struct Line: Equatable {
        let side: VoteSide?
        let avatars: [MonacoAvatar.Source]
        let text: String
    }

    let lines: [Line]
    let identifierPrefix: String
    var identifierSuffix: String? = nil

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .subheadline) private var glyphSize: CGFloat = 17

    static let avatarSize: CGFloat = 32
    static let avatarOverlap: CGFloat = 10
    static let maxAvatars = 5

    static func spokenLabel(_ line: Line) -> String {
        switch line.side {
        case .yes: return "Voted yes: \(line.text)."
        case .no: return "Voted no: \(line.text)."
        case nil: return "\(line.text)."
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Rectangle().fill(MonacoTheme.hairline).frame(height: 1)
            ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                row(line)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-sides", identifierSuffix))
    }

    private func row(_ line: Line) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            HStack(spacing: MonacoTheme.Space.sm) {
                glyph(line.side)
                avatarStack(line.avatars)
            }
            Text(line.text)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(line.side == nil ? MonacoTheme.secondaryText : MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(line))
    }

    private func glyph(_ side: VoteSide?) -> some View {
        let symbol: String
        let color: Color
        switch side {
        case .yes: (symbol, color) = ("checkmark", MonacoTheme.profit)
        case .no: (symbol, color) = ("xmark", MonacoTheme.loss)
        case nil: (symbol, color) = ("minus", MonacoTheme.tertiaryText)
        }
        return Image(systemName: symbol)
            .resizable()
            .scaledToFit()
            .fontWeight(.semibold)
            .foregroundStyle(color)
            .frame(width: glyphSize, height: glyphSize)
    }

    private func avatarStack(_ avatars: [MonacoAvatar.Source]) -> some View {
        HStack(spacing: -Self.avatarOverlap) {
            ForEach(avatars.prefix(Self.maxAvatars)) { source in
                MonacoAvatar(
                    photoURL: source.photoURL, displayName: source.displayName, size: Self.avatarSize,
                    seed: source.id
                )
                .overlay(Circle().strokeBorder(MonacoTheme.canvas, lineWidth: MonacoAvatarRing.width))
            }
        }
    }
}
