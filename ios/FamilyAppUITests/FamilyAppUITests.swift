import XCTest

/// Settings written by Scripts/ui-test.sh to build/ui-test/settings.json:
/// the test API address and its admin account (ADMIN_USERNAME / ADMIN_PASSWORD).
private struct UITestSettings: Decodable {
    let serverURL: String
    let adminUsername: String
    let adminPassword: String
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

/// Walks through every screen after login: first admin login with the forced
/// password change, lists, watchlist, places, events and settings, in English,
/// then checks the tabs in French.
@MainActor
final class FamilyAppUITests: XCTestCase {
    private var app = XCUIApplication()
    private var settings: UITestSettings!

    func testScreensAfterLogin() throws {
        continueAfterFailure = false
        settings = try UITestSettings.load()
        app.launchArguments = ["-uiTesting", "-uiTestReset", "-AppleLanguages", "(en)", "-AppleLocale", "en_US"]
        app.launch()

        firstAdminLogin()
        checkLists()
        checkWatchlist()
        checkPlaces()
        checkEvents()
        checkSettings()
        checkIPadLandscape()
        checkFrenchTabs()
    }

    // MARK: Steps

    private func firstAdminLogin() {
        snapshot("00-login")
        type(settings.serverURL, into: app.textFields["auth.server"])
        type(settings.adminUsername, into: app.textFields["auth.username"])
        typeSecure(settings.adminPassword, into: app.secureTextFields["auth.password"])
        app.buttons["auth.submit"].tap()

        // The admin password comes from the server settings: it must be changed first.
        let newPassword = app.secureTextFields["password.new"]
        XCTAssertTrue(newPassword.waitForExistence(timeout: 15), "no forced password change after the first login")
        XCTAssertFalse(tab("Lists").exists, "the app must stay locked until the password changes")
        snapshot("00-choose-password")
        typeSecure("password-1", into: newPassword)
        typeSecure("password-1", into: app.secureTextFields["password.confirm"])
        app.buttons["password.save"].tap()
        XCTAssertTrue(tab("Lists").waitForExistence(timeout: 15), "the password change did not unlock the app")
    }

