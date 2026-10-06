import Foundation

extension ChatSession {
    public func open() async {
        guard !isOpen else { return }
        isOpen = true
        openGeneration += 1
        let generation = openGeneration
        openBuffer = []
        catchUpOwed = false
        subscribe()
        if !channelAttached { await awaitFirstAttach(generation: generation) }
        guard generation == openGeneration else { return }
        guard isOpen else {
            openBuffer = nil
            return
        }
        if state.timeline.newestID == nil {
            await loadNewest()
        } else {
            await catchUp()
        }
        if catchUpOwed { await catchUp() }
        catchUpOwed = false
        let buffered = openBuffer ?? []
        openBuffer = nil
        for message in buffered { await apply(.messageCreated(message)) }
        if state.isClosed, attached.isEmpty { stopListening() }
    }

    public func reload() async {
        await loadNewest()
        if isOpen, !state.isClosed { subscribe() }
    }

    func bufferWhileOpening(_ message: ChatMessage) -> Bool {
        guard openBuffer != nil else { return false }
        openBuffer?.append(message)
        return true
    }

    func applyAttach(resumed: Bool) async {
        if let attachSignal {
            attachSignal.yield(true)
        } else if !resumed {
            if openBuffer != nil { catchUpOwed = true } else { await catchUp() }
        }
    }

    private func awaitFirstAttach(generation: Int) async {
        let (stream, continuation) = AsyncStream.makeStream(of: Bool.self, bufferingPolicy: .bufferingNewest(1))
        attachSignal = continuation
        let timeout = attachTimeout
        let clock = clock
        let waiter = Task {
            for await attached in stream { return attached }
            return false
        }
        let timer = Task {
            try? await clock.sleep(for: timeout)
            continuation.finish()
        }
        _ = await waiter.value
        timer.cancel()
        if generation == openGeneration { attachSignal = nil }
        continuation.finish()
    }
}
