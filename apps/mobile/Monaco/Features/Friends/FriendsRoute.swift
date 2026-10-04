import SwiftUI

nonisolated struct FriendsRoute: AppRoute {
    @MainActor func destination() -> ContactsExplainerView {
        ContactsExplainerView()
    }
}