    private func checkLists() {
        tab("Lists").tap()
        app.buttons["toolbar.add"].tap()
        type("Groceries", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()

        let list = app.staticTexts["Groceries"].firstMatch
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

        // On iPad the lists stay visible next to the items: no back button.
        if !isPad {
            app.navigationBars.buttons.element(boundBy: 0).tap()
        }
        XCTAssertTrue(app.staticTexts["2 left"].waitForExistence(timeout: 5), "list count not updated")
        snapshot("02-lists")
    }

    private func checkWatchlist() {
        tab("Watchlist").tap()
        app.buttons["toolbar.add"].tap()
        type("Arrival", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()
        let movie = app.staticTexts["Arrival"]
        XCTAssertTrue(movie.waitForExistence(timeout: 10))

        app.buttons["watchlist.pick"].tap()
        XCTAssertTrue(app.alerts.staticTexts["Arrival"].waitForExistence(timeout: 5), "pick for us did not pick")
        snapshot("03-watchlist-pick")
        app.alerts.buttons["Cancel"].tap()

        // Rows have swipe actions; the iPad poster grid has the same actions
        // in its long-press menu.
        if isPad {
            movie.press(forDuration: 1.2)
            // The status filter also has a "Watched" button: tap the other one,
            // the menu item.
            let watched = app.buttons.matching(NSPredicate(format: "label == %@", "Watched"))
            XCTAssertTrue(watched.element(boundBy: 1).waitForExistence(timeout: 5), "long-press menu not shown")
            let filterButton = app.segmentedControls["status.picker"].buttons["Watched"]
            let menuItem = watched.allElementsBoundByIndex.first { $0.frame != filterButton.frame }
            XCTAssertNotNil(menuItem, "no Watched item in the long-press menu")
            menuItem?.tap()
        } else {
            movie.swipeRight()
            app.buttons["Watched"].firstMatch.tap()
        }
        app.segmentedControls["status.picker"].buttons.element(boundBy: 2).tap()
        XCTAssertTrue(app.staticTexts["Arrival"].waitForExistence(timeout: 5), "movie not in Watched")
        snapshot("04-watchlist-watched")

        // The details page (no TMDB key in the test API: our own data only).
        app.staticTexts["Arrival"].firstMatch.tap()
        XCTAssertTrue(app.buttons["details.edit"].waitForExistence(timeout: 5), "details page not opened")
        snapshot("08-details")
        app.navigationBars.buttons.element(boundBy: 0).tap()
    }

    private func checkPlaces() {
        tab("Places").tap()
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
        tab("Events").tap()
        app.buttons["toolbar.add"].firstMatch.tap()
        type("Dinner with friends", into: app.textFields["editor.name"])
        app.buttons["editor.save"].tap()
        XCTAssertTrue(app.staticTexts["Dinner with friends"].waitForExistence(timeout: 10))
        snapshot("06-events")
    }

    private func checkSettings() {
        tab("Settings").tap()
        let invite = app.buttons["settings.invite"]
        XCTAssertTrue(invite.waitForExistence(timeout: 5))
        invite.tap()
        XCTAssertTrue(app.staticTexts["settings.inviteCode"].waitForExistence(timeout: 5), "no invite code")
        snapshot("07-settings")
    }

    /// Relaunches in French (the login is kept) and checks the tab titles.
    private func checkFrenchTabs() {
        app.terminate()
        app.launchArguments = ["-uiTesting", "-AppleLanguages", "(fr)", "-AppleLocale", "fr_FR"]
        app.launch()
        for (index, title) in ["Listes", "À regarder", "Lieux", "Événements", "Réglages"].enumerated() {
            let frenchTab = tab(title)
            XCTAssertTrue(frenchTab.waitForExistence(timeout: 10), "French tab \(title) missing")
            frenchTab.tap()
            snapshot("1\(index)-fr-\(title)")
        }
    }

    /// iPad only: the main screens in landscape, for the screenshots.
    private func checkIPadLandscape() {
        guard isPad else { return }
        XCUIDevice.shared.orientation = .landscapeLeft
        defer { XCUIDevice.shared.orientation = .portrait }
        tab("Lists").tap()
        XCTAssertTrue(app.staticTexts["2 left"].waitForExistence(timeout: 5), "lists column missing in landscape")
        snapshot("20-ipad-landscape-lists")
        tab("Watchlist").tap()
        XCTAssertTrue(app.staticTexts["Arrival"].waitForExistence(timeout: 5))
        snapshot("21-ipad-landscape-watchlist")
        app.staticTexts["Arrival"].firstMatch.tap()
        XCTAssertTrue(app.buttons["details.edit"].waitForExistence(timeout: 5))
        snapshot("22-ipad-landscape-details")
        app.navigationBars.buttons.element(boundBy: 0).tap()
    }

    // MARK: Helpers

    /// True on iPad: the layout has a sidebar, two columns and a poster grid.
    private var isPad: Bool { UIDevice.current.userInterfaceIdiom == .pad }

    /// A main screen, by its title: a tab bar button on iPhone, a top bar
    /// button (portrait) or a sidebar cell (landscape) on iPad.
    private func tab(_ title: String) -> XCUIElement {
        let types = [XCUIElement.ElementType.button.rawValue, XCUIElement.ElementType.cell.rawValue]
        let predicate = NSPredicate(format: "label == %@ AND elementType IN %@", title, types)
        return app.descendants(matching: .any).matching(predicate).firstMatch
    }

    private func type(_ text: String, into field: XCUIElement) {
        XCTAssertTrue(field.waitForExistence(timeout: 5), "missing field \(field)")
        field.tap()
        field.typeText(text)
    }

    /// Secure fields drop keys when the text comes in one burst, mostly on a
    /// fresh simulator: wait for the keyboard, type one character at a time,
    /// and start again (up to 3 times) when the length is wrong. The length is
    /// all a test can read from a secure field.
    private func typeSecure(_ text: String, into field: XCUIElement) {
        XCTAssertTrue(field.waitForExistence(timeout: 5), "missing field \(field)")
        var typed = 0
        for _ in 1...3 {
            field.tap()
            // Wait for the field to take the keys. Not for an on-screen
            // keyboard: an iPad simulator may use the Mac keyboard instead.
            let focused = XCTNSPredicateExpectation(predicate: NSPredicate(format: "hasKeyboardFocus == true"), object: field)
            _ = XCTWaiter.wait(for: [focused], timeout: 5)
            let existing = secureLength(field)
            if existing > 0 {
                field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: existing))
            }
            for character in text {
                field.typeText(String(character))
                Thread.sleep(forTimeInterval: 0.15)
            }
            typed = secureLength(field)
            if typed == text.count { return }
        }
        XCTFail("secure field got \(typed) of \(text.count) characters")
    }

    /// Number of characters in a secure field (its value is one bullet each,
    /// or the placeholder when empty).
    private func secureLength(_ field: XCUIElement) -> Int {
        let value = field.value as? String ?? ""
        return value == field.placeholderValue ? 0 : value.count
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
