/* eslint-env mocha */
const { wait, startDriver, connectWS, expectEvent } = require("../utils");
const expect = require("chai").expect;
const VirtualDevice = require("./mock/VirtualDevice");
const path = require("path");

function expectMessageType(ws, msgType) {
    return expectEvent(ws, "message", (s) => {
      const msg = JSON.parse(s);
      return msg.type === msgType;
    });
};

function sendCmd(ws, cmd) {
    return ws.send(JSON.stringify(cmd));
}

async function expectCmdReply(ws, cmd, replyType, replyCheck) {
    const replyPromise = expectMessageType(ws, replyType);
    sendCmd(ws, cmd);

    return replyPromise.then(JSON.parse).then(replyCheck)
}

async function expectStatusReply(ws, replyCheck) {
    return expectCmdReply(ws, { type: "GetStatus" }, "Status", replyCheck);
}

async function expectBroadcast(ws, check) {
    return expectMessageType(ws, "Broadcast").then(JSON.parse).then(check)
}

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

    // Drive should not auto-connect since manual-connect is specified
    await expectStatusReply(flexWS, (statusAfterRegistration) => {
        expect(statusAfterRegistration.address).to.be.null;
        expect(statusAfterRegistration.deviceInfo).to.be.null;
    });

    // Send command to connect to the virtual device
    const cmd = {
      type: "Connect",
      address: virtualDevice.address,
    };
    sendCmd(flexWS, cmd);
    await expectStatusReply(flexWS, (statusAfterConnect) => {
        expect(statusAfterConnect.address).to.be.equal(virtualDevice.address);
        expect(statusAfterConnect.deviceInfo.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);
    });
  });

  it("AUTO-CONNECT: send broadcasts about status changes", async function () {
    this.timeout(10000);

    // Connect to flex endpoint with multiple clients
    const flexWS1 = await connectWS("ws://127.0.0.1:8382/flex");
    const flexWS2 = await connectWS("ws://127.0.0.1:8382/flex");
    const clients = [ flexWS1, flexWS2 ];

    // Initial status is null
    for (const ws of clients) {
        await expectStatusReply(ws, (statusInitial) => {
            expect(statusInitial.address).to.be.null;
            expect(statusInitial.deviceInfo).to.be.null;
        });
    };

    const broadcast1 = expectBroadcast(flexWS1, (broadcast) => {
        expect(broadcast.message.type).to.be.equal("Status");
        expect(broadcast.message.address).to.be.equal(virtualDevice.address);
        expect(broadcast.message.deviceInfo.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);
        return broadcast
    });
    const broadcast2 = expectBroadcast(flexWS2, (b) => { return b });

    await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
    expect(virtualDevice.isRegistered()).to.be.true;

    // this will await for Flex backgroundScanIntervalSeconds, which is 2 seconds currently
    expect(await broadcast1).to.deep.equal(await broadcast2);

    for (const ws of clients) {
        // Reply to GetStatus should match the Status Broadcast
        expectStatusReply(ws, (status) => {
            expect(status).to.deep.equal(broadcastChecked.message);
        });
    };

    const disconnectBroadcast1 = expectBroadcast(flexWS1, (broadcast) => {
        expect(broadcast.message.type).to.be.equal("Status");
        expect(broadcast.message.address).to.be.null;
        expect(broadcast.message.deviceInfo).to.be.null;
        return broadcast
    });
    const disconnectBroadcast2 = expectBroadcast(flexWS2, (b) => { return b });

    await virtualDevice.serialPort.close();
    expect(await disconnectBroadcast1).to.deep.equal(await disconnectBroadcast2);
  });

  it("AUTO-CONNECT: can replay recording and receive data via WebSocket", async function () {
    this.timeout(10000);

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    // Wait for connection
    const deviceConnected = expectBroadcast(flexWS, (msg) => {
      expect(msg.message.address).to.be.equal(virtualDevice.address);
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
