#!/usr/bin/env bash
# Runs the UI tests in the simulator against a throwaway API and MongoDB.
#
# 1. Starts MongoDB and the API (built from ../api) in Docker, on a private network.
# 2. Gives the API a throwaway admin account (ADMIN_USERNAME / ADMIN_PASSWORD).
# 3. Writes build/ui-test/settings.json (server address + admin account), read
#    by the tests.
# 4. Runs the tests, then removes the containers.
#
# Usage: Scripts/ui-test.sh ["iPhone 17"]   (device type, optional)
# The tests run on their own simulator, "FamilyApp UI Tests", created once:
# other simulators (and other work on them) are never touched.
# Screenshots end up in build/ui-test/screenshots.
set -euo pipefail

cd "$(dirname "$0")/.."
device_type="${1:-iPhone 17}"
simulator_name="FamilyApp UI Tests"
port=8188
prefix=family-uitest
work=build/ui-test

cleanup() {
  docker rm -f "${prefix}-api" "${prefix}-mongo" >/dev/null 2>&1 || true
  docker network rm "${prefix}" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

echo "Starting MongoDB and the API..."
docker network create "${prefix}" >/dev/null
docker run -d --name "${prefix}-mongo" --network "${prefix}" mongo:8 >/dev/null
until docker exec "${prefix}-mongo" mongosh --quiet --eval 'db.runCommand({ ping: 1 }).ok' >/dev/null 2>&1; do sleep 1; done
docker build -q -t family-api:uitest ../api >/dev/null
admin_password="uitest-$(openssl rand -hex 6)"
docker run -d --name "${prefix}-api" --network "${prefix}" -p "${port}:8080" \
  -e "MONGODB_URI=mongodb://${prefix}-mongo:27017" \
  -e "ADMIN_USERNAME=admin" -e "ADMIN_PASSWORD=${admin_password}" family-api:uitest >/dev/null
for _ in $(seq 1 30); do
  curl -fs "http://localhost:${port}/healthz" >/dev/null && break
  sleep 1
done
curl -fs "http://localhost:${port}/healthz" >/dev/null || { echo "The API did not start"; docker logs "${prefix}-api"; exit 1; }

rm -rf "${work}/screenshots" "${work}/result.xcresult"
mkdir -p "${work}/screenshots"
cat > "${work}/settings.json" <<JSON
{
  "serverURL": "http://localhost:${port}",
  "adminUsername": "admin",
  "adminPassword": "${admin_password}",
  "screenshotsDir": "$(pwd)/${work}/screenshots"
}
JSON

# Find or create the dedicated simulator.
simulator=$(xcrun simctl list devices available -j | python3 -c '
import json, sys
name = sys.argv[1]
for devices in json.load(sys.stdin)["devices"].values():
    for device in devices:
        if device["name"] == name:
            print(device["udid"]); sys.exit()
' "$simulator_name")
if [ -z "$simulator" ]; then
  simulator=$(xcrun simctl create "$simulator_name" "$device_type")
fi

xcodegen generate --quiet
echo "Running the UI tests on ${simulator_name} (${simulator})..."
xcodebuild test \
  -project FamilyApp.xcodeproj -scheme FamilyApp \
  -destination "id=${simulator}" \
  -derivedDataPath "${work}/derived" \
  -resultBundlePath "${work}/result.xcresult" \
  -test-timeouts-enabled YES -default-test-execution-time-allowance 300 \
  -quiet
echo "UI tests passed. Screenshots: $(pwd)/${work}/screenshots"
