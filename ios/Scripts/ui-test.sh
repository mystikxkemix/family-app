#!/usr/bin/env bash
# Runs the UI tests in the simulator against a throwaway API and MongoDB.
#
# 1. Starts MongoDB and the API (built from ../api) in Docker, on a private network.
# 2. Reads the one-time setup code from the API logs.
# 3. Writes build/ui-test/settings.json (server address + setup code), read by
#    the tests. No environment variables: the values stay in this local file.
# 4. Runs the tests, then removes the containers.
#
# Usage: Scripts/ui-test.sh ["iPhone 17"]   (simulator name, optional)
# Screenshots end up in build/ui-test/screenshots.
set -euo pipefail

cd "$(dirname "$0")/.."
simulator="${1:-iPhone 17}"
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
docker run -d --name "${prefix}-api" --network "${prefix}" -p "${port}:8080" \
  -e "MONGODB_URI=mongodb://${prefix}-mongo:27017" family-api:uitest >/dev/null
for _ in $(seq 1 30); do
  curl -fs "http://localhost:${port}/healthz" >/dev/null && break
  sleep 1
done
setup_code=$(docker logs "${prefix}-api" 2>&1 | grep -o '"setupCode":"[^"]*"' | cut -d'"' -f4)
[ -n "$setup_code" ] || { echo "No setup code in the API logs"; docker logs "${prefix}-api"; exit 1; }

rm -rf "${work}/screenshots" "${work}/result.xcresult"
mkdir -p "${work}/screenshots"
cat > "${work}/settings.json" <<JSON
{
  "serverURL": "http://localhost:${port}",
  "setupCode": "${setup_code}",
  "screenshotsDir": "$(pwd)/${work}/screenshots"
}
JSON

xcodegen generate --quiet
echo "Running the UI tests on ${simulator}..."
xcodebuild test \
  -project FamilyApp.xcodeproj -scheme FamilyApp \
  -destination "platform=iOS Simulator,name=${simulator}" \
  -derivedDataPath "${work}/derived" \
  -resultBundlePath "${work}/result.xcresult" \
  -quiet
echo "UI tests passed. Screenshots: $(pwd)/${work}/screenshots"
