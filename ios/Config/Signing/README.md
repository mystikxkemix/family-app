# Signing profiles

The app is signed by more than one Apple team (one per phone). Team ids and
bundle ids are personal, so they live in gitignored files here.

1. Copy `example.xcconfig` to `<name>.xcconfig`, one per team, and fill in
   `DEVELOPMENT_TEAM` and `APP_BUNDLE_ID`.
2. Build and install with the script:

   ```sh
   ios/Scripts/install.sh <name>              # builds and installs on the plugged-in iPhone
   ios/Scripts/install.sh <name> <device-id>  # pick a device (see: xcrun devicectl list devices)
   ```

3. To build from the Xcode UI instead, create `Config/Local.xcconfig` with:

   ```
   #include "Signing/<name>.xcconfig"
   ```

Xcode must be logged into the Apple account of each team
(Xcode > Settings > Accounts): automatic signing creates the App ID, the push
capability and the provisioning profile on the first build.
