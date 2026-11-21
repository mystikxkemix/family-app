import Foundation

/// The language the app shows (French or English), sent to the API as
/// `lang` so TMDB texts match the app.
enum AppLanguage {
    static var code: String {
        let current = Bundle.main.preferredLocalizations.first ?? "en"
        return current.hasPrefix("fr") ? "fr" : "en"
    }

    /// The `lang` query item for search and details calls.
    static var queryItem: URLQueryItem {
        URLQueryItem(name: "lang", value: code)
    }
}
