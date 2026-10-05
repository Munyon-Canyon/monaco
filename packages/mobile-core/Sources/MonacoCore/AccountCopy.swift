import Foundation

public enum AccountCopy {
    public static let deleteTitle = "Delete account"
    public static let explainer =
        "Deleting your account removes your name, photo, phone and X from Monaco. Your handle stays reserved. "
        + "Your transaction history stays, because cabal records need it. This can't be undone."
    public static let cashOutStep = "Cash out of every cabal"
    public static func yourSlice(_ amount: String) -> String { "Your slice \(amount)" }
    public static let noCabalMoney = "No cabal holds money of yours."
    public static let withdrawStep = "Withdraw your balance"
    public static let accountBalance = "Account balance"
    public static let done = "Done"
    public static let confirmTitle = "Delete your Monaco account?"
    public static let confirmDelete = "Delete"
    public static let cancel = "Cancel"
    public static let deleting = "Deleting…"
    public static let deleted = "Your account was deleted."
    public static let loadFailed = "Couldn't load your account."
    public static let tryAgain = "Try again"
    public static let cashOutFirst = "Cash out of every cabal first."
    public static let withdrawFirst = "Withdraw your balance first."

    public static let auditedStrings = [
        deleteTitle, explainer, cashOutStep, yourSlice("$0.00"), noCabalMoney, withdrawStep, accountBalance, done,
        confirmTitle, confirmDelete, cancel, deleting, deleted, loadFailed, tryAgain, cashOutFirst, withdrawFirst,
    ]
}
