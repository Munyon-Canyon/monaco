import MonacoCore
import SwiftUI

enum HomeBellMetrics {
    static let badgeHeight: CGFloat = 18
    static let badgeInset: CGFloat = 2
}

struct HomeBellButton: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: HomeBellModel?

    var body: some View {
        MonacoGlassButton(
            systemImage: MonacoIcon.bell.symbol, accessibilityLabel: model?.accessibilityLabel ?? "Catch up",
            style: .quiet, placement: .toolbar
        ) {
            environment.navigator.open(ActivityFeedRoute(), in: .home)
        }
        .overlay(alignment: .topTrailing) { badge }
        .accessibilityIdentifier("home-notifications-button")
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private var badge: some View {
        if let text = model?.badgeText {
            Text(text)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.voteNeededLabel)
                .lineLimit(1)
                .padding(.horizontal, MonacoTheme.Space.xs + 1)
                .frame(minWidth: HomeBellMetrics.badgeHeight, minHeight: HomeBellMetrics.badgeHeight)
                .background(MonacoTheme.voteNeededFill, in: Capsule())
                .offset(x: HomeBellMetrics.badgeInset, y: HomeBellMetrics.badgeInset)
                .allowsHitTesting(false)
                .accessibilityHidden(true)
                .accessibilityIdentifier("home-bell-badge")
        }
    }

    private func preparedModel() -> HomeBellModel {
        if let model { return model }
        let created = HomeBellModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
