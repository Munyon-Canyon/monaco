import XCTest

@testable import MonacoCore

final class PlatformBalanceDTOTests: XCTestCase {
    func testFundGroupResponseDTO_decodesFromAPI() throws {
        let json = """
            {
              "depositId": "dep-001",
              "groupId": "grp-001",
              "amount": 1000000,
              "status": "pending",
              "fromAddress": "MemberAddr1111111111111111111111111111"
            }
            """
        let dto = try JSONDecoder().decode(FundGroupResponseDTO.self, from: Data(json.utf8))
        XCTAssertEqual(dto.depositId, "dep-001")
        XCTAssertEqual(dto.groupId, "grp-001")
        XCTAssertEqual(dto.amount, 1_000_000)
        XCTAssertEqual(dto.status, "pending")
    }
}
