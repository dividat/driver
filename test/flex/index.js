/* eslint-env mocha */
const { wait, startDriver, connectWS, expectEvent } = require("../utils");
const expect = require("chai").expect;
const VirtualDevice = require("./mock/VirtualDevice");
const path = require("path");

describe("Basic Flex functionality with Passthru device", () => {
  var driver;
  var virtualDevice;

  beforeEach(async () => {
    // Start driver
    var code = 0;
    driver = startDriver().on("exit", (c) => {
      code = c;
    });

    // Give driver 500ms to start up
    await wait(500);
    expect(code).to.be.equal(0);
    driver.removeAllListeners();

    // Create virtual Flex device with specified USB details
    virtualDevice = new VirtualDevice({
      idVendor: "16c0",
      product: "PASSTHRU",
    });
    await virtualDevice.initialize();
  });

  afterEach(async () => {
    driver.kill();
    if (virtualDevice && virtualDevice.serialPort) {
      virtualDevice.serialPort.close();
    }
  });


  it("MANUAL-CONNECT: register virtual device and check status changes", async function () {
    this.timeout(3000);

    await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
    expect(virtualDevice.isRegistered()).to.be.true;

    await wait(500);

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex", { }, [ "manual-connect" ]);

    function expectMessageType(msgType) {
        return expectEvent(flexWS, "message", (s) => {
          const msg = JSON.parse(s);
          return msg.type === msgType;
        });
    };

    // Drive should not auto-connect since manual-connect is specified
    const statusAfterRegistrationP = expectMessageType("Status");
    flexWS.send(JSON.stringify({ type: "GetStatus" }));
    const statusAfterRegistration = JSON.parse(await statusAfterRegistrationP);

    expect(statusAfterRegistration.address).to.be.null;
    expect(statusAfterRegistration.deviceInfo).to.be.null;


    const statusAfterConnectP = expectMessageType("Status");
    // Send command to connect to the virtual device
    const cmd = JSON.stringify({
      type: "Connect",
      address: virtualDevice.address,
    });
    flexWS.send(cmd);
    flexWS.send(JSON.stringify({ type: "GetStatus" }));
    const statusAfterConnect = JSON.parse(await statusAfterConnectP);

    expect(statusAfterConnect.address).to.be.equal(virtualDevice.address);
    expect(statusAfterConnect.deviceInfo.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);
  });

  it("AUTO-CONNECT: get broadcasts about devices and auto-connects to them", async function () {
    this.timeout(10000);

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    function expectMessageType(msgType) {
        return expectEvent(flexWS, "message", (s) => {
          const msg = JSON.parse(s);
          return msg.type === msgType;
        });
    };

    // Initial status is null
    const statusInitialP = expectMessageType("Status");
    flexWS.send(JSON.stringify({ type: "GetStatus" }));
    const statusInitial = JSON.parse(await statusInitialP);

    expect(statusInitial.address).to.be.null;
    expect(statusInitial.deviceInfo).to.be.null;


    // Expect a Status Broadcast after device is connected
    const broadcastP = expectMessageType("Broadcast");

    await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
    expect(virtualDevice.isRegistered()).to.be.true;

    // this will await for Flex backgroundScanIntervalSeconds, which is 2 seconds currently
    const broadcast = JSON.parse(await broadcastP);

    expect(broadcast.message.type).to.be.equal("Status");
    expect(broadcast.message.address).to.be.equal(virtualDevice.address);
    expect(broadcast.message.deviceInfo.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);

    // Reply to GetStatus should match the Status Broadcast
    const statusFromCmdP = expectMessageType("Status");
    flexWS.send(JSON.stringify({ type: "GetStatus" }));
    const statusFromCmd = JSON.parse(await statusFromCmdP);
    expect(statusFromCmd).to.deep.equal(broadcast.message);


    const disconnectBroadcastP = expectMessageType("Broadcast");
    await virtualDevice.serialPort.close();
    const disconnectBroadcast = JSON.parse(await disconnectBroadcastP);

    expect(disconnectBroadcast.message.type).to.be.equal("Status");
    expect(disconnectBroadcast.message.address).to.be.null;
    expect(disconnectBroadcast.message.deviceInfo).to.be.null;
  });

  it("AUTO-CONNECT: can replay recording and receive data via WebSocket", async function () {
    this.timeout(10000);

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    // Wait for connection
    const deviceConnected = expectEvent(flexWS, "message", (s) => {
      const msg = JSON.parse(s);
      const isBroadcast = msg.type === "Broadcast";
      const isConnected = msg.message.address === virtualDevice.address;
      return isBroadcast && isConnected
    });

    // Register virtual device with driver
    await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
    await deviceConnected;


    const recordingPath = path.join(__dirname, "test-recording.dat");
    // Load and decode the recording file to compare with received data
    const fs = require("fs");
    const recordingContent = fs.readFileSync(recordingPath, "utf8");
    const recordingLines = recordingContent.trim().split("\n");

    // Extract and decode all base64 data from recording
    let expectedBinaryData = [];
    for (const line of recordingLines) {
      const [, base64Data] = line.split(", ");
      const binaryData = Buffer.from(base64Data, "base64");
      expectedBinaryData.push(binaryData);
    }
    expectedBinaryData = Buffer.concat(expectedBinaryData);

    // Set up promise to collect WebSocket data
    let receivedData = Buffer.from("");
    const expectData = new Promise((resolve, reject) => {

      const timeout = setTimeout(() => {
        if (receivedData.length === 0) {
          reject(new Error("No data received within timeout"));
        } else {
          reject(
            new Error(
              "Not all bytes received in time: " +
                `${receivedData.length} out of ${expectedBinaryData.length}`,
            ),
          );
        }
      }, 8000);

      flexWS.on("message", function message(data, isBinary) {
        if (isBinary) {
          receivedData = Buffer.concat([receivedData, data]);
        }
        if (receivedData.length === expectedBinaryData.length) {
          clearTimeout(timeout);
          resolve();
          return;
        }
      });
    });

    // Start replaying the recording
    setTimeout(() => {
      virtualDevice.replayRecording(recordingPath, false)
    }, 0);

    // Wait for data to be received
    await expectData;

    // Verify we received data
    expect(receivedData.length).to.be.equal(expectedBinaryData.length);

    // Verify that received data matches the first frame from recording
    expect(receivedData).to.deep.equal(expectedBinaryData);
  });
});
