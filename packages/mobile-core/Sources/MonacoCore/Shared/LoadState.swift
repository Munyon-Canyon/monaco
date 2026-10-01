import MonacoAPI

public enum LoadState<Value: Sendable>: Sendable {
    case idle
    case loading
    case loaded(Value)
    case failed(APIError)
}

extension LoadState: Equatable where Value: Equatable {}
