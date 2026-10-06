import MonacoAPI
import MonacoCore
import SwiftUI

/// Every screen the Cabals tab can push.
///
/// The tab owns one route value, and rows hand over a route instead of a view.
/// Two things follow from that:
///
/// - A destination is decided once, when the row is tapped. A board reload that
///   flips a row to "you're in" can no longer swap the screen a member is
///   already reading.
/// - The tab can *replace* the top screen. "New cabal" becomes the cabal that
///   was just created, and a join screen becomes the cabal that was just
///   joined, so Back always lands on the tab — never on a live form that would
///   create a second cabal.
enum CabalsRoute: Hashable, Identifiable {
    /// A cabal the viewer is already in. The name is what the row that pushed
    /// it knew; the invite-code route has none until the cabal loads.
    case cabal(id: String, name: String?)
    /// Join by pasting an invite code a friend shared.
    case joinByCode
    /// The "New cabal" form.
    case create

    var id: Self { self }
}

/// Builds the screen behind a route. The Cabals tab declares this once, so no
/// row view ever constructs a destination.
struct CabalsRouteDestination: View {
    @ObservedObject var auth: PrivyAuthService
    let route: CabalsRoute
    /// The create write, so the tab's sample harness can drive it without a backend.
    let actions: CabalsActionSource
    /// A cabal was created; the owner replaces this screen with it.
    var onCreated: (Components.Schemas.Cabal) -> Void

    var body: some View {
        switch route {
        case .cabal(let id, let name):
            GroupDetailView(auth: auth, groupId: id, groupName: name)
        case .joinByCode:
            JoinCabalView()
        case .create:
            CreateGroupView(actions: actions, onCreated: onCreated)
        }
    }
}
