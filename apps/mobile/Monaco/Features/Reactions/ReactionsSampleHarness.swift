#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ReactionsSampleScenario: String, CaseIterable {
    case row
    case trayOpen
    case mine
    case failed
    case picker
    case search

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
    @State private var isPickerOpen: Bool
    private let frequent: FrequentEmojiStore

    init(scenario: ReactionsSampleScenario) {
        self.scenario = scenario
        let defaults = UserDefaults(suiteName: "reactions-harness") ?? .standard
        defaults.removePersistentDomain(forName: "reactions-harness")
        frequent = FrequentEmojiStore(defaults: defaults)
        for emoji in ["🤝", "🙌", "💪", "👀", "🎉", "😂", "❤️", "👍"] { frequent.record(emoji) }
        let items = [Components.Schemas.FeedItem.threads[0], .samples[1], .samples[3]]
        _model = State(
            initialValue: ReactionsModel.sample(
                reactions: Dictionary(uniqueKeysWithValues: items.map { ($0.id, $0.reactions) }),
                fails: scenario == .failed, clock: ContinuousClock()))
        _items = State(initialValue: items)
        _isPickerOpen = State(initialValue: scenario == .picker || scenario == .search)
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                ForEach(items, id: \.id) { item in
                    FeedPost(item: item, now: Date(), opensPost: false) {
                        ReactionRow(
                            target: .feedItem(item.id),
                            reactions: model.reactions(for: .feedItem(item.id), fallback: item.reactions),
                            startsOpen: scenario == .trayOpen && item.id == items.first?.id,
                            onToggle: { toggle($0, on: item) }
                        ) {
                            MonacoReactionChip.reply(count: Int(item.commentCount)) {}
                        }
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    MonacoRule(color: MonacoTheme.hairline)
                        .padding(.leading, FeedPostMetrics.textColumn)
                }
            }
        }
        .background(MonacoTheme.background)
        .sheet(isPresented: $isPickerOpen) {
            if let item = items.first {
                EmojiPickerSheet(initialQuery: scenario == .search ? "eyes" : "") { toggle($0, on: item) }
            }
        }
        .environment(\.frequentEmojiStore, frequent)
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
