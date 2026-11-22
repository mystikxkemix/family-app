# iPhone app: build, sign, test

## Build for the simulator

```sh
cd ios && xcodegen generate
xcodebuild build -project FamilyApp.xcodeproj -scheme FamilyApp \
  -destination 'generic/platform=iOS Simulator' -derivedDataPath build/sim CODE_SIGNING_ALLOWED=NO
```

The `.xcodeproj` is generated and gitignored: change `project.yml`, never the
project in Xcode.

## Signing and install on a phone

- One gitignored profile per Apple team: `ios/Config/Signing/<name>.xcconfig`
  (`DEVELOPMENT_TEAM`, `APP_BUNDLE_ID`, `APS_ENVIRONMENT`). Each team needs its
  own bundle id. The real profiles and device ids are in `CLAUDE.local.md`.
- Install: `ios/Scripts/install.sh <profile> <device-id>` (device ids:
  `xcrun devicectl list devices`; a paired phone shows "available (paired)").
- Xcode has no Apple account signed in on this Mac, so automatic signing can
  only use the team **wildcard** profile, which has no push. Such profiles
  set `CODE_SIGN_ENTITLEMENTS = FamilyApp/Resources/FamilyApp-NoPush.entitlements`.
  An empty `CODE_SIGN_ENTITLEMENTS =` does not work (ignored).
- Push needs: the team account in Xcode > Settings > Accounts, the line above
  removed, and the team's APNs key in the server settings (topic = bundle id).
- `devicectl ... process launch` fails with "Locked" when the phone is locked:
  not an error of the app.

## Localization

`FamilyApp/Resources/Localizable.xcstrings` (EN source, FR) and
`InfoPlist.xcstrings` (app name: Family / Famille). After a build, every key
the compiler extracted is in `build/sim/**/*.stringsdata`; check that each has
a French entry (see the `check-strings` step in the `family-feature` skill).
Keys with `extractionState: manual` (the `push.*` keys) are used by APNs, not
by code: keep them.

## UI tests

`ios/Scripts/ui-test.sh` (see the `family-ui-tests` skill). Facts learned the
hard way:

- Each device type has its own dedicated simulator, "FamilyApp UI Tests -
  <device type>" (the script creates it): `ios/Scripts/ui-test.sh` (iPhone 17)
  or `ios/Scripts/ui-test.sh "iPad Pro 11-inch (M5)"`. Another tool on this Mac
  also boots "iPhone 17" simulators: a name destination is ambiguous and the
  run hangs. Run one device type at a time (fixed container names and port).
- Screenshots: `ios/build/ui-test/screenshots/<device type>/`.
- Tabs are found by label (`app.buttons["Lists"]`), which works for the
  iPhone tab bar and the iPad sidebar/top bar. On iPad the test skips the
  list "back" button (both columns are visible), long-presses the poster grid
  instead of swiping, and takes landscape screenshots (`20-`, `21-`, `22-`).
- The app runs with `-uiTesting`: AutoFill hints are removed
  (`UITextContentType.autoFill`), else the simulator's strong-password AutoFill
  swallows keys and "Save password?" sheets cover the app.
- `-uiTestReset` (Debug only) starts logged out.
- Secure fields can still drop keys on a fresh simulator: the test types one
  character at a time and checks the length.
- `xcodebuild` per-test time limit is 300 s; a run longer than ~6 min is stuck.

## iPad

- Same app (`TARGETED_DEVICE_FAMILY = 1,2`); all orientations and Split View
  on iPad (`UISupportedInterfaceOrientations~ipad`), portrait only on iPhone.
- Layouts switch on `horizontalSizeClass`, never on the device: an iPad in a
  narrow Split View gets the iPhone layout.
- `TabView` uses `.tabViewStyle(.sidebarAdaptable)`: tab bar on iPhone,
  sidebar/top bar on iPad.
- Regular width: Lists = `NavigationSplitView` (lists | items, the counts
  reload through `ListDetailView.onChange`); Watchlist = poster grid with a
  long-press menu (status, interest, delete); other screens use
  `readableContentWidth()` (`Core/Layout.swift`: centered ~720 pt column via
  content margins, so list backgrounds still fill the screen).
- Keyboard: ⌘N on each "new" button, ⌘R on `WideRefreshButton` (regular width
  only).
- Install on the iPad like a phone: `ios/Scripts/install.sh <profile> <iPad id>`
  (the iPad must be registered in the team's devices).

## App icon and splash

Made with the Codex CLI: see `ios/Design/README.md`. Gotchas: run
`codex exec ... < /dev/null` (it waits on stdin otherwise) and use
`--image=FILE` with `=` (with a space it eats the prompt).
