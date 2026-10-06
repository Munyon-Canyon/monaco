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

    func testExistingGroupFixtures_stillDecode() throws {
        let search = try Self.decodeFixture(GroupSearchResponseDTO.self, named: "groups_search")
        XCTAssertFalse(search.groups.isEmpty)
        XCTAssertNil(search.groups[0].pictureUrl)
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
              "proposals": []\(tail)
            }
            """
    }

    private static func decodeFixture<T: Decodable>(_ type: T.Type, named name: String) throws -> T {
        let url = try XCTUnwrap(Bundle.module.url(forResource: name, withExtension: "json"))
        return try JSONDecoder().decode(type, from: Data(contentsOf: url))
    }
}
