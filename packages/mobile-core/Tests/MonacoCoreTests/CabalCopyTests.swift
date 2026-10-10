import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CabalCopyTests: XCTestCase {
    func testMemberCopy() {
        XCTAssertEqual(CabalCopy.memberCount(1), "1 member")
        XCTAssertEqual(CabalCopy.memberCount(3), "3 members")
        let named = member(displayName: "Kai", handle: "kai", role: "creator")
        XCTAssertEqual(CabalCopy.memberName(named), "Kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: " ", handle: "kai", role: "member")), "@kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: "", handle: nil, role: "member")), "Member")
    }

    func testOnlyTheCreatorSeesPendingRequests() {
        XCTAssertEqual(CabalCopy.requestBadge(myCabal(role: "creator", pending: 2)), 2)
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "creator", pending: 0)))
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "member", pending: 2)))
        XCTAssertEqual(CabalCopy.requestCount(1), "1 request to join")
        XCTAssertEqual(CabalCopy.requestCount(2), "2 requests to join")
    }

    func testTheUnreadBadgeFormatterHidesZeroAndCapsAtNinetyNine() {
        XCTAssertNil(CabalCopy.unreadBadge(0))
        XCTAssertEqual(CabalCopy.unreadBadge(7), "7")
        XCTAssertEqual(CabalCopy.unreadBadge(99), "99")
        XCTAssertEqual(CabalCopy.unreadBadge(100), "99+")
        XCTAssertEqual(CabalCopy.unreadLabel(1), "1 unread message")
        XCTAssertEqual(CabalCopy.unreadLabel(3), "3 unread messages")
        XCTAssertEqual(CabalCopy.unreadLabel(100), "More than 99 unread messages")
    }

    private func myCabal(role: String, pending: Int32) -> Components.Schemas.MyCabal {
        .init(
            id: "c-1", name: "QA pot", pictureUrl: nil, role: role, canVote: true, memberCount: 2,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: pending, unreadCount: 0)
    }

    private func member(displayName: String, handle: String?, role: String) -> Components.Schemas.CabalMember {
        .init(
            userId: "u-1", handle: handle, displayName: displayName, photoUrl: nil, role: role, canVote: true,
            joinedAt: Date(timeIntervalSince1970: 0))
    }
}
