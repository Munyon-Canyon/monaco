import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The cabal picture: what the group payloads decode into.
final class CabalPictureTests: XCTestCase {
    // MARK: - Group view decode

    func testGroupView_decodesPictureAndCreatorFlag() throws {
        let json = Self.groupViewJSON(
            extra: #""pictureUrl": "https://cdn.test/groups/g1/abc.jpg", "isCreator": true"#
        )

        let view = try JSONDecoder().decode(GroupViewDTO.self, from: Data(json.utf8))

        XCTAssertEqual(view.pictureUrl, "https://cdn.test/groups/g1/abc.jpg")
        XCTAssertEqual(view.isCreator, true)
        XCTAssertTrue(view.viewerIsCreator)
    }

    func testGroupView_decodesWithoutAPicture() throws {
        let json = Self.groupViewJSON(extra: #""pictureUrl": null, "isCreator": false"#)

        let view = try JSONDecoder().decode(GroupViewDTO.self, from: Data(json.utf8))

        XCTAssertNil(view.pictureUrl)
        XCTAssertEqual(view.isCreator, false)
        XCTAssertFalse(view.viewerIsCreator)
        // The rest of the payload still lands, so a cabal without a picture is
        // not a degraded cabal.
        XCTAssertEqual(view.name, "Weekend investors")
        XCTAssertEqual(view.members.count, 1)
    }

    /// A server that predates the picture fields must keep working: the app is
    /// released ahead of the backend often enough that this cannot throw.
    func testGroupView_decodesWhenTheServerOmitsBothFieldsEntirely() throws {
        let view = try JSONDecoder().decode(GroupViewDTO.self, from: Data(Self.groupViewJSON().utf8))

        XCTAssertNil(view.pictureUrl)
        XCTAssertNil(view.isCreator)
        XCTAssertFalse(view.viewerIsCreator, "a missing isCreator must not offer the picture controls")
    }

    func testGroupViewFixture_stillDecodes() throws {
        let view = try Self.decodeFixture(GroupViewDTO.self, named: "group_view")
        XCTAssertNil(view.pictureUrl)
        XCTAssertFalse(view.viewerIsCreator)
    }

    // MARK: - Board rows

    func testDiscoveryRow_decodesPictureAndToleratesItsAbsence() throws {
        let withPicture = #"""
            {
              "groupId": "g1", "name": "Weekend investors", "memberCount": 4,
              "potValueUsd": "623.01", "percentReturn": "0.12", "dollarPnl": "+48.20",
              "isJoined": true, "joinMode": "open",
              "pictureUrl": "https://cdn.test/groups/g1/abc.jpg"
            }
            """#
        let row = try JSONDecoder().decode(GroupDiscoveryRowDTO.self, from: Data(withPicture.utf8))
        XCTAssertEqual(row.pictureUrl, "https://cdn.test/groups/g1/abc.jpg")

        let without = #"""
            {
              "groupId": "g2", "name": "No picture", "memberCount": 1,
              "potValueUsd": "0.00", "percentReturn": null, "dollarPnl": "+0.00",
              "isJoined": false, "joinMode": "request"
            }
            """#
        let bare = try JSONDecoder().decode(GroupDiscoveryRowDTO.self, from: Data(without.utf8))
        XCTAssertNil(bare.pictureUrl)
        XCTAssertEqual(bare.name, "No picture")
    }

    func testHomeGroupBoardRow_decodesPicture() throws {
        let json = #"""
            {
              "groupId": "g1", "name": "Weekend investors", "potValueUsd": "623.01",
              "percentReturn": "0.12", "dollarPnl": "+48.20", "isJoined": true,
              "pictureUrl": "https://cdn.test/groups/g1/abc.jpg"
            }
            """#
        let row = try JSONDecoder().decode(HomeGroupBoardRowDTO.self, from: Data(json.utf8))
        XCTAssertEqual(row.pictureUrl, "https://cdn.test/groups/g1/abc.jpg")
    }

    func testHomeMyGroupRow_decodesPictureAndToleratesItsAbsence() throws {
        let json = #"""
            {
              "groupId": "g1", "name": "Weekend investors", "equityUsd": "311.50",
              "slicePercent": "0.42", "dollarPnl": "+48.20", "percentReturn": "0.124"
            }
            """#
        let row = try JSONDecoder().decode(HomeMyGroupRowDTO.self, from: Data(json.utf8))
        XCTAssertNil(row.pictureUrl)
        XCTAssertEqual(row.equityUsd, "311.50")
    }

    func testExistingGroupFixtures_stillDecode() throws {
        let search = try Self.decodeFixture(GroupSearchResponseDTO.self, named: "groups_search")
        XCTAssertFalse(search.groups.isEmpty)
        XCTAssertNil(search.groups[0].pictureUrl)

        let home = try Self.decodeFixture(HomeViewDTO.self, named: "home_view")
        XCTAssertFalse(home.groups.isEmpty)
        XCTAssertNil(home.groups[0].pictureUrl)
    }

    // MARK: - Helpers

    /// A minimal group view, optionally with extra top-level keys.
    private static func groupViewJSON(extra: String = "") -> String {
        let tail = extra.isEmpty ? "" : ", \(extra)"
        return """
            {
              "id": "g1",
              "name": "Weekend investors",
              "treasuryAddress": "So11111111111111111111111111111111111111112",
              "potTotalUsd": "623.01",
              "pot": [],
              "you": {
                "shareUnits": "500000", "equityUsd": "311.50", "slicePercent": "0.42",
                "dollarPnl": "+48.20", "percentReturn": "0.124"
              },
              "members": [
                { "rank": 1, "userId": "u1", "displayName": "Alfred", "percentReturn": "0.124", "dollarPnl": "+48.20" }
              ],
              "proposals": []\(tail)
            }
            """
    }

    private static func decodeFixture<T: Decodable>(_ type: T.Type, named name: String) throws -> T {
        let url = try XCTUnwrap(Bundle.module.url(forResource: name, withExtension: "json"))
        return try JSONDecoder().decode(type, from: Data(contentsOf: url))
    }
}
