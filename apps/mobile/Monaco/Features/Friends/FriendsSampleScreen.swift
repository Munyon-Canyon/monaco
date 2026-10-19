#if DEBUG
import MonacoCore
import SwiftUI

enum FriendsSampleState: String, CaseIterable {
    case results
    case following
    case followingEmpty
    case noResults
    case loading
    case error
}

struct FriendsSampleScreen: View {
    @State private var state: FriendsSampleState
    @State private var query: String
    @State private var tab: FriendsTab

    init(state: FriendsSampleState) {
        _state = State(initialValue: state)
        _query = State(initialValue: state == .noResults ? "zzq" : "")
        _tab = State(initialValue: state == .following || state == .followingEmpty ? .following : .discover)
    }

    private static let people = [
        FriendsPerson(id: "sample-mia", handle: "mia", displayName: "Mia Chen", photoURL: nil, followedByMe: false),
        FriendsPerson(id: "sample-noah", handle: "noah", displayName: "Noah Park", photoURL: nil, followedByMe: false),
        FriendsPerson(id: "sample-zoe", handle: "zoe", displayName: "Zoe Ruiz", photoURL: nil, followedByMe: false),
    ]

    private static let followed = people.map { person in
        var followed = person
        followed.followedByMe = true
        return followed
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                FriendsPeopleFrame(query: $query, tab: $tab, showsTabs: true) { content }
                    .padding(.vertical, MonacoTheme.Space.m)
            }
            .monacoCanvas()
            .navigationTitle(FriendsScreen.title)
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    @ViewBuilder private var content: some View {
        if tab == .following {
            followingContent
        } else {
            discoverContent
        }
    }

    @ViewBuilder private var discoverContent: some View {
        switch state {
        case .loading:
            MonacoRowSkeleton(rows: 4, markShape: .circle)
                .accessibilityIdentifier("friends-search-loading")
        case .error:
            EmptyState.loadFailed(identifier: "friends-search-error") { state = .results }
        case .noResults:
            FriendsNoResults {
                query = ""
                state = .results
            }
            .accessibilityIdentifier("friends-search-empty")
        default:
            list(Self.people) { person in
                .follow(isBusy: false, identifier: "friends-search-follow-\(person.id)", action: {})
            }
        }
    }

    @ViewBuilder private var followingContent: some View {
        if state == .followingEmpty {
            FriendsFollowingEmpty()
                .accessibilityIdentifier("friends-following-empty")
        } else {
            list(Self.followed) { _ in .badge }
        }
    }

    private func list(
        _ people: [FriendsPerson], accessory: @escaping (FriendsPerson) -> FriendsPersonRow.Accessory
    ) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoGroupedList {
                ForEach(people) { person in
                    FriendsPersonRow(person: person, accessory: accessory(person), isLast: person.id == people.last?.id)
                        .accessibilityIdentifier("friends-search-\(person.handle)")
                }
            }
            FriendsSearchNotice()
        }
    }
}
#endif
