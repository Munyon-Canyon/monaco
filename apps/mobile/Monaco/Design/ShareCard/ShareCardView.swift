import MonacoCore
import SwiftUI

struct ShareCardView: View {
    let content: ShareCardContent
    let face: UIImage?

    static let size = CGSize(width: 360, height: 450)

    private typealias Palette = MonacoTheme.ShareCard

    private var tint: Color {
        switch content.direction {
        case .up: Palette.profit
        case .down: Palette.loss
        case .flat: Palette.muted
        }
    }

    var body: some View {
        VStack(spacing: 12) {
            header
            ShareCardCurve(points: content.points, tint: tint)
                .frame(maxHeight: .infinity)
            panel
            stats
            footer
        }
        .padding(20)
        .frame(width: Self.size.width, height: Self.size.height)
        .background(Palette.canvas)
    }

    private var header: some View {
        HStack(alignment: .center, spacing: 12) {
            ShareCardFace(
                image: face, name: content.faceName, seed: content.faceSeed, size: 44,
                shape: content.kind == .me ? .circle : .tile)
            VStack(alignment: .leading, spacing: 4) {
                Text(content.title)
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundStyle(Palette.ink)
                    .lineLimit(1)
                if !content.subtitle.isEmpty {
                    Text(content.subtitle)
                        .font(.system(size: 13))
                        .foregroundStyle(Palette.muted)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            if let dateText = content.dateText {
                Text(dateText)
                    .font(.system(size: 12))
                    .foregroundStyle(Palette.muted)
                    .lineLimit(1)
                    .fixedSize()
            }
        }
    }

    private var panel: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("All time")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundStyle(Palette.muted)
                Spacer(minLength: 8)
                Text(content.status)
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(Palette.ink)
                    .lineLimit(1)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 4)
                    .background(Palette.canvas, in: Capsule())
            }
            Text(content.gain)
                .font(.system(size: 34, weight: .bold))
                .monospacedDigit()
                .foregroundStyle(tint)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
            returnLine
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 16)
        .padding(.top, 8)
        .padding(.bottom, 12)
        .background(Palette.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
    }

    @ViewBuilder
    private var returnLine: some View {
        let magnitude = content.returnText.trimmingCharacters(in: CharacterSet(charactersIn: "+\u{2212}"))
        switch content.direction {
        case .up: line("▲ \(magnitude)")
        case .down: line("▼ \(magnitude)")
        case .flat: line(magnitude)
        }
    }

    private func line(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 15, weight: .semibold))
            .monospacedDigit()
            .foregroundStyle(tint)
            .lineLimit(1)
    }

    private var stats: some View {
        HStack(alignment: .top, spacing: 12) {
            ForEach(Array(content.stats.enumerated()), id: \.offset) { _, stat in
                VStack(alignment: .leading, spacing: 4) {
                    Text(stat.label)
                        .font(.system(size: 12))
                        .foregroundStyle(Palette.muted)
                        .lineLimit(1)
                    Text(stat.value)
                        .font(.system(size: 16, weight: .semibold))
                        .monospacedDigit()
                        .foregroundStyle(Palette.ink)
                        .lineLimit(1)
                        .minimumScaleFactor(0.7)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    private var footer: some View {
        HStack(alignment: .center, spacing: 8) {
            MonacoMark(size: 20)
            Text("Monaco")
                .font(.system(size: 17, weight: .bold))
                .foregroundStyle(Palette.ink)
            Spacer(minLength: 8)
            VStack(alignment: .trailing, spacing: 4) {
                Text("Join me on Monaco")
                    .font(.system(size: 12))
                    .foregroundStyle(Palette.muted)
                Text(content.footerLink)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(Palette.brand)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
            }
        }
    }
}

#if DEBUG
#Preview("Gain") {
    ShareCardView(content: ShareCardSamples.cabalGain, face: nil)
        .environment(\.colorScheme, .dark)
}

#Preview("Loss") {
    ShareCardView(content: ShareCardSamples.me(.loss), face: nil)
        .environment(\.colorScheme, .dark)
}
#endif
