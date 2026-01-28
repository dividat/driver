const { wait, startDriver, connectWS, expectEvent } = require("../utils");
const expect = require("chai").expect;
const VirtualDevice = require("./mock/VirtualDevice");
const path = require("path");
const {
  waitForEndpoint,
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

// Connects to the WebSocket and verifies driver is connected to the device.
async function connectAndVerifyWS(device) {
  const flexWS = await connectWS("ws://127.0.0.1:8382/flex");

  await wait(30);

  expectStatusReply(flexWS, (status) => {
    expect(status.address).to.be.equal(device.address);
  });

  return flexWS;
}

async function withDeviceAndClient(deviceConfig) {
  const virtualDevice = new VirtualDevice(deviceConfig);
  await virtualDevice.initialize();

  await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
  expect(virtualDevice.isRegistered()).to.be.true;

  const flexWS = await connectAndVerifyWS(virtualDevice);

  return [virtualDevice, flexWS];
}

describe("Flex functionality", () => {
  var driver;

  beforeEach(async () => {
    var code = 0;
    driver = startDriver().on("exit", (c) => {
      code = c;
    });

    await waitForEndpoint("http://127.0.0.1:8382/flex");
    expect(code).to.be.equal(0);
    driver.removeAllListeners();
  });

  afterEach(async () => {
    driver.kill();
  });

  describe("Generic features (PASSTHRU device/reader)", () => {
    var virtualDevice;

    beforeEach(async () => {
      virtualDevice = new VirtualDevice({
        idVendor: "16c0",
        product: "PASSTHRU",
      });
      await virtualDevice.initialize();
    });

    it("Commands: Connect and GetStatus", async function () {
      this.timeout(3000);

      await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
      expect(virtualDevice.isRegistered()).to.be.true;

      await wait(20);

      // Connect flex endpoint client
      const flexWS = await connectWS("ws://127.0.0.1:8382/flex", { }, [ "manual-connect" ]);

      // Drive should not auto-connect since manual-connect is specified
      await expectStatusReply(flexWS, (statusAfterRegistration) => {
          expect(statusAfterRegistration.address).to.be.null;
          expect(statusAfterRegistration.device).to.be.null;
      });

      // Send command to connect to the virtual device
      const cmd = {
        type: "Connect",
        address: virtualDevice.address,
      };
      sendCmd(flexWS, cmd);
      await expectStatusReply(flexWS, (statusAfterConnect) => {
          expect(statusAfterConnect.address).to.be.equal(virtualDevice.address);
          expect(statusAfterConnect.device.deviceType).to.be.equal("flex");
          expect(statusAfterConnect.device.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);
      });
    });

    it("Commands: Discover", async function () {
      const virtualDevice1 = virtualDevice;

      // Second virtual Flex device
      const virtualDevice2 = new VirtualDevice({
        idVendor: "16c0",
        manufacturer: "SecondVendor",
        product: "PASSTHRU",
      });
      await virtualDevice2.initialize();

      //
      const virtualDeviceIgnored = new VirtualDevice({
        idVendor: "14f2",
        manufacturer: "IgnoreMe",
        product: "NotAFlex",
      });
      await virtualDeviceIgnored.initialize();

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
      await virtualDeviceIgnored.registerWithDriver("http://127.0.0.1:8382");

      sendCmd(flexWS, {
        type: 'Discover',
        duration: 5
      });

      const devices = await discovered;

      expect(devices).to.have.length(2);

      const receivedFields = devices.map((d) => {
          return {
              path: d.device.usbDevice.path,
              manufacturer: d.device.usbDevice.manufacturer,
              product: d.device.usbDevice.product
            }
      });
      const actualFields = [virtualDevice1, virtualDevice2].map((d) => {
          return {
              path: d.address,
              manufacturer: d.manufacturer,
              product: d.product
          }
      });
      expect(receivedFields).to.have.deep.members(actualFields);
    });

    it("Multiple clients can Connect and GetStatus at the same time", async function () {
      // obviously not a complete test, but sufficient to detect certain hiccups in the driver
      this.timeout(2000);

      await virtualDevice.registerWithDriver("http://127.0.0.1:8382");

      const clients = [...Array(5).keys()].map((_) => {
          return connectWS("ws://127.0.0.1:8382/flex").then((ws) => {
              return expectStatusReply(ws, (status) => {
                  expect(status.address).to.be.equal(virtualDevice.address);
              });
          });
      });

      await Promise.all(clients);
    });

    it("Broadcasts: Status on Connect and Disconnect ", async function () {
      this.timeout(10000);

      // Connect to flex endpoint with multiple clients
      const flexWS1 = await connectWS("ws://127.0.0.1:8382/flex");
      const flexWS2 = await connectWS("ws://127.0.0.1:8382/flex");
      const clients = [ flexWS1, flexWS2 ];

      // Initial status is null
      for (const ws of clients) {
          await expectStatusReply(ws, (statusInitial) => {
              expect(statusInitial.address).to.be.null;
              expect(statusInitial.device).to.be.null;
          });
      };

      const broadcast1 = expectBroadcast(flexWS1, (broadcast) => {
          expect(broadcast.message.type).to.be.equal("Status");
          expect(broadcast.message.address).to.be.equal(virtualDevice.address);
          expect(broadcast.message.device.usbDevice.serialNumber).to.be.equal(virtualDevice.serialNumber);
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
          expect(broadcast.message.device).to.be.null;
          return broadcast
      });
      const disconnectBroadcast2 = expectBroadcast(flexWS2, (b) => { return b });

      await virtualDevice.serialPort.close();
      expect(await disconnectBroadcast1).to.deep.equal(await disconnectBroadcast2);
    });

    it("Can receive binary data verbatim with passthru", async function () {
      this.timeout(10000);

      await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
      const flexWS = await connectAndVerifyWS(virtualDevice);

      // generate some random data
      const binaryData = Buffer.alloc(2048);
      for (let i = 0; i < binaryData.length; i++) {
        binaryData[i] = Math.floor(Math.random() * 256);
      }
      const binaryDataChunks = splitBufferRandomly(binaryData, 64, 256);

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
                  `${receivedData.length} out of ${binaryData.length}`,
              ),
            );
          }
        }, 8000);

        flexWS.on("message", function message(data, isBinary) {
          if (isBinary) {
            receivedData = Buffer.concat([receivedData, data]);
          }
          if (receivedData.length === binaryData.length) {
            clearTimeout(timeout);
            resolve();
            return;
          }
        });
      });

      for (const chunk of binaryDataChunks) {
        virtualDevice.serialPort.write(chunk);
        await wait(10)
      }

      await expectData;

      expect(receivedData.length).to.be.equal(binaryData.length);
      expect(receivedData).to.deep.equal(binaryData);
    });
  });

  describe("PASSTHRU-PretendFlex device", () => {
    var virtualDevice;
    var flexWS;

    beforeEach(async function() {
      this.timeout(3000);
      [virtualDevice, flexWS] = await withDeviceAndClient({
        idVendor: "16c0",
        product: "PASSTHRU-PretendFlex",
      });
    });

    it("present PASSTHRU-<foo> as <foo> device to client", async function () {
      await expectStatusReply(flexWS, (status) => {
          expect(status.address).to.be.equal(virtualDevice.address);
          expect(status.device.usbDevice.product).to.be.equal("PretendFlex");
      });
    });
  });

  describe("Flex v4 device", () => {
    var virtualDevice;
    var flexWS;

    beforeEach(async function() {
      this.timeout(3000);
      [virtualDevice, flexWS] = await withDeviceAndClient({
        idVendor: "16c0",
        manufacturer: "Teensyduino",
        bcdDevice: "0277",
      });
    });

    it("can receive 8-bit synthetic data via WebSocket", async function () {
      this.timeout(10000);

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
        virtualDevice.serialPort.write(generateFlexSerialFrame(i, 8));
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
    });
  });

  describe("Flex v5 device", () => {
    var virtualDevice;
    var flexWS;

    beforeEach(async function() {
      this.timeout(3000);
      [virtualDevice, flexWS] = await withDeviceAndClient({
        idVendor: "16c0",
        manufacturer: "Teensyduino",
        bcdDevice: "0278",
      });
    });

    it("can receive 12-bit synthetic data via WebSocket", async function () {
      this.timeout(10000);

      // Track commands received from driver to know when mode switch is complete
      const modeSwitchDone = new Promise((resolve) => {
        let modeSwitchComplete = false;
        let seenUM = false;
        virtualDevice.serialPort.on("data", (data) => {
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

      // Switch to 12-bit mode by sending UM\n command
      const switchTo12BitCmd = Buffer.from("UM\n");
      flexWS.send(switchTo12BitCmd);

      // Send dummy data to unblock the old reader (it's blocking on ReadByte)
      // NOTE: this is a theoretical bug in the driver, but this happens only in
      // the synthetic setup and there's no point patching to-be-legacy device
      // corner-cases at this point
      await wait(50);
      virtualDevice.serialPort.write(Buffer.from([0x00]));

      await modeSwitchDone;

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

      for (let i = 0; i < numFrames; i++) {
        virtualDevice.serialPort.write(generateFlexSerialFrame(i, 12));
      }

      await expectData;

      expect(receivedFrames.length).to.be.equal(numFrames);

      // Check each frame's content
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
    });
  });

  describe("Sensitronics device", () => {
    var virtualDevice;
    var flexWS;

    beforeEach(async function() {
      this.timeout(3000);

      virtualDevice = new VirtualDevice({
        idVendor: "16c0",
        idProduct: "0483",
        manufacturer: "Dividat",
        product: "FlexV6",
      });
      await virtualDevice.initialize();
    });

    it("driver sends start command, chunks frames", async function () {
      this.timeout(10000);

      // Set up listener for start command before connecting
      const startCmdReceived = new Promise((resolve) => {
        virtualDevice.serialPort.on("data", (data) => {
          const str = data.toString();
          if (str.includes("S\n")) {
            resolve();
          }
        });
      });

      await virtualDevice.registerWithDriver("http://127.0.0.1:8382");
      expect(virtualDevice.isRegistered()).to.be.true;

      flexWS = await connectAndVerifyWS(virtualDevice);

      // Generate random frames
      const numFrames = 30;
      const generatedFrames = [];
      for (let i = 0; i < numFrames; i++) {
        generatedFrames.push(generateRandomSensitronicsFrame(50));
      }

      const allFramesBuffer = Buffer.concat(generatedFrames);

      // Split the buffer into random chunks to simulate fragmented transmission
      const chunks = splitBufferRandomly(allFramesBuffer, 1, 15);

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

      // wait for start command before producing data
      await startCmdReceived;
      for (const chunk of chunks) {
        virtualDevice.serialPort.write(chunk);
      }

      await expectData;

      expect(receivedFrames.length).to.be.equal(numFrames);

      for (let i = 0; i < numFrames; i++) {
        expect(
          receivedFrames[i].equals(generatedFrames[i]),
          `Frame ${i} mismatch`
        ).to.be.true;
      }
    });
  });
});
