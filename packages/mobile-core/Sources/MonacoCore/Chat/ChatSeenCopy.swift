public enum ChatSeenCopy {
    public static func label(count: Int) -> String? {
        count > 0 ? "Seen by \(count)" : nil
    }
}
