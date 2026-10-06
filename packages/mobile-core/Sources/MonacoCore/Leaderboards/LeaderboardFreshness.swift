import Foundation

public func freshnessLabel(computedAt: Date, now: Date) -> String {
    let minutes = Int(now.timeIntervalSince(computedAt) / 60)
    if minutes < 3 { return "Live" }
    if minutes < 60 { return "Updated \(minutes) min ago" }
    return "Updated \(minutes / 60) h ago"
}
