/// The one line a screen shows in a toast when an `APIClient` call fails.
public enum ToastCopy {
    public static func message(for error: APIError) -> String {
        switch error {
        case .missingAccessToken: "Your account didn't load"
        case .problem(let problem): problem.message
        case .transport: "You're offline. Try again."
        case .inFlight: "Still working on it."
        case .decoding: "Something went wrong. Try again."
        case .signedOut: "Please sign in again."
        case .accountDeleted: "This account was deleted."
        }
    }
}
