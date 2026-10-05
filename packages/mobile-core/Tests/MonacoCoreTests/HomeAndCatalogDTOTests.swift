import XCTest

@testable import MonacoCore

final class HomeBoardDTOCodingTests: XCTestCase {
    func testDecodesGroupAndPeopleRowsWithTheirServerKeys() throws {
        let json = """
            {"groups":[{"groupId":"g","name":"Cabal","potValueUsd":"10.00","percentReturn":null,\
            "dollarPnl":"1.00","isJoined":true,"pictureUrl":"https://cdn.example.com/g.png"}],\
            "people":[{"userId":"u","displayName":"Jordan","profilePhotoUrl":null,"percentReturn":"2.0",\
            "dollarPnl":"0.50"}]}
            """
        let home = try JSONDecoder().decode(HomeViewDTO.self, from: Data(json.utf8))
        XCTAssertEqual(home.groups.first?.groupID, "g")
        XCTAssertEqual(home.groups.first?.pictureUrl, "https://cdn.example.com/g.png")
        XCTAssertNil(home.groups.first?.percentReturn)
        XCTAssertEqual(home.people.first?.userID, "u")
        XCTAssertEqual(home.people.first?.percentReturn, "2.0")
    }

    func testEncodesBackToTheServerKeys() throws {
        let row = HomeGroupBoardRowDTO(
            groupID: "g", name: "Cabal", potValueUsd: "1", percentReturn: nil, dollarPnl: "0", isJoined: false)
        let person = HomePeopleBoardRowDTO(userID: "u", displayName: "J", percentReturn: nil, dollarPnl: "0")
        let data = try JSONEncoder().encode(HomeViewDTO(groups: [row], people: [person]))
        let text = String(decoding: data, as: UTF8.self)
        XCTAssertTrue(text.contains("\"groupId\""))
        XCTAssertTrue(text.contains("\"userId\""))
        XCTAssertEqual(try JSONDecoder().decode(HomeViewDTO.self, from: data).groups, [row])
    }
}

final class CatalogDTOTests: XCTestCase {
    func testDefaultsWhenTheServerOmitsOptionalFields() {
        let asset = CatalogAssetDTO(symbol: "AAPLx", name: "Apple")
        XCTAssertEqual(asset.resolvedKind, .stock)
        XCTAssertEqual(asset.resolvedDecimals, AssetCatalogDefaults.decimals)
        XCTAssertTrue(asset.isTradable)
    }

    func testRoutableFalseIsNotTradableAndPresentFieldsWin() {
        let asset = CatalogAssetDTO(
            symbol: "tSpaceX", name: "SpaceX", routable: false, kind: .preIpo, tokenDecimals: 6)
        XCTAssertFalse(asset.isTradable)
        XCTAssertEqual(asset.resolvedKind, .preIpo)
        XCTAssertEqual(asset.resolvedDecimals, 6)
    }

    func testSearchResponseRoundTrips() throws {
        let response = SearchAssetsResponseDTO(assets: [CatalogAssetDTO(symbol: "A", name: "A")], hasMore: true)
        let decoded = try JSONDecoder().decode(
            SearchAssetsResponseDTO.self, from: try JSONEncoder().encode(response))
        XCTAssertEqual(decoded, response)
    }
}
