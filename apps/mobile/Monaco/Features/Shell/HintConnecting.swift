import MonacoAPI

nonisolated protocol HintConnecting: HintSource, Sendable {
    func start() async
    func stop() async
}

nonisolated final class LiveHintConnection: HintConnecting, Sendable {
    private let stream: HintStream

    init(_ stream: HintStream) {
        self.stream = stream
    }

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        stream.hints(matching: filter)
    }

    func start() async {
        await stream.start()
    }

    func stop() async {
        await stream.stop()
    }
}
