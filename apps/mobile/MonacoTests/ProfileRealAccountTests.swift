import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProfileRealAccountTests {
    static let userID = "01a11f58-0e65-747e-977e-ff693162ced4"

    static let user = #"""
        {"bio":"Professional trader","blocked_by_me":false,"cabal_count":2,"cabals_in_common":0,"created_at":"2026-10-09T06:27:06.981284Z","crown":false,"display_name":"mrrobot","followed_by_me":false,"follower_count":1,"following_count":1,"handle":"mrrobot","id":"01a11f58-0e65-747e-977e-ff693162ced4","photo_url":"https://komoccjaeypsrgocrnht.supabase.co/storage/v1/object/public/avatars/01a11f58-0e65-747e-977e-ff693162ced4/01a11f5d-ae6e-7c82-9561-92c564c93702.jpg","pnl_1m_micros":-447934,"return_1m_bps":-118,"return_period":"1M","return_private":false,"standing":null}
        """#
    static let portfolio = #"""
        {"arriving_micros":0,"cabals":[{"arriving_micros":0,"cabal":{"id":"01a11f16-370b-7e97-801a-0da29a9d4916","name":"Big Ballers","picture_url":null},"can_vote":true,"member_count":3,"net_contributed_micros":7996453,"open_vote_count":0,"ownership_bps":6159,"pnl_micros":-447934,"pot_value_micros":12257463,"return_bps":-561,"role":"member","share_units":8120362,"slice_bps":8831,"stale":false,"value_micros":7548519},{"arriving_micros":0,"cabal":{"id":"01a15773-e05b-7508-b935-9b3257755247","name":"Ac Capital Management","picture_url":"https://komoccjaeypsrgocrnht.supabase.co/storage/v1/object/public/avatars/cabals/01a15773-e05b-7508-b935-9b3257755247/01a15773-e17d-7672-b1fd-361b70e22111.jpg"},"can_vote":true,"member_count":1,"net_contributed_micros":1000000,"open_vote_count":0,"ownership_bps":10000,"pnl_micros":0,"pot_value_micros":1000000,"return_bps":0,"role":"admin","share_units":1000000,"slice_bps":1169,"stale":false,"value_micros":1000000}],"cash_micros":9383547,"computed_at":"2026-11-03T05:59:58.405377Z","net_worth_micros":17932066,"pnl_micros":-447934,"prices_as_of":"2026-11-03T05:56:00Z","return_bps":-498,"total_value_micros":8548519}
        """#
    static let balance = #"""
        {"arriving_micros":"0","as_of":"2026-11-03T06:00:57.197533223Z","available_micros":"9383547","deposit_address":"DepositAddressForTests111111111111111111111","in_flight_micros":"0","on_chain_micros":"9383547"}
        """#
    static let cabals = #"""
        [{"can_vote":true,"id":"01a11f16-370b-7e97-801a-0da29a9d4916","joined_at":"2026-10-13T06:37:20.721736Z","member_count":3,"name":"Big Ballers","pending_request_count":0,"picture_url":null,"role":"member","status":"active","unread_count":0},{"can_vote":true,"id":"01a15773-e05b-7508-b935-9b3257755247","joined_at":"2026-10-20T03:56:14.29933Z","member_count":1,"name":"Ac Capital Management","pending_request_count":0,"picture_url":"https://komoccjaeypsrgocrnht.supabase.co/storage/v1/object/public/avatars/cabals/01a15773-e05b-7508-b935-9b3257755247/01a15773-e17d-7672-b1fd-361b70e22111.jpg","role":"creator","status":"active","unread_count":0}]
        """#
    static let activity = #"""
        {"items":[{"at":"2026-11-03T02:27:03.541514Z","body":"GIF","cabal":{"id":"01a11f16-370b-7e97-801a-0da29a9d4916","name":"Big Ballers","picture_url":null},"comment_id":"01a19f3b-42f5-77d6-90e2-84189ef402c2","kind":"replied","proposal_id":"01a19f3a-c233-74a1-9175-9e5f6b8c6855","subject":"Buy METAx","usdc_micros":1000000},{"at":"2026-11-03T02:26:57.900896Z","body":"Test","cabal":{"id":"01a11f16-370b-7e97-801a-0da29a9d4916","name":"Big Ballers","picture_url":null},"comment_id":"01a19f3b-2cec-7dab-bad2-5e85eaff9b80","kind":"replied","proposal_id":"01a19f3a-c233-74a1-9175-9e5f6b8c6855","subject":"Buy METAx","usdc_micros":1000000},{"at":"2026-11-03T02:26:33.985993Z","cabal":{"id":"01a11f16-370b-7e97-801a-0da29a9d4916","name":"Big Ballers","picture_url":null},"choice":"yes","kind":"voted","proposal_id":"01a19f3a-c233-74a1-9175-9e5f6b8c6855","subject":"Buy METAx","symbol":"METAx","title":"Voted","usdc_micros":1000000}],"next_cursor":null}
        """#

    static func transport() -> StubTransport {
        func replies(_ body: String) -> [StubTransport.Reply] { Array(repeating: .json(.ok, body), count: 8) }
        return StubTransport(routes: [
            "/v1/users/\(userID)": replies(user),
            "/v1/me/portfolio": replies(portfolio),
            "/v1/me/balance": replies(balance),
            "/v1/me/cabals": replies(cabals),
            "/v1/me/cabal-invites": replies("[]"),
            "/v1/users/\(userID)/activity?limit=20": replies(activity),
        ])
    }

    @Test(.timeLimit(.minutes(2)))
    func anAccountShapedLikeTheFoundersShowsEveryOwnProfileSection() async throws {
        let transport = Self.transport()
        let scene = try #require(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: auth, api: api, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        environment.viewer = Viewer(userID: Self.userID, handle: "mrrobot")
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { ProfileScreen() }
                .environment(environment)
                .environment(ToastCenter())
                .environment(AppSessionStore())
                .environmentObject(auth)
        )
        window.makeKeyAndVisible()
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until {
            AccessibilityTree.identifiers(in: window).contains("profile-cabal-row-01a15773-e05b-7508-b935-9b3257755247")
        }
        for _ in 0..<2_000 { await Task.yield() }

        let identifiers = AccessibilityTree.identifiers(in: window)
        let expected = [
            "profile-card", "profile-bio", "profile-positions", "profile-pnl-1m", "profile-follow-counts",
            "profile-standing", "profile-cash-row", "profile-balance-value", "profile-tab-cabals",
            "profile-tab-activity", "profile-cabal-row-01a11f16-370b-7e97-801a-0da29a9d4916",
            "profile-cabal-row-01a15773-e05b-7508-b935-9b3257755247", "profile-add-money-link",
        ]
        for identifier in expected {
            #expect(identifiers.contains(identifier), "missing \(identifier); shown: \(identifiers)")
        }
    }
}
