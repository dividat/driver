#!/usr/bin/env node
// Replay Senso Flex serial data recordings to a running Driver via mock device

const path = require("path");
const argv = require("minimist")(process.argv.slice(2));

// Show help if requested
if (argv["help"] || argv["h"]) {
  console.log(`
Usage: node index.js [options] <recording-file>

Replay Senso Flex serial data recordings to a running Driver via a mock device.

Arguments:
  recording-file          Path to the recording file (default: rec/flex/zero.dat)

Options:
  --speed <number>        Replay speed multiplier (default: 1)
                          Use values > 1 to speed up, < 1 to slow down
  --once                  Play recording once and exit (default: loop continuously)
  --driver-url <url>      URL of the running Driver (default: http://127.0.0.1:8382)
  -h, --help              Show this help message

Examples:
  node index.js recording.dat                     Replay at normal speed, looping
  node index.js --speed 2 recording.dat           Replay at 2x speed
  node index.js --once --speed 0.5 recording.dat  Replay once at half speed

Note: The Driver must be running with test mode enabled for mock device registration.
`);
  process.exit(0);
}

// Import VirtualDevice from test utilities
const VirtualDevice = require("../../test/flex/mock/VirtualDevice");

// Parse CLI arguments
const recFile = argv["_"].pop() || "rec/flex/zero.dat";
const speed = parseFloat(argv["speed"]) || 1;
const loop = !argv["once"];
const driverUrl = argv["driver-url"] || "http://127.0.0.1:8382";

// USB descriptors matching the passthru device
const PASSTHRU_USB_INFO = {
  idVendor: "16c0",
  product: "PASSTHRU",
};

async function main() {
  console.log(`Replay Flex Recording Tool`);
  console.log(`--------------------------`);
  console.log(`Recording file: ${recFile}`);
  console.log(`Speed: ${speed}x`);
  console.log(`Loop: ${loop}`);
  console.log(`Driver URL: ${driverUrl}`);
  console.log();

  // Check if recording file exists
  const fs = require("fs");
  if (!fs.existsSync(recFile)) {
    console.error(`Error: Recording file not found: ${recFile}`);
    process.exit(1);
  }

  // Create virtual device with passthru USB descriptors
  const virtualDevice = new VirtualDevice(PASSTHRU_USB_INFO);

  // Initialize the virtual serial port
  console.log("Initializing virtual device...");
  try {
    await virtualDevice.initialize();
    console.log(`Virtual serial port created at: ${virtualDevice.address}`);
  } catch (error) {
    console.error(`Failed to initialize virtual device: ${error.message}`);
    process.exit(1);
  }

  // Register mock device with the Driver
  console.log(`Registering mock device with Driver at ${driverUrl}...`);
  try {
    await virtualDevice.registerWithDriver(driverUrl);
    console.log(`Mock device registered with ID: ${virtualDevice.registeredId}`);
  } catch (error) {
    console.error(`Failed to register mock device with Driver: ${error.message}`);
    console.error(`Make sure the Driver is running with test mode enabled.`);
    virtualDevice.serialPort.close();
    process.exit(1);
  }

  // Track if we're shutting down to suppress expected errors
  let isShuttingDown = false;

  // Handle errors from the serial port (e.g., socat exit on SIGINT)
  virtualDevice.serialPort.on("error", (error) => {
    if (!isShuttingDown) {
      console.error(`Serial port error: ${error.message}`);
    }
  });

  // Handle graceful shutdown
  const cleanup = async () => {
    if (isShuttingDown) return;
    isShuttingDown = true;

    console.log("\nShutting down...");
    virtualDevice.stopReplay();

    // Close serial port first to prevent error events from socat
    if (virtualDevice.serialPort) {
      virtualDevice.serialPort.close();
    }

    try {
      await virtualDevice.unregisterFromDriver(driverUrl);
      console.log("Unregistered mock device from Driver.");
    } catch (error) {
      console.warn(`Warning: Failed to unregister device: ${error.message}`);
    }

    process.exit(0);
  };

  process.on("SIGINT", cleanup);
  process.on("SIGTERM", cleanup);

  // Start replaying the recording
  console.log(`\nStarting replay of ${recFile}...`);
  try {
    await virtualDevice.replayRecording(recFile, loop, speed);
    if (!loop) {
      console.log("End of recording reached, exiting.");
      await cleanup();
    }
  } catch (error) {
    console.error(`Replay error: ${error.message}`);
    await cleanup();
  }
}

main().catch((error) => {
  console.error(`Unexpected error: ${error.message}`);
  process.exit(1);
});
