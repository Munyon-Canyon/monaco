import Foundation

public enum OnboardingCopy {
    public static let handleTitle = "Pick your handle"
    public static let continueLabel = "Continue"
    public static let signOut = "Sign out"
    public static let newAccountTitle = "Create a new account?"
    public static let newAccountBody = "If you already have one, sign in with the method you used before."
    public static let createNewAccount = "Create new account"
    public static let signInAnotherWay = "Sign in another way"

    public static let restrictedTitle = "Your account is restricted"
    public static let restrictedBody = "You can still withdraw your balance and cash out of your cabals."
    public static let withdraw = "Withdraw"
    public static let cashOut = "Cash out"
    public static let deleteAccount = "Delete account"

    public static let underReviewNotice = "Your account is under review. You can still withdraw and cash out."

    public static let auditedStrings = [
        handleTitle, continueLabel, signOut, newAccountTitle, newAccountBody, createNewAccount, signInAnotherWay,
        restrictedTitle, restrictedBody, withdraw, cashOut, deleteAccount,
        underReviewNotice,
    ]
}
