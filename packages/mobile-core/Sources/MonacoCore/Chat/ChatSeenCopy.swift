import MonacoAPI

public enum ChatSeenCopy {
    public static let sheetTitle = "Seen by"
    public static let loadFailed = "Couldn't load who has seen this."
    public static let retry = "Try again"

    public static func label(count: Int) -> String? {
        count > 0 ? "Seen by \(count)" : nil
    }

    public static func name(_ member: Components.Schemas.ChatSeenMember) -> String {
        let name = member.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        return name.isEmpty ? "Former member" : name
    }

    public static func handle(_ member: Components.Schemas.ChatSeenMember) -> String? {
        guard let handle = member.handle, !handle.isEmpty else { return nil }
        return "@\(handle)"
    }
}
