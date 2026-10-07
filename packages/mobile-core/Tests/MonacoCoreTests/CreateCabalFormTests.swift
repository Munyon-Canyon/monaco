import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CreateCabalFormTests: XCTestCase {
    func testEveryJoinModeReachesTheWire() throws {
        let wire = try CabalJoinMode.allCases.map { mode in
            try body(CreateCabalForm(name: "Pot", joinMode: mode))["join_mode"] as? String
        }
        XCTAssertEqual(wire, ["open", "request"])
    }

    func testEveryVoterModeReachesTheWireAndJustMeSendsNoIds() throws {
        let bodies = try CabalVoterMode.allCases.map { try body(CreateCabalForm(name: "Pot", voterMode: $0)) }
        XCTAssertEqual(bodies.map { $0["voter_mode"] as? String }, ["all", "list"])
        for body in bodies {
            XCTAssertEqual(
                Set(body.keys), ["name", "join_mode", "voter_mode", "threshold", "proposal_expiry_seconds"])
        }
    }

    func testEveryThresholdReachesTheWire() throws {
        let wire = try CabalThreshold.allCases.map { threshold in
            try body(CreateCabalForm(name: "Pot", threshold: threshold))["threshold"] as? String
        }
        XCTAssertEqual(wire, ["majority", "unanimous"])
    }

    func testEveryExpiryReachesTheWire() throws {
        let wire = try CabalProposalExpiry.allCases.map { expiry in
            try body(CreateCabalForm(name: "Pot", expiry: expiry))["proposal_expiry_seconds"] as? Int
        }
        XCTAssertEqual(wire, [3600, 86_400, 604_800])
    }

    func testTheDefaultsMatchTheFormAMemberFirstSees() throws {
        let sent = try body(CreateCabalForm(name: "Pot"))
        XCTAssertEqual(sent["join_mode"] as? String, "open")
        XCTAssertEqual(sent["voter_mode"] as? String, "all")
        XCTAssertEqual(sent["threshold"] as? String, "majority")
        XCTAssertEqual(sent["proposal_expiry_seconds"] as? Int, 604_800)
    }

    func testTheRequestLeavesSlippageToTheServer() throws {
        let sent = try body(CreateCabalForm(name: "Pot", joinMode: .request, voterMode: .justMe))
        XCTAssertNil(sent["slippage_bps"])
    }

    func testTheNameIsTrimmedBeforeItIsSent() throws {
        let form = CreateCabalForm(name: "  \n QA pot \t ")
        XCTAssertEqual(form.trimmedName, "QA pot")
        XCTAssertEqual(form.input?.name, "QA pot")
        XCTAssertEqual(try body(form)["name"] as? String, "QA pot")
    }

    func testTheNameIsMeasuredInScalarsAfterTrimming() {
        XCTAssertEqual(CreateCabalForm(name: "ab").nameProblem, .tooShort)
        XCTAssertEqual(CreateCabalForm(name: "  ab  ").nameProblem, .tooShort)
        XCTAssertEqual(CreateCabalForm(name: "éé").nameProblem, .tooShort)
        XCTAssertNil(CreateCabalForm(name: "abc").nameProblem)
        XCTAssertNil(CreateCabalForm(name: " ééé ").nameProblem)
        XCTAssertNil(CreateCabalForm(name: String(repeating: "x", count: 40)).nameProblem)
        XCTAssertNil(CreateCabalForm(name: "  " + String(repeating: "é", count: 40) + "  ").nameProblem)
        XCTAssertEqual(CreateCabalForm(name: String(repeating: "x", count: 41)).nameProblem, .tooLong)
        XCTAssertEqual(CreateCabalForm(name: String(repeating: "é", count: 41)).nameProblem, .tooLong)
    }

    func testAnEmptyOrBlankNameIsEmptyAndHasNoMessage() {
        XCTAssertEqual(CreateCabalForm().nameProblem, .empty)
        XCTAssertEqual(CreateCabalForm(name: "  \n\t ").nameProblem, .empty)
        XCTAssertNil(CreateCabalForm.NameProblem.empty.message)
    }

    func testControlCharactersAreRefused() {
        XCTAssertEqual(CreateCabalForm(name: "line\nbreak").nameProblem, .invalid)
        XCTAssertEqual(CreateCabalForm(name: "tab\tbed").nameProblem, .invalid)
        XCTAssertEqual(CreateCabalForm(name: "ab\u{0}c").nameProblem, .invalid)
    }

    func testEveryProblemThatHasAMessageSaysCabalCopyAndNeverGroup() {
        let messages = [CreateCabalForm.NameProblem.tooShort, .tooLong, .invalid].compactMap(\.message)
        XCTAssertEqual(messages.count, 3)
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(messages))
    }

    func testThePickersKeepTheirLabels() {
        XCTAssertEqual(CabalJoinMode.allCases.map(\.label), ["Anyone", "I approve"])
        XCTAssertEqual(CabalVoterMode.allCases.map(\.label), ["Everyone", "Just me"])
        XCTAssertEqual(CabalThreshold.allCases.map(\.label), ["Majority", "Everyone agrees"])
        XCTAssertEqual(CabalProposalExpiry.allCases.map(\.label), ["1 hour", "1 day", "1 week"])
    }

    func testEveryPickerCaptionIsDistinctAndCleanCopy() {
        let captions = [
            CabalJoinMode.allCases.map(\.caption), CabalVoterMode.allCases.map(\.caption),
            CabalThreshold.allCases.map(\.caption), CabalProposalExpiry.allCases.map(\.caption),
        ]
        for options in captions {
            XCTAssertEqual(Set(options).count, options.count, options.description)
            XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(options), options.description)
        }
        XCTAssertEqual(CabalProposalExpiry.oneHour.caption, "A vote that hasn't passed closes after 1 hour.")
        XCTAssertEqual(Set(CabalJoinMode.allCases.map(\.id)), ["open", "request"])
        XCTAssertEqual(CabalVoterMode.allCases.map(\.id), ["all", "list"])
        XCTAssertEqual(CabalThreshold.allCases.map(\.id), ["majority", "unanimous"])
        XCTAssertEqual(CabalProposalExpiry.allCases.map(\.id), [3600, 86_400, 604_800])
    }

    func testThereIsAnInputExactlyWhenTheNameHasNoProblem() {
        for name in ["", " ", "ab", "abc", String(repeating: "x", count: 41), "a\nbc", "QA pot"] {
            let form = CreateCabalForm(name: name)
            XCTAssertEqual(form.input == nil, form.nameProblem != nil, name)
        }
    }

    func testTheInputCarriesEveryRule() {
        let form = CreateCabalForm(
            name: "QA pot", joinMode: .request, voterMode: .justMe, threshold: .unanimous, expiry: .oneHour)
        XCTAssertEqual(
            form.input,
            CreateCabalInput(
                name: "QA pot", joinMode: .request, voterMode: .justMe, threshold: .unanimous,
                proposalExpirySeconds: 3600))
    }

    private func body(_ form: CreateCabalForm) throws -> [String: Any] {
        let input = try XCTUnwrap(form.input)
        let data = try JSONEncoder().encode(input.request)
        return try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }
}
