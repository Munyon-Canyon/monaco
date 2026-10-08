import MonacoCore
import SwiftUI

struct FriendsOnMonacoView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Bindable var model: FriendsOnMonacoModel
    var onDone: (() -> Void)?
    @State private var search: PeopleSearchModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if let search {
                    PeopleSearchField(model: search)
                    if search.isSearching {
                        PeopleSearchResults(model: search)
                    } else {
                        matches
                    }
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Friends on Monaco")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { done }
        .task {
            if search == nil { search = PeopleSearchModel(api: environment.api, clock: ContinuousClock()) }
            await model.loadIfGranted()
        }
        .onChange(of: search?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: false)
        }
        .onChange(of: model.toastTick) { _, _ in
            guard let message = model.toast else { return }
            toasts.current = MonacoToast(message: message, isSuccess: false)
        }
    }

    @ToolbarContentBuilder
    private var done: some ToolbarContent {
        if let onDone {
            ToolbarItem(placement: .topBarTrailing) {
                Button("Done", action: onDone)
                    .accessibilityIdentifier("friends-done")
            }
        }
    }

    @ViewBuilder
    private var matches: some View {
        switch model.phase {
        case .idle, .checking:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text("Checking your contacts…")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                BoardRowSkeleton()
            }
            .accessibilityIdentifier("friends-checking")
        case .failed:
            MonacoErrorRow(thing: "your friends", identifier: "friends-try-again") {
                Task { await model.retry() }
            }
        case .empty:
            EmptyState(title: "None of your contacts are on Monaco yet")
                .accessibilityIdentifier("friends-empty")
        case .loaded:
            MonacoGroupedList {
                ForEach(Array(model.friends.enumerated()), id: \.element.id) { index, friend in
                    FriendMatchRow(
                        friend: friend,
                        isLast: index == model.friends.count - 1,
                        isToggling: model.isToggling(friend.id)
                    ) {
                        Task { await model.follow(friend.id) }
                    }
                }
            }
        }
    }
}

private struct PeopleSearchField: View {
    @Bindable var model: PeopleSearchModel

    var body: some View {
        MonacoSearchField(placeholder: "Search by name or @handle", text: $model.query)
            .textInputAutocapitalization(.never)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("friends-search-field")
    }
}

private struct FriendMatchRow: View {
    let friend: FriendOnMonaco
    let isLast: Bool
    let isToggling: Bool
    let follow: () -> Void

    private var name: String { friend.displayName.isEmpty ? friend.handle : friend.displayName }

    var body: some View {
        ZStack(alignment: .trailing) {
            NavigationLink(
                value: AnyAppRoute(
                    UserProfileRoute(
                        userID: friend.id,
                        preview: UserPreview(displayName: name, handle: friend.handle, photoURL: friend.photoURL)
                    ))
            ) {
                MonacoRow(
                    title: name,
                    subtitle: "@\(friend.handle)",
                    isLast: isLast,
                    leading: {
                        MonacoAvatar(photoURL: friend.photoURL, displayName: name, size: 40, seed: friend.id)
                    }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("friends-row-\(friend.handle)")
            followButton
                .padding(.trailing, MonacoTheme.Space.m)
        }
    }

    @ViewBuilder
    private var followButton: some View {
        if friend.followedByMe {
            Button("Following", action: follow)
                .buttonStyle(.monacoSecondary)
                .disabled(true)
                .accessibilityIdentifier("friends-follow-\(friend.handle)")
        } else {
            Button("Follow", action: follow)
                .buttonStyle(.monacoPrimary)
                .disabled(isToggling)
                .accessibilityIdentifier("friends-follow-\(friend.handle)")
        }
    }
}
