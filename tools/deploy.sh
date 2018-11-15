#!/usr/bin/env bash

# exit on failure
set -e

# Handle failures (stolen from https://stackoverflow.com/a/25515370)
die() { 
  echo "$*" >&2
  echo "DEPLOYMENT FAILED."
  exit 1 
}
try() { "$@" || die "ERROR: $*"; }
tryF() { 
  if [ "$($*)" != "0" ]; then
    die "ERROR: $*"
  fi
}

# Templated variables
VERSION="@version@"
CHANNEL="@channel@"
RELEASE_URL="@releaseUrl@"
RELEASE_BUCKET="@releaseBucket@"
UNSINGED_RELEASE_DIR="@unsignedReleaseDir@"

# Tools
UPX="@upx@"
OPENSSL="@openssl@"
OSSLSIGNCODE="@osslsigncode@"
AWSCLI="@awscli@"
TREE="@tree@"
CURL="@curl@"

function checkChannelAndVersion() {
  if [[ $CHANNEL != "master" && $CHANNEL != "develop" ]]; then
    die "ERROR: Can not deploy to channel $CHANNEL."
  fi

  local SEMVER_REGEX="^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$"
  if [[ ! $VERSION =~ $SEMVER_REGEX ]]; then
    die "ERROR: Version $VERSION is not valid."
  fi
}

function writeSignature() {
  $OPENSSL dgst -sha256 -sign $CHECKSUM_SIGNING_CERT $1 | $OPENSSL base64 -A -out $1.sig
  # make sure signature file exists and is non-zero
  if [ ! -s "$1.sig" ]; then
    echo 1
  fi
  echo 0
}

function createSignedRelease() {
  local unsigned=$1
  local signed=$2
  
  # copy unsigned content
  try mkdir -p $signed
  try cp -r "$unsigned/$CHANNEL" $signed
  try chmod -R u+w $signed

  local versionDir=$signed/$CHANNEL/$VERSION

  # Prep the linux binary
  local linuxBin=$versionDir/dividat-driver-linux-amd64-$VERSION
  try $UPX $linuxBin
  tryF writeSignature $linuxBin

  # Prep the windows binary
  local windowsBin=$versionDir/dividat-driver-windows-amd64-$VERSION.exe
  try $UPX $windowsBin
  echo -n "Signing windows executable ... "
  try $OSSLSIGNCODE sign \
    -pkcs12 $CODE_SIGNING_CERT \
    -h sha1 \
    -n "Dividat Driver" \
    -i "https://www.dividat.com/" \
    -t http://timestamp.verisign.com/scripts/timstamp.dll \
    -in $windowsBin \
    -out $windowsBin.signed
  try mv $windowsBin.signed $windowsBin
  tryF writeSignature $windowsBin

  # Sign the latest file
  tryF writeSignature $signed/$CHANNEL/latest
}

function deploy(){
  try $AWSCLI s3 cp $releaseDir/$CHANNEL/$VERSION $RELEASE_BUCKET/$CHANNEL/$VERSION/ --recursive \
    --acl public-read \
    --cache-control max-age=0
  try $AWSCLI s3 cp $releaseDir/$CHANNEL/latest $RELEASE_BUCKET/$CHANNEL/latest \
    --acl public-read \
    --cache-control max-age=0
  try $AWSCLI s3 cp $releaseDir/$CHANNEL/latest.sig $RELEASE_BUCKET/$CHANNEL/latest.sig \
    --acl public-read \
    --cache-control max-age=0
}

# Create a temporary directory for signed release
releaseDir=$(mktemp -d)
function cleanup() {
  rm -r "$releaseDir"
}
trap "cleanup" EXIT

# Check for valid channel and version
checkChannelAndVersion

# Check that env variable CODE_SIGNING_CERT and CHECKSUM_SIGNING_CERT are set
if [[ ! -r $CODE_SIGNING_CERT || ! -r $CHECKSUM_SIGNING_CERT ]]; then
  die "ERROR: CODE_SIGNING_CERT or CHECKSUM_SIGNING_CERT not set properly."
fi

# Create signed release
createSignedRelease $UNSINGED_RELEASE_DIR $releaseDir

echo
echo "Release created:"
$TREE --du -h $releaseDir --noreport
echo

# Display currently deployed latest
echo -n "Channel 'master' is at: "
$CURL -s "$RELEASE_URL/master/latest"

echo -n "Channel 'develop' is at: "
$CURL -s "$RELEASE_URL/develop/latest"

read -p "About to deploy $VERSION to '$CHANNEL'. Proceed? [y/N]" confirmation
if [ ${confirmation:-N} == "y" ]; then
  deploy
else
  echo "Aborting."
fi

