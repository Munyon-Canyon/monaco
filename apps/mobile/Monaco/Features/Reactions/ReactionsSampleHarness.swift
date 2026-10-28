#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ReactionsSampleScenario: String, CaseIterable {
    case row
    case trayOpen
    case mine
    case failed

    static let launchArgument = "-reactionsHarness"

    static func matching(_ arguments: [String]) -> ReactionsSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ReactionsSampleScenario(rawValue: arguments[flag + 1])
    }
}

private struct ReactionsSampleScreen: View {
    let scenario: ReactionsSampleScenario
    @State private var model: ReactionsModel
    @State private var items: [Components.Schemas.FeedItem]
    @State private var toast: MonacoToast?

    init(scenario: ReactionsSampleScenario) {
        self.scenario = scenario
        let items = [0, 1, 3].map { Components.Schemas.FeedItem.samples[$0] }
        _model = State(
            initialValue: ReactionsModel.sample(
                reactions: Dictionary(uniqueKeysWithValues: items.map { ($0.id, $0.reactions) }),
                fails: scenario == .failed, clock: ContinuousClock()))
        _items = State(initialValue: items)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            ForEach(items, id: \.id) { item in
                VStack(alignment: .leading, spacing: 0) {
                    Text(item.title).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
                    ReactionRow(
                        target: .feedItem(item.id),
                        reactions: model.reactions(for: .feedItem(item.id), fallback: item.reactions),
                        startsOpen: scenario == .trayOpen && item.id == items.first?.id,
                        onToggle: { toggle($0, on: item) }
                    ) {
                        MonacoReactionChip.reply(count: Int(item.commentCount)) {}
                    }
                }
            }
            Spacer()
        }
        .padding(MonacoTheme.Space.gutter)
        .background(MonacoTheme.background)
        .monacoToast($toast)
        .onChange(of: model.noticeTick) {
            toast = model.notice.map { MonacoToast(message: $0) }
        }
    }

    private func toggle(_ emoji: String, on item: Components.Schemas.FeedItem) {
        Task {
            let current = model.reactions(for: .feedItem(item.id), fallback: item.reactions)
            let next = await model.toggle(emoji, on: .feedItem(item.id), current: current)
            if let index = items.firstIndex(where: { $0.id == item.id }) { items[index].reactions = next }
        }
    }
}

final class ReactionsSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = ReactionsSampleScenario.matching(arguments) else { return nil }
        return AnyView(ReactionsSampleScreen(scenario: scenario))
    }
}
#endif
