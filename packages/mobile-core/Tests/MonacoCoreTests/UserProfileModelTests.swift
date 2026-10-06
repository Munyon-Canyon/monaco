import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

@MainActor
final class UserProfileModelTests: XCTestCase {
    func testLoadReadsTheProfile() async throws {
        let profile = UserProfileSupport.profile(followers: 12, following: 8, followed: true)
        let transport = try StubTransport(scripted: [.profile(profile)])
        let model = UserProfileSupport.model(transport)

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.profile?.displayName, "Maya Angelou")
        XCTAssertEqual(model.profile?.handle, "maya")
        XCTAssertEqual(model.profile?.photoUrl, "https://cdn.example.com/photos/maya.jpg")
        XCTAssertEqual(model.followerCount, 12)
        XCTAssertEqual(model.followingCount, 8)
        XCTAssertTrue(model.followedByMe)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertEqual(sent.map(\.path), ["/v1/users/\(UserProfileSupport.userID)"])
    }

    func testAnUnknownUserIsUnavailable() async throws {
        let transport = StubTransport(UserProfileSupport.problem(404, "user_not_found", "No such user."))
        let model = UserProfileSupport.model(transport)

        await model.load()

        XCTAssertEqual(model.phase, .unavailable)
        XCTAssertNil(model.profile)
        XCTAssertEqual(model.toastTick, 0)
    }

    func testABannedUserIsUnavailable() async throws {
        let transport = StubTransport(UserProfileSupport.problem(403, "user_banned", "This account is banned."))
        let model = UserProfileSupport.model(transport)

        await model.load()

        XCTAssertEqual(model.phase, .unavailable)
        XCTAssertNil(model.profile)
    }

    func testAFailedFirstLoadStaysFailedWithoutAToast() async throws {
        let transport = StubTransport(UserProfileSupport.problem(500, "internal", "Something went wrong."))
        let model = UserProfileSupport.model(transport)

        await model.load()

        XCTAssertEqual(model.phase, .failed)
        XCTAssertEqual(model.toastTick, 0)
        XCTAssertNil(model.profile)
    }

    func testFollowPostsTheSourceThenRefetchesTheCounts() async throws {
        let before = UserProfileSupport.profile(followers: 12, following: 8, followed: false)
        let after = UserProfileSupport.profile(followers: 40, following: 8, followed: true)
        let transport = try StubTransport(scripted: [
            .profile(before),
            .json(.ok, #"{"following":true}"#),
            .profile(after),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.follow(userID: UserProfileSupport.userID)

        XCTAssertTrue(model.followedByMe)
        XCTAssertEqual(model.followerCount, 40)
        XCTAssertEqual(model.followingCount, 8)
        XCTAssertFalse(model.isToggling(UserProfileSupport.userID))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .post, .get])
        XCTAssertEqual(
            sent.map(\.path),
            [
                "/v1/users/\(UserProfileSupport.userID)",
                "/v1/users/\(UserProfileSupport.userID)/follow",
                "/v1/users/\(UserProfileSupport.userID)",
            ]
        )
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["source": "profile"])
    }

    func testFollowFromPhoneSendsThatSource() async throws {
        let transport = StubTransport(.json(.ok, #"{"following":true}"#))
        let model = UserProfileSupport.model(transport)

        await model.follow(userID: UserProfileSupport.otherID, source: "phone")

        XCTAssertFalse(model.followedByMe)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertEqual(sent.first?.path, "/v1/users/\(UserProfileSupport.otherID)/follow")
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["source": "phone"])
    }

    func testTheServerAnswerWinsOverTheOptimisticFlip() async throws {
        let before = UserProfileSupport.profile(followers: 1, following: 1, followed: false)
        let after = UserProfileSupport.profile(followers: 1, following: 1, followed: false)
        let transport = try StubTransport(scripted: [
            .profile(before),
            .json(.ok, #"{"following":false}"#),
            .profile(after),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.follow(userID: UserProfileSupport.userID)

        XCTAssertFalse(model.followedByMe)
        XCTAssertEqual(model.followerCount, 1)
    }

    func testAFailedFollowRollsBackAndDoesNotRefetch() async throws {
        let profile = UserProfileSupport.profile(followers: 12, following: 8, followed: false)
        let transport = try StubTransport(scripted: [
            .profile(profile),
            .gate,
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        let follow = Task { await model.follow(userID: UserProfileSupport.userID) }
        await transport.waitForRequests(2)

        XCTAssertTrue(model.followedByMe)
        XCTAssertTrue(model.isToggling(UserProfileSupport.userID))
        await transport.releaseGate(UserProfileSupport.problem(500, "internal", "Something went wrong."))
        await follow.value

        XCTAssertFalse(model.followedByMe)
        XCTAssertEqual(model.followerCount, 12)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.toastTick, 1)
        XCTAssertNotNil(model.lastError)
        XCTAssertFalse(model.isToggling(UserProfileSupport.userID))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .post])
    }

    func testABannedFollowMakesTheProfileUnavailable() async throws {
        let profile = UserProfileSupport.profile(followers: 1, following: 0, followed: false)
        let transport = try StubTransport(scripted: [
            .profile(profile),
            UserProfileSupport.problem(403, "user_banned", "This account is banned."),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.follow(userID: UserProfileSupport.userID)

        XCTAssertEqual(model.phase, .unavailable)
        XCTAssertNil(model.profile)
        XCTAssertFalse(model.followedByMe)
        XCTAssertEqual(model.toastTick, 0)
    }

    func testUnfollowDeletesThenRefetchesTheCounts() async throws {
        let before = UserProfileSupport.profile(followers: 9, following: 4, followed: true)
        let after = UserProfileSupport.profile(followers: 3, following: 4, followed: false)
        let transport = try StubTransport(scripted: [
            .profile(before),
            .json(.ok, #"{"following":false}"#),
            .profile(after),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.unfollow(userID: UserProfileSupport.userID)

        XCTAssertFalse(model.followedByMe)
        XCTAssertEqual(model.followerCount, 3)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .delete, .get])
        XCTAssertEqual(sent[1].path, "/v1/users/\(UserProfileSupport.userID)/follow")
    }

    func testAFailedRefreshKeepsTheProfileAndToasts() async throws {
        let profile = UserProfileSupport.profile(followers: 2, following: 2, followed: true)
        let transport = try StubTransport(scripted: [
            .profile(profile),
            UserProfileSupport.problem(500, "internal", "Something went wrong."),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.refresh()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.followerCount, 2)
        XCTAssertTrue(model.followedByMe)
        XCTAssertEqual(model.toastTick, 1)
    }

    func testEachTapSendsItsOwnIdempotencyKey() async throws {
        let before = UserProfileSupport.profile(followers: 1, following: 0, followed: false)
        let followed = UserProfileSupport.profile(followers: 2, following: 0, followed: true)
        let unfollowed = UserProfileSupport.profile(followers: 1, following: 0, followed: false)
        let transport = try StubTransport(scripted: [
            .profile(before),
            .json(.ok, #"{"following":true}"#),
            .profile(followed),
            .json(.ok, #"{"following":false}"#),
            .profile(unfollowed),
        ])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.follow(userID: UserProfileSupport.userID)
        await model.unfollow(userID: UserProfileSupport.userID)

        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[name] }
        let writes = keys.filter { $0 != nil }
        XCTAssertEqual(writes.count, 2)
        XCTAssertNotEqual(writes[0], writes[1])
    }

    func testASecondFollowOfSomeoneAlreadyFollowedDoesNotPost() async throws {
        let profile = UserProfileSupport.profile(followers: 4, following: 1, followed: true)
        let transport = try StubTransport(scripted: [.profile(profile)])
        let model = UserProfileSupport.model(transport)

        await model.load()
        await model.follow(userID: UserProfileSupport.userID)

        XCTAssertTrue(model.followedByMe)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
    }
}

@MainActor
final class UserProfileListModelTests: XCTestCase {
    func testFollowersPageToTheEnd() async throws {
        let transport = try StubTransport(scripted: [.page(.sampleFirst), .page(.sampleLast)])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        await model.loadMoreFollowers()
        await model.loadMoreFollowers()

        XCTAssertEqual(model.followersPhase, .loaded)
        XCTAssertFalse(model.followersLoadingMore)
        let page = Components.Schemas.FollowsPage.self
        XCTAssertEqual(model.followerRows.map(\.id), (page.sampleFirst.items + page.sampleLast.items).map(\.userId))
        XCTAssertEqual(model.followerRows.map(\.followedByMe), [false, true, false])
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 2)
        XCTAssertEqual(paths[0], "/v1/users/\(UserProfileSupport.userID)/followers?limit=30")
        XCTAssertTrue(paths[1].contains("cursor=\(page.sampleCursor)"))
        XCTAssertTrue(paths[1].contains("/followers"))
    }

    func testAnEmptyFollowingListIsEmpty() async throws {
        let transport = try StubTransport(scripted: [.page(.sampleEmpty)])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowing()

        XCTAssertEqual(model.followingPhase, .empty)
        XCTAssertTrue(model.followingRows.isEmpty)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/users/\(UserProfileSupport.userID)/following?limit=30"])
    }

    func testAFailedFollowerPageKeepsTheRowsAndToasts() async throws {
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            UserProfileSupport.problem(500, "internal", "Something went wrong."),
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        await model.loadMoreFollowers()

        XCTAssertEqual(model.followersPhase, .loaded)
        XCTAssertTrue(model.followersPageFailed)
        XCTAssertEqual(model.followerRows.map(\.id), Components.Schemas.FollowsPage.sampleFirst.items.map(\.userId))
        XCTAssertEqual(model.toastTick, 1)
        XCTAssertNotNil(model.lastError)
    }

    func testRetryAfterAFailedRefreshOfAFullyLoadedListRefetchesPageOne() async throws {
        let page = Components.Schemas.FollowsPage.self
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            .page(.sampleLast),
            UserProfileSupport.problem(500, "internal", "Something went wrong."),
            .page(.sampleFirst),
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        await model.loadMoreFollowers()
        await model.loadFollowers()
        XCTAssertTrue(model.followersPageFailed)
        await model.retryFollowers()

        XCTAssertFalse(model.followersPageFailed)
        XCTAssertEqual(model.followerRows.map(\.id), page.sampleFirst.items.map(\.userId))
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 4)
        XCTAssertEqual(paths[3], "/v1/users/\(UserProfileSupport.userID)/followers?limit=30")
    }

    func testRetryAfterAFailedLaterPageFetchesThatPage() async throws {
        let page = Components.Schemas.FollowsPage.self
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            UserProfileSupport.problem(500, "internal", "Something went wrong."),
            .page(.sampleLast),
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowing()
        await model.loadMoreFollowing()
        XCTAssertTrue(model.followingPageFailed)
        await model.retryFollowing()

        XCTAssertFalse(model.followingPageFailed)
        XCTAssertEqual(model.followingRows.map(\.id), (page.sampleFirst.items + page.sampleLast.items).map(\.userId))
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 3)
        XCTAssertTrue(paths[2].contains("cursor=\(page.sampleCursor)"))
        XCTAssertTrue(paths[2].contains("/following"))
    }

    func testAFailedFirstListLoadHasNoRowsAndNoToast() async throws {
        let transport = StubTransport(UserProfileSupport.problem(500, "internal", "Something went wrong."))
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()

        XCTAssertEqual(model.followersPhase, .failed)
        XCTAssertTrue(model.followerRows.isEmpty)
        XCTAssertEqual(model.toastTick, 0)
    }

    func testFollowingAListRowFlipsThatRowWithoutRefetchingTheProfile() async throws {
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            .json(.ok, #"{"following":true}"#),
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        await model.follow(userID: UserProfileSupport.otherID)

        XCTAssertEqual(model.followerRows.first?.followedByMe, true)
        XCTAssertEqual(model.phase, .idle)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .post])
    }

    func testAReloadedFollowerPageDropsAStaleFollowOverride() async throws {
        let refreshed = Components.Schemas.FollowsPage(
            items: [
                Components.Schemas.FollowUser(
                    userId: UserProfileSupport.otherID, handle: "kai", displayName: "Kai Cenat",
                    photoUrl: nil, followedByMe: false)
            ],
            nextCursor: nil
        )
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            .json(.ok, #"{"following":true}"#),
            .page(refreshed),
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        await model.follow(userID: UserProfileSupport.otherID)
        XCTAssertEqual(model.followerRows.first?.followedByMe, true)

        await model.loadFollowers()

        XCTAssertEqual(model.followerRows.first?.id, UserProfileSupport.otherID)
        XCTAssertEqual(model.followerRows.first?.followedByMe, false)
    }

    func testAFailedRowFollowFlipsThenRollsBack() async throws {
        let transport = try StubTransport(scripted: [
            .page(.sampleFirst),
            .gate,
        ])
        let model = UserProfileSupport.model(transport)

        await model.loadFollowers()
        let follow = Task { await model.follow(userID: UserProfileSupport.otherID) }
        await transport.waitForRequests(2)

        XCTAssertEqual(model.followerRows.first?.followedByMe, true)
        XCTAssertTrue(model.isToggling(UserProfileSupport.otherID))
        await transport.releaseGate(UserProfileSupport.problem(500, "internal", "Something went wrong."))
        await follow.value

        XCTAssertEqual(model.followerRows.first?.followedByMe, false)
        XCTAssertFalse(model.isToggling(UserProfileSupport.otherID))
        XCTAssertEqual(model.toastTick, 1)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .post])
    }
}

@MainActor
private enum UserProfileSupport {
    static let userID = "01890a5d-ac96-774b-bcce-b302099a8058"
    static let otherID = "01890a5d-ac96-774b-bcce-b302099a8059"

    static func model(_ transport: StubTransport) -> UserProfileModel {
        UserProfileModel(
            userID: userID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    static func profile(followers: Int, following: Int, followed: Bool) -> Components.Schemas.PublicProfile {
        Components.Schemas.PublicProfile(
            id: userID, handle: "maya", displayName: "Maya Angelou",
            photoUrl: "https://cdn.example.com/photos/maya.jpg", followerCount: followers, followingCount: following,
            followedByMe: followed
        )
    }

    static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}

extension StubTransport.Reply {
    fileprivate static func profile(_ profile: Components.Schemas.PublicProfile) throws -> Self {
        .json(.ok, String(decoding: try JSONEncoder().encode(profile), as: UTF8.self))
    }

    fileprivate static func page(_ page: Components.Schemas.FollowsPage) throws -> Self {
        .json(.ok, String(decoding: try JSONEncoder().encode(page), as: UTF8.self))
    }
}
