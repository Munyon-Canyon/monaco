import Foundation

public func freshnessLabel(computedAt: Date, now: Date) -> String {
    let minutes = Int(now.timeIntervalSince(computedAt) / 60)
    if minutes < 3 { return "Live" }
    if minutes < 60 { return "Updated \(minutes) min ago" }
    let hours = minutes / 60
    if hours < 24 { return "Updated \(hours) h ago" }
    let days = hours / 24
    return days == 1 ? "Updated 1 day ago" : "Updated \(days) days ago"
}
