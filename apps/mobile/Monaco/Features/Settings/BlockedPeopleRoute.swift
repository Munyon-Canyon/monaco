import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct BlockedPeopleRoute: AppRoute {
    @MainActor func destination() -> some View {
        BlockedPeopleView()
    }
}

struct BlockedPeopleView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: BlockedPeopleModel?

    var body: some View {
        ScrollView {
            content
        }
        .monacoCanvas()
        .navigationTitle("Blocked people")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let created = model ?? BlockedPeopleModel(api: environment.api)
            model = created
            await created.load()
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
                EmptyState(title: "No one blocked", message: "People you block show up here.")
                    .accessibilityIdentifier("blocked-people-empty")
            } else {
                list(people)
            }
        }
    }

    private func list(_ people: [BlockedPerson]) -> some View {
        MonacoGroupedList {
            ForEach(people) { person in
                MonacoRow(title: person.title, subtitle: person.subtitle, isLast: person.id == people.last?.id) {
                    MonacoAvatar(photoURL: person.photoURL, displayName: person.displayName, seed: person.id)
                        .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                }
                .accessibilityIdentifier("blocked-person-\(person.id)")
            }
        }
        .padding(.top, MonacoTheme.Space.s)
    }
}
