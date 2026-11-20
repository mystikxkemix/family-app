---
name: family-ui-tests
description: Run the Family app UI tests in the simulator against a throwaway API, read failures and screenshots. Use after UI changes, or when asked to test the screens or run the UI tests.
---

# UI tests

1. Run: `ios/Scripts/ui-test.sh > ios/build/ui-test-run.log 2>&1` in the
   background (a full run takes 3-6 minutes). It starts MongoDB + the API in
   Docker with a random admin password, writes
   `ios/build/ui-test/settings.json`, and tests on the dedicated simulator
   "FamilyApp UI Tests".
2. Failures:

   ```sh
   xcrun xcresulttool get test-results tests --path ios/build/ui-test/result.xcresult | python3 -c "
   import json,sys
   def walk(n):
     if n.get('nodeType')=='Failure Message': print(n.get('name'))
     for c in n.get('children',[]): walk(c)
   for t in json.load(sys.stdin)['testNodes']: walk(t)"
   ```

   The UI tree at failure: `xcrun xcresulttool export attachments --path ... --output-path <dir>`,
   then the `.txt` attachment that starts with `Application,`.
3. Screenshots: `ios/build/ui-test/screenshots/*.png` (step name in the file name).
4. Stuck run (no new screenshot for 2 minutes): stop it
   (`pkill -f "Scripts/ui-test.sh"; pkill -f "xcodebuild test -project FamilyApp"`)
   and remove the containers (`docker rm -f family-uitest-api family-uitest-mongo`).
   Known causes are in `.claude/docs/ios.md` (UI tests).
