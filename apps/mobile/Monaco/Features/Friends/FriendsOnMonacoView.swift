import MonacoCore
import SwiftUI

struct FriendsOnMonacoView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var search: PeopleSearchModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if let search {
                    PeopleSearchField(model: search)
                    if search.isSearching {
                        PeopleSearchResults(model: search)
                    } else {
                        contacts
                    }
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Friends on Monaco")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if search == nil { search = PeopleSearchModel(api: environment.api, clock: ContinuousClock()) }
        }
        .onChange(of: search?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: false)
        }
    }

    private var contacts: some View {
        MonacoGroupedList {
            NavigationLink(value: AnyAppRoute(ContactsExplainerRoute())) {
                MonacoRow(
                    title: "Find friends from your contacts",
                    chevron: true,
                    isLast: true,
                    leading: { StockMark(systemImage: "person.crop.circle.badge.plus", size: 40) }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("friends-from-contacts")
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
