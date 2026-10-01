import XCTest

/// Every accessibility audit on a sample screen. `AccessibilityAuditAllowlist.txt`
/// rows are `class`, audit type, and element identifier, separated by tabs. A
/// matching row is skipped. The file only shrinks.
enum SampleAccessibilityAudit {
    @MainActor
    static func run(
        _ app: XCUIApplication,
        in test: XCTestCase,
        file: StaticString = #filePath,
        line: UInt = #line
    ) throws {
        let className = String(describing: type(of: test))
        let skipped = allowlist(in: test, file: file, line: line)
        var failures: [String] = []
        // One type at a time. A single `.all` pass on a chart-heavy screen times out
        // before it reports anything, and a timeout hides every other finding.
        // Dynamic Type is the slow pass. Running it first keeps a chart-heavy screen
        // inside the audit's own time budget.
        for audit in auditTypes.sorted(by: { lhs, rhs in
            (lhs == .dynamicType ? 0 : 1) < (rhs == .dynamicType ? 0 : 1)
        }) {
            do {
                try app.performAccessibilityAudit(for: audit) { issue in
                    let kind = auditKind(issue.auditType)
                    let elementID = issue.element?.identifier ?? ""
                    if skipped.contains(where: {
                        $0.className == className && $0.kind == kind && $0.elementID == elementID
                    }) {
                        return true
                    }
                    let label = (issue.element?.label ?? "").replacingOccurrences(of: "\n", with: " ")
                    let value = (issue.element?.value as? String ?? "").replacingOccurrences(of: "\n", with: " ")
                    let detail = issue.detailedDescription.replacingOccurrences(of: "\n", with: " | ")
                    failures.append(
                        "\(className)\t\(kind)\t\(elementID)\tlabel=\(label)\tvalue=\(value)\t\(detail)"
                    )
                    return true
                }
            } catch {
                let kind = auditKind(audit)
                failures.append(
                    "\(className)\t\(kind)\t\tlabel=\tvalue=\taudit error: \(error.localizedDescription)"
                )
            }
        }
        if !failures.isEmpty {
            XCTFail(
                "accessibility audit found \(failures.count) issue(s):\n\(failures.joined(separator: "\n"))",
                file: file,
                line: line
            )
        }
    }

    private struct Row {
        var className: String
        var kind: String
        var elementID: String
    }

    private static func allowlist(
        in test: XCTestCase,
        file: StaticString,
        line: UInt
    ) -> [Row] {
        guard
            let url = Bundle(for: type(of: test)).url(
                forResource: "AccessibilityAuditAllowlist",
                withExtension: "txt"
            )
        else {
            XCTFail(
                "AccessibilityAuditAllowlist.txt is missing from the UI test bundle",
                file: file,
                line: line
            )
            return []
        }
        guard let text = try? String(contentsOf: url, encoding: .utf8) else {
            XCTFail("AccessibilityAuditAllowlist.txt could not be read", file: file, line: line)
            return []
        }
        var rows: [Row] = []
        for rawLine in text.split(whereSeparator: \.isNewline) {
            let raw = String(rawLine)
            if raw.trimmingCharacters(in: .whitespaces).isEmpty || raw.hasPrefix("#") {
                continue
            }
            let parts = raw.split(separator: "\t", omittingEmptySubsequences: false).map(String.init)
            guard parts.count == 3 else {
                XCTFail("allowlist row needs three tab-separated fields: \(raw)", file: file, line: line)
                continue
            }
            rows.append(Row(className: parts[0], kind: parts[1], elementID: parts[2]))
        }
        return rows
    }

    private static let auditTypes: [XCUIAccessibilityAuditType] = [
        .contrast,
        .elementDetection,
        .hitRegion,
        .sufficientElementDescription,
        .dynamicType,
        .textClipped,
        .trait,
    ]

    private static func auditKind(_ type: XCUIAccessibilityAuditType) -> String {
        let named: [(XCUIAccessibilityAuditType, String)] = [
            (.contrast, "contrast"),
            (.elementDetection, "elementDetection"),
            (.hitRegion, "hitRegion"),
            (.sufficientElementDescription, "sufficientElementDescription"),
            (.dynamicType, "dynamicType"),
            (.textClipped, "textClipped"),
            (.trait, "trait"),
        ]
        let hits = named.filter { type.contains($0.0) }.map(\.1)
        return hits.isEmpty ? "unknown" : hits.joined(separator: "+")
    }
}
