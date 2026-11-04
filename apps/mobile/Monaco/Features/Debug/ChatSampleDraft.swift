#if DEBUG
enum ChatSampleDraft {
    static let flag = "-MonacoChatSampleDraft"
    static let presets = ["nvda": "Should we buy more NVDA?"]

    static func text(in arguments: [String]) -> String? {
        guard let index = arguments.firstIndex(of: flag), arguments.indices.contains(index + 1) else {
            return nil
        }
        let value = arguments[index + 1]
        return presets[value] ?? value
    }
}
#endif
