import Foundation
import OpenAPIRuntime

struct FlexibleISO8601: DateTranscoder {
    func encode(_ date: Date) throws -> String {
        try ISO8601DateTranscoder.iso8601.encode(date)
    }

    func decode(_ dateString: String) throws -> Date {
        if dateString.contains(".") {
            let milliseconds = Self.truncatingFraction(dateString, toDigits: 3)
            if let date = try? ISO8601DateTranscoder.iso8601WithFractionalSeconds.decode(milliseconds) {
                return date
            }
        }
        if let date = try? ISO8601DateTranscoder.iso8601.decode(dateString) {
            return date
        }
        if let dot = dateString.firstIndex(of: ".") {
            let tail = dateString[dateString.index(after: dot)...].firstIndex { !$0.isNumber } ?? dateString.endIndex
            let whole = String(dateString[..<dot]) + dateString[tail...]
            if let date = try? ISO8601DateTranscoder.iso8601.decode(whole) {
                return date
            }
        }
        throw DecodingError.dataCorrupted(
            .init(codingPath: [], debugDescription: "Expected date string to be ISO8601-formatted.")
        )
    }

    private static func truncatingFraction(_ value: String, toDigits digits: Int) -> String {
        guard let dot = value.firstIndex(of: ".") else { return value }
        let start = value.index(after: dot)
        let end = value[start...].firstIndex { !$0.isNumber } ?? value.endIndex
        let fraction = value[start..<end]
        guard fraction.count > digits else { return value }
        return String(value[..<start]) + fraction.prefix(digits) + value[end...]
    }
}
