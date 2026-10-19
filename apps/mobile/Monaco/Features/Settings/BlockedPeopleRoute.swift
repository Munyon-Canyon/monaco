import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct BlockedPeopleRoute: AppRoute {
    @MainActor func destination() -> some View {
        BlockedPeopleView()
    }
}

struct BlockedPeopleView: View {
    var unblocksFirstOnLoad = false

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: BlockedPeopleModel?

    init(model: BlockedPeopleModel? = nil, unblocksFirstOnLoad: Bool = false) {
        _model = State(initialValue: model)
        self.unblocksFirstOnLoad = unblocksFirstOnLoad
    }

    var body: some View {
        ScrollView {
            content
        }
        .monacoCanvas()
        .navigationTitle("Blocked people")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let created = model ?? BlockedPeopleModel(api: environment.api, hints: environment.hints)
            model = created
            await created.load()
            if unblocksFirstOnLoad, case .loaded(let people) = created.state, let first = people.first {
                await unblock(first)
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .circle, hasTrailing: false)
                .accessibilityIdentifier("blocked-people-loading")
        case .failed:
            MonacoErrorRow(thing: "blocked people", identifier: "blocked-people-failed") {
                Task { await model?.load() }
            }
        case .loaded(let people):
            if people.isEmpty {
                EmptyState(
                    title: "No one blocked", message: "People you block show up here.",
                    systemImage: "hand.raised"
                )
                .accessibilityIdentifier("blocked-people-empty")
            } else {
                list(people)
            }
        }
    }

    private func list(_ people: [BlockedPerson]) -> some View {
        MonacoGroupedList {
            ForEach(people) { person in
                row(person, isLast: person.id == people.last?.id)
            }
        }
        .padding(.top, MonacoTheme.Space.s)
    }

    private func row(_ person: BlockedPerson, isLast: Bool) -> some View {
        let preview = UserPreview(displayName: person.displayName, handle: person.handle, photoURL: person.photoURL)
        return NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: person.id, preview: preview))) {
            MonacoRow(title: person.title, subtitle: person.subtitle, isLast: isLast, trailingIsInteractive: true) {
                MonacoAvatar(photoURL: person.photoURL, displayName: person.displayName, seed: person.id)
            } trailing: {
                Button("Unblock") { Task { await unblock(person) } }
                    .buttonStyle(.monacoCompact)
                    .accessibilityIdentifier("blocked-unblock-\(person.id)")
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("blocked-person-\(person.id)")
    }

    private func unblock(_ person: BlockedPerson) async {
        guard let model else { return }
        let outcome = await model.unblock(userID: person.id)
        let message = model.actions.toast(for: outcome, after: .unblock(handle: person.handle))
        if outcome == .ok {
            toasts.show(success: message)
        } else {
            toasts.current = MonacoToast(message: message)
        }
    }
}

#if DEBUG
final class BlockedPeopleFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow20aScenario.matching(arguments) else { return nil }
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        return AnyView(
            NavigationStack {
                BlockedPeopleView(model: .preview(answering: scenario), unblocksFirstOnLoad: acts)
            }
        )
    }
}

final class BlockedPeopleSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoBlockedPeopleSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        switch arguments[flag + 1] {
        case "people": return AnyView(NavigationStack { BlockedPeopleView(model: .preview()) })
        case "empty": return AnyView(NavigationStack { BlockedPeopleView(model: .preview(empty: true)) })
        default: return nil
        }
    }
}
#endif
