/* eslint-env mocha */
const { wait, startDriver, connectWS, expectEvent } = require("../utils");
const expect = require("chai").expect;
const VirtualDevice = require("./mock/VirtualDevice");
const path = require("path");
const {
  generateFlexSerialFrame,
  generateRandomSensitronicsFrame,
  splitBufferRandomly,
} = require("./helpers");

function expectMessageType(ws, msgType) {
    return expectEvent(ws, "message", (s) => {
      const msg = JSON.parse(s);
      return msg.type === msgType;
    }).then(JSON.parse);
};

function sendCmd(ws, cmd) {
    return ws.send(JSON.stringify(cmd));
}

function expectCmdReply(ws, cmd, replyType, replyCheck) {
    const replyPromise = expectMessageType(ws, replyType);
    sendCmd(ws, cmd);

    return replyPromise.then(replyCheck)
}

function expectStatusReply(ws, replyCheck) {
    return expectCmdReply(ws, { type: "GetStatus" }, "Status", replyCheck);
}

function expectBroadcast(ws, check) {
    return expectMessageType(ws, "Broadcast").then(check)
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

  it("present PASSTHRU-<foo> as <foo> device to client", async function () {
    this.timeout(3000);

    // Create virtual Flex device with specified USB details
    const passthruFlexDevice = new VirtualDevice({
      idVendor: "16c0",
      product: "PASSTHRU-PretendFlex",
    });
    await passthruFlexDevice.initialize();

    await passthruFlexDevice.registerWithDriver("http://127.0.0.1:8382");
    expect(passthruFlexDevice.isRegistered()).to.be.true;

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");
    const cmd = {
      type: "Connect",
      address: passthruFlexDevice.address,
    };
    sendCmd(flexWS, cmd);

    await expectStatusReply(flexWS, (status) => {
        expect(status.address).to.be.equal(passthruFlexDevice.address);
        expect(status.deviceInfo.usbDevice.product).to.be.equal("PretendFlex");
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


  it("MANUAL-CONNECT: can discover and list devices", async function () {
    const virtualDevice1 = virtualDevice; // set up in beforeEach

    // Create virtual Flex device with specified USB details
    const virtualDevice2 = new VirtualDevice({
      idVendor: "16c0",
      product: "NEWDEVICE",
    });
    await virtualDevice2.initialize();

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    const discovered = new Promise((resolve, reject) => {
        const values = [];
        flexWS.on("message", (a) => {
            const msg = JSON.parse(a);
            expect(msg.type).to.equal("Discovered")
            values.push(msg);
            if (values.length === 2) {
                resolve(values);
            }
        });
    });

    await virtualDevice1.registerWithDriver("http://127.0.0.1:8382");
    await virtualDevice2.registerWithDriver("http://127.0.0.1:8382");

    sendCmd(flexWS, {
      type: 'Discover',
      duration: 5
    });

    const devices = await discovered;

    expect(devices).to.have.length(2);

    const receivedFields = devices.map((d) => {
        return { path: d.usbDevice.path, product: d.usbDevice.product }
    });
    const actualFields = [virtualDevice1, virtualDevice2].map((d) => {
        return { path: d.address, product: d.product }
    });
    expect(receivedFields).to.have.deep.members(actualFields);

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

  it("Flex v4: can receive 8-bit synthetic data via WebSocket", async function () {
    this.timeout(10000);

    const flexV4Device = new VirtualDevice({
      idVendor: "16c0",
      manufacturer: "Teensyduino",
      bcdDevice: "0277",
    });
    await flexV4Device.initialize();

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    const deviceConnected = expectBroadcast(flexWS, (msg) => {
      expect(msg.message.address).to.be.equal(flexV4Device.address);
    });

    // Register virtual device with driver
    await flexV4Device.registerWithDriver("http://127.0.0.1:8382");
    await deviceConnected;

    // Generate synthetic frames (no mode switch needed, driver starts in 8-bit mode)
    const numFrames = 24;

    // Set up promise to collect WebSocket data
    const receivedFrames = [];
    const expectData = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (receivedFrames.length === 0) {
          reject(new Error("No data received within timeout"));
        } else if (receivedFrames.length < numFrames) {
          reject(
            new Error(
              `Expected ${numFrames} frames, got: ${receivedFrames.length}`
            )
          );
        }
      }, 8000);

      flexWS.on("message", function message(data, isBinary) {
        if (isBinary) {
          receivedFrames.push(Buffer.from(data));
        }
        if (receivedFrames.length === numFrames) {
          clearTimeout(timeout);
          resolve();
        }
      });
    });

    // Send the synthetic serial data to the device
    for (let i = 0; i < numFrames; i++) {
      flexV4Device.serialPort.write(generateFlexSerialFrame(i, 8));
    }

    // Wait for data to be received
    await expectData;

    // Verify we received the correct number of frames
    expect(receivedFrames.length).to.be.equal(numFrames);

    // Check each frame's content
    // The driver forwards the sample data (without headers) in 8-bit mode:
    // 3 bytes per sample: row, col, pressure
    for (let frameIdx = 0; frameIdx < numFrames; frameIdx++) {
      const frame = receivedFrames[frameIdx];

      // Each frame should have 2 samples * 3 bytes = 6 bytes
      expect(frame.length).to.be.equal(6);

      // Sample 1: (frameIdx, 1, frameIdx*2+1)
      expect(frame[0]).to.be.equal(frameIdx);   // row
      expect(frame[1]).to.be.equal(1);           // col
      expect(frame[2]).to.be.equal(frameIdx * 2 + 1); // pressure

      // Sample 2: (1, frameIdx, frameIdx*3+1)
      expect(frame[3]).to.be.equal(1);           // row
      expect(frame[4]).to.be.equal(frameIdx);    // col
      expect(frame[5]).to.be.equal(frameIdx * 3 + 1); // pressure
    }

    // Clean up
    flexV4Device.serialPort.close();
  });

  it("Flex v5: can receive 12-bit synthetic data via WebSocket", async function () {
    this.timeout(10000);

    // Create V5 device (bcdDevice > 0x0277)
    const flexV5Device = new VirtualDevice({
      idVendor: "16c0",
      manufacturer: "Teensyduino",
      bcdDevice: "0278",
    });
    await flexV5Device.initialize();

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    const deviceConnected = expectBroadcast(flexWS, (msg) => {
      expect(msg.message.address).to.be.equal(flexV5Device.address);
    });

    // Track commands received from driver to know when mode switch is complete
    const modeSwitchDone = new Promise((resolve) => {
      let modeSwitchComplete = false;
      let seenUM = false;
      flexV5Device.serialPort.on("data", (data) => {
        const str = data.toString();
        // After mode switch, driver sends UM\n then S\n
        if (str.includes("UM")) {
          seenUM = true;
        }
        // When we see S\n after UM, the mode switch is complete
        if (seenUM && str.includes("S\n") && !modeSwitchComplete) {
          modeSwitchComplete = true;
          resolve();
        }
      });
    });

    // Register virtual device with driver
    await flexV5Device.registerWithDriver("http://127.0.0.1:8382");
    await deviceConnected;

    // Switch to 12-bit mode by sending UM\n command
    const switchTo12BitCmd = Buffer.from("UM\n");
    flexWS.send(switchTo12BitCmd);

    // Send dummy data to unblock the old reader (it's blocking on ReadByte)
    // NOTE: this is theoretical bug in the driver, but this happens only in the
    // synthetic setup and there's no point patching to-be-legacy device
    // corner-cases at this point
    await wait(50);
    flexV5Device.serialPort.write(Buffer.from([0x00]));

    // Wait for driver to complete mode switch (sends UM\n then S\n)
    await modeSwitchDone;

    // Generate synthetic frames
    const numFrames = 24;

    // Set up promise to collect WebSocket data
    const receivedFrames = [];
    const expectData = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (receivedFrames.length === 0) {
          reject(new Error("No data received within timeout"));
        } else if (receivedFrames.length < numFrames) {
          reject(
            new Error(
              `Expected ${numFrames} frames, got: ${receivedFrames.length}`
            )
          );
        }
      }, 8000);

      flexWS.on("message", function message(data, isBinary) {
        if (isBinary) {
          receivedFrames.push(Buffer.from(data));
        }
        if (receivedFrames.length === numFrames) {
          clearTimeout(timeout);
          resolve();
        }
      });
    });

    // Send the synthetic serial data to the device
    for (let i = 0; i < numFrames; i++) {
      flexV5Device.serialPort.write(generateFlexSerialFrame(i, 12));
    }

    // Wait for data to be received
    await expectData;

    // Verify we received the correct number of frames
    expect(receivedFrames.length).to.be.equal(numFrames);

    // Check each frame's content
    // The driver forwards the sample data (without headers) in 12-bit mode:
    // 4 bytes per sample: row, col, pressure_msb, pressure_lsb
    for (let frameIdx = 0; frameIdx < numFrames; frameIdx++) {
      const frame = receivedFrames[frameIdx];

      // Each frame should have 2 samples * 4 bytes = 8 bytes
      expect(frame.length).to.be.equal(8);

      // Sample 1: (frameIdx, 1, frameIdx*2+1)
      expect(frame[0]).to.be.equal(frameIdx);   // row
      expect(frame[1]).to.be.equal(1);           // col
      const pressure1 = frame.readUInt16BE(2);
      expect(pressure1).to.be.equal(frameIdx * 2 + 1);

      // Sample 2: (1, frameIdx, frameIdx*3+1)
      expect(frame[4]).to.be.equal(1);           // row
      expect(frame[5]).to.be.equal(frameIdx);    // col
      const pressure2 = frame.readUInt16BE(6);
      expect(pressure2).to.be.equal(frameIdx * 3 + 1);
    }

    // Clean up
    flexV5Device.serialPort.close();
  });

  it("Sensitronics: driver starts send command, chunks frames", async function () {
    this.timeout(10000);

    // Create Sensitronics device
    const sensitronicsDevice = new VirtualDevice({
      idVendor: "16c0",
      idProduct: "0483",
      manufacturer: "Sensitronics",
      product: "Dividat16x16",
    });
    await sensitronicsDevice.initialize();

    // Connect flex endpoint client
    const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

    const deviceConnected = expectBroadcast(flexWS, (msg) => {
      expect(msg.message.address).to.be.equal(sensitronicsDevice.address);
    });

    // Wait for driver to send start measurement command
    const startCmdReceived = new Promise((resolve) => {
      sensitronicsDevice.serialPort.on("data", (data) => {
        const str = data.toString();
        if (str.includes("S\n")) {
          resolve();
        }
      });
    });

    // Register virtual device with driver
    await sensitronicsDevice.registerWithDriver("http://127.0.0.1:8382");
    await deviceConnected;

    // Wait for driver to be ready to receive data
    await startCmdReceived;

    // Generate random frames
    const numFrames = 24;
    const generatedFrames = [];
    for (let i = 0; i < numFrames; i++) {
      generatedFrames.push(generateRandomSensitronicsFrame(50));
    }

    // Concatenate all frames into one buffer
    const allFramesBuffer = Buffer.concat(generatedFrames);

    // Split the buffer into random chunks to simulate fragmented transmission
    const chunks = splitBufferRandomly(allFramesBuffer, 1, 15);

    // Set up promise to collect WebSocket data
    const receivedFrames = [];
    const expectData = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (receivedFrames.length === 0) {
          reject(new Error("No data received within timeout"));
        } else if (receivedFrames.length < numFrames) {
          reject(
            new Error(
              `Expected ${numFrames} frames, got: ${receivedFrames.length}`
            )
          );
        }
      }, 8000);

      flexWS.on("message", function message(data, isBinary) {
        if (isBinary) {
          receivedFrames.push(Buffer.from(data));
        }
        if (receivedFrames.length === numFrames) {
          clearTimeout(timeout);
          resolve();
        }
      });
    });

    // Send the chunks with small delays to simulate real transmission
    for (const chunk of chunks) {
      sensitronicsDevice.serialPort.write(chunk);
    }

    // Wait for data to be received
    await expectData;

    // Verify we received the correct number of frames
    expect(receivedFrames.length).to.be.equal(numFrames);

    // Verify each received frame matches what we sent
    for (let i = 0; i < numFrames; i++) {
      expect(
        receivedFrames[i].equals(generatedFrames[i]),
        `Frame ${i} mismatch`
      ).to.be.true;
    }

    // Clean up
    sensitronicsDevice.serialPort.close();
  });

});
