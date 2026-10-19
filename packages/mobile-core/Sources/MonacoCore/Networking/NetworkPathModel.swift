import Foundation
import Observation

#if canImport(Network)
import Network
#endif

@Observable
@MainActor
public final class NetworkPathModel {
    public private(set) var isOnline = true

    private let forcedOffline: Bool
    #if canImport(Network)
    @ObservationIgnored private var monitor: NWPathMonitor?
    #endif

    public init(forcedOffline: Bool = false) {
        self.forcedOffline = forcedOffline
        isOnline = !forcedOffline
    }

    public func start() {
        guard !forcedOffline else { return }
        #if canImport(Network)
        guard monitor == nil else { return }
        let monitor = NWPathMonitor()
        self.monitor = monitor
        monitor.pathUpdateHandler = { [weak self] path in
            let online = path.status != .unsatisfied
            Task { @MainActor in self?.isOnline = online }
        }
        monitor.start(queue: DispatchQueue(label: "monaco.network-path"))
        #endif
    }

    public func stop() {
        #if canImport(Network)
        monitor?.cancel()
        monitor = nil
        #endif
    }
}
