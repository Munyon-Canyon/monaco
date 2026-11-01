import Foundation

public enum StakeValueNote: Equatable, Sendable {
    case settling
    case valueAsOf(Date?)

    public static func make(valueMicros: Int64, arrivingMicros: Int64, isStale: Bool, valuedAt: Date?) -> Self? {
        if valueMicros <= 0, arrivingMicros > 0 { return .settling }
        return isStale ? .valueAsOf(valuedAt) : nil
    }

    public func text(now: Date, calendar: Calendar = .current) -> String {
        switch self {
        case .settling:
            return "Settling"
        case .valueAsOf(let date):
            guard let date else { return "Value from an earlier run" }
            let formatter = DateFormatter()
            formatter.locale = Locale(identifier: "en_US_POSIX")
            formatter.calendar = calendar
            formatter.timeZone = calendar.timeZone
            formatter.dateFormat = calendar.isDate(date, inSameDayAs: now) ? "h:mm a" : "MMM d"
            return "Value as of \(formatter.string(from: date))"
        }
    }
}
