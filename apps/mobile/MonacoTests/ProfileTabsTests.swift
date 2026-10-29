import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ProfileTabsTests {
    @Test func theHarnessScenariosResolve() {
        for name in ["tabsCabals", "tabsUnvalued", "tabsActivity", "tabsEmpty", "tabsError"] {
            #expect(ProfileSampleScenario.matching(["-MonacoProfileSample", name]) != nil, "\(name)")
        }
    }

    @Test func theTabSwitchChangesTheSelection() {
        var selection = ProfileTabsSelection.cabals
        let binding = Binding(get: { selection }, set: { selection = $0 })
        _ = ProfileTabsContent(
            cabals: .loaded([]), activity: .loaded([]), hasMoreActivity: false, selection: binding)
        binding.wrappedValue = .activity
        #expect(selection == .activity)
    }

    @Test func inviteRowsKeepTheirIdentifier() throws {
        let rows = ProfileCabalRows.make(cabals: [], portfolio: nil, invites: [.sample])
        guard case .invited(let invite) = try #require(rows.first) else {
            Issue.record("want an invitation")
            return
        }
        #expect(ProfileCabalRowView(invite: invite, isLast: true).identifier == "profile-invite-row-\(invite.cabalID)")
    }
}
