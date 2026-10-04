import SwiftUI

struct ProposalCoinBurst: View {
    static let coins = 14

    let trigger: Int

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var progress: CGFloat = 0
    @State private var isPlaying = false

    var body: some View {
        ZStack {
            if isPlaying {
                ForEach(0..<Self.coins, id: \.self) { index in
                    coin(index)
                }
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
        .onChange(of: trigger) { _, _ in play() }
    }

    private func coin(_ index: Int) -> some View {
        let angle = Angle.degrees(Double(index) / Double(Self.coins) * 360 - 90)
        let distance: CGFloat = 90 + CGFloat(index % 3) * 28
        return Circle()
            .fill(MonacoTheme.goldGlyph)
            .overlay(Circle().strokeBorder(MonacoTheme.gold, lineWidth: 2))
            .frame(width: 18, height: 18)
            .offset(
                x: cos(angle.radians) * distance * progress,
                y: sin(angle.radians) * distance * progress + 60 * progress * progress
            )
            .opacity(1 - progress)
            .scaleEffect(1 - 0.3 * progress)
    }

    private func play() {
        guard !reduceMotion else { return }
        progress = 0
        isPlaying = true
        withAnimation(.easeOut(duration: 1.1)) {
            progress = 1
        } completion: {
            isPlaying = false
        }
    }
}
