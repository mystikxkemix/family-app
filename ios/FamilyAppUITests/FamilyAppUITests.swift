import XCTest

/// Settings written by Scripts/ui-test.sh to build/ui-test/settings.json.
/// A local file, not environment variables: the server address and the setup
/// code never leave this machine.
private struct UITestSettings: Decodable {
    let serverURL: String
    let setupCode: String
    let screenshotsDir: String

    static func load() throws -> UITestSettings {
        // FamilyAppUITests/FamilyAppUITests.swift -> ios/build/ui-test/settings.json
        let iosDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        let file = iosDirectory.appending(path: "build/ui-test/settings.json")
        guard let data = try? Data(contentsOf: file) else {
            throw XCTSkip("No \(file.path). Run the UI tests with Scripts/ui-test.sh.")
        }
        return try JSONDecoder().decode(UITestSettings.self, from: data)
    }
}

/// Walks through every screen after login: first account, lists, watchlist,
/// places, events and settings, in English, then checks the tabs in French.
@MainActor
final class FamilyAppUITests: XCTestCase {
    private var app = XCUIApplication()
    private var settings: UITestSettings!

    func testScreensAfterLogin() throws {
        continueAfterFailure = false
        settings = try UITestSettings.load()
        app.launchArguments = ["-uiTestReset", "-AppleLanguages", "(en)", "-AppleLocale", "en_US"]
        app.launch()

        createFirstAccount()
        checkLists()
        checkWatchlist()
        checkPlaces()
        checkEvents()
        checkSettings()
        checkFrenchTabs()
    }

    // MARK: Steps

    private func createFirstAccount() {
        snapshot("00-login")
        // Password first, in "Log in" mode: the simulator's strong-password
        // AutoFill swallows typing in a "new password" field.
        type(settings.serverURL, into: app.textFields["auth.server"])
        type("alex", into: app.textFields["auth.username"])
        typeSecure("password-1", into: app.secureTextFields["auth.password"])
        app.segmentedControls["auth.mode"].buttons.element(boundBy: 2).tap()
        type("Alex", into: app.textFields["auth.displayName"])
        type(settings.setupCode, into: app.textFields["auth.code"])
        type("Our home", into: app.textFields["auth.householdName"])
        app.buttons["auth.submit"].tap()
        XCTAssertTrue(app.tabBars.buttons["Lists"].waitForExistence(timeout: 15), "login did not reach the tabs")
    }

    private func checkLists() {
        app.tabBars.buttons["Lists"].tap()
        app.buttons["toolbar.add"].tap()
        type("Groceries", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()

        let list = app.staticTexts["Groceries"]
        XCTAssertTrue(list.waitForExistence(timeout: 10))
        list.tap()

        let addField = app.textFields["list.addItem"]
        XCTAssertTrue(addField.waitForExistence(timeout: 5))
        for item in ["Milk", "Eggs", "Bread"] {
            addField.tap()
            addField.typeText(item + "\n")
            XCTAssertTrue(app.staticTexts[item].waitForExistence(timeout: 5), "\(item) not added")
        }
        app.staticTexts["Milk"].tap()
        XCTAssertTrue(element(containing: "in the cart").waitForExistence(timeout: 5), "checked item not moved")
        snapshot("01-list-detail")

        app.navigationBars.buttons.element(boundBy: 0).tap()
        XCTAssertTrue(app.staticTexts["2 left"].waitForExistence(timeout: 5), "list count not updated")
        snapshot("02-lists")
    }

    private func checkWatchlist() {
        app.tabBars.buttons["Watchlist"].tap()
        app.buttons["toolbar.add"].tap()
        type("Arrival", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()
        let movie = app.staticTexts["Arrival"]
        XCTAssertTrue(movie.waitForExistence(timeout: 10))

        app.buttons["watchlist.pick"].tap()
        XCTAssertTrue(app.alerts.staticTexts["Arrival"].waitForExistence(timeout: 5), "pick for us did not pick")
        snapshot("03-watchlist-pick")
        app.alerts.buttons["Cancel"].tap()

        movie.swipeRight()
        app.buttons["Watched"].firstMatch.tap()
        app.segmentedControls["status.picker"].buttons.element(boundBy: 2).tap()
        XCTAssertTrue(app.staticTexts["Arrival"].waitForExistence(timeout: 5), "movie not in Watched")
        snapshot("04-watchlist-watched")
    }

    private func checkPlaces() {
        app.tabBars.buttons["Places"].tap()
        app.buttons["toolbar.add"].tap()
        type("Chez Lulu", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()
        let place = app.staticTexts["Chez Lulu"]
        XCTAssertTrue(place.waitForExistence(timeout: 10))
        snapshot("05-places")

        place.swipeRight()
        app.buttons["Tried"].firstMatch.tap()
        app.segmentedControls["status.picker"].buttons.element(boundBy: 1).tap()
        XCTAssertTrue(app.staticTexts["Chez Lulu"].waitForExistence(timeout: 5), "place not in Tried")
    }

    private func checkEvents() {
        app.tabBars.buttons["Events"].tap()
        app.buttons["toolbar.add"].firstMatch.tap()
        type("Dinner with friends", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()
        XCTAssertTrue(app.staticTexts["Dinner with friends"].waitForExistence(timeout: 10))
        snapshot("06-events")
    }

    private func checkSettings() {
        app.tabBars.buttons["Settings"].tap()
        let invite = app.buttons["settings.invite"]
        XCTAssertTrue(invite.waitForExistence(timeout: 5))
        invite.tap()
        XCTAssertTrue(app.staticTexts["settings.inviteCode"].waitForExistence(timeout: 5), "no invite code")
        snapshot("07-settings")
    }

    /// Relaunches in French (the login is kept) and checks the tab titles.
    private func checkFrenchTabs() {
        app.terminate()
        app.launchArguments = ["-AppleLanguages", "(fr)", "-AppleLocale", "fr_FR"]
        app.launch()
        for (index, title) in ["Listes", "À regarder", "Lieux", "Événements", "Réglages"].enumerated() {
            let tab = app.tabBars.buttons[title]
            XCTAssertTrue(tab.waitForExistence(timeout: 10), "French tab \(title) missing")
            tab.tap()
            snapshot("1\(index)-fr-\(title)")
        }
    }

    // MARK: Helpers

    private func type(_ text: String, into field: XCUIElement) {
        XCTAssertTrue(field.waitForExistence(timeout: 5), "missing field \(field)")
        field.tap()
        field.typeText(text)
    }

    /// Secure fields drop keys when the text comes in one burst: type one
    /// character at a time, then check the length.
    private func typeSecure(_ text: String, into field: XCUIElement) {
        XCTAssertTrue(field.waitForExistence(timeout: 5), "missing field \(field)")
        field.tap()
        for character in text {
            field.typeText(String(character))
        }
        let typed = (field.value as? String)?.count ?? 0
        XCTAssertEqual(typed, text.count, "secure field got \(typed) of \(text.count) characters")
    }

    /// An element whose label contains the text, ignoring case (section
    /// headers can be shown in upper case).
    private func element(containing text: String) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS[c] %@", text)).firstMatch
    }

    /// Keeps a screenshot in the test report and in build/ui-test/screenshots.
    private func snapshot(_ name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        let file = URL(fileURLWithPath: settings.screenshotsDir).appending(path: "\(name).png")
        try? screenshot.pngRepresentation.write(to: file)
    }
}
