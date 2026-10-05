import SwiftUI

nonisolated struct FriendsRoute: AppRoute {
    @MainActor func destination() -> FriendsOnMonacoView {
        FriendsOnMonacoView()
    }
}

nonisolated struct ContactsExplainerRoute: AppRoute {
    @MainActor func destination() -> ContactsExplainerView {
        ContactsExplainerView()
    }
}
